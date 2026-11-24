package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	initModeExisting = "existing"
	initModeNew      = "new"
)

func newInitCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var pkgFlag string
	var variantFlag string
	var modeFlag string
	var tokenFlag string
	var tokenStdin bool
	var artifactFlag string
	var aliasFlag string
	var skipVerify bool
	var extraFields []string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize CafeBazaar in the current Android app folder",
		Long:  "Detects Gradle variants and package names, links an existing CafeBazaar app token (or submits a brand-new app via the Nasher token and captures its token), saves the secret in bazaar's background store, and writes .bazaar.yaml.",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runInit(c.Context(), rt, initOptions{
				PackageName: pkgFlag,
				Variant:     variantFlag,
				Mode:        modeFlag,
				Token:       tokenFlag,
				TokenStdin:  tokenStdin,
				Artifact:    artifactFlag,
				Alias:       aliasFlag,
				SkipVerify:  skipVerify,
				ExtraFields: extraFields,
			})
		},
	}

	cmd.Flags().StringVar(&pkgFlag, "package", "", "Android package name to initialize")
	cmd.Flags().StringVar(&variantFlag, "variant", "", "Gradle build variant to bind as default")
	cmd.Flags().StringVar(&modeFlag, "mode", "", "App onboarding mode: existing or new")
	cmd.Flags().StringVar(&tokenFlag, "token", "", "App Pishkhan token for existing apps")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read app Pishkhan token from stdin")
	cmd.Flags().StringVar(&artifactFlag, "artifact", "", "Initial APK or AAB path when creating a new app")
	cmd.Flags().StringVar(&aliasFlag, "alias", "", "Optional short alias for this app")
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skip live Pishkhan token verification")
	cmd.Flags().StringArrayVar(&extraFields, "field", nil, "Additional key=value field for Nasher POST /v1/apps/ (repeatable)")

	registerFlagCompletion(ctx, cmd, "package", completeAppPackages(rt))
	registerFlagCompletion(ctx, cmd, "variant", completeGradleVariants(rt))
	registerFlagCompletion(ctx, cmd, "artifact", completeArtifactPaths(rt, android.ArtifactTypeAPK, "apk"))
	return cmd
}

type initOptions struct {
	PackageName string
	Variant     string
	Mode        string
	Token       string
	TokenStdin  bool
	Artifact    string
	Alias       string
	SkipVerify  bool
	ExtraFields []string
}

