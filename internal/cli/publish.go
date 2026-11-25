package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/publish"
	"github.com/spf13/cobra"
)

func newPublishCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var variantFlag string
	var bumpFlag string
	var versionCode int
	var versionName string
	var buildFlag bool
	var formatKind string
	var apkPath string
	var aabPath string
	var binPath string
	var arch string
	var changelogFA string
	var changelogFAFile string
	var changelogEN string
	var changelogENFile string
	var devNote string
	var rollout int
	var autoPublish bool
	var noCommit bool
	var replaceDraft bool
	var gitCommit bool
	var gitTag bool
	var gitPush bool
	var dryRun bool
	var pollInterval time.Duration

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Run the end-to-end Android release pipeline (detect -> bump -> build -> git -> upload -> commit)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runPublishCmd(c.Context(), rt, publishCmdFlags{
				Variant:         variantFlag,
				Bump:            bumpFlag,
				VersionCode:     versionCode,
				VersionName:     versionName,
				Build:           buildFlag,
				FormatKind:      formatKind,
				APKPath:         apkPath,
				AABPath:         aabPath,
				BinPath:         binPath,
				Arch:            arch,
				ChangelogFA:     changelogFA,
				ChangelogFAFile: changelogFAFile,
				ChangelogEN:     changelogEN,
				ChangelogENFile: changelogENFile,
				DevNote:         devNote,
				Rollout:         rollout,
				RolloutSet:      c.Flags().Changed("rollout"),
				AutoPublish:     autoPublish,
				AutoPublishSet:  c.Flags().Changed("auto-publish"),
				NoCommit:        noCommit,
				ReplaceDraft:    replaceDraft,
				GitCommit:       gitCommit,
				GitTag:          gitTag,
				GitPush:         gitPush,
				DryRun:          dryRun,
				PollInterval:    pollInterval,
			})
		},
	}

	cmd.Flags().StringVar(&variantFlag, "variant", "", "Gradle build variant (auto-detected if omitted)")
	cmd.Flags().StringVar(&bumpFlag, "bump", "", "Bump version before publishing: code, patch, minor, major")
	cmd.Flags().IntVar(&versionCode, "version-code", 0, "Explicit new versionCode")
	cmd.Flags().StringVar(&versionName, "version-name", "", "Explicit new versionName")
	cmd.Flags().BoolVar(&buildFlag, "build", false, "Run ./gradlew assemble/bundle before uploading")
	cmd.Flags().StringVar(&formatKind, "format", "", "Artifact format to build/select: apk or aab")
	cmd.Flags().StringVar(&apkPath, "apk", "", "Explicit APK file path")
	cmd.Flags().StringVar(&aabPath, "aab", "", "Explicit AAB file path")
	cmd.Flags().StringVar(&binPath, "bin", "", "Explicit Bundle Signer .bin file path")
	cmd.Flags().StringVar(&arch, "arch", "", "APK architecture: all, armeabi-v7a, arm64-v8a")
	cmd.Flags().StringVar(&changelogFA, "changelog-fa", "", "Persian (Farsi) changelog")
	cmd.Flags().StringVar(&changelogFAFile, "changelog-fa-file", "", "Read UTF-8 Persian changelog from file")
	cmd.Flags().StringVar(&changelogEN, "changelog-en", "", "English changelog")
	cmd.Flags().StringVar(&changelogENFile, "changelog-en-file", "", "Read UTF-8 English changelog from file")
	cmd.Flags().StringVar(&devNote, "note", "", "Developer note for CafeBazaar reviewers")
	cmd.Flags().IntVar(&rollout, "rollout", 0, "Staged rollout percentage (1 to 100)")
	cmd.Flags().BoolVar(&autoPublish, "auto-publish", false, "Publish automatically after approval")
	cmd.Flags().BoolVar(&noCommit, "no-commit", false, "Upload package but leave release uncommitted")
	cmd.Flags().BoolVar(&replaceDraft, "replace-draft", false, "Delete an existing uncommitted release before creating a new one")
	cmd.Flags().BoolVar(&gitCommit, "git-commit", false, "Commit the Gradle version bump to Git")
	cmd.Flags().BoolVar(&gitTag, "git-tag", false, "Create an annotated Git tag for the release version")
	cmd.Flags().BoolVar(&gitPush, "push", false, "Push Git commit and tag to origin (asks confirmation)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview every release step without modifying Gradle, Git, or CafeBazaar")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", pishkhan.DefaultBundlePollInterval, "AAB bundle status polling interval")

	registerFlagCompletion(ctx, cmd, "variant", completeGradleVariants(rt))
	registerFlagCompletion(ctx, cmd, "bump", completeBumpFlag(ctx))
	registerFlagCompletion(ctx, cmd, "arch", completeArchFlag(ctx))
	registerFlagCompletion(ctx, cmd, "apk", completeArtifactPaths(rt, android.ArtifactTypeAPK, "apk"))
	registerFlagCompletion(ctx, cmd, "aab", completeArtifactPaths(rt, android.ArtifactTypeAAB, "aab"))
	registerFlagCompletion(ctx, cmd, "bin", completeArtifactPaths(rt, android.ArtifactTypeBin, "bin"))

	return cmd
}

