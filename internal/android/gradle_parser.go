package android

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	namespaceRe           = regexp.MustCompile(`\bnamespace\s*(?:=|\s)\s*["']([^"']+)["']`)
	applicationIDRe       = regexp.MustCompile(`\bapplicationId\s*(?:=|\s)\s*["']([^"']+)["']`)
	applicationIDSuffixRe = regexp.MustCompile(`\bapplicationIdSuffix\s*(?:=|\s)\s*["']([^"']+)["']`)
	versionNameSuffixRe   = regexp.MustCompile(`\bversionNameSuffix\s*(?:=|\s)\s*["']([^"']+)["']`)
	versionCodeIntRe      = regexp.MustCompile(`\bversionCode\s*(?:=|\s)\s*(\d+)`)
	versionCodeVarRe      = regexp.MustCompile(`\bversionCode\s*(?:=|\s)\s*([^\r\n]+)`)
	versionNameStrRe      = regexp.MustCompile(`\bversionName\s*(?:=|\s)\s*["']([^"']+)["']`)
	versionNameVarRe      = regexp.MustCompile(`\bversionName\s*(?:=|\s)\s*([^\r\n]+)`)
	dimensionRe           = regexp.MustCompile(`\bdimension\s*(?:=|\s)\s*["']([^"']+)["']`)
	flavorDimensionsRe    = regexp.MustCompile(`flavorDimensions\s*(?:\+=|=|\s|\()\s*([^\r\n]+)`)

	namedBlockHeaderRe     = regexp.MustCompile(`^(?:create|getByName|named|register|maybeCreate)(?:<[^>]+>)?\s*\(\s*["']([^"']+)["']\s*\)$`)
	delegatedBlockHeaderRe = regexp.MustCompile(`^(?:val|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s+by\s+(?:getting|creating|registering|existing)\b`)
	identBlockHeaderRe     = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)$`)
	propRefRe              = regexp.MustCompile(`(?:project\.property|findProperty|providers\.gradleProperty)\s*\(\s*["']([^"']+)["']\s*\)`)
	libsVersionRefRe       = regexp.MustCompile(`libs\.versions\.([A-Za-z0-9_.]+)`)
)

type gradleBlock struct {
	Header string
	Body   string
}

func parseModuleIfAndroidApp(
	ctx context.Context,
	rootDir string,
	modSpec string,
	props map[string]string,
	tomlVersions map[string]string,
) (AppModule, bool, error) {
	modDir := moduleDirFromSpec(ctx, rootDir, modSpec)
	buildFile := findBuildFile(ctx, modDir)
	if buildFile == "" {
		return AppModule{}, false, nil
	}

	raw, err := os.ReadFile(buildFile)
	if err != nil {
		return AppModule{}, false, fmt.Errorf("read gradle build file %q: %w", buildFile, err)
	}
	cleaned := stripComments(ctx, string(raw))
	if !hasAndroidAppPlugin(ctx, cleaned) {
		return AppModule{}, false, nil
	}

	androidBody, ok := extractNamedBlock(ctx, cleaned, "android")
	if !ok {
		return AppModule{}, false, nil
	}

	mod := AppModule{
		Name:        modSpec,
		Dir:         modDir,
		BuildFile:   buildFile,
		IsKotlinDSL: strings.HasSuffix(buildFile, FileBuildKTS),
	}

	if m := namespaceRe.FindStringSubmatch(androidBody); len(m) >= 2 {
		mod.Namespace = m[1]
	}

	if dcBody, hasDC := extractNamedBlock(ctx, androidBody, "defaultConfig"); hasDC {
		if m := applicationIDRe.FindStringSubmatch(dcBody); len(m) >= 2 {
			mod.ApplicationID = m[1]
		}
		mod.VersionCode = resolveVersionCode(ctx, dcBody, props, tomlVersions)
		mod.VersionName = resolveVersionName(ctx, dcBody, props, tomlVersions)
	}

	if mod.ApplicationID == "" {
		mod.ApplicationID = mod.Namespace
	}

	mod.FlavorDimensions = parseFlavorDimensions(ctx, androidBody)
	mod.BuildTypes = parseBuildTypes(ctx, androidBody)
	mod.ProductFlavors = parseProductFlavors(ctx, androidBody)
	mod.Variants = synthesizeVariants(ctx, mod)
	mod.Artifacts = DiscoverArtifacts(ctx, mod)

	return mod, true, nil
}

func hasAndroidAppPlugin(ctx context.Context, content string) bool {
	_ = ctx
	markers := []string{
		"com.android.application",
		"android.application",
		"androidApplication",
		"android-application",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func parseBuildTypes(ctx context.Context, androidBody string) []BuildType {
	byName := map[string]BuildType{
		BuildTypeDebug:   {Name: BuildTypeDebug},
		BuildTypeRelease: {Name: BuildTypeRelease},
	}
	order := []string{BuildTypeDebug, BuildTypeRelease}

	if btBody, ok := extractNamedBlock(ctx, androidBody, "buildTypes"); ok {
		for _, blk := range parseChildBlocks(ctx, btBody) {
			name := extractBlockName(ctx, blk.Header)
			if name == "" || name == "all" || name == "configureEach" {
				continue
			}
			bt := byName[name]
			bt.Name = name
			if m := applicationIDSuffixRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
				bt.ApplicationIDSuffix = m[1]
			}
			if m := versionNameSuffixRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
				bt.VersionNameSuffix = m[1]
			}
			if _, exists := byName[name]; !exists {
				order = append(order, name)
			}
			byName[name] = bt
		}
	}

	out := make([]BuildType, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func parseFlavorDimensions(ctx context.Context, androidBody string) []string {
	_ = ctx
	var dims []string
	seen := map[string]struct{}{}
	for _, match := range flavorDimensionsRe.FindAllStringSubmatch(androidBody, -1) {
		if len(match) < 2 {
			continue
		}
		for _, q := range quotedRe.FindAllStringSubmatch(match[1], -1) {
			if len(q) >= 2 {
				if _, ok := seen[q[1]]; !ok {
					seen[q[1]] = struct{}{}
					dims = append(dims, q[1])
				}
			}
		}
	}
	return dims
}

func parseProductFlavors(ctx context.Context, androidBody string) []ProductFlavor {
	pfBody, ok := extractNamedBlock(ctx, androidBody, "productFlavors")
	if !ok {
		return nil
	}

	blocks := parseChildBlocks(ctx, pfBody)
	flavors := make([]ProductFlavor, 0, len(blocks))
	for _, blk := range blocks {
		name := extractBlockName(ctx, blk.Header)
		if name == "" || name == "all" || name == "configureEach" {
			continue
		}
		pf := ProductFlavor{Name: name}
		if m := dimensionRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
			pf.Dimension = m[1]
		}
		if m := applicationIDRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
			pf.ApplicationID = m[1]
		}
		if m := applicationIDSuffixRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
			pf.ApplicationIDSuffix = m[1]
		}
		if m := versionNameSuffixRe.FindStringSubmatch(blk.Body); len(m) >= 2 {
			pf.VersionNameSuffix = m[1]
		}
		flavors = append(flavors, pf)
	}
	return flavors
}

func resolveVersionCode(ctx context.Context, dcBody string, props, tomlVersions map[string]string) int {
	if m := versionCodeIntRe.FindStringSubmatch(dcBody); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if m := versionCodeVarRe.FindStringSubmatch(dcBody); len(m) >= 2 {
		resolved := resolveExternalRef(ctx, m[1], props, tomlVersions)
		if n, err := strconv.Atoi(resolved); err == nil {
			return n
		}
	}
	return 0
}

func resolveVersionName(ctx context.Context, dcBody string, props, tomlVersions map[string]string) string {
	if m := versionNameStrRe.FindStringSubmatch(dcBody); len(m) >= 2 {
		return m[1]
	}
	if m := versionNameVarRe.FindStringSubmatch(dcBody); len(m) >= 2 {
		return resolveExternalRef(ctx, m[1], props, tomlVersions)
	}
	return ""
}

func resolveExternalRef(ctx context.Context, expr string, props, tomlVersions map[string]string) string {
	_ = ctx
	trimmed := strings.TrimSpace(expr)
	if m := propRefRe.FindStringSubmatch(trimmed); len(m) >= 2 {
		if val, ok := props[m[1]]; ok {
			return val
		}
	}
	if m := libsVersionRefRe.FindStringSubmatch(trimmed); len(m) >= 2 {
		key := strings.TrimSuffix(m[1], ".get")
		key = strings.TrimSuffix(key, ".toInt")
		if val, ok := tomlVersions[key]; ok {
			return val
		}
		dashed := strings.ReplaceAll(key, ".", "-")
		if val, ok := tomlVersions[dashed]; ok {
			return val
		}
	}
	if val, ok := props[trimmed]; ok {
		return val
	}
	return ""
}

func extractNamedBlock(ctx context.Context, src, name string) (string, bool) {
	_ = ctx
	for _, blk := range parseChildBlocks(ctx, src) {
		if extractBlockName(ctx, blk.Header) == name {
			return blk.Body, true
		}
	}
	return "", false
}

func extractBlockName(ctx context.Context, header string) string {
	_ = ctx
	trimmed := strings.TrimSpace(header)
	if m := delegatedBlockHeaderRe.FindStringSubmatch(trimmed); len(m) >= 2 {
		return m[1]
	}
	if m := namedBlockHeaderRe.FindStringSubmatch(trimmed); len(m) >= 2 {
		return m[1]
	}
	if m := identBlockHeaderRe.FindStringSubmatch(trimmed); len(m) >= 2 {
		return m[1]
	}
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		last := fields[len(fields)-1]
		if m := namedBlockHeaderRe.FindStringSubmatch(last); len(m) >= 2 {
			return m[1]
		}
		if m := identBlockHeaderRe.FindStringSubmatch(last); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

func parseChildBlocks(ctx context.Context, src string) []gradleBlock {
	_ = ctx
	var blocks []gradleBlock
	n := len(src)
	i := 0
	var headerStart int
	inQuote := byte(0)

	for i < n {
		ch := src[i]
		if inQuote != 0 {
			if ch == '\\' && i+1 < n {
				i += 2
				continue
			}
			if ch == inQuote {
				inQuote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			i++
			continue
		}
		if ch == '\n' || ch == ';' {
			headerStart = i + 1
			i++
			continue
		}
		if ch == '{' {
			header := strings.TrimSpace(src[headerStart:i])
			endIdx := findMatchingBrace(ctx, src, i)
			if endIdx < 0 {
				break
			}
			body := src[i+1 : endIdx]
			if header != "" {
				blocks = append(blocks, gradleBlock{
					Header: header,
					Body:   body,
				})
			}
			i = endIdx + 1
			headerStart = i
			continue
		}
		i++
	}
	return blocks
}

func findMatchingBrace(ctx context.Context, src string, openIdx int) int {
	_ = ctx
	depth := 0
	inQuote := byte(0)
	for i := openIdx; i < len(src); i++ {
		ch := src[i]
		if inQuote != 0 {
			if ch == '\\' && i+1 < len(src) {
				i++
				continue
			}
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func stripComments(ctx context.Context, src string) string {
	_ = ctx
	var b strings.Builder
	b.Grow(len(src))
	inQuote := byte(0)
	n := len(src)
	for i := 0; i < n; i++ {
		ch := src[i]
		if inQuote != 0 {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < n {
				i++
				b.WriteByte(src[i])
				continue
			}
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			b.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			if i < n {
				b.WriteByte('\n')
			}
			continue
		}
		if ch == '/' && i+1 < n && src[i+1] == '*' {
			i += 2
			for i+1 < n && (src[i] != '*' || src[i+1] != '/') {
				if src[i] == '\n' {
					b.WriteByte('\n')
				}
				i++
			}
			i++
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func loadGradleProperties(ctx context.Context, rootDir string) map[string]string {
	_ = ctx
	out := map[string]string{}
	file, err := os.Open(filepath.Join(rootDir, FileGradleProperties))
	if err != nil {
		return out
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	scanErr := scanner.Err()
	if closeErr := file.Close(); closeErr != nil || scanErr != nil {
		return out
	}
	return out
}

func loadVersionCatalog(ctx context.Context, rootDir string) map[string]string {
	_ = ctx
	out := map[string]string{}
	file, err := os.Open(filepath.Join(rootDir, FileLibsVersionsTOML))
	if err != nil {
		return out
	}

	inVersions := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inVersions = strings.TrimSpace(line[1:len(line)-1]) == "versions"
			continue
		}
		if !inVersions {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			val := strings.TrimSpace(v)
			val = strings.Trim(val, `"'`)
			out[strings.TrimSpace(k)] = val
		}
	}
	scanErr := scanner.Err()
	if closeErr := file.Close(); closeErr != nil || scanErr != nil {
		return out
	}
	return out
}
