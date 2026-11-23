package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	pf.BoolVar(&rt.Flags.Verbose, flagVerbose, false, "Enable verbose diagnostics")
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
			_ = r.Printer.PrintWarning(cbCtx, fmt.Sprintf("OS keychain unavailable (%v); storing credentials in %s (0600)", reason, credPath))
		})
	}

	return nil
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
		return resolved, &output.ExitError{Code: output.ExitAuth, Err: err}
	}
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
	if r.Flags.Verbose && proxyURL != "" && r.Printer != nil {
		if err := r.Printer.PrintInfo(ctx, fmt.Sprintf("Using proxy: %s", httpclient.MaskProxyURL(ctx, proxyURL))); err != nil {
			return nil, err
		}
	}
	return pishkhan.New(ctx, pishkhan.Config{
		BaseURL:  r.Deps.PishkhanBaseURL,
		Token:    token,
		ProxyURL: proxyURL,
	})
}

func (r *AppRuntime) newCommentsClient(ctx context.Context, token string) (*comments.Client, error) {
	proxyURL := r.resolveProxyURL(ctx)
	if r.Flags.Verbose && proxyURL != "" && r.Printer != nil {
		if err := r.Printer.PrintInfo(ctx, fmt.Sprintf("Using proxy: %s", httpclient.MaskProxyURL(ctx, proxyURL))); err != nil {
			return nil, err
		}
	}
	return comments.New(ctx, comments.Config{
		BaseURL:  r.Deps.CommentsBaseURL,
		Token:    token,
		ProxyURL: proxyURL,
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
	if r.Flags.Verbose && proxyURL != "" && r.Printer != nil {
		if err := r.Printer.PrintInfo(ctx, fmt.Sprintf("Using proxy: %s", httpclient.MaskProxyURL(ctx, proxyURL))); err != nil {
			return nil, err
		}
	}
	return publisher.New(ctx, publisher.Config{
		BaseURL:        baseURL,
		Token:          token,
		AuthHeaderName: header,
		ProxyURL:       proxyURL,
	})
}

func (r *AppRuntime) resolvePublisherToken(ctx context.Context) (string, string, error) {
	if r.Env.PublisherToken != "" {
		return r.Env.PublisherToken, auth.BackendEnv, nil
	}
	account := auth.DefaultAccount
	if r.Global != nil && r.Global.Publisher != nil && r.Global.Publisher.Account != "" {
		account = r.Global.Publisher.Account
	}
	tok, backend, err := r.Store.Get(ctx, auth.PublisherKey(ctx, account))
	if err != nil {
		if errors.Is(err, auth.ErrSecretNotFound) {
			return "", "", &output.ExitError{
				Code: output.ExitAuth,
				Err:  errors.New("developer Nasher publisher token not configured; run `bazaar auth login` or set CAFEBAZAAR_PUBLISHER_TOKEN"),
			}
		}
		return "", "", err
	}
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
