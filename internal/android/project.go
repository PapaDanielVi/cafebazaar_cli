package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// FileSettingsKTS is the Kotlin DSL Gradle settings filename.
	FileSettingsKTS = "settings.gradle.kts"
	// FileSettingsGroovy is the Groovy DSL Gradle settings filename.
	FileSettingsGroovy = "settings.gradle"
	// FileBuildKTS is the Kotlin DSL Gradle build filename.
	FileBuildKTS = "build.gradle.kts"
	// FileBuildGroovy is the Groovy DSL Gradle build filename.
	FileBuildGroovy = "build.gradle"
	// FileGradleProperties is the Gradle properties filename.
	FileGradleProperties = "gradle.properties"
	// FileLibsVersionsTOML is the default Gradle version catalog relative path.
	FileLibsVersionsTOML = "gradle/libs.versions.toml"
	// FileGradleWrapper is the Unix Gradle wrapper executable name.
	FileGradleWrapper = "gradlew"

	// BuildTypeDebug is the implicit Android debug build type.
	BuildTypeDebug = "debug"
	// BuildTypeRelease is the implicit Android release build type.
	BuildTypeRelease = "release"

	defaultAppModule = ":app"
)

var (
	// ErrNotAndroidProject indicates that no Gradle Android application module was found.
	ErrNotAndroidProject = errors.New("no Android Gradle project found from current directory")
	// ErrVariantNotFound indicates that the requested build variant does not exist.
	ErrVariantNotFound = errors.New("android build variant not found")

	includeRe = regexp.MustCompile(`include\s*\(?([^)\r\n]+)\)?`)
	quotedRe  = regexp.MustCompile(`["']([^"']+)["']`)
)

// BuildType describes an Android Gradle buildType block.
type BuildType struct {
	Name                string `json:"name" yaml:"name"`
	ApplicationIDSuffix string `json:"application_id_suffix,omitempty" yaml:"application_id_suffix,omitempty"`
	VersionNameSuffix   string `json:"version_name_suffix,omitempty" yaml:"version_name_suffix,omitempty"`
}

// ProductFlavor describes an Android Gradle productFlavor block.
type ProductFlavor struct {
	Name                string `json:"name" yaml:"name"`
	Dimension           string `json:"dimension,omitempty" yaml:"dimension,omitempty"`
	ApplicationID       string `json:"application_id,omitempty" yaml:"application_id,omitempty"`
	ApplicationIDSuffix string `json:"application_id_suffix,omitempty" yaml:"application_id_suffix,omitempty"`
	VersionNameSuffix   string `json:"version_name_suffix,omitempty" yaml:"version_name_suffix,omitempty"`
}

// Variant represents a synthesized Android build variant (productFlavors x buildTypes).
type Variant struct {
	ModuleName   string   `json:"module_name" yaml:"module_name"`
	Name         string   `json:"name" yaml:"name"`
	BuildType    string   `json:"build_type" yaml:"build_type"`
	Flavors      []string `json:"flavors,omitempty" yaml:"flavors,omitempty"`
	PackageName  string   `json:"package_name" yaml:"package_name"`
	VersionCode  int      `json:"version_code" yaml:"version_code"`
	VersionName  string   `json:"version_name" yaml:"version_name"`
	AssembleTask string   `json:"assemble_task" yaml:"assemble_task"`
	BundleTask   string   `json:"bundle_task" yaml:"bundle_task"`
}

// AppModule describes a detected Android application module.
type AppModule struct {
	Name             string          `json:"name" yaml:"name"`
	Dir              string          `json:"dir" yaml:"dir"`
	BuildFile        string          `json:"build_file" yaml:"build_file"`
	IsKotlinDSL      bool            `json:"is_kotlin_dsl" yaml:"is_kotlin_dsl"`
	Namespace        string          `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	ApplicationID    string          `json:"application_id" yaml:"application_id"`
	VersionCode      int             `json:"version_code" yaml:"version_code"`
	VersionName      string          `json:"version_name" yaml:"version_name"`
	FlavorDimensions []string        `json:"flavor_dimensions,omitempty" yaml:"flavor_dimensions,omitempty"`
	BuildTypes       []BuildType     `json:"build_types" yaml:"build_types"`
	ProductFlavors   []ProductFlavor `json:"product_flavors,omitempty" yaml:"product_flavors,omitempty"`
	Variants         []Variant       `json:"variants" yaml:"variants"`
	Artifacts        []Artifact      `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
}

// Project describes a detected Android Gradle project rooted at RootDir.
type Project struct {
	RootDir    string      `json:"root_dir" yaml:"root_dir"`
	HasWrapper bool        `json:"has_wrapper" yaml:"has_wrapper"`
	Modules    []AppModule `json:"modules" yaml:"modules"`
}

// DetectProject walks upward from startDir and parses all Android application modules.
func DetectProject(ctx context.Context, startDir string) (*Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("detect android project cancelled: %w", err)
	}
	absStart, err := filepath.Abs(strings.TrimSpace(startDir))
	if err != nil {
		return nil, fmt.Errorf("resolve directory %q: %w", startDir, err)
	}

	rootDir, settingsPath, found := findProjectRoot(ctx, absStart)
	if !found {
		return nil, ErrNotAndroidProject
	}

	props := loadGradleProperties(ctx, rootDir)
	tomlVersions := loadVersionCatalog(ctx, rootDir)

	modulePaths := discoverModulePaths(ctx, rootDir, settingsPath)
	modules := make([]AppModule, 0, len(modulePaths))
	for _, modSpec := range modulePaths {
		mod, ok, parseErr := parseModuleIfAndroidApp(ctx, rootDir, modSpec, props, tomlVersions)
		if parseErr != nil {
			return nil, parseErr
		}
		if ok {
			modules = append(modules, mod)
		}
	}

	if len(modules) == 0 {
		return nil, ErrNotAndroidProject
	}

	sort.Slice(modules, func(i, j int) bool {
		if modules[i].Name == defaultAppModule && modules[j].Name != defaultAppModule {
			return true
		}
		if modules[j].Name == defaultAppModule && modules[i].Name != defaultAppModule {
			return false
		}
		return modules[i].Name < modules[j].Name
	})

	_, wrapperErr := os.Stat(filepath.Join(rootDir, FileGradleWrapper))
	return &Project{
		RootDir:    rootDir,
		HasWrapper: wrapperErr == nil,
		Modules:    modules,
	}, nil
}

