package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/comments"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	// BinaryName is the CLI executable name.
	BinaryName = "bazaar"

	flagApp        = "app"
	flagYes        = "yes"
	flagNoUI       = "no-ui"
	flagPlain      = "plain"
	flagOutput     = "output"
	flagRTL        = "rtl"
	flagNoColor    = "no-color"
	flagVerbose    = "verbose"
	flagConfigPath = "config"
	flagProxy      = "proxy"

	cmdUseList   = "list"
	cmdUseStatus = "status"

	labelPackage     = "Package"
	labelPackageName = "Package Name"
	labelStatus      = "Status"
	labelVersion     = "Version"
	labelVariant     = "Variant"
	labelArtifact    = "Artifact"
	labelFile        = "File"
	labelStorage     = "Storage"
	labelNextStep    = "Next Step"

	jsonKeyPackageName = "package_name"
	jsonKeyBackend     = "backend"
	//nolint:gosec // JSON output key name, not a credential.
	jsonKeyToken = "token"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
	// Commit is set at build time via ldflags.
	Commit = "none"
	// BuildDate is set at build time via ldflags.
	BuildDate = "unknown"
)

// Dependencies allows injecting IO streams, working directory, stores, and API base URLs for testing.
type Dependencies struct {
	In               io.Reader
	Out              io.Writer
	Err              io.Writer
	WorkDir          string
	LookupEnv        func(string) (string, bool)
	SecretStore      auth.SecretStore
	PishkhanBaseURL  string
	CommentsBaseURL  string
	PublisherBaseURL string
}

// GlobalFlags holds persistent CLI flags shared across all commands.
type GlobalFlags struct {
	App        string
	Yes        bool
	NoUI       bool
	Plain      bool
	Output     string
	RTL        string
	NoColor    bool
	Verbose    bool
	ConfigPath string
	Proxy      string
}

// AppRuntime aggregates resolved configuration, Android detection, token store, and UI printer.
type AppRuntime struct {
	Deps           Dependencies
	Flags          GlobalFlags
	Env            config.EnvSettings
	Cwd            string
	ConfigPath     string
	Global         *config.GlobalConfig
	Project        *config.ProjectConfig
	ProjectCfgPath string
	Android        *android.Project
	Store          auth.SecretStore
	Printer        *output.Printer
}

// NewRootCmd constructs the complete bazaar Cobra command tree.
func NewRootCmd(ctx context.Context, deps Dependencies) *cobra.Command {
	if deps.In == nil {
		deps.In = os.Stdin
	}
	if deps.Out == nil {
		deps.Out = os.Stdout
	}
	if deps.Err == nil {
		deps.Err = os.Stderr
	}

	rt := &AppRuntime{Deps: deps}

	cmd := &cobra.Command{
		Use:           BinaryName,
		Short:         "CafeBazaar CLI for Android app publishing, releases, and user reviews",
		Long:          "bazaar brings the CafeBazaar Pishkhan Release API, Comments API, Nasher Publisher API, and Android Gradle release automation to your terminal and CI/CD pipelines.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			_ = args
			return rt.bootstrap(c.Context())
		},
	}

	cmd.SetIn(deps.In)
	cmd.SetOut(deps.Out)
	cmd.SetErr(deps.Err)

	pf := cmd.PersistentFlags()
	pf.StringVar(&rt.Flags.App, flagApp, "", "Target app package name or alias (overrides folder auto-detection)")
	pf.BoolVarP(&rt.Flags.Yes, flagYes, "y", false, "Confirm dangerous actions non-interactively (for CI/CD)")
	pf.BoolVar(&rt.Flags.NoUI, flagNoUI, false, "Disable interactive UI components and spinners (headless CLI mode)")
	pf.BoolVar(&rt.Flags.Plain, flagPlain, false, "Use plain line-oriented output without ANSI styling")
	pf.StringVarP(&rt.Flags.Output, flagOutput, "o", "", "Output format: table, plain, json, yaml")
	pf.StringVar(&rt.Flags.RTL, flagRTL, "", "Persian (Farsi) terminal shaping mode: auto, on, off")
	pf.BoolVar(&rt.Flags.NoColor, flagNoColor, false, "Disable colored output")
	pf.BoolVarP(&rt.Flags.Verbose, flagVerbose, "v", false, "Enable verbose diagnostics to stderr (logs config, commands, and HTTP traffic with all tokens redacted)")
	pf.StringVar(&rt.Flags.ConfigPath, flagConfigPath, "", "Path to global bazaar config.yaml")
	pf.StringVar(&rt.Flags.Proxy, flagProxy, "", "Proxy URL for CafeBazaar API calls (http://, https://, socks5://, socks5h://, or direct)")

	registerGlobalFlagCompletions(ctx, cmd, rt)

	cmd.AddCommand(
		newAuthCmd(ctx, rt),
		newInitCmd(ctx, rt),
		newStatusCmd(ctx, rt),
		newAndroidCmd(ctx, rt),
		newReleaseCmd(ctx, rt),
		newPublishCmd(ctx, rt),
		newReviewCmd(ctx, rt),
		newAppCmd(ctx, rt),
		newVersionCmd(ctx, rt),
	)

	return cmd
}

