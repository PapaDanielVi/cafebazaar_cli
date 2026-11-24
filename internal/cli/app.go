package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newAppCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "List, switch, and submit CafeBazaar apps",
	}
	appCmd.AddCommand(
		newAppListCmd(ctx, rt),
		newAppUseCmd(ctx, rt),
		newAppCreateCmd(ctx, rt),
	)
	return appCmd
}

func newAppListCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   cmdUseList,
		Short: "List all registered apps in bazaar",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAppList(c.Context(), rt)
		},
	}
}

func runAppList(ctx context.Context, rt *AppRuntime) error {
	apps := rt.Global.SortedApps(ctx)
	if printed, err := rt.Printer.PrintStructured(ctx, apps); printed || err != nil {
		return err
	}
	if len(apps) == 0 {
		return rt.Printer.PrintInfo(ctx, "No apps registered yet. Run `bazaar init` inside an Android project or `bazaar auth app add`.")
	}

	rows := make([][]string, 0, len(apps))
	for _, a := range apps {
		marker := ""
		if a.PackageName == rt.Global.DefaultApp {
			marker = "*"
		}
		rows = append(rows, []string{marker, a.PackageName, fallbackDash(ctx, a.Alias), a.TokenSource, a.Backend, strings.Join(a.Paths, ", ")})
	}
	return rt.Printer.PrintTable(ctx, []string{"Def", labelPackageName, "Alias", "Origin", labelStorage, "Bound Folders"}, rows)
}

func newAppUseCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var localOnly bool
	cmd := &cobra.Command{
		Use:               "use <package-name>",
		Short:             "Set the default app globally or bind it to the current project folder",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeAppPackages(rt),
		RunE: func(c *cobra.Command, args []string) error {
			entry, ok := rt.Global.FindApp(c.Context(), args[0])
			if !ok {
				return fmt.Errorf("app %q is not registered; run `bazaar init` or `bazaar auth app add %s` first", args[0], args[0])
			}
			if localOnly {
				dir := projectRootOrCwd(c.Context(), rt)
				if err := rt.Global.BindPath(c.Context(), entry.PackageName, dir); err != nil {
					return err
				}
				if err := config.SaveGlobal(c.Context(), rt.ConfigPath, rt.Global); err != nil {
					return err
				}
				return rt.Printer.PrintSuccess(c.Context(), fmt.Sprintf("Bound folder %s to %s.", dir, entry.PackageName))
			}
			rt.Global.DefaultApp = entry.PackageName
			if err := config.SaveGlobal(c.Context(), rt.ConfigPath, rt.Global); err != nil {
				return err
			}
			return rt.Printer.PrintSuccess(c.Context(), fmt.Sprintf("Set default app to %s.", entry.PackageName))
		},
	}
	cmd.Flags().BoolVar(&localOnly, "local", false, "Bind this app to the current directory instead of changing the global default")
	return cmd
}

func newAppCreateCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var pkgFlag string
	var issueAgent bool
	var authHeader string
	var baseURL string
	var newApp newAppFlags

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new app via the Nasher Publisher API (POST /v1/apps/) and auto-save its app token",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			newApp.AutoPublishSet = c.Flags().Changed(flagAutoPublish)
			return runAppCreate(c.Context(), rt, pkgFlag, issueAgent, authHeader, baseURL, newApp)
		},
	}

	cmd.Flags().StringVar(&pkgFlag, "package", "", "Android package name (auto-detected from Gradle if omitted)")
	cmd.Flags().BoolVar(&issueAgent, "issue-api-agent", true, "Request CafeBazaar to issue and return a per-app Pishkhan token")
	cmd.Flags().StringVar(&authHeader, "auth-header", "", "Override Publisher API auth header name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override Publisher API base URL")
	registerNewAppFlags(ctx, cmd, rt, &newApp)

	registerFlagCompletion(ctx, cmd, "package", completeAppPackages(rt))
	return cmd
}

func runAppCreate(
	ctx context.Context,
	rt *AppRuntime,
	pkgFlag string,
	issueAgent bool,
	authHeader string,
	baseURL string,
	newApp newAppFlags,
) error {
	pubToken, _, err := rt.resolvePublisherToken(ctx)
	if err != nil {
		return err
	}

	var mod android.AppModule
	if rt.Android != nil {
		if m, ok := rt.Android.PrimaryModule(ctx, ""); ok {
			mod = m
		}
	}

	pkg := strings.TrimSpace(pkgFlag)
	if pkg == "" {
		chosenPkg, _, pErr := promptPackageFromAndroid(ctx, rt)
		if pErr != nil {
			return pErr
		}
		pkg = chosenPkg
	}

	req, err := collectCreateAppRequest(ctx, rt, pkg, mod, issueAgent, newApp)
	if err != nil {
		return err
	}
	if err := confirmCreateAppRequest(ctx, rt, req); err != nil {
		return err
	}

	client, err := rt.newPublisherClient(ctx, pubToken, authHeader, baseURL)
	if err != nil {
		return err
	}
	onProg, finish := rt.Printer.UploadProgress(ctx, "Submitting new app")
	resp, err := client.CreateApp(ctx, req, onProg)
	finish(ctx)
	if err != nil {
		return err
	}

	for _, w := range resp.Warnings {
		if wErr := rt.Printer.PrintWarning(ctx, w); wErr != nil {
			return wErr
		}
	}

	savedBackend := ""
	if resp.AppToken != "" {
		targetPkg := resp.PackageName
		if targetPkg == "" {
			targetPkg = pkg
		}
		savedBackend, err = saveAppTokenAndMetadata(ctx, rt, saveAppInput{
			PackageName: targetPkg,
			Token:       resp.AppToken,
			TokenSource: config.TokenSourcePublisherAPI,
			BindDir:     projectRootOrCwd(ctx, rt),
			SetDefault:  rt.Global.DefaultApp == "",
		})
		if err != nil {
			return err
		}
	}

	if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
		return pErr
	}

	items := []output.KeyValue{
		{Key: labelPackageName, Value: fallbackDash(ctx, resp.PackageName)},
		{Key: "Message", Value: fallbackDash(ctx, resp.Message)},
	}
	if resp.VersionName != "" || resp.VersionCode > 0 {
		items = append(items, output.KeyValue{
			Key:   labelVersion,
			Value: fmt.Sprintf("%s (code %d)", fallbackDash(ctx, resp.VersionName), resp.VersionCode),
		})
	}
	if resp.ReviewState != "" {
		items = append(items, output.KeyValue{Key: "Review State", Value: resp.ReviewState})
	}
	if resp.AppURL != "" {
		items = append(items, output.KeyValue{Key: "App URL", Value: resp.AppURL})
	}
	if resp.AppToken != "" {
		items = append(items, output.KeyValue{
			Key:   "App Token Saved",
			Value: fmt.Sprintf("%s (%s)", auth.MaskToken(ctx, resp.AppToken), savedBackend),
		})
	} else {
		items = append(items, output.KeyValue{
			Key:   "App Token",
			Value: "Not returned; generate manually in Pishkhan and run `bazaar init`",
		})
	}
	return rt.Printer.PrintCard(ctx, "New App Submitted", items)
}