type publishCmdFlags struct {
	Variant         string
	Bump            string
	VersionCode     int
	VersionName     string
	Build           bool
	FormatKind      string
	APKPath         string
	AABPath         string
	BinPath         string
	Arch            string
	ChangelogFA     string
	ChangelogFAFile string
	ChangelogEN     string
	ChangelogENFile string
	DevNote         string
	Rollout         int
	RolloutSet      bool
	AutoPublish     bool
	AutoPublishSet  bool
	NoCommit        bool
	ReplaceDraft    bool
	GitCommit       bool
	GitTag          bool
	GitPush         bool
	DryRun          bool
	PollInterval    time.Duration
}

func runPublishCmd(ctx context.Context, rt *AppRuntime, f publishCmdFlags) error {
	var mod android.AppModule
	var chosenVariant android.Variant

	variantName := strings.TrimSpace(f.Variant)
	if variantName == "" {
		variantName = rt.Env.Variant
	}
	if variantName == "" && rt.Project != nil {
		variantName = rt.Project.DefaultVariant
	}

	if rt.Android != nil {
		prefMod := ""
		if rt.Project != nil {
			prefMod = rt.Project.Module
		}
		if m, ok := rt.Android.PrimaryModule(ctx, prefMod); ok {
			mod = m
			if variantName != "" {
				if v, err := mod.FindVariant(ctx, variantName); err == nil {
					chosenVariant = v
				}
			} else if len(mod.Variants) > 0 && rt.Printer.Mode.Interactive && f.APKPath == "" && f.AABPath == "" {
				opts := make([]output.SelectOption, 0, len(mod.Variants))
				for _, v := range mod.Variants {
					opts = append(opts, output.SelectOption{
						Label: fmt.Sprintf("%s  ->  %s (v%s)", v.Name, v.PackageName, v.VersionName),
						Value: v.Name,
					})
				}
				picked, err := rt.Printer.Select(ctx, "Select Gradle Variant to Publish", "Auto-detected from Gradle buildTypes and productFlavors.", opts)
				if err != nil {
					return err
				}
				if v, err := mod.FindVariant(ctx, picked); err == nil {
					chosenVariant = v
				}
			} else if v, err := mod.FindVariant(ctx, android.BuildTypeRelease); err == nil {
				chosenVariant = v
			}
		}
	}

	resolved, err := requireInitializedApp(ctx, rt, chosenVariant.Name)
	if err != nil {
		return err
	}
	client, err := rt.newPishkhanClient(ctx, resolved.Token)
	if err != nil {
		return err
	}

	fa, err := readUTF8TextFlagOrFile(ctx, f.ChangelogFA, f.ChangelogFAFile, rt.Env.ChangelogFA)
	if err != nil {
		return err
	}
	en, err := readUTF8TextFlagOrFile(ctx, f.ChangelogEN, f.ChangelogENFile, rt.Env.ChangelogEN)
	if err != nil {
		return err
	}
	note := strings.TrimSpace(f.DevNote)
	if note == "" {
		note = rt.Env.DevNote
	}

	// In interactive mode, offer Git commit-log suggestions for English changelog and bump/git choices.
	if rt.Printer.Mode.Interactive && en == "" && rt.Android != nil {
		if repoInfo, gErr := gitflow.Inspect(ctx, rt.Android.RootDir); gErr == nil {
			if commits, cErr := gitflow.CommitsSinceTag(ctx, repoInfo.RootDir, repoInfo.LatestTag, 10); cErr == nil && len(commits) > 0 {
				suggested := gitflow.FormatSuggestedChangelog(ctx, commits)
				if edited, pErr := rt.Printer.MultilineInput(ctx, "English Changelog (suggested from Git commits)", "Edit or leave blank to skip.", suggested); pErr == nil {
					en = edited
				}
			}
		}
	}
	if rt.Printer.Mode.Interactive && fa == "" && !f.NoCommit {
		if enteredFA, pErr := rt.Printer.MultilineInput(ctx, "Persian Changelog (changelog_fa)", "Enter Persian release notes for CafeBazaar users (UTF-8).", ""); pErr == nil {
			fa = enteredFA
		}
	}

	arch := f.Arch
	if arch == "" {
		arch = rt.Env.Arch
	}
	rollout := f.Rollout
	rolloutExplicit := f.RolloutSet
	if rollout == 0 && rt.Env.Rollout > 0 {
		rollout = rt.Env.Rollout
		rolloutExplicit = true
	}
	if rollout == 0 && rt.Project != nil && rt.Project.StagedRollout > 0 {
		rollout = rt.Project.StagedRollout
	}
	stagedRolloutEnabled := rt.Project != nil &&
		(rt.Project.StagedRolloutEnabled ||
			(rt.Project.StagedRollout >= pishkhan.MinRolloutPercentage && rt.Project.StagedRollout < pishkhan.MaxRolloutPercentage))
	autoPub := f.AutoPublish
	autoPubExplicit := f.AutoPublishSet
	if !f.AutoPublishSet {
		if rt.Env.AutoPublish != nil {
			autoPub = *rt.Env.AutoPublish
			autoPubExplicit = true
		} else if rt.Project != nil && rt.Project.AutoPublish != nil {
			autoPub = *rt.Project.AutoPublish
			autoPubExplicit = true
		}
	}

	tagPrefix := ""
	commitTpl := ""
	signerCmd := ""
	if rt.Project != nil {
		tagPrefix = rt.Project.TagPrefix
		commitTpl = rt.Project.CommitTemplate
		signerCmd = rt.Project.BundleSignerCmd
	}

	res, err := publish.Run(ctx, publish.Options{
		Project:              rt.Android,
		Module:               mod,
		Variant:              chosenVariant,
		PackageName:          resolved.PackageName,
		PishkhanClient:       client,
		Printer:              rt.Printer,
		DryRun:               f.DryRun,
		BumpKind:             android.BumpKind(strings.ToLower(strings.TrimSpace(f.Bump))),
		VersionCode:          f.VersionCode,
		VersionName:          f.VersionName,
		BuildArtifact:        f.Build,
		ArtifactKind:         f.FormatKind,
		APKPath:              f.APKPath,
		AABPath:              f.AABPath,
		BinPath:              f.BinPath,
		BundleSignerCmd:      signerCmd,
		Architecture:         pishkhan.Architecture(arch),
		PollInterval:         f.PollInterval,
		ChangelogFA:          fa,
		ChangelogEN:          en,
		DeveloperNote:        note,
		Rollout:              rollout,
		RolloutExplicit:      rolloutExplicit,
		StagedRolloutEnabled: stagedRolloutEnabled,
		AutoPublish:          autoPub,
		AutoPublishExplicit:  autoPubExplicit,
		NoCommit:             f.NoCommit,
		ReplaceDraft:         f.ReplaceDraft,
		GitCommit:            f.GitCommit,
		GitTag:               f.GitTag,
		GitPush:              f.GitPush,
		TagPrefix:            tagPrefix,
		CommitTemplate:       commitTpl,
	})
	if err != nil {
		return err
	}

	if printed, pErr := rt.Printer.PrintStructured(ctx, res); printed || pErr != nil {
		return pErr
	}

	title := "CafeBazaar Publish Complete"
	if res.DryRun {
		title = "CafeBazaar Publish Plan (Dry Run)"
	}
	items := []output.KeyValue{
		{Key: labelPackageName, Value: res.PackageName},
		{Key: labelVariant, Value: fallbackDash(ctx, res.Variant)},
		{Key: labelArtifact, Value: fallbackDash(ctx, relativeDisplayPath(ctx, rt.Cwd, res.ArtifactPath))},
		{Key: "Committed", Value: fmt.Sprintf("%v", res.Committed)},
	}
	if res.Rollout > 0 {
		items = append(items, output.KeyValue{Key: "Rollout", Value: fmt.Sprintf("%d%%", res.Rollout)})
	}
	if res.Committed || res.DryRun {
		items = append(items, output.KeyValue{Key: "Auto Publish", Value: fmt.Sprintf("%v", res.AutoPublish)})
	}
	if res.GitTagName != "" {
		items = append(items, output.KeyValue{Key: "Git Tag", Value: res.GitTagName})
	}
	if err := rt.Printer.PrintCard(ctx, title, items); err != nil {
		return err
	}
	for _, step := range res.Steps {
		if err := rt.Printer.PrintSuccess(ctx, step); err != nil {
			return err
		}
	}
	return nil
}