// ExitCode maps runtime errors to deterministic process exit codes for CI pipelines.
func ExitCode(ctx context.Context, err error) int {
	_ = ctx
	if err == nil {
		return output.ExitOK
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 0 {
		return exitErr.Code
	}
	if errors.Is(err, output.ErrConfirmationRequired) || errors.Is(err, output.ErrActionCancelled) {
		return output.ExitConfirmation
	}
	if errors.Is(err, config.ErrAppNotInitialized) || errors.Is(err, auth.ErrSecretNotFound) || httpclient.IsForbidden(ctx, err) {
		return output.ExitAuth
	}
	var apiErr *httpclient.APIError
	if errors.As(err, &apiErr) || errors.Is(err, pishkhan.ErrBundleFailed) {
		return output.ExitAPI
	}
	return output.ExitGeneral
}

func (r *AppRuntime) bootstrap(ctx context.Context) error {
	r.Env = config.ReadEnv(ctx, r.Deps.LookupEnv)

	cwd := strings.TrimSpace(r.Deps.WorkDir)
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
	}
	r.Cwd = cwd

	cfgPath := strings.TrimSpace(r.Flags.ConfigPath)
	if cfgPath == "" {
		cfgPath = r.Env.ConfigPath
	}
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultConfigPath(ctx)
		if err != nil {
			return err
		}
	}
	r.ConfigPath = cfgPath

	globalCfg, err := config.LoadGlobal(ctx, cfgPath)
	if err != nil {
		return err
	}
	r.Global = globalCfg

	projCfg, projPath, err := config.FindProjectConfig(ctx, cwd)
	if err != nil {
		return err
	}
	r.Project = projCfg
	r.ProjectCfgPath = projPath

	if androidProj, detErr := android.DetectProject(ctx, cwd); detErr == nil {
		r.Android = androidProj
	}

	mode := output.ResolveMode(ctx, output.ModeInput{
		FlagOutput:  r.Flags.Output,
		FlagNoUI:    r.Flags.NoUI,
		FlagPlain:   r.Flags.Plain,
		FlagNoColor: r.Flags.NoColor,
		FlagYes:     r.Flags.Yes,
		FlagVerbose: r.Flags.Verbose,
		FlagRTL:     r.Flags.RTL,
		ConfigRTL:   r.Global.RTLMode,
		Env:         r.Env,
		Stdin:       r.Deps.In,
		Stdout:      r.Deps.Out,
		LookupEnv:   r.Deps.LookupEnv,
	})
	r.Printer = output.NewPrinter(ctx, r.Deps.In, r.Deps.Out, r.Deps.Err, mode)

	if r.Deps.SecretStore != nil {
		r.Store = r.Deps.SecretStore
	} else {
		credPath := config.CredentialsPathForConfig(ctx, cfgPath)
		primary := auth.NewKeyringStore(ctx, auth.ServiceName)
		fallback := auth.NewFileStore(ctx, credPath)
		r.Store = auth.NewFallbackStore(ctx, primary, fallback, r.Env.NoKeyring, func(cbCtx context.Context, reason error) {
			if warnErr := r.Printer.PrintWarning(cbCtx, fmt.Sprintf("OS keychain unavailable (%v); storing credentials in %s (0600)", reason, credPath)); warnErr != nil {
				return
			}
		})
	}

	r.logBootstrapDiagnostics(ctx)

	return nil
}

