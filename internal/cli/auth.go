package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	pishkhanDashboardURL = "https://pishkhan.cafebazaar.ir"
	nasherSettingsURL    = "https://pishkhan.cafebazaar.ir/settings/publisher-api"
)

func newAuthCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage CafeBazaar Nasher developer and per-app Pishkhan tokens",
	}

	authCmd.AddCommand(
		newAuthLoginShortcutCmd(ctx, rt),
		newAuthStatusCmd(ctx, rt),
		newAuthPublisherCmd(ctx, rt),
		newAuthAppCmd(ctx, rt),
	)
	return authCmd
}

func newAuthLoginShortcutCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	cmd := newPublisherLoginCmd(ctx, rt)
	cmd.Use = "login"
	cmd.Short = "Log in with your CafeBazaar Nasher (Publisher) developer account token"
	return cmd
}

func newAuthPublisherCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	pubCmd := &cobra.Command{
		Use:   "publisher",
		Short: "Manage the developer-account Nasher (Publisher) token",
	}
	pubCmd.AddCommand(
		newPublisherLoginCmd(ctx, rt),
		newPublisherStatusCmd(ctx, rt),
		newPublisherLogoutCmd(ctx, rt),
	)
	return pubCmd
}

func newPublisherLoginCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var tokenFlag string
	var tokenStdin bool
	var account string
	var authHeader string
	var baseURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save your CafeBazaar Nasher developer token",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runPublisherLogin(c.Context(), rt, tokenFlag, tokenStdin, account, authHeader, baseURL)
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "Nasher developer token (prefer interactive prompt or --token-stdin)")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read Nasher developer token from stdin")
	cmd.Flags().StringVar(&account, "account", auth.DefaultAccount, "Developer account label")
	cmd.Flags().StringVar(&authHeader, "auth-header", "", "Override Publisher API authentication header name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override Publisher API base URL")
	return cmd
}

func runPublisherLogin(ctx context.Context, rt *AppRuntime, tokenFlag string, tokenStdin bool, account, authHeader, baseURL string) error {
	key := auth.PublisherKey(ctx, account)
	if _, _, existingErr := rt.Store.Get(ctx, key); existingErr == nil {
		if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
			Title:       "Replace Existing Nasher Developer Token",
			Warning:     "A developer publisher token is already saved in bazaar's credential store.",
			Details:     []output.KeyValue{{Key: "Account", Value: account}},
			PromptLabel: "Overwrite the existing Nasher developer token?",
		}); err != nil {
			return err
		}
	}

	token, err := readTokenInput(ctx, rt, tokenFlag, tokenStdin,
		"CafeBazaar Nasher (Publisher) Developer Token",
		fmt.Sprintf("Generate one at %s and paste it below.", nasherSettingsURL),
	)
	if err != nil {
		return err
	}

	backend, err := rt.Store.Set(ctx, key, token)
	if err != nil {
		return err
	}

	rt.Global.Publisher = &config.PublisherConfig{
		Account:    account,
		AuthHeader: strings.TrimSpace(authHeader),
		BaseURL:    strings.TrimSpace(baseURL),
		Backend:    backend,
		AddedAt:    time.Now().UTC(),
	}
	if err := config.SaveGlobal(ctx, rt.ConfigPath, rt.Global); err != nil {
		return err
	}

	payload := map[string]any{
		"account": account,
		"backend": backend,
		"token":   auth.MaskToken(ctx, token),
	}
	if printed, err := rt.Printer.PrintStructured(ctx, payload); printed || err != nil {
		return err
	}
	if err := rt.Printer.PrintSuccess(ctx, fmt.Sprintf("Saved Nasher developer token (%s) in %s.", auth.MaskToken(ctx, token), backend)); err != nil {
		return err
	}
	return rt.Printer.PrintInfo(ctx, "Next: cd into an Android app folder and run `bazaar init` to link or submit your app.")
}

func newPublisherStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "status",
		Short: "Check developer Nasher token status",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runPublisherStatus(c.Context(), rt)
		},
	}
}

