package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
)

const (
	// ResolutionEnvToken means the app token came directly from CAFEBAZAAR_APP_TOKEN.
	ResolutionEnvToken = "env_token"
	// ResolutionFlag means the app was chosen via --app or CAFEBAZAAR_APP.
	ResolutionFlag = "flag"
	// ResolutionProjectConfig means the app was chosen via .bazaar.yaml.
	ResolutionProjectConfig = "project_config"
	// ResolutionBoundPath means the app was matched by a bound project directory.
	ResolutionBoundPath = "bound_path"
	// ResolutionGradleMatch means the app was matched by auto-detected Gradle package names.
	ResolutionGradleMatch = "gradle_match"
	// ResolutionDefaultApp means the app came from GlobalConfig.DefaultApp.
	ResolutionDefaultApp = "default_app"
)

var (
	// ErrAppNotInitialized indicates that no app token is configured for the current project or flag.
	ErrAppNotInitialized = errors.New("app is not initialized; run `bazaar init` in your Android project folder")
	// ErrAmbiguousApp indicates that multiple registered apps match the current Android project.
	ErrAmbiguousApp = errors.New("multiple registered apps match this project; specify --app or run `bazaar init`")
)

// ResolveInput supplies all signals used to automatically select an app and its token.
type ResolveInput struct {
	FlagApp          string
	FlagVariant      string
	Cwd              string
	Env              EnvSettings
	Global           *GlobalConfig
	Project          *ProjectConfig
	DetectedPackages []string
	Store            auth.SecretStore
}

// ResolvedApp contains the selected app package name, secret token, and resolution metadata.
type ResolvedApp struct {
	PackageName string
	Alias       string
	Token       string
	TokenSource string
	ResolvedBy  string
	Candidates  []string
}

// ResolveApp determines the active app and loads its secret key from env or the CLI's token store.
func ResolveApp(ctx context.Context, in ResolveInput) (ResolvedApp, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedApp{}, fmt.Errorf("resolve app cancelled: %w", err)
	}

	if in.Env.AppToken != "" {
		pkg := strings.TrimSpace(in.FlagApp)
		if pkg == "" {
			pkg = strings.TrimSpace(in.Env.App)
		}
		if pkg == "" && in.Project != nil {
			pkg = in.Project.ResolveVariantPackage(ctx, in.FlagVariant)
		}
		if pkg == "" && len(in.DetectedPackages) == 1 {
			pkg = in.DetectedPackages[0]
		}
		return ResolvedApp{
			PackageName: pkg,
			Token:       in.Env.AppToken,
			TokenSource: auth.BackendEnv,
			ResolvedBy:  ResolutionEnvToken,
		}, nil
	}

	explicit := strings.TrimSpace(in.FlagApp)
	if explicit == "" {
		explicit = strings.TrimSpace(in.Env.App)
	}
	if explicit != "" {
		return loadRegisteredApp(ctx, in.Global, in.Store, explicit, ResolutionFlag)
	}

	if in.Project != nil {
		variant := strings.TrimSpace(in.FlagVariant)
		if variant == "" {
			variant = in.Env.Variant
		}
		if variant == "" {
			variant = in.Project.DefaultVariant
		}
		if pkg := in.Project.ResolveVariantPackage(ctx, variant); pkg != "" {
			return loadRegisteredApp(ctx, in.Global, in.Store, pkg, ResolutionProjectConfig)
		}
	}

	if in.Global != nil && strings.TrimSpace(in.Cwd) != "" {
		entry, ok, err := in.Global.MatchBoundPath(ctx, in.Cwd)
		if err != nil {
			return ResolvedApp{}, err
		}
		if ok {
			return loadRegisteredApp(ctx, in.Global, in.Store, entry.PackageName, ResolutionBoundPath)
		}
	}

	if in.Global != nil && len(in.DetectedPackages) > 0 {
		matched := matchDetectedPackages(ctx, in.Global, in.DetectedPackages)
		switch len(matched) {
		case 1:
			return loadRegisteredApp(ctx, in.Global, in.Store, matched[0], ResolutionGradleMatch)
		default:
			if len(matched) > 1 {
				return ResolvedApp{Candidates: matched}, fmt.Errorf("%w (%s)", ErrAmbiguousApp, strings.Join(matched, ", "))
			}
		}
	}

	if in.Global != nil && strings.TrimSpace(in.Global.DefaultApp) != "" {
		return loadRegisteredApp(ctx, in.Global, in.Store, in.Global.DefaultApp, ResolutionDefaultApp)
	}

	return ResolvedApp{Candidates: in.DetectedPackages}, ErrAppNotInitialized
}

func loadRegisteredApp(
	ctx context.Context,
	global *GlobalConfig,
	store auth.SecretStore,
	pkgOrAlias string,
	resolvedBy string,
) (ResolvedApp, error) {
	pkg := strings.TrimSpace(pkgOrAlias)
	var alias string
	if global != nil {
		if entry, ok := global.FindApp(ctx, pkgOrAlias); ok {
			pkg = entry.PackageName
			alias = entry.Alias
		}
	}

	if store == nil {
		return ResolvedApp{}, ErrAppNotInitialized
	}

	secret, backend, err := store.Get(ctx, auth.AppKey(ctx, pkg))
	if err != nil {
		if errors.Is(err, auth.ErrSecretNotFound) {
			return ResolvedApp{PackageName: pkg}, fmt.Errorf("no saved token for app %q: %w", pkg, ErrAppNotInitialized)
		}
		return ResolvedApp{}, fmt.Errorf("load token for app %q: %w", pkg, err)
	}

	return ResolvedApp{
		PackageName: pkg,
		Alias:       alias,
		Token:       secret,
		TokenSource: backend,
		ResolvedBy:  resolvedBy,
	}, nil
}

func matchDetectedPackages(ctx context.Context, global *GlobalConfig, detected []string) []string {
	_ = ctx
	seen := map[string]struct{}{}
	matched := make([]string, 0, len(detected))
	for _, pkg := range detected {
		if _, dup := seen[pkg]; dup {
			continue
		}
		seen[pkg] = struct{}{}
		if _, ok := global.Apps[pkg]; ok {
			matched = append(matched, pkg)
		}
	}
	return matched
}