func (r *AppRuntime) isVerbose(ctx context.Context) bool {
	_ = ctx
	return r != nil && r.Printer != nil && r.Printer.Mode.Verbose
}

func (r *AppRuntime) debugf(ctx context.Context, format string, args ...any) {
	if !r.isVerbose(ctx) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	msg = httpclient.ScrubSecrets(ctx, msg, r.envSecretValues(ctx))
	if err := r.Printer.PrintDebug(ctx, msg); err != nil {
		return
	}
}

func (r *AppRuntime) envSecretValues(ctx context.Context) []string {
	_ = ctx
	if r == nil {
		return nil
	}
	secrets := make([]string, 0, 2)
	if strings.TrimSpace(r.Env.AppToken) != "" {
		secrets = append(secrets, r.Env.AppToken)
	}
	if strings.TrimSpace(r.Env.PublisherToken) != "" {
		secrets = append(secrets, r.Env.PublisherToken)
	}
	return secrets
}

func (r *AppRuntime) httpDebugLogger(ctx context.Context) httpclient.DebugLogFunc {
	if !r.isVerbose(ctx) {
		return nil
	}
	return func(logCtx context.Context, msg string) {
		r.debugf(logCtx, "%s", msg)
	}
}

func (r *AppRuntime) logBootstrapDiagnostics(ctx context.Context) {
	if !r.isVerbose(ctx) {
		return
	}
	r.debugf(ctx, "CLI version=%s commit=%s cwd=%s", Version, Commit, r.Cwd)
	r.debugf(ctx, "Mode interactive=%v format=%s color=%v rtl=%v yes=%v width=%d",
		r.Printer.Mode.Interactive,
		r.Printer.Mode.Format,
		r.Printer.Mode.Color,
		r.Printer.Mode.RTL,
		r.Printer.Mode.Yes,
		r.Printer.Mode.Width,
	)
	r.debugf(ctx, "Global config path=%s registered_apps=%d default_app=%q",
		r.ConfigPath,
		len(r.Global.Apps),
		r.Global.DefaultApp,
	)
	if r.Project != nil {
		autoPub := "unset"
		if r.Project.AutoPublish != nil {
			autoPub = fmt.Sprintf("%v", *r.Project.AutoPublish)
		}
		r.debugf(ctx, "Project config path=%s app=%q module=%q default_variant=%q auto_publish=%s staged_rollout=%d staged_rollout_enabled=%v tag_prefix=%q proxy=%q",
			r.ProjectCfgPath,
			r.Project.App,
			r.Project.Module,
			r.Project.DefaultVariant,
			autoPub,
			r.Project.StagedRollout,
			r.Project.StagedRolloutEnabled,
			r.Project.TagPrefix,
			httpclient.MaskProxyURL(ctx, r.Project.Proxy),
		)
	} else {
		r.debugf(ctx, "Project config path=none")
	}
	if r.Android != nil {
		r.debugf(ctx, "Android project root=%s modules=%d packages=%v",
			r.Android.RootDir,
			len(r.Android.Modules),
			r.Android.UniquePackageNames(ctx),
		)
	} else {
		r.debugf(ctx, "Android project detected=false")
	}
	r.debugf(ctx, "Env ci=%v no_keyring=%v app=%q variant=%q rollout=%d proxy=%q app_token=%s publisher_token=%s",
		r.Env.CI,
		r.Env.NoKeyring,
		r.Env.App,
		r.Env.Variant,
		r.Env.Rollout,
		httpclient.MaskProxyURL(ctx, r.Env.Proxy),
		tokenPresenceLabel(ctx, r.Env.AppToken),
		tokenPresenceLabel(ctx, r.Env.PublisherToken),
	)
	if proxyURL := r.resolveProxyURL(ctx); proxyURL != "" {
		r.debugf(ctx, "Effective proxy=%s", httpclient.MaskProxyURL(ctx, proxyURL))
	} else {
		r.debugf(ctx, "Effective proxy=system-env")
	}
}