func runPublisherStatus(ctx context.Context, rt *AppRuntime) error {
	tok, backend, err := rt.resolvePublisherToken(ctx)
	status := map[string]any{
		"configured": err == nil,
		"backend":    backend,
		"token":      auth.MaskToken(ctx, tok),
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, status); printed || pErr != nil {
		return pErr
	}
	if err != nil {
		return rt.Printer.PrintCard(ctx, "Nasher Developer Token", []output.KeyValue{
			{Key: "Status", Value: rt.Printer.Theme.RenderBadge(ctx, "MISSING")},
			{Key: "Next Step", Value: "Run `bazaar auth login` or set CAFEBAZAAR_PUBLISHER_TOKEN"},
		})
	}
	return rt.Printer.PrintCard(ctx, "Nasher Developer Token", []output.KeyValue{
		{Key: "Status", Value: rt.Printer.Theme.RenderBadge(ctx, "READY")},
		{Key: "Token", Value: auth.MaskToken(ctx, tok)},
		{Key: "Storage", Value: backend},
	})
}

func newPublisherLogoutCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved developer Nasher token",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runPublisherLogout(c.Context(), rt)
		},
	}
}

func runPublisherLogout(ctx context.Context, rt *AppRuntime) error {
	account := auth.DefaultAccount
	if rt.Global.Publisher != nil && rt.Global.Publisher.Account != "" {
		account = rt.Global.Publisher.Account
	}
	if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:       "Remove Nasher Developer Token",
		Warning:     "You will no longer be able to submit new apps until you log in again.",
		Details:     []output.KeyValue{{Key: "Account", Value: account}},
		PromptLabel: "Remove the stored Nasher developer token?",
	}); err != nil {
		return err
	}

	if err := rt.Store.Delete(ctx, auth.PublisherKey(ctx, account)); err != nil && !errors.Is(err, auth.ErrSecretNotFound) {
		return err
	}
	rt.Global.Publisher = nil
	if err := config.SaveGlobal(ctx, rt.ConfigPath, rt.Global); err != nil {
		return err
	}
	return rt.Printer.PrintSuccess(ctx, "Removed Nasher developer token.")
}

func newAuthAppCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "Manage per-app CafeBazaar Pishkhan secret tokens",
	}
	appCmd.AddCommand(
		newAuthAppAddCmd(ctx, rt),
		newAuthAppListCmd(ctx, rt),
		newAuthAppStatusCmd(ctx, rt),
		newAuthAppRemoveCmd(ctx, rt),
	)
	return appCmd
}

func newAuthAppAddCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var tokenFlag string
	var tokenStdin bool
	var alias string
	var displayName string
	var skipVerify bool
	var setDefault bool

	cmd := &cobra.Command{
		Use:               "add [package-name]",
		Short:             "Save a per-app Pishkhan API token (auto-suggests Gradle build-type packages)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAppPackages(rt),
		RunE: func(c *cobra.Command, args []string) error {
			var pkgArg string
			if len(args) > 0 {
				pkgArg = args[0]
			}
			return runAuthAppAdd(c.Context(), rt, pkgArg, tokenFlag, tokenStdin, alias, displayName, skipVerify, setDefault)
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "App Pishkhan token (prefer interactive prompt or --token-stdin)")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read app token from stdin")
	cmd.Flags().StringVar(&alias, "alias", "", "Short alias for this app")
	cmd.Flags().StringVar(&displayName, "name", "", "Human-readable app display name")
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skip live Pishkhan token verification")
	cmd.Flags().BoolVar(&setDefault, "default", false, "Set this app as the global default")
	return cmd
}

