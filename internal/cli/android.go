package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newAndroidCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	androidCmd := &cobra.Command{
		Use:   "android",
		Short: "Inspect Android Gradle modules, build variants, artifacts, and bump versions",
	}

	androidCmd.AddCommand(
		newAndroidInfoCmd(ctx, rt),
		newAndroidVariantsCmd(ctx, rt),
		newAndroidBumpCmd(ctx, rt),
	)
	return androidCmd
}

func newAndroidInfoCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "info",
		Short: "Show auto-detected Android Gradle project metadata, variants, and build artifacts",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAndroidInfo(c.Context(), rt)
		},
	}
}

func runAndroidInfo(ctx context.Context, rt *AppRuntime) error {
	if rt.Android == nil {
		return android.ErrNotAndroidProject
	}
	var gitInfo *gitflow.RepoInfo
	if info, err := gitflow.Inspect(ctx, rt.Android.RootDir); err == nil {
		gitInfo = &info
	}

	payload := map[string]any{
		"project": rt.Android,
		"git":     gitInfo,
	}
	if printed, err := rt.Printer.PrintStructured(ctx, payload); printed || err != nil {
		return err
	}

	mod, _ := rt.Android.PrimaryModule(ctx, "")
	items := []output.KeyValue{
		{Key: "Project Root", Value: shortenDisplayPath(ctx, rt.Android.RootDir)},
		{Key: "Primary Module", Value: mod.Name},
		{Key: "Build File", Value: relativeDisplayPath(ctx, rt.Android.RootDir, mod.BuildFile)},
		{Key: "Base App ID", Value: mod.ApplicationID},
		{Key: "Version", Value: fmt.Sprintf("%s (code %d)", mod.VersionName, mod.VersionCode)},
		{Key: "Gradle Wrapper", Value: fmt.Sprintf("%v", rt.Android.HasWrapper)},
		{Key: "Detected Variants", Value: fmt.Sprintf("%d", len(mod.Variants))},
		{Key: "Detected Artifacts", Value: fmt.Sprintf("%d", len(mod.Artifacts))},
	}
	if gitInfo != nil {
		items = append(items, output.KeyValue{Key: "Git Branch / Tag", Value: fmt.Sprintf("%s / %s", gitInfo.Branch, fallbackDash(ctx, gitInfo.LatestTag))})
	}
	if err := rt.Printer.PrintCard(ctx, "Android Project Overview", items); err != nil {
		return err
	}

	if err := printVariantsTable(ctx, rt, rt.Android.AllVariants(ctx)); err != nil {
		return err
	}
	if len(mod.Artifacts) > 0 {
		rows := make([][]string, 0, len(mod.Artifacts))
		for _, a := range mod.Artifacts {
			rows = append(rows, []string{strings.ToUpper(a.Type), fallbackDash(ctx, a.VariantName), output.HumanBytes(ctx, a.SizeBytes), a.RelPath})
		}
		return rt.Printer.PrintTable(ctx, []string{"Type", "Variant", "Size", "Path"}, rows)
	}
	return nil
}

func newAndroidVariantsCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "variants",
		Short: "List all Gradle build types, flavors, and resolved package names",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			if rt.Android == nil {
				return android.ErrNotAndroidProject
			}
			variants := rt.Android.AllVariants(c.Context())
			if printed, err := rt.Printer.PrintStructured(c.Context(), variants); printed || err != nil {
				return err
			}
			return printVariantsTable(c.Context(), rt, variants)
		},
	}
}

func printVariantsTable(ctx context.Context, rt *AppRuntime, variants []android.Variant) error {
	rows := make([][]string, 0, len(variants))
	for _, v := range variants {
		regBadge := rt.Printer.Theme.RenderBadge(ctx, "UNREGISTERED")
		if isRegisteredApp(ctx, rt.Global, v.PackageName) {
			regBadge = rt.Printer.Theme.RenderBadge(ctx, "READY")
		}
		rows = append(rows, []string{v.ModuleName, v.Name, v.BuildType, v.PackageName, v.VersionName, regBadge, v.AssembleTask})
	}
	return rt.Printer.PrintTable(ctx, []string{"Module", "Variant", "BuildType", "Package Name", "Version", "Auth", "Gradle Task"}, rows)
}

func newAndroidBumpCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var bumpKind string
	var versionCode int
	var versionName string
	var moduleName string
	var syncRemote bool
	var dryRun bool
	var gitCommit bool
	var gitTag bool

	cmd := &cobra.Command{
		Use:   "bump",
		Short: "Increment Gradle versionCode and versionName cleanly",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runAndroidBump(c.Context(), rt, bumpKind, versionCode, versionName, moduleName, syncRemote, dryRun, gitCommit, gitTag)
		},
	}

	cmd.Flags().StringVar(&bumpKind, "bump", string(android.BumpCode), "Version bump kind: code, patch, minor, major")
	cmd.Flags().IntVar(&versionCode, "version-code", 0, "Explicit target versionCode")
	cmd.Flags().StringVar(&versionName, "version-name", "", "Explicit target versionName")
	cmd.Flags().StringVar(&moduleName, "module", "", "Target Android module (defaults to :app)")
	cmd.Flags().BoolVar(&syncRemote, "sync-remote", false, "Ensure versionCode is higher than CafeBazaar's live published/committed versionCode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the version change without editing Gradle files")
	cmd.Flags().BoolVar(&gitCommit, "git-commit", false, "Commit the updated Gradle build file to Git")
	cmd.Flags().BoolVar(&gitTag, "git-tag", false, "Create an annotated Git tag for the new version")

	_ = cmd.RegisterFlagCompletionFunc("bump", completeBumpFlag(ctx))
	return cmd
}

func runAndroidBump(
	ctx context.Context,
	rt *AppRuntime,
	bumpKind string,
	versionCode int,
	versionName string,
	moduleName string,
	syncRemote bool,
	dryRun bool,
	gitCommit bool,
	gitTag bool,
) error {
	if rt.Android == nil {
		return android.ErrNotAndroidProject
	}
	mod, ok := rt.Android.PrimaryModule(ctx, moduleName)
	if !ok {
		return android.ErrNotAndroidProject
	}

	minCode := 0
	if syncRemote {
		resolved, err := requireInitializedApp(ctx, rt, "")
		if err != nil {
			return err
		}
		client, err := rt.newPishkhanClient(ctx, resolved.Token)
		if err != nil {
			return err
		}
		if pub, err := client.LastPublishedRelease(ctx); err == nil && pub.Exists && pub.Release != nil {
			for _, pkg := range pub.Release.Packages {
				if pkg.VersionCode > minCode {
					minCode = pkg.VersionCode
				}
			}
		}
		if com, err := client.LastCommittedRelease(ctx); err == nil && com.Exists && com.Release != nil {
			for _, pkg := range com.Release.Packages {
				if pkg.VersionCode > minCode {
					minCode = pkg.VersionCode
				}
			}
		}
	}

	plan, err := android.ComputeBump(ctx, mod, android.BumpOptions{
		Kind:           android.BumpKind(strings.ToLower(strings.TrimSpace(bumpKind))),
		MinVersionCode: minCode,
		VersionCode:    versionCode,
		VersionName:    versionName,
	})
	if err != nil {
		return err
	}

	if err := android.ApplyBump(ctx, plan, dryRun); err != nil {
		return err
	}

	var tagName string
	var commitMsg string
	if !dryRun && (gitCommit || gitTag) {
		repoInfo, gErr := gitflow.Inspect(ctx, rt.Android.RootDir)
		if gErr != nil {
			return gErr
		}
		tpl := config.DefaultCommitTemplate
		prefix := config.DefaultTagPrefix
		if rt.Project != nil {
			if rt.Project.CommitTemplate != "" {
				tpl = rt.Project.CommitTemplate
			}
			if rt.Project.TagPrefix != "" {
				prefix = rt.Project.TagPrefix
			}
		}
		if gitCommit {
			commitMsg = fmt.Sprintf(tpl, plan.NewVersionName, plan.NewVersionCode)
			if err := gitflow.CommitFiles(ctx, repoInfo.RootDir, []string{plan.BuildFile}, commitMsg); err != nil {
				return err
			}
		}
		if gitTag && plan.NewVersionName != "" {
			tagName = prefix + plan.NewVersionName
			if err := gitflow.CreateTag(ctx, repoInfo.RootDir, tagName, fmt.Sprintf("Release %s (%d)", plan.NewVersionName, plan.NewVersionCode)); err != nil {
				return err
			}
		}
	}

	payload := map[string]any{
		"dry_run":    dryRun,
		"plan":       plan,
		"git_commit": commitMsg,
		"git_tag":    tagName,
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, payload); printed || pErr != nil {
		return pErr
	}

	title := "Android Version Bumped"
	if dryRun {
		title = "Android Version Bump (Dry Run)"
	}
	items := []output.KeyValue{
		{Key: "Build File", Value: relativeDisplayPath(ctx, rt.Android.RootDir, plan.BuildFile)},
		{Key: "versionCode", Value: fmt.Sprintf("%d -> %d", plan.OldVersionCode, plan.NewVersionCode)},
		{Key: "versionName", Value: fmt.Sprintf("%s -> %s", plan.OldVersionName, plan.NewVersionName)},
	}
	if commitMsg != "" {
		items = append(items, output.KeyValue{Key: "Git Commit", Value: commitMsg})
	}
	if tagName != "" {
		items = append(items, output.KeyValue{Key: "Git Tag", Value: tagName})
	}
	return rt.Printer.PrintCard(ctx, title, items)
}