func tokenPresenceLabel(ctx context.Context, raw string) string {
	_ = ctx
	if strings.TrimSpace(raw) != "" {
		return httpclient.RedactedValue
	}
	return "unset"
}

func (r *AppRuntime) resolveActiveApp(ctx context.Context, variant string) (config.ResolvedApp, error) {
	var detected []string
	if r.Android != nil {
		detected = r.Android.UniquePackageNames(ctx)
	}
	resolved, err := config.ResolveApp(ctx, config.ResolveInput{
		FlagApp:          r.Flags.App,
		FlagVariant:      variant,
		Cwd:              r.Cwd,
		Env:              r.Env,
		Global:           r.Global,
		Project:          r.Project,
		DetectedPackages: detected,
		Store:            r.Store,
	})
	if err != nil {
		r.debugf(ctx, "App resolution failed (variant=%q): %v", variant, err)
		return resolved, &output.ExitError{Code: output.ExitAuth, Err: err}
	}
	r.debugf(ctx, "Resolved app package=%q resolved_by=%s backend=%s token=%s",
		resolved.PackageName,
		resolved.ResolvedBy,
		resolved.TokenSource,
		httpclient.RedactedValue,
	)
	return resolved, nil
}

func (r *AppRuntime) resolveProxyURL(ctx context.Context) string {
	_ = ctx
	if v := strings.TrimSpace(r.Flags.Proxy); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Env.Proxy); v != "" {
		return v
	}
	if r.Project != nil && strings.TrimSpace(r.Project.Proxy) != "" {
		return strings.TrimSpace(r.Project.Proxy)
	}
	if r.Global != nil && strings.TrimSpace(r.Global.Proxy) != "" {
		return strings.TrimSpace(r.Global.Proxy)
	}
	return ""
}

func (r *AppRuntime) newPishkhanClient(ctx context.Context, token string) (*pishkhan.Client, error) {
	proxyURL := r.resolveProxyURL(ctx)
	r.debugf(ctx, "Initializing Pishkhan API client (base_url=%q proxy=%q token=%s)",
		r.Deps.PishkhanBaseURL,
		httpclient.MaskProxyURL(ctx, proxyURL),
		httpclient.RedactedValue,
	)
	return pishkhan.New(ctx, pishkhan.Config{
		BaseURL:       r.Deps.PishkhanBaseURL,
		Token:         token,
		ClientAppName: httpclient.DefaultClientAppName,
		CLIVersion:    Version,
		ProxyURL:      proxyURL,
		DebugLog:      r.httpDebugLogger(ctx),
	})
}

func (r *AppRuntime) newCommentsClient(ctx context.Context, token string) (*comments.Client, error) {
	proxyURL := r.resolveProxyURL(ctx)
	r.debugf(ctx, "Initializing Comments API client (base_url=%q proxy=%q token=%s)",
		r.Deps.CommentsBaseURL,
		httpclient.MaskProxyURL(ctx, proxyURL),
		httpclient.RedactedValue,
	)
	return comments.New(ctx, comments.Config{
		BaseURL:       r.Deps.CommentsBaseURL,
		Token:         token,
		ClientAppName: httpclient.DefaultClientAppName,
		CLIVersion:    Version,
		ProxyURL:      proxyURL,
		DebugLog:      r.httpDebugLogger(ctx),
	})
}