func runAuthAppAdd(
	ctx context.Context,
	rt *AppRuntime,
	pkgArg string,
	tokenFlag string,
	tokenStdin bool,
	alias string,
	displayName string,
	skipVerify bool,
	setDefault bool,
) error {
	pkg := strings.TrimSpace(pkgArg)
	var defaultVariant string
	if pkg == "" {
		chosenPkg, chosenVariant, err := promptPackageFromAndroid(ctx, rt)
		if err != nil {
			return err
		}
		pkg = chosenPkg
		defaultVariant = chosenVariant
	}

	if _, exists := rt.Global.Apps[pkg]; exists {
		if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
			Title:       "Overwrite Existing App Token",
			TargetApp:   pkg,
			Warning:     "A Pishkhan API token is already saved for this package name.",
			PromptLabel: "Overwrite the existing app token?",
		}); err != nil {
			return err
		}
	}

	token, err := readTokenInput(ctx, rt, tokenFlag, tokenStdin,
		fmt.Sprintf("Pishkhan API Token for %s", pkg),
		fmt.Sprintf("Open %s -> Select app -> API Pishkhan -> Generate New Token, then paste it here.", pishkhanDashboardURL),
	)
	if err != nil {
		return err
	}

	if !skipVerify {
		if err := verifyAppToken(ctx, rt, token); err != nil {
			return err
		}
	}

	backend, err := saveAppTokenAndMetadata(ctx, rt, saveAppInput{
		PackageName:    pkg,
		Token:          token,
		Alias:          alias,
		DisplayName:    displayName,
		TokenSource:    config.TokenSourceManual,
		DefaultVariant: defaultVariant,
		BindDir:        projectRootOrCwd(ctx, rt),
		SetDefault:     setDefault || rt.Global.DefaultApp == "",
	})
	if err != nil {
		return err
	}

	payload := map[string]any{
		"package_name": pkg,
		"alias":        alias,
		"backend":      backend,
		"token":        auth.MaskToken(ctx, token),
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, payload); printed || pErr != nil {
		return pErr
	}
	return rt.Printer.PrintSuccess(ctx, fmt.Sprintf("Saved Pishkhan token for %s (%s) in %s.", pkg, auth.MaskToken(ctx, token), backend))
}

func newAuthAppListCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "list",
		Short: "List all apps with saved Pishkhan tokens",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAppList(c.Context(), rt)
		},
	}
}

func newAuthAppStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "status",
		Short: "Verify the active app's Pishkhan token against CafeBazaar",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAuthAppStatus(c.Context(), rt)
		},
	}
}

func runAuthAppStatus(ctx context.Context, rt *AppRuntime) error {
	resolved, err := rt.resolveActiveApp(ctx, "")
	if err != nil {
		return err
	}
	client, err := rt.newPishkhanClient(ctx, resolved.Token)
	if err != nil {
		return err
	}

	var pubRespExists bool
	if err := rt.Printer.WithSpinner(ctx, "Verifying Pishkhan token with CafeBazaar", func(spinCtx context.Context) error {
		resp, callErr := client.LastPublishedRelease(spinCtx)
		if callErr != nil {
			return callErr
		}
		pubRespExists = resp.Exists
		return nil
	}); err != nil {
		return err
	}

	data := map[string]any{
		"package_name":  resolved.PackageName,
		"resolved_by":   resolved.ResolvedBy,
		"token_source":  resolved.TokenSource,
		"token":         auth.MaskToken(ctx, resolved.Token),
		"valid":         true,
		"has_published": pubRespExists,
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, data); printed || pErr != nil {
		return pErr
	}
	return rt.Printer.PrintCard(ctx, "App Token Status", []output.KeyValue{
		{Key: "Status", Value: rt.Printer.Theme.RenderBadge(ctx, "CONNECTED")},
		{Key: "Package", Value: resolved.PackageName},
		{Key: "Resolved By", Value: resolved.ResolvedBy},
		{Key: "Token", Value: auth.MaskToken(ctx, resolved.Token)},
		{Key: "Storage", Value: resolved.TokenSource},
	})
}

func newAuthAppRemoveCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:               "remove <package-name>",
		Short:             "Delete a saved app token from bazaar",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeAppPackages(rt),
		RunE: func(c *cobra.Command, args []string) error {
			return runAuthAppRemove(c.Context(), rt, args[0])
		},
	}
}

