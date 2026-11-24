package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newReleaseCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	relCmd := &cobra.Command{
		Use:   "release",
		Short: "Direct 1-to-1 commands for all 10 CafeBazaar Pishkhan Release API endpoints",
	}

	relCmd.AddCommand(
		newReleaseStatusCmd(ctx, rt),
		newReleaseCreateCmd(ctx, rt),
		newReleaseDeleteCmd(ctx, rt),
		newReleaseUploadAPKCmd(ctx, rt),
		newReleaseUploadAABCmd(ctx, rt),
		newReleaseUploadBinCmd(ctx, rt),
		newReleaseBundleStatusCmd(ctx, rt),
		newReleaseCommitCmd(ctx, rt),
	)
	return relCmd
}

func newReleaseStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var only string
	cmd := &cobra.Command{
		Use:   cmdUseStatus,
		Short: "Fetch last published, last committed, and last uncommitted releases",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runReleaseStatus(c.Context(), rt, only)
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Fetch only one release bucket: published, committed, or uncommitted")
	return cmd
}

func runReleaseStatus(ctx context.Context, rt *AppRuntime, only string) error {
	resolved, err := requireInitializedApp(ctx, rt, "")
	if err != nil {
		return err
	}
	client, err := rt.newPishkhanClient(ctx, resolved.Token)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(only)) {
	case "published":
		resp, callErr := client.LastPublishedRelease(ctx)
		if callErr != nil {
			return callErr
		}
		if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
			return pErr
		}
		return rt.Printer.PrintCard(ctx, "Last Published Release", []output.KeyValue{
			{Key: labelPackage, Value: resolved.PackageName},
			{Key: labelStatus, Value: formatReleaseSummary(ctx, rt.Printer, "PUBLISHED", resp)},
		})
	case "committed":
		resp, callErr := client.LastCommittedRelease(ctx)
		if callErr != nil {
			return callErr
		}
		if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
			return pErr
		}
		return rt.Printer.PrintCard(ctx, "Last Committed Release", []output.KeyValue{
			{Key: labelPackage, Value: resolved.PackageName},
			{Key: labelStatus, Value: formatReleaseSummary(ctx, rt.Printer, "COMMITTED", resp)},
		})
	case "uncommitted":
		resp, callErr := client.LastUncommittedRelease(ctx)
		if callErr != nil {
			return callErr
		}
		if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
			return pErr
		}
		return rt.Printer.PrintCard(ctx, "Last Uncommitted Release", []output.KeyValue{
			{Key: labelPackage, Value: resolved.PackageName},
			{Key: labelStatus, Value: formatReleaseSummary(ctx, rt.Printer, "UNCOMMITTED", resp)},
		})
	case "":
		return runUnifiedStatus(ctx, rt, "")
	default:
		return fmt.Errorf("invalid --only %q: expected published, committed, or uncommitted", only)
	}
}

func newReleaseCreateCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "create",
		Short: "Create a new uncommitted release (POST /v1/apps/releases/)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			var resp pishkhan.ReleaseResponse
			if err := rt.Printer.WithSpinner(c.Context(), "Creating new uncommitted release", func(spinCtx context.Context) error {
				var callErr error
				resp, callErr = client.CreateRelease(spinCtx)
				return callErr
			}); err != nil {
				return err
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			return rt.Printer.PrintSuccess(c.Context(), fmt.Sprintf("%s (%s)", resp.Message, resolved.PackageName))
		},
	}
}

func newReleaseDeleteCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "delete",
		Short: "Delete the current uncommitted release (DELETE /v1/apps/releases/)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			if _, err := rt.Printer.ConfirmDangerousAction(c.Context(), output.ConfirmRequest{
				Title:       "Delete Uncommitted Release",
				TargetApp:   resolved.PackageName,
				Warning:     "Permanently deletes the uncommitted draft release and all APK/AAB/bin files attached to it on CafeBazaar.",
				PromptLabel: "Delete the uncommitted release?",
			}); err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			var resp pishkhan.ActionResponse
			if err := rt.Printer.WithSpinner(c.Context(), "Deleting uncommitted release", func(spinCtx context.Context) error {
				var callErr error
				resp, callErr = client.DeleteRelease(spinCtx)
				return callErr
			}); err != nil {
				return err
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			return rt.Printer.PrintSuccess(c.Context(), resp.Message)
		},
	}
}

func newReleaseUploadAPKCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var arch string
	cmd := &cobra.Command{
		Use:               "upload-apk [apk-path]",
		Short:             "Upload an APK to the current uncommitted release (POST /v1/apps/releases/upload/)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeArtifactPaths(rt, android.ArtifactTypeAPK, "apk"),
		RunE: func(c *cobra.Command, args []string) error {
			filePath := ""
			if len(args) > 0 {
				filePath = args[0]
			} else {
				filePath = firstDetectedArtifact(c.Context(), rt, android.ArtifactTypeAPK)
			}
			if filePath == "" {
				return errors.New("no APK file specified or detected in build/outputs")
			}
			effectiveArch := arch
			if effectiveArch == "" {
				effectiveArch = rt.Env.Arch
			}
			if effectiveArch == "" {
				effectiveArch = string(pishkhan.ArchitectureAll)
			}
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			onProg, finish := rt.Printer.UploadProgress(c.Context(), "Uploading "+filepath.Base(filePath))
			resp, err := client.UploadAPK(c.Context(), pishkhan.UploadAPKRequest{
				FilePath:     filePath,
				Architecture: pishkhan.Architecture(effectiveArch),
			}, onProg)
			finish(c.Context())
			if err != nil {
				return err
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			return rt.Printer.PrintCard(c.Context(), "APK Uploaded", []output.KeyValue{
				{Key: labelPackage, Value: resolved.PackageName},
				{Key: labelFile, Value: filepath.Base(filePath)},
				{Key: "Architecture", Value: effectiveArch},
				{Key: labelVersion, Value: fmt.Sprintf("%s (code %d)", resp.Package.VersionName, resp.Package.VersionCode)},
				{Key: "Size", Value: output.HumanBytes(c.Context(), resp.Package.APKSize)},
				{Key: "SHA-1", Value: resp.Package.SHA1Hash},
			})
		},
	}
	cmd.Flags().StringVar(&arch, "arch", "", "CPU architecture: all, armeabi-v7a, arm64-v8a")
	registerFlagCompletion(ctx, cmd, "arch", completeArchFlag(ctx))
	return cmd
}

func newReleaseUploadAABCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:               "upload-aab [aab-path]",
		Short:             "Upload an AAB bundle to the current uncommitted release (POST /v1/apps/releases/upload-aab/)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeArtifactPaths(rt, android.ArtifactTypeAAB, "aab"),
		RunE: func(c *cobra.Command, args []string) error {
			filePath := ""
			if len(args) > 0 {
				filePath = args[0]
			} else {
				filePath = firstDetectedArtifact(c.Context(), rt, android.ArtifactTypeAAB)
			}
			if filePath == "" {
				return errors.New("no AAB file specified or detected in build/outputs")
			}
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			onProg, finish := rt.Printer.UploadProgress(c.Context(), "Uploading "+filepath.Base(filePath))
			resp, err := client.UploadAAB(c.Context(), filePath, onProg)
			finish(c.Context())
			if err != nil {
				return err
			}
			for _, w := range resp.Warnings {
				if wErr := rt.Printer.PrintWarning(c.Context(), w); wErr != nil {
					return wErr
				}
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			return rt.Printer.PrintCard(c.Context(), "AAB Uploaded", []output.KeyValue{
				{Key: labelPackage, Value: resolved.PackageName},
				{Key: labelFile, Value: filepath.Base(filePath)},
				{Key: "Bundle State", Value: fmt.Sprintf("%s (%s)", resp.Bundle.State, pishkhan.DescribeBundleState(c.Context(), resp.Bundle.State))},
				{Key: labelNextStep, Value: "Upload the Bundle Signer .bin file with `bazaar release upload-bin <file.bin>`"},
			})
		},
	}
}

func newReleaseUploadBinCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:               "upload-bin [bin-path]",
		Short:             "Upload a Bundle Signer .bin file (POST /v1/apps/releases/upload-bin/)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeArtifactPaths(rt, android.ArtifactTypeBin, "bin"),
		RunE: func(c *cobra.Command, args []string) error {
			filePath := ""
			if len(args) > 0 {
				filePath = args[0]
			} else {
				filePath = firstDetectedArtifact(c.Context(), rt, android.ArtifactTypeBin)
			}
			if filePath == "" {
				return errors.New("no Bundle Signer .bin file specified or detected")
			}
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			onProg, finish := rt.Printer.UploadProgress(c.Context(), "Uploading "+filepath.Base(filePath))
			resp, err := client.UploadBin(c.Context(), filePath, onProg)
			finish(c.Context())
			if err != nil {
				return err
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			return rt.Printer.PrintCard(c.Context(), "Bundle Signer .bin Uploaded", []output.KeyValue{
				{Key: labelPackage, Value: resolved.PackageName},
				{Key: labelFile, Value: filepath.Base(filePath)},
				{Key: "Bundle State", Value: fmt.Sprintf("%s (%s)", resp.Bundle.State, pishkhan.DescribeBundleState(c.Context(), resp.Bundle.State))},
				{Key: labelNextStep, Value: "Watch processing with `bazaar release bundle-status --watch`"},
			})
		},
	}
}

func newReleaseBundleStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var watch bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "bundle-status",
		Short: "Check AAB bundle processing status (GET /v1/apps/releases/bundle-status/)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			resolved, err := requireInitializedApp(c.Context(), rt, "")
			if err != nil {
				return err
			}
			client, err := rt.newPishkhanClient(c.Context(), resolved.Token)
			if err != nil {
				return err
			}
			if watch {
				info, waitErr := client.WaitForBundle(c.Context(), interval, func(tickCtx context.Context, b pishkhan.BundleInfo) error {
					return rt.Printer.PrintInfo(tickCtx, fmt.Sprintf("Bundle state: %s (%s)", b.State, pishkhan.DescribeBundleState(tickCtx, b.State)))
				})
				if printed, pErr := rt.Printer.PrintStructured(c.Context(), info); printed || pErr != nil {
					if waitErr != nil {
						return waitErr
					}
					return pErr
				}
				if waitErr != nil {
					return waitErr
				}
				return rt.Printer.PrintSuccess(c.Context(), "Bundle processing completed (state=D).")
			}

			resp, err := client.BundleStatus(c.Context())
			if err != nil {
				return err
			}
			if printed, pErr := rt.Printer.PrintStructured(c.Context(), resp); printed || pErr != nil {
				return pErr
			}
			items := []output.KeyValue{
				{Key: labelPackage, Value: resolved.PackageName},
				{Key: "State", Value: fmt.Sprintf("%s %s", rt.Printer.Theme.RenderBadge(c.Context(), string(resp.Bundle.State)), pishkhan.DescribeBundleState(c.Context(), resp.Bundle.State))},
			}
			if len(resp.Bundle.Errors) > 0 {
				items = append(items, output.KeyValue{Key: "Errors", Value: strings.Join(resp.Bundle.Errors, "; ")})
			}
			return rt.Printer.PrintCard(c.Context(), "Bundle Processing Status", items)
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Poll until bundle reaches Done (D) or Error (E)")
	cmd.Flags().DurationVar(&interval, "interval", pishkhan.DefaultBundlePollInterval, "Polling interval when --watch is enabled (CafeBazaar recommends >= 60s)")
	return cmd
}

func newReleaseCommitCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var changelogFA string
	var changelogFAFile string
	var changelogEN string
	var changelogENFile string
	var devNote string
	var rollout int
	var autoPublish bool
	var autoPublishSet bool

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit the uncommitted release and request CafeBazaar review (POST /v1/apps/releases/commit/)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			autoPublishSet = c.Flags().Changed("auto-publish")
			return runReleaseCommit(c.Context(), rt, commitCmdOptions{
				ChangelogFA:     changelogFA,
				ChangelogFAFile: changelogFAFile,
				ChangelogEN:     changelogEN,
				ChangelogENFile: changelogENFile,
				DevNote:         devNote,
				Rollout:         rollout,
				AutoPublish:     autoPublish,
				AutoPublishSet:  autoPublishSet,
			})
		},
	}
	cmd.Flags().StringVar(&changelogFA, "changelog-fa", "", "Persian (Farsi) changelog for users")
	cmd.Flags().StringVar(&changelogFAFile, "changelog-fa-file", "", "Read UTF-8 Persian changelog from file")
	cmd.Flags().StringVar(&changelogEN, "changelog-en", "", "English changelog for users")
	cmd.Flags().StringVar(&changelogENFile, "changelog-en-file", "", "Read UTF-8 English changelog from file")
	cmd.Flags().StringVar(&devNote, "note", "", "Developer note for CafeBazaar review team")
	cmd.Flags().IntVar(&rollout, "rollout", 0, "Staged rollout percentage (1 to 100, default 100)")
	cmd.Flags().BoolVar(&autoPublish, "auto-publish", false, "Publish automatically on CafeBazaar after approval")
	return cmd
}