func runInit(ctx context.Context, rt *AppRuntime, opts initOptions) error {
	projectDir := projectRootOrCwd(ctx, rt)
	var selectedModule android.AppModule
	var selectedVariant android.Variant
	variantMap := map[string]string{}

	if rt.Android != nil {
		mod, ok := rt.Android.PrimaryModule(ctx, "")
		if ok {
			selectedModule = mod
			for _, v := range mod.Variants {
				if v.PackageName != "" {
					variantMap[v.Name] = v.PackageName
				}
			}
			if opts.Variant != "" {
				v, err := mod.FindVariant(ctx, opts.Variant)
				if err != nil {
					return err
				}
				selectedVariant = v
			} else if opts.PackageName != "" {
				if v, err := mod.FindVariant(ctx, opts.PackageName); err == nil {
					selectedVariant = v
				}
			}
		}
	}

	pkg := strings.TrimSpace(opts.PackageName)
	if pkg == "" && selectedVariant.PackageName != "" {
		pkg = selectedVariant.PackageName
	}
	if pkg == "" {
		chosenPkg, chosenVar, err := promptPackageFromAndroid(ctx, rt)
		if err != nil {
			return err
		}
		pkg = chosenPkg
		if selectedVariant.Name == "" && chosenVar != "" {
			selectedVariant.Name = chosenVar
		}
	}

	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		if !rt.Printer.Mode.Interactive {
			mode = initModeExisting
		} else {
			chosenMode, err := rt.Printer.Select(ctx,
				fmt.Sprintf("Initialize %s", pkg),
				"Is this app already created in CafeBazaar Pishkhan, or is it a brand-new app?",
				[]output.SelectOption{
					{Label: "Existing app on CafeBazaar (generate & paste app token from Pishkhan dashboard)", Value: initModeExisting},
					{Label: "New app (submit initial version with Nasher developer token & auto-save app token)", Value: initModeNew},
				},
			)
			if err != nil {
				return err
			}
			mode = chosenMode
		}
	}

	var token string
	var tokenSource string
	switch mode {
	case initModeExisting:
		tokenSource = config.TokenSourceManual
		if err := rt.Printer.PrintInfo(ctx, fmt.Sprintf("Open %s -> Select %s -> API Pishkhan -> Generate New Token.", pishkhanDashboardURL, pkg)); err != nil {
			return err
		}
		var err error
		token, err = readTokenInput(ctx, rt, opts.Token, opts.TokenStdin,
			fmt.Sprintf("Paste Pishkhan App Token for %s", pkg),
			"Stored securely in bazaar's background credential store, never committed to Git.",
		)
		if err != nil {
			return err
		}
		if !opts.SkipVerify {
			if err := verifyAppToken(ctx, rt, token); err != nil {
				return err
			}
		}

	case initModeNew:
		tokenSource = config.TokenSourcePublisherAPI
		issuedToken, err := submitNewAppDuringInit(ctx, rt, pkg, selectedModule, opts)
		if err != nil {
			return err
		}
		if issuedToken != "" {
			token = issuedToken
		} else {
			if err := rt.Printer.PrintWarning(ctx, "CafeBazaar did not return an automatic app token. Generate one from the app page in Pishkhan and paste it below."); err != nil {
				return err
			}
			tokenSource = config.TokenSourceManual
			token, err = readTokenInput(ctx, rt, opts.Token, opts.TokenStdin,
				fmt.Sprintf("Fallback Pishkhan App Token for %s", pkg),
				fmt.Sprintf("Open %s -> Select %s -> API Pishkhan -> Generate New Token.", pishkhanDashboardURL, pkg),
			)
			if err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("invalid --mode %q: must be %q or %q", mode, initModeExisting, initModeNew)
	}

	backend, err := saveAppTokenAndMetadata(ctx, rt, saveAppInput{
		PackageName:    pkg,
		Token:          token,
		Alias:          opts.Alias,
		TokenSource:    tokenSource,
		DefaultVariant: selectedVariant.Name,
		BindDir:        projectDir,
		SetDefault:     rt.Global.DefaultApp == "",
	})
	if err != nil {
		return err
	}

	projCfg := &config.ProjectConfig{
		App:            pkg,
		Module:         selectedModule.Name,
		DefaultVariant: selectedVariant.Name,
		Apps:           variantMap,
	}
	cfgFilePath, err := config.SaveProjectConfig(ctx, projectDir, projCfg)
	if err != nil {
		return err
	}

	result := map[string]any{
		jsonKeyPackageName: pkg,
		"variant":          selectedVariant.Name,
		"mode":             mode,
		"token_source":     tokenSource,
		"storage":          backend,
		jsonKeyToken:       auth.MaskToken(ctx, token),
		"project_config":   cfgFilePath,
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, result); printed || pErr != nil {
		return pErr
	}

	if err := rt.Printer.PrintCard(ctx, "CafeBazaar App Initialized", []output.KeyValue{
		{Key: labelStatus, Value: rt.Printer.Theme.RenderBadge(ctx, "READY")},
		{Key: labelPackageName, Value: pkg},
		{Key: "Default Variant", Value: fallbackDash(ctx, selectedVariant.Name)},
		{Key: "Secret Token", Value: fmt.Sprintf("%s (%s)", auth.MaskToken(ctx, token), backend)},
		{Key: "Project File", Value: cfgFilePath},
	}); err != nil {
		return err
	}
	return rt.Printer.PrintInfo(ctx, "Whenever you cd into this folder, bazaar will automatically use this app's saved token.")
}