func runAuthAppRemove(ctx context.Context, rt *AppRuntime, pkgOrAlias string) error {
	pkg := pkgOrAlias
	if entry, ok := rt.Global.FindApp(ctx, pkgOrAlias); ok {
		pkg = entry.PackageName
	}

	if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:       "Remove App Token",
		TargetApp:   pkg,
		Warning:     "Removes the stored Pishkhan API token and folder bindings for this app.",
		PromptLabel: fmt.Sprintf("Remove saved credentials for %s?", pkg),
	}); err != nil {
		return err
	}

	if err := rt.Store.Delete(ctx, auth.AppKey(ctx, pkg)); err != nil && !errors.Is(err, auth.ErrSecretNotFound) {
		return err
	}
	delete(rt.Global.Apps, pkg)
	if rt.Global.DefaultApp == pkg {
		rt.Global.DefaultApp = ""
	}
	if err := config.SaveGlobal(ctx, rt.ConfigPath, rt.Global); err != nil {
		return err
	}
	return rt.Printer.PrintSuccess(ctx, fmt.Sprintf("Removed saved token for %s.", pkg))
}

func newAuthStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "status",
		Short: "Show Nasher developer token, registered apps, and current folder resolution",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAuthOverview(c.Context(), rt)
		},
	}
}

func runAuthOverview(ctx context.Context, rt *AppRuntime) error {
	pubTok, pubBackend, pubErr := rt.resolvePublisherToken(ctx)
	activeApp, activeErr := rt.resolveActiveApp(ctx, "")
	apps := rt.Global.SortedApps(ctx)

	overview := map[string]any{
		"publisher": map[string]any{
			"configured": pubErr == nil,
			"backend":    pubBackend,
			"token":      auth.MaskToken(ctx, pubTok),
		},
		"active_folder_app": map[string]any{
			"resolved":     activeErr == nil,
			"package_name": activeApp.PackageName,
			"resolved_by":  activeApp.ResolvedBy,
			"storage":      activeApp.TokenSource,
		},
		"registered_apps": apps,
	}
	if printed, err := rt.Printer.PrintStructured(ctx, overview); printed || err != nil {
		return err
	}

	pubStatus := rt.Printer.Theme.RenderBadge(ctx, "MISSING")
	if pubErr == nil {
		pubStatus = fmt.Sprintf("%s %s (%s)", rt.Printer.Theme.RenderBadge(ctx, "READY"), auth.MaskToken(ctx, pubTok), pubBackend)
	}
	activeStatus := rt.Printer.Theme.RenderBadge(ctx, "UNINITIALIZED")
	if activeErr == nil {
		activeStatus = fmt.Sprintf("%s %s (via %s)", rt.Printer.Theme.RenderBadge(ctx, "ACTIVE"), activeApp.PackageName, activeApp.ResolvedBy)
	}

	if err := rt.Printer.PrintCard(ctx, "CafeBazaar Auth Overview", []output.KeyValue{
		{Key: "Nasher Developer Token", Value: pubStatus},
		{Key: "Current Folder App", Value: activeStatus},
		{Key: "Registered Apps", Value: fmt.Sprintf("%d", len(apps))},
		{Key: "Config File", Value: rt.ConfigPath},
	}); err != nil {
		return err
	}

	if len(apps) > 0 {
		rows := make([][]string, 0, len(apps))
		for _, a := range apps {
			marker := ""
			if a.PackageName == rt.Global.DefaultApp {
				marker = "*"
			}
			rows = append(rows, []string{marker, a.PackageName, a.Alias, a.TokenSource, a.Backend, strings.Join(a.Paths, ", ")})
		}
		return rt.Printer.PrintTable(ctx, []string{"Def", "Package Name", "Alias", "Origin", "Storage", "Bound Folders"}, rows)
	}
	return nil
}

type saveAppInput struct {
	PackageName    string
	Token          string
	Alias          string
	DisplayName    string
	TokenSource    string
	DefaultVariant string
	BindDir        string
	SetDefault     bool
}