type commitCmdOptions struct {
	ChangelogFA     string
	ChangelogFAFile string
	ChangelogEN     string
	ChangelogENFile string
	DevNote         string
	Rollout         int
	AutoPublish     bool
	AutoPublishSet  bool
}

func runReleaseCommit(ctx context.Context, rt *AppRuntime, opts commitCmdOptions) error {
	resolved, err := requireInitializedApp(ctx, rt, "")
	if err != nil {
		return err
	}

	fa, err := readUTF8TextFlagOrFile(ctx, opts.ChangelogFA, opts.ChangelogFAFile, rt.Env.ChangelogFA)
	if err != nil {
		return err
	}
	en, err := readUTF8TextFlagOrFile(ctx, opts.ChangelogEN, opts.ChangelogENFile, rt.Env.ChangelogEN)
	if err != nil {
		return err
	}
	note := strings.TrimSpace(opts.DevNote)
	if note == "" {
		note = rt.Env.DevNote
	}

	rollout := opts.Rollout
	if rollout == 0 && rt.Env.Rollout > 0 {
		rollout = rt.Env.Rollout
	}
	if rollout == 0 && rt.Project != nil && rt.Project.StagedRollout > 0 {
		rollout = rt.Project.StagedRollout
	}
	if rollout == 0 {
		rollout = pishkhan.MaxRolloutPercentage
	}

	autoPub := opts.AutoPublish
	if !opts.AutoPublishSet {
		if rt.Env.AutoPublish != nil {
			autoPub = *rt.Env.AutoPublish
		} else if rt.Project != nil && rt.Project.AutoPublish != nil {
			autoPub = *rt.Project.AutoPublish
		}
	}

	warning := "This submits the release to CafeBazaar's review queue."
	if autoPub {
		warning = "AUTO-PUBLISH IS ENABLED: this release will publish automatically to CafeBazaar users once approved."
	}
	if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:     "Commit Release for CafeBazaar Review",
		TargetApp: resolved.PackageName,
		Warning:   warning,
		Details: []output.KeyValue{
			{Key: "Staged Rollout", Value: fmt.Sprintf("%d%%", rollout)},
			{Key: "Auto Publish", Value: fmt.Sprintf("%v", autoPub)},
			{Key: "Changelog (FA)", Value: fallbackDash(ctx, output.TruncateRunes(ctx, fa, 60))},
			{Key: "Changelog (EN)", Value: fallbackDash(ctx, output.TruncateRunes(ctx, en, 60))},
			{Key: "Developer Note", Value: fallbackDash(ctx, note)},
		},
		PromptLabel: "Commit and submit this release now?",
	}); err != nil {
		return err
	}

	client, err := rt.newPishkhanClient(ctx, resolved.Token)
	if err != nil {
		return err
	}
	var resp pishkhan.ReleaseResponse
	if err := rt.Printer.WithSpinner(ctx, "Submitting release to CafeBazaar (may take up to 10m)", func(spinCtx context.Context) error {
		var callErr error
		resp, callErr = client.CommitRelease(spinCtx, pishkhan.CommitReleaseRequest{
			ChangelogEN:             en,
			ChangelogFA:             fa,
			DeveloperNote:           note,
			StagedRolloutPercentage: rollout,
			AutoPublish:             autoPub,
		})
		return callErr
	}); err != nil {
		return err
	}

	if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
		return pErr
	}
	return rt.Printer.PrintSuccess(ctx, fmt.Sprintf("%s (%s, rollout=%d%%, auto_publish=%v)", resp.Message, resolved.PackageName, rollout, autoPub))
}

func firstDetectedArtifact(ctx context.Context, rt *AppRuntime, artifactType string) string {
	if rt.Android == nil {
		return ""
	}
	mod, ok := rt.Android.PrimaryModule(ctx, "")
	if !ok {
		return ""
	}
	for _, a := range mod.Artifacts {
		if a.Type == artifactType {
			return a.Path
		}
	}
	return ""
}