// PrimaryModule returns the module matching preferredName, or the first detected app module.
func (p *Project) PrimaryModule(ctx context.Context, preferredName string) (AppModule, bool) {
	_ = ctx
	if p == nil || len(p.Modules) == 0 {
		return AppModule{}, false
	}
	want := strings.TrimSpace(preferredName)
	if want != "" {
		normalized := normalizeModuleName(ctx, want)
		for _, mod := range p.Modules {
			if mod.Name == normalized || strings.TrimPrefix(mod.Name, ":") == strings.TrimPrefix(normalized, ":") {
				return mod, true
			}
		}
	}
	return p.Modules[0], true
}

// AllVariants returns every synthesized build variant across all detected application modules.
func (p *Project) AllVariants(ctx context.Context) []Variant {
	_ = ctx
	if p == nil {
		return nil
	}
	var out []Variant
	for _, mod := range p.Modules {
		out = append(out, mod.Variants...)
	}
	return out
}

// UniquePackageNames returns deduplicated package names across all detected build variants.
func (p *Project) UniquePackageNames(ctx context.Context) []string {
	variants := p.AllVariants(ctx)
	seen := make(map[string]struct{}, len(variants))
	pkgs := make([]string, 0, len(variants))
	for _, v := range variants {
		if v.PackageName == "" {
			continue
		}
		if _, exists := seen[v.PackageName]; exists {
			continue
		}
		seen[v.PackageName] = struct{}{}
		pkgs = append(pkgs, v.PackageName)
	}
	return pkgs
}

// FindVariant finds a build variant by variant name or resolved package name.
func (m AppModule) FindVariant(ctx context.Context, nameOrPkg string) (Variant, error) {
	_ = ctx
	query := strings.TrimSpace(nameOrPkg)
	if query == "" {
		return Variant{}, ErrVariantNotFound
	}
	for _, v := range m.Variants {
		if strings.EqualFold(v.Name, query) || v.PackageName == query {
			return v, nil
		}
	}
	return Variant{}, fmt.Errorf("%w: %q", ErrVariantNotFound, query)
}

func findProjectRoot(ctx context.Context, startDir string) (string, string, bool) {
	_ = ctx
	dir := startDir
	var fallbackRoot string
	for {
		for _, settingsName := range []string{FileSettingsKTS, FileSettingsGroovy} {
			candidate := filepath.Join(dir, settingsName)
			if _, err := os.Stat(candidate); err == nil {
				return dir, candidate, true
			}
		}
		if fallbackRoot == "" {
			if _, err := os.Stat(filepath.Join(dir, FileGradleWrapper)); err == nil {
				fallbackRoot = dir
			} else if buildFile := findBuildFile(ctx, dir); buildFile != "" {
				if isAndroidAppFile(ctx, buildFile) {
					fallbackRoot = dir
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if fallbackRoot != "" {
		return fallbackRoot, "", true
	}
	return "", "", false
}

func discoverModulePaths(ctx context.Context, rootDir, settingsPath string) []string {
	seen := map[string]struct{}{}
	add := func(spec string) []string {
		return nil
	}
	_ = add

	var specs []string
	appendUnique := func(mod string) {
		norm := normalizeModuleName(ctx, mod)
		if _, ok := seen[norm]; ok {
			return
		}
		seen[norm] = struct{}{}
		specs = append(specs, norm)
	}

	if settingsPath != "" {
		if raw, err := os.ReadFile(settingsPath); err == nil {
			cleaned := stripComments(ctx, string(raw))
			for _, match := range includeRe.FindAllStringSubmatch(cleaned, -1) {
				if len(match) < 2 {
					continue
				}
				for _, q := range quotedRe.FindAllStringSubmatch(match[1], -1) {
					if len(q) >= 2 {
						appendUnique(q[1])
					}
				}
			}
		}
	}

	if len(specs) == 0 {
		if findBuildFile(ctx, filepath.Join(rootDir, "app")) != "" {
			appendUnique(defaultAppModule)
		}
		if findBuildFile(ctx, rootDir) != "" {
			appendUnique(":")
		}
	}

	return specs
}

func normalizeModuleName(ctx context.Context, raw string) string {
	_ = ctx
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == ":" || trimmed == "." {
		return ":"
	}
	if !strings.HasPrefix(trimmed, ":") {
		trimmed = ":" + trimmed
	}
	return trimmed
}

func moduleDirFromSpec(ctx context.Context, rootDir, spec string) string {
	_ = ctx
	if spec == ":" {
		return rootDir
	}
	parts := strings.Split(strings.TrimPrefix(spec, ":"), ":")
	elems := append([]string{rootDir}, parts...)
	return filepath.Join(elems...)
}

func findBuildFile(ctx context.Context, dir string) string {
	_ = ctx
	for _, name := range []string{FileBuildKTS, FileBuildGroovy} {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func isAndroidAppFile(ctx context.Context, buildFile string) bool {
	raw, err := os.ReadFile(buildFile)
	if err != nil {
		return false
	}
	content := stripComments(ctx, string(raw))
	return hasAndroidAppPlugin(ctx, content)
}