func saveAppTokenAndMetadata(ctx context.Context, rt *AppRuntime, in saveAppInput) (string, error) {
	backend, err := rt.Store.Set(ctx, auth.AppKey(ctx, in.PackageName), in.Token)
	if err != nil {
		return "", err
	}

	entry := rt.Global.Apps[in.PackageName]
	entry.PackageName = in.PackageName
	if in.Alias != "" {
		entry.Alias = in.Alias
	}
	if in.DisplayName != "" {
		entry.DisplayName = in.DisplayName
	}
	entry.TokenSource = in.TokenSource
	entry.Backend = backend
	if in.DefaultVariant != "" {
		entry.DefaultVariant = in.DefaultVariant
	}
	entry.AddedAt = time.Now().UTC()
	rt.Global.Apps[in.PackageName] = entry

	if strings.TrimSpace(in.BindDir) != "" {
		if err := rt.Global.BindPath(ctx, in.PackageName, in.BindDir); err != nil {
			return backend, err
		}
	}
	if in.SetDefault {
		rt.Global.DefaultApp = in.PackageName
	}

	if err := config.SaveGlobal(ctx, rt.ConfigPath, rt.Global); err != nil {
		return backend, err
	}
	return backend, nil
}

func verifyAppToken(ctx context.Context, rt *AppRuntime, token string) error {
	client, err := rt.newPishkhanClient(ctx, token)
	if err != nil {
		return err
	}
	return rt.Printer.WithSpinner(ctx, "Validating app token with CafeBazaar Pishkhan", func(spinCtx context.Context) error {
		_, callErr := client.LastPublishedRelease(spinCtx)
		if callErr != nil {
			return fmt.Errorf("token validation failed: %w", callErr)
		}
		return nil
	})
}

func readTokenInput(ctx context.Context, rt *AppRuntime, flagVal string, fromStdin bool, title, description string) (string, error) {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal), nil
	}
	if fromStdin {
		raw, err := io.ReadAll(rt.Deps.In)
		if err != nil {
			return "", fmt.Errorf("read token from stdin: %w", err)
		}
		tok := strings.TrimSpace(string(raw))
		if tok == "" {
			return "", auth.ErrEmptySecret
		}
		return tok, nil
	}
	tok, err := rt.Printer.Input(ctx, title, description, "Paste secret token...", "", true)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(tok) == "" {
		return "", auth.ErrEmptySecret
	}
	return strings.TrimSpace(tok), nil
}

func promptPackageFromAndroid(ctx context.Context, rt *AppRuntime) (string, string, error) {
	if rt.Android != nil {
		variants := rt.Android.AllVariants(ctx)
		if len(variants) > 0 && rt.Printer.Mode.Interactive {
			opts := make([]output.SelectOption, 0, len(variants)+1)
			seen := map[string]struct{}{}
			variantByPkg := map[string]string{}
			for _, v := range variants {
				if v.PackageName == "" {
					continue
				}
				if _, ok := seen[v.PackageName]; ok {
					continue
				}
				seen[v.PackageName] = struct{}{}
				variantByPkg[v.PackageName] = v.Name
				opts = append(opts, output.SelectOption{
					Label: fmt.Sprintf("%s  (Gradle variant: %s)", v.PackageName, v.Name),
					Value: v.PackageName,
				})
			}
			opts = append(opts, output.SelectOption{
				Label: "Enter custom package name...",
				Value: "__custom__",
			})

			chosen, err := rt.Printer.Select(ctx, "Select Android Package Name", "Detected from Gradle buildTypes and productFlavors.", opts)
			if err != nil {
				return "", "", err
			}
			if chosen != "__custom__" {
				return chosen, variantByPkg[chosen], nil
			}
		}
	}

	pkg, err := rt.Printer.Input(ctx, "Android Package Name", "For example: com.example.myapp", "com.example.myapp", "", false)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(pkg) == "" {
		return "", "", errors.New("package name must not be empty")
	}
	return strings.TrimSpace(pkg), "", nil
}

func projectRootOrCwd(ctx context.Context, rt *AppRuntime) string {
	_ = ctx
	if rt.Android != nil && rt.Android.RootDir != "" {
		return rt.Android.RootDir
	}
	return rt.Cwd
}
