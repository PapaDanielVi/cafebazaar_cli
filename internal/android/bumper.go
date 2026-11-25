package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// BumpKind controls how versionName is incremented alongside versionCode.
type BumpKind string

const (
	// BumpCode increments only versionCode and leaves versionName unchanged.
	BumpCode BumpKind = "code"
	// BumpPatch increments the patch segment of versionName and versionCode.
	BumpPatch BumpKind = "patch"
	// BumpMinor increments the minor segment of versionName and resets patch to 0.
	BumpMinor BumpKind = "minor"
	// BumpMajor increments the major segment of versionName and resets minor and patch to 0.
	BumpMajor BumpKind = "major"
)

var (
	// ErrInvalidBumpKind indicates an unsupported version bump strategy.
	ErrInvalidBumpKind = errors.New("bump kind must be one of code, patch, minor, major")
	// ErrVersionFieldNotFound indicates that versionCode or versionName could not be located in the Gradle file.
	ErrVersionFieldNotFound = errors.New("version field not found in Gradle build file")

	replaceVersionCodeRe = regexp.MustCompile(`(\bversionCode\s*(?:=|\s)\s*)(\d+)`)
	replaceVersionNameRe = regexp.MustCompile(`(\bversionName\s*(?:=|\s)\s*["'])([^"']+)(["'])`)
)

// BumpOptions configures how versionCode and versionName should be updated.
type BumpOptions struct {
	Kind           BumpKind
	MinVersionCode int
	VersionCode    int
	VersionName    string
}

// BumpPlan describes the computed version change for an Android module.
type BumpPlan struct {
	BuildFile      string `json:"build_file" yaml:"build_file"`
	OldVersionCode int    `json:"old_version_code" yaml:"old_version_code"`
	NewVersionCode int    `json:"new_version_code" yaml:"new_version_code"`
	OldVersionName string `json:"old_version_name" yaml:"old_version_name"`
	NewVersionName string `json:"new_version_name" yaml:"new_version_name"`
}

// ValidateBumpKind verifies that kind is a supported BumpKind.
func ValidateBumpKind(ctx context.Context, kind BumpKind) error {
	_ = ctx
	switch kind {
	case "", BumpCode, BumpPatch, BumpMinor, BumpMajor:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidBumpKind, kind)
	}
}

// BumpSemver increments a semantic version string according to kind.
func BumpSemver(ctx context.Context, current string, kind BumpKind) (string, error) {
	if err := ValidateBumpKind(ctx, kind); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(current)
	if kind == "" || kind == BumpCode || trimmed == "" {
		return trimmed, nil
	}

	core := trimmed
	if idx := strings.IndexAny(core, "-+"); idx >= 0 {
		core = core[:idx]
	}
	parts := strings.Split(core, ".")
	nums := make([]int, 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return "", fmt.Errorf("parse versionName %q: %w", current, err)
		}
		nums[i] = n
	}

	switch kind {
	case BumpMajor:
		nums[0]++
		nums[1] = 0
		nums[2] = 0
	case BumpMinor:
		nums[1]++
		nums[2] = 0
	case BumpPatch:
		nums[2]++
	}

	if len(parts) == 2 && kind != BumpPatch {
		return fmt.Sprintf("%d.%d", nums[0], nums[1]), nil
	}
	return fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2]), nil
}

// ComputeBump builds a BumpPlan from mod and opts without modifying any file on disk.
func ComputeBump(ctx context.Context, mod AppModule, opts BumpOptions) (BumpPlan, error) {
	if err := ValidateBumpKind(ctx, opts.Kind); err != nil {
		return BumpPlan{}, err
	}

	baseCode := max(mod.VersionCode, opts.MinVersionCode)
	newCode := baseCode + 1
	if opts.VersionCode > 0 {
		newCode = opts.VersionCode
	}

	newName := mod.VersionName
	if strings.TrimSpace(opts.VersionName) != "" {
		newName = strings.TrimSpace(opts.VersionName)
	} else if opts.Kind != "" && opts.Kind != BumpCode {
		bumped, err := BumpSemver(ctx, mod.VersionName, opts.Kind)
		if err != nil {
			return BumpPlan{}, err
		}
		newName = bumped
	}

	return BumpPlan{
		BuildFile:      mod.BuildFile,
		OldVersionCode: mod.VersionCode,
		NewVersionCode: newCode,
		OldVersionName: mod.VersionName,
		NewVersionName: newName,
	}, nil
}

// ApplyBump rewrites versionCode and versionName inside plan.BuildFile while preserving formatting.
func ApplyBump(ctx context.Context, plan BumpPlan, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply version bump cancelled: %w", err)
	}
	raw, err := os.ReadFile(plan.BuildFile)
	if err != nil {
		return fmt.Errorf("read build file %q: %w", plan.BuildFile, err)
	}
	content := string(raw)

	if !replaceVersionCodeRe.MatchString(content) {
		return fmt.Errorf("%w: versionCode in %s", ErrVersionFieldNotFound, plan.BuildFile)
	}
	updated := replaceFirstSubmatch(ctx, content, replaceVersionCodeRe, func(m []string) string {
		return m[1] + strconv.Itoa(plan.NewVersionCode)
	})

	if plan.NewVersionName != "" && plan.NewVersionName != plan.OldVersionName {
		if !replaceVersionNameRe.MatchString(updated) {
			return fmt.Errorf("%w: versionName in %s", ErrVersionFieldNotFound, plan.BuildFile)
		}
		updated = replaceFirstSubmatch(ctx, updated, replaceVersionNameRe, func(m []string) string {
			return m[1] + plan.NewVersionName + m[3]
		})
	}

	if dryRun {
		return nil
	}

	cleanBuildFile := filepath.Clean(plan.BuildFile)
	baseName := filepath.Base(cleanBuildFile)
	if baseName != FileBuildGroovy && baseName != FileBuildKTS {
		return fmt.Errorf("refusing to modify non-Gradle build file %q", cleanBuildFile)
	}
	info, err := os.Stat(cleanBuildFile)
	if err != nil {
		return fmt.Errorf("stat build file %q: %w", cleanBuildFile, err)
	}
	//nolint:gosec // Build file path is validated to be a Gradle build script.
	if err := os.WriteFile(cleanBuildFile, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write updated build file %q: %w", cleanBuildFile, err)
	}
	return nil
}

func replaceFirstSubmatch(ctx context.Context, s string, re *regexp.Regexp, repl func([]string) string) string {
	_ = ctx
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return s
	}
	groups := make([]string, len(loc)/2)
	for i := 0; i < len(loc); i += 2 {
		if loc[i] >= 0 && loc[i+1] >= 0 {
			groups[i/2] = s[loc[i]:loc[i+1]]
		}
	}
	return s[:loc[0]] + repl(groups) + s[loc[1]:]
}