func submitNewAppDuringInit(
	ctx context.Context,
	rt *AppRuntime,
	pkg string,
	mod android.AppModule,
	opts initOptions,
) (string, error) {
	pubToken, _, err := rt.resolvePublisherToken(ctx)
	if err != nil {
		return "", err
	}

	artifactPath := strings.TrimSpace(opts.Artifact)
	if artifactPath == "" && len(mod.Artifacts) > 0 {
		for _, a := range mod.Artifacts {
			if a.Type == android.ArtifactTypeAPK || a.Type == android.ArtifactTypeAAB {
				artifactPath = a.Path
				break
			}
		}
	}
	if artifactPath == "" && rt.Printer.Mode.Interactive {
		artifactPath, err = rt.Printer.Input(ctx, "Initial APK or AAB Path", "Path to the initial release package for CafeBazaar review (leave blank for metadata-only JSON).", "app/build/outputs/apk/release/app-release.apk", "", false)
		if err != nil {
			return "", err
		}
	}

	extra, err := parseKeyValueFlags(ctx, opts.ExtraFields)
	if err != nil {
		return "", err
	}

	if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:     "Create New App via Nasher Publisher API",
		TargetApp: pkg,
		Warning:   "This creates a brand-new app on your CafeBazaar developer account and submits its first version for review.",
		Details: []output.KeyValue{
			{Key: "Endpoint", Value: "POST " + publisher.EndpointApps},
			{Key: "Issue App Token", Value: "true (issue_api_agent)"},
			{Key: labelArtifact, Value: fallbackDash(ctx, artifactPath)},
		},
		PromptLabel: fmt.Sprintf("Submit new app %s to CafeBazaar?", pkg),
	}); err != nil {
		return "", err
	}

	pubClient, err := rt.newPublisherClient(ctx, pubToken, "", "")
	if err != nil {
		return "", err
	}

	onProgress, finish := rt.Printer.UploadProgress(ctx, "Submitting new app")
	resp, err := pubClient.CreateApp(ctx, publisher.CreateAppRequest{
		PackageName:   pkg,
		IssueAPIAgent: true,
		ArtifactPath:  artifactPath,
		ExtraFields:   extra,
	}, onProgress)
	finish(ctx)
	if err != nil {
		return "", err
	}

	for _, w := range resp.Warnings {
		if wErr := rt.Printer.PrintWarning(ctx, w); wErr != nil {
			return "", wErr
		}
	}
	return resp.AppToken, nil
}

func parseKeyValueFlags(ctx context.Context, pairs []string) (map[string]string, error) {
	_ = ctx
	out := make(map[string]string, len(pairs))
	for _, raw := range pairs {
		k, v, ok := strings.Cut(raw, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid --field %q: expected key=value", raw)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}

func fallbackDash(ctx context.Context, s string) string {
	_ = ctx
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func requireInitializedApp(ctx context.Context, rt *AppRuntime, variant string) (config.ResolvedApp, error) {
	resolved, err := rt.resolveActiveApp(ctx, variant)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, config.ErrAppNotInitialized) && rt.Printer.Mode.Interactive && rt.Android != nil {
		if warnErr := rt.Printer.PrintWarning(ctx, "This Android app folder is not initialized with a CafeBazaar app token yet."); warnErr != nil {
			return config.ResolvedApp{}, warnErr
		}
		runNow, cErr := rt.Printer.Confirm(ctx, "Run `bazaar init` now?", "Initialize this folder and save its CafeBazaar token.", true)
		if cErr == nil && runNow {
			if initErr := runInit(ctx, rt, initOptions{Variant: variant}); initErr != nil {
				return config.ResolvedApp{}, initErr
			}
			// Reload project config and retry resolution.
			if projCfg, projPath, findErr := config.FindProjectConfig(ctx, rt.Cwd); findErr == nil {
				rt.Project = projCfg
				rt.ProjectCfgPath = projPath
			}
			return rt.resolveActiveApp(ctx, variant)
		}
	}
	return config.ResolvedApp{}, err
}
