package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	initModeExisting        = "existing"
	initModeNew             = "new"
	initCustomPackageOption = "__custom__"
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
				matching := findVariantsByPackage(ctx, mod, opts.PackageName)
				switch {
				case len(matching) > 1 && rt.Printer.Mode.Interactive:
					pickedVariant, _, err := SelectInitVariant(ctx, rt.Printer, mod, opts.PackageName)
					if err != nil {
						return err
					}
					selectedVariant = pickedVariant
				case len(matching) > 0:
					selectedVariant = matching[0]
				default:
					if v, err := mod.FindVariant(ctx, opts.PackageName); err == nil {
						selectedVariant = v
					}
				}
			}
		}
	}

	pkg := strings.TrimSpace(opts.PackageName)
	if pkg == "" && selectedVariant.PackageName != "" {
		pkg = selectedVariant.PackageName
	}
	if pkg == "" {
		if selectedModule.Name != "" && len(selectedModule.Variants) > 0 && rt.Printer.Mode.Interactive {
			pickedVariant, pickedPkg, err := SelectInitVariant(ctx, rt.Printer, selectedModule, "")
			if err != nil {
				return err
			}
			selectedVariant = pickedVariant
			pkg = pickedPkg
		} else {
			chosenPkg, chosenVar, err := promptPackageFromAndroid(ctx, rt)
			if err != nil {
				return err
			}
			pkg = chosenPkg
			if selectedVariant.Name == "" && chosenVar != "" {
				if v, findErr := selectedModule.FindVariant(ctx, chosenVar); findErr == nil {
					selectedVariant = v
				} else {
					selectedVariant.Name = chosenVar
				}
			}
		}
	}

	if selectedModule.Name != "" {
		var err error
		selectedVariant, pkg, err = ResolveTaggedRepoVariant(ctx, rt.Printer, projectDir, selectedModule, selectedVariant, opts.Variant, pkg)
		if err != nil {
			return err
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

// SelectInitVariant lists all detected Gradle build variants (without collapsing variants
// that share a package name) so the developer can choose the target variant for bazaar init.
func SelectInitVariant(
	ctx context.Context,
	printer *output.Printer,
	mod android.AppModule,
	filterPkg string,
) (android.Variant, string, error) {
	if printer == nil {
		return android.Variant{}, "", errors.New("printer must not be nil")
	}

	candidates := mod.Variants
	trimmedPkg := strings.TrimSpace(filterPkg)
	if trimmedPkg != "" {
		if matched := findVariantsByPackage(ctx, mod, trimmedPkg); len(matched) > 0 {
			candidates = matched
		}
	}

	ordered := orderVariantsForInit(ctx, candidates)
	if len(ordered) == 0 {
		pkg, err := printer.Input(ctx, "Android Package Name", "For example: com.example.myapp", "com.example.myapp", trimmedPkg, false)
		if err != nil {
			return android.Variant{}, "", err
		}
		if strings.TrimSpace(pkg) == "" {
			return android.Variant{}, "", errors.New("package name must not be empty")
		}
		return android.Variant{}, strings.TrimSpace(pkg), nil
	}

	opts := make([]output.SelectOption, 0, len(ordered)+1)
	byName := make(map[string]android.Variant, len(ordered))
	for _, v := range ordered {
		byName[v.Name] = v
		opts = append(opts, output.SelectOption{
			Label: formatInitVariantLabel(ctx, v),
			Value: v.Name,
		})
	}
	if trimmedPkg == "" {
		opts = append(opts, output.SelectOption{
			Label: "Enter custom package name...",
			Value: initCustomPackageOption,
		})
	}

	title := "Select Target Build Variant"
	desc := "Detected Gradle buildTypes and productFlavors. Choose the target variant for CafeBazaar releases."
	if trimmedPkg != "" {
		title = fmt.Sprintf("Select Target Build Variant for %s", trimmedPkg)
		desc = "Multiple Gradle build variants share this package name. Choose the default release variant."
	}

	chosen, err := printer.Select(ctx, title, desc, opts)
	if err != nil {
		return android.Variant{}, "", err
	}
	if chosen == initCustomPackageOption {
		pkg, inErr := printer.Input(ctx, "Android Package Name", "For example: com.example.myapp", "com.example.myapp", "", false)
		if inErr != nil {
			return android.Variant{}, "", inErr
		}
		if strings.TrimSpace(pkg) == "" {
			return android.Variant{}, "", errors.New("package name must not be empty")
		}
		return android.Variant{}, strings.TrimSpace(pkg), nil
	}

	selected, ok := byName[chosen]
	if !ok {
		return android.Variant{}, "", fmt.Errorf("selected variant %q not found", chosen)
	}
	pkg := selected.PackageName
	if pkg == "" {
		pkg = trimmedPkg
	}
	if pkg == "" {
		var inErr error
		pkg, inErr = printer.Input(ctx, "Android Package Name", "For example: com.example.myapp", "com.example.myapp", "", false)
		if inErr != nil {
			return android.Variant{}, "", inErr
		}
		if strings.TrimSpace(pkg) == "" {
			return android.Variant{}, "", errors.New("package name must not be empty")
		}
	}
	return selected, strings.TrimSpace(pkg), nil
}

// ResolveTaggedRepoVariant prompts the developer to switch from a debug variant to a
// non-debug target variant when initializing inside a Git repository that already has tags.
func ResolveTaggedRepoVariant(
	ctx context.Context,
	printer *output.Printer,
	projectDir string,
	mod android.AppModule,
	current android.Variant,
	explicitVariant string,
	currentPkg string,
) (android.Variant, string, error) {
	if strings.TrimSpace(explicitVariant) != "" || printer == nil || !printer.Mode.Interactive {
		return current, currentPkg, nil
	}
	if !strings.EqualFold(current.BuildType, android.BuildTypeDebug) {
		return current, currentPkg, nil
	}

	repoInfo, gitErr := gitflow.Inspect(ctx, projectDir)
	if gitErr != nil || !repoInfo.HasTags {
		//nolint:nilerr // Non-Git folders simply skip the tagged-repo release variant prompt.
		return current, currentPkg, nil
	}

	targets := listReleaseTargetVariants(ctx, mod, current)
	if len(targets) == 0 {
		return current, currentPkg, nil
	}

	tagNote := "This Git repository already has tags."
	if repoInfo.LatestTag != "" {
		tagNote = fmt.Sprintf("This Git repository already has tags (latest: %s).", repoInfo.LatestTag)
	}

	if len(targets) == 1 {
		releaseVariant := targets[0]
		desc := fmt.Sprintf(
			"%s Switch default variant from %s to %s for CafeBazaar releases?",
			tagNote,
			current.Name,
			releaseVariant.Name,
		)
		useRelease, err := printer.Confirm(
			ctx,
			fmt.Sprintf("Use %q variant instead of %q?", releaseVariant.Name, current.Name),
			desc,
			true,
		)
		if err != nil {
			return current, currentPkg, err
		}
		if !useRelease {
			return current, currentPkg, nil
		}
		return applyChosenVariant(ctx, current, releaseVariant, currentPkg), resolveNextPkg(ctx, current, releaseVariant, currentPkg), nil
	}

	opts := make([]output.SelectOption, 0, len(targets)+1)
	byName := make(map[string]android.Variant, len(targets))
	for _, v := range targets {
		byName[v.Name] = v
		opts = append(opts, output.SelectOption{
			Label: formatInitVariantLabel(ctx, v),
			Value: v.Name,
		})
	}
	opts = append(opts, output.SelectOption{
		Label: fmt.Sprintf("Keep %s (debug)", current.Name),
		Value: current.Name,
	})

	chosen, err := printer.Select(
		ctx,
		fmt.Sprintf("Select target release variant instead of %q", current.Name),
		fmt.Sprintf("%s Choose which build variant to use for CafeBazaar releases.", tagNote),
		opts,
	)
	if err != nil {
		return current, currentPkg, err
	}
	if chosen == current.Name {
		return current, currentPkg, nil
	}
	target, ok := byName[chosen]
	if !ok {
		return current, currentPkg, nil
	}
	return applyChosenVariant(ctx, current, target, currentPkg), resolveNextPkg(ctx, current, target, currentPkg), nil
}

func applyChosenVariant(ctx context.Context, current, target android.Variant, currentPkg string) android.Variant {
	_ = ctx
	_ = current
	_ = currentPkg
	return target
}

func resolveNextPkg(ctx context.Context, current, target android.Variant, currentPkg string) string {
	_ = ctx
	if target.PackageName != "" && (strings.TrimSpace(currentPkg) == "" || currentPkg == current.PackageName) {
		return target.PackageName
	}
	return currentPkg
}

func findVariantsByPackage(ctx context.Context, mod android.AppModule, pkg string) []android.Variant {
	_ = ctx
	want := strings.TrimSpace(pkg)
	if want == "" {
		return nil
	}
	var out []android.Variant
	for _, v := range mod.Variants {
		if v.PackageName == want {
			out = append(out, v)
		}
	}
	return out
}

func orderVariantsForInit(ctx context.Context, variants []android.Variant) []android.Variant {
	_ = ctx
	var releaseTier []android.Variant
	var customTier []android.Variant
	var debugTier []android.Variant
	for _, v := range variants {
		switch {
		case strings.EqualFold(v.BuildType, android.BuildTypeRelease):
			releaseTier = append(releaseTier, v)
		case strings.EqualFold(v.BuildType, android.BuildTypeDebug):
			debugTier = append(debugTier, v)
		default:
			customTier = append(customTier, v)
		}
	}
	ordered := make([]android.Variant, 0, len(variants))
	ordered = append(ordered, releaseTier...)
	ordered = append(ordered, customTier...)
	ordered = append(ordered, debugTier...)
	return ordered
}

func listReleaseTargetVariants(ctx context.Context, mod android.AppModule, current android.Variant) []android.Variant {
	_ = ctx
	var exactRelease []android.Variant
	var otherRelease []android.Variant
	var otherNonDebug []android.Variant

	for _, v := range mod.Variants {
		if strings.EqualFold(v.BuildType, android.BuildTypeDebug) {
			continue
		}
		if strings.EqualFold(v.BuildType, android.BuildTypeRelease) {
			if slices.Equal(v.Flavors, current.Flavors) {
				exactRelease = append(exactRelease, v)
			} else {
				otherRelease = append(otherRelease, v)
			}
			continue
		}
		otherNonDebug = append(otherNonDebug, v)
	}

	out := make([]android.Variant, 0, len(exactRelease)+len(otherRelease)+len(otherNonDebug))
	out = append(out, exactRelease...)
	out = append(out, otherRelease...)
	out = append(out, otherNonDebug...)
	return out
}

func formatInitVariantLabel(ctx context.Context, v android.Variant) string {
	_ = ctx
	if v.PackageName != "" {
		return fmt.Sprintf("%s  ->  %s (buildType: %s)", v.Name, v.PackageName, v.BuildType)
	}
	return fmt.Sprintf("%s (buildType: %s)", v.Name, v.BuildType)
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