func (r *AppRuntime) newPublisherClient(ctx context.Context, token, headerOverride, baseURLOverride string) (*publisher.Client, error) {
	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = r.Deps.PublisherBaseURL
	}
	if baseURL == "" && r.Global != nil && r.Global.Publisher != nil {
		baseURL = r.Global.Publisher.BaseURL
	}
	header := strings.TrimSpace(headerOverride)
	if header == "" && r.Global != nil && r.Global.Publisher != nil {
		header = r.Global.Publisher.AuthHeader
	}
	proxyURL := r.resolveProxyURL(ctx)
	r.debugf(ctx, "Initializing Nasher Publisher API client (base_url=%q auth_header=%q proxy=%q token=%s)",
		baseURL,
		header,
		httpclient.MaskProxyURL(ctx, proxyURL),
		httpclient.RedactedValue,
	)
	return publisher.New(ctx, publisher.Config{
		BaseURL:        baseURL,
		Token:          token,
		AuthHeaderName: header,
		ClientAppName:  httpclient.DefaultClientAppName,
		CLIVersion:     Version,
		ProxyURL:       proxyURL,
		DebugLog:       r.httpDebugLogger(ctx),
	})
}

func (r *AppRuntime) resolvePublisherToken(ctx context.Context) (string, string, error) {
	if r.Env.PublisherToken != "" {
		r.debugf(ctx, "Resolved Nasher publisher token backend=%s token=%s", auth.BackendEnv, httpclient.RedactedValue)
		return r.Env.PublisherToken, auth.BackendEnv, nil
	}
	account := auth.DefaultAccount
	if r.Global != nil && r.Global.Publisher != nil && r.Global.Publisher.Account != "" {
		account = r.Global.Publisher.Account
	}
	tok, backend, err := r.Store.Get(ctx, auth.PublisherKey(ctx, account))
	if err != nil {
		r.debugf(ctx, "Nasher publisher token lookup failed (account=%q): %v", account, err)
		if errors.Is(err, auth.ErrSecretNotFound) {
			return "", "", &output.ExitError{
				Code: output.ExitAuth,
				Err:  errors.New("developer Nasher publisher token not configured; run `bazaar auth login` or set CAFEBAZAAR_PUBLISHER_TOKEN"),
			}
		}
		return "", "", err
	}
	r.debugf(ctx, "Resolved Nasher publisher token account=%q backend=%s token=%s", account, backend, httpclient.RedactedValue)
	return tok, backend, nil
}

func readUTF8TextFlagOrFile(ctx context.Context, inline, filePath, envFallback string) (string, error) {
	_ = ctx
	if strings.TrimSpace(filePath) != "" {
		raw, err := os.ReadFile(strings.TrimSpace(filePath))
		if err != nil {
			return "", fmt.Errorf("read utf-8 text file %q: %w", filePath, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	if strings.TrimSpace(inline) != "" {
		return strings.TrimSpace(inline), nil
	}
	return strings.TrimSpace(envFallback), nil
}

func shortenDisplayPath(ctx context.Context, fullPath string) string {
	_ = ctx
	trimmed := strings.TrimSpace(fullPath)
	if trimmed == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return trimmed
	}
	cleanHome := filepath.Clean(home)
	cleanPath := filepath.Clean(trimmed)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "~" + string(filepath.Separator) + strings.TrimPrefix(cleanPath, prefix)
	}
	return cleanPath
}

func relativeDisplayPath(ctx context.Context, rootDir, targetPath string) string {
	if strings.TrimSpace(rootDir) != "" && strings.TrimSpace(targetPath) != "" {
		if rel, err := filepath.Rel(rootDir, targetPath); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return shortenDisplayPath(ctx, targetPath)
}

func joinShortPaths(ctx context.Context, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	shortened := make([]string, 0, len(paths))
	for _, p := range paths {
		shortened = append(shortened, shortenDisplayPath(ctx, p))
	}
	return strings.Join(shortened, ", ")
}
