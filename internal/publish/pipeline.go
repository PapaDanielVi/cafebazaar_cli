package publish

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
)

const (
	osWindows   = "windows"
	shellUnix   = "sh"
	shellUnixC  = "-c"
	shellWin    = "cmd"
	shellWinC   = "/C"
	defaultTagP = "v"
)

// Options configures an end-to-end Android release pipeline execution.
type Options struct {
	Project         *android.Project
	Module          android.AppModule
	Variant         android.Variant
	PackageName     string
	PishkhanClient  *pishkhan.Client
	Printer         *output.Printer
	DryRun          bool
	BumpKind        android.BumpKind
	VersionCode     int
	VersionName     string
	BuildArtifact   bool
	ArtifactKind    string
	APKPath         string
	AABPath         string
	BinPath         string
	BundleSignerCmd string
	Architecture    pishkhan.Architecture
	PollInterval    time.Duration
	ChangelogFA     string
	ChangelogEN     string
	DeveloperNote   string
	Rollout         int
	AutoPublish     bool
	NoCommit        bool
	ReplaceDraft    bool
	GitCommit       bool
	GitTag          bool
	GitPush         bool
	TagPrefix       string
	CommitTemplate  string
}

// Result records every action taken (or planned during --dry-run) by the pipeline.
type Result struct {
	DryRun        bool                   `json:"dry_run" yaml:"dry_run"`
	PackageName   string                 `json:"package_name" yaml:"package_name"`
	Variant       string                 `json:"variant,omitempty" yaml:"variant,omitempty"`
	BumpPlan      *android.BumpPlan      `json:"bump_plan,omitempty" yaml:"bump_plan,omitempty"`
	BuiltTask     string                 `json:"built_task,omitempty" yaml:"built_task,omitempty"`
	ArtifactPath  string                 `json:"artifact_path,omitempty" yaml:"artifact_path,omitempty"`
	BinPath       string                 `json:"bin_path,omitempty" yaml:"bin_path,omitempty"`
	GitCommitMsg  string                 `json:"git_commit_message,omitempty" yaml:"git_commit_message,omitempty"`
	GitTagName    string                 `json:"git_tag_name,omitempty" yaml:"git_tag_name,omitempty"`
	BundleState   pishkhan.BundleState   `json:"bundle_state,omitempty" yaml:"bundle_state,omitempty"`
	Committed     bool                   `json:"committed" yaml:"committed"`
	Release       *pishkhan.ReleaseInfo  `json:"release,omitempty" yaml:"release,omitempty"`
	Steps         []string               `json:"steps" yaml:"steps"`
}

// Run executes (or previews during DryRun) the full Android to CafeBazaar release pipeline.
func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.PishkhanClient == nil {
		return Result{}, errors.New("pishkhan client must not be nil")
	}
	if opts.Printer == nil {
		return Result{}, errors.New("printer must not be nil")
	}

	res := Result{
		DryRun:      opts.DryRun,
		PackageName: opts.PackageName,
		Variant:     opts.Variant.Name,
	}

	// Step 1: Fetch remote CafeBazaar status to compute safe next versionCode and check draft releases.
	var publishedResp, committedResp, uncommittedResp pishkhan.ReleaseResponse
	if err := opts.Printer.WithSpinner(ctx, "Checking live CafeBazaar release status", func(spinCtx context.Context) error {
		var err error
		if publishedResp, err = opts.PishkhanClient.LastPublishedRelease(spinCtx); err != nil {
			return err
		}
		if committedResp, err = opts.PishkhanClient.LastCommittedRelease(spinCtx); err != nil {
			return err
		}
		if uncommittedResp, err = opts.PishkhanClient.LastUncommittedRelease(spinCtx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return res, err
	}

	maxRemoteCode := highestVersionCode(ctx, publishedResp, committedResp)
	res.Steps = append(res.Steps, fmt.Sprintf("Checked CafeBazaar status (highest remote versionCode=%d, uncommitted=%v)", maxRemoteCode, uncommittedResp.Exists))

	// Step 2: Optional Gradle version bump.
	activeVersionCode := opts.Module.VersionCode
	activeVersionName := opts.Module.VersionName
	if opts.BumpKind != "" || opts.VersionCode > 0 || strings.TrimSpace(opts.VersionName) != "" {
		plan, err := android.ComputeBump(ctx, opts.Module, android.BumpOptions{
			Kind:           opts.BumpKind,
			MinVersionCode: maxRemoteCode,
			VersionCode:    opts.VersionCode,
			VersionName:    opts.VersionName,
		})
		if err != nil {
			return res, err
		}
		res.BumpPlan = &plan
		activeVersionCode = plan.NewVersionCode
		activeVersionName = plan.NewVersionName

		if err := android.ApplyBump(ctx, plan, opts.DryRun); err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, fmt.Sprintf("Bump Gradle version: %s (%d) -> %s (%d)", plan.OldVersionName, plan.OldVersionCode, plan.NewVersionName, plan.NewVersionCode))
	}

	// Step 3: Optional Gradle build.
	if opts.BuildArtifact && opts.Project != nil && opts.Variant.Name != "" {
		task := opts.Variant.AssembleTask
		if strings.EqualFold(opts.ArtifactKind, android.ArtifactTypeAAB) {
			task = opts.Variant.BundleTask
		}
		res.BuiltTask = task
		if opts.DryRun {
			res.Steps = append(res.Steps, fmt.Sprintf("Build Gradle task %s (dry-run)", task))
		} else {
			if err := opts.Printer.PrintInfo(ctx, fmt.Sprintf("Running Gradle task %s", task)); err != nil {
				return res, err
			}
			if err := android.RunGradleTask(ctx, opts.Project.RootDir, task, opts.Printer.Err, opts.Printer.Err); err != nil {
				return res, err
			}
			res.Steps = append(res.Steps, fmt.Sprintf("Built Gradle task %s", task))
			opts.Module.Artifacts = android.DiscoverArtifacts(ctx, opts.Module)
		}
	}

	// Step 4: Resolve artifact path (explicit or auto-detected from build outputs).
	apkPath, aabPath, binPath, err := resolveArtifacts(ctx, opts)
	if err != nil {
		return res, err
	}
	if aabPath != "" {
		res.ArtifactPath = aabPath
		res.BinPath = binPath
	} else {
		res.ArtifactPath = apkPath
	}

	// Step 5: Optional Git commit, tag, and push.
	if err := runGitSteps(ctx, opts, &res, activeVersionName, activeVersionCode); err != nil {
		return res, err
	}

	// Step 6: CafeBazaar release create / upload / wait / commit.
	if opts.DryRun {
		if uncommittedResp.Exists {
			if opts.ReplaceDraft {
				res.Steps = append(res.Steps, "Delete existing uncommitted release and create a new one (dry-run)")
			} else {
				res.Steps = append(res.Steps, "Reuse existing uncommitted release (dry-run)")
			}
		} else {
			res.Steps = append(res.Steps, "Create new uncommitted release via POST /v1/apps/releases/ (dry-run)")
		}
		if aabPath != "" {
			res.Steps = append(res.Steps, fmt.Sprintf("Upload AAB %s and bin %s, then poll bundle status (dry-run)", aabPath, binPath))
		} else {
			res.Steps = append(res.Steps, fmt.Sprintf("Upload APK %s (arch=%s) (dry-run)", apkPath, effectiveArch(ctx, opts.Architecture)))
		}
		if !opts.NoCommit {
			res.Steps = append(res.Steps, fmt.Sprintf("Commit release (rollout=%d%%, auto_publish=%v) (dry-run)", effectiveRollout(ctx, opts.Rollout), opts.AutoPublish))
		}
		return res, nil
	}

	if uncommittedResp.Exists {
		if opts.ReplaceDraft {
			if _, err := opts.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
				Title:       "Delete Existing Uncommitted Release",
				TargetApp:   opts.PackageName,
				Warning:     "All packages already uploaded to the uncommitted draft release will be permanently removed.",
				PromptLabel: "Delete existing uncommitted draft and create a fresh release?",
			}); err != nil {
				return res, err
			}
			if _, err := opts.PishkhanClient.DeleteRelease(ctx); err != nil {
				return res, err
			}
			if _, err := opts.PishkhanClient.CreateRelease(ctx); err != nil {
				return res, err
			}
			res.Steps = append(res.Steps, "Deleted previous draft and created a fresh uncommitted release")
		} else {
			res.Steps = append(res.Steps, "Reused existing uncommitted release")
		}
	} else {
		if _, err := opts.PishkhanClient.CreateRelease(ctx); err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, "Created new uncommitted release")
	}

	if aabPath != "" {
		onAAB, finAAB := opts.Printer.UploadProgress(ctx, "Uploading AAB")
		aabResp, err := opts.PishkhanClient.UploadAAB(ctx, aabPath, onAAB)
		finAAB(ctx)
		if err != nil {
			return res, err
		}
		for _, warn := range aabResp.Warnings {
			if wErr := opts.Printer.PrintWarning(ctx, warn); wErr != nil {
				return res, wErr
			}
		}
		res.Steps = append(res.Steps, fmt.Sprintf("Uploaded AAB %s (state=%s)", filepath.Base(aabPath), aabResp.Bundle.State))

		if binPath == "" && strings.TrimSpace(opts.BundleSignerCmd) != "" {
			if err := runBundleSignerHook(ctx, opts, aabPath); err != nil {
				return res, err
			}
			binPath = android.FindNearbyBin(ctx, aabPath)
			res.BinPath = binPath
		}
		if binPath == "" {
			return res, errors.New("AAB uploaded, but no Bundle Signer .bin file was found; provide --bin <path> or configure bundle_signer_cmd in .bazaar.yaml")
		}

		onBin, finBin := opts.Printer.UploadProgress(ctx, "Uploading .bin")
		binResp, err := opts.PishkhanClient.UploadBin(ctx, binPath, onBin)
		finBin(ctx)
		if err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, fmt.Sprintf("Uploaded Bundle Signer file %s (state=%s)", filepath.Base(binPath), binResp.Bundle.State))

		finalBundle, err := opts.PishkhanClient.WaitForBundle(ctx, opts.PollInterval, func(tickCtx context.Context, info pishkhan.BundleInfo) error {
			return opts.Printer.PrintInfo(tickCtx, fmt.Sprintf("Bundle status: %s (%s)", info.State, pishkhan.DescribeBundleState(tickCtx, info.State)))
		})
		res.BundleState = finalBundle.State
		if err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, "Bundle processing finished (state=D)")
	} else {
		arch := effectiveArch(ctx, opts.Architecture)
		onAPK, finAPK := opts.Printer.UploadProgress(ctx, "Uploading APK")
		apkResp, err := opts.PishkhanClient.UploadAPK(ctx, pishkhan.UploadAPKRequest{
			FilePath:     apkPath,
			Architecture: arch,
		}, onAPK)
		finAPK(ctx)
		if err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, fmt.Sprintf("Uploaded APK %s (v%s, code %d, arch=%s)", filepath.Base(apkPath), apkResp.Package.VersionName, apkResp.Package.VersionCode, arch))
	}

	if opts.NoCommit {
		res.Steps = append(res.Steps, "Skipped commit (--no-commit); draft release remains uncommitted in Pishkhan")
		return res, nil
	}

	rollout := effectiveRollout(ctx, opts.Rollout)
	warning := "This submits the release to CafeBazaar for review."
	if opts.AutoPublish {
		warning = "AUTO-PUBLISH IS ENABLED: this release will go live to users automatically once approved by CafeBazaar."
	}
	if _, err := opts.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:     "Commit Release to CafeBazaar",
		TargetApp: opts.PackageName,
		Warning:   warning,
		Details: []output.KeyValue{
			{Key: "Artifact", Value: filepath.Base(res.ArtifactPath)},
			{Key: "Rollout", Value: fmt.Sprintf("%d%%", rollout)},
			{Key: "Auto Publish", Value: fmt.Sprintf("%v", opts.AutoPublish)},
			{Key: "Changelog (FA)", Value: output.TruncateRunes(ctx, opts.ChangelogFA, 60)},
			{Key: "Changelog (EN)", Value: output.TruncateRunes(ctx, opts.ChangelogEN, 60)},
		},
		PromptLabel: "Submit this release for CafeBazaar review?",
	}); err != nil {
		return res, err
	}

	var commitResp pishkhan.ReleaseResponse
	if err := opts.Printer.WithSpinner(ctx, "Submitting and publishing release on CafeBazaar (may take up to 10m)", func(spinCtx context.Context) error {
		var callErr error
		commitResp, callErr = opts.PishkhanClient.CommitRelease(spinCtx, pishkhan.CommitReleaseRequest{
			ChangelogEN:             opts.ChangelogEN,
			ChangelogFA:             opts.ChangelogFA,
			DeveloperNote:           opts.DeveloperNote,
			StagedRolloutPercentage: rollout,
			AutoPublish:             opts.AutoPublish,
		})
		return callErr
	}); err != nil {
		return res, err
	}
	res.Committed = true
	res.Release = commitResp.Release
	res.Steps = append(res.Steps, "Committed release to CafeBazaar review queue")

	return res, nil
}

func resolveArtifacts(ctx context.Context, opts Options) (string, string, string, error) {
	apk := strings.TrimSpace(opts.APKPath)
	aab := strings.TrimSpace(opts.AABPath)
	bin := strings.TrimSpace(opts.BinPath)

	if apk == "" && aab == "" {
		wantType := strings.ToLower(strings.TrimSpace(opts.ArtifactKind))
		for _, a := range opts.Module.Artifacts {
			if a.Type == android.ArtifactTypeBin {
				continue
			}
			if wantType != "" && a.Type != wantType {
				continue
			}
			if opts.Variant.Name != "" && a.VariantName != "" && !strings.EqualFold(a.VariantName, opts.Variant.Name) {
				continue
			}
			if a.Type == android.ArtifactTypeAAB {
				aab = a.Path
			} else {
				apk = a.Path
			}
			break
		}
	}

	if aab != "" && bin == "" {
		bin = android.FindNearbyBin(ctx, aab)
	}

	if apk == "" && aab == "" && !opts.DryRun {
		return "", "", "", errors.New("no APK or AAB artifact found; pass --apk/--aab or pass --build to compile with Gradle")
	}
	return apk, aab, bin, nil
}

func runGitSteps(ctx context.Context, opts Options, res *Result, verName string, verCode int) error {
	if !opts.GitCommit && !opts.GitTag && !opts.GitPush {
		return nil
	}
	if opts.Project == nil {
		return errors.New("git workflow requires an Android project directory")
	}

	repoInfo, err := gitflow.Inspect(ctx, opts.Project.RootDir)
	if err != nil {
		return err
	}

	if opts.GitCommit && res.BumpPlan != nil {
		tpl := opts.CommitTemplate
		if strings.TrimSpace(tpl) == "" {
			tpl = "chore(release): bump version to v%s (%d)"
		}
		msg := fmt.Sprintf(tpl, verName, verCode)
		res.GitCommitMsg = msg
		if opts.DryRun {
			res.Steps = append(res.Steps, fmt.Sprintf("Git commit %q (dry-run)", msg))
		} else {
			if err := gitflow.CommitFiles(ctx, repoInfo.RootDir, []string{res.BumpPlan.BuildFile}, msg); err != nil {
				return err
			}
			res.Steps = append(res.Steps, fmt.Sprintf("Committed Gradle version bump: %s", msg))
		}
	}

	if opts.GitTag && verName != "" {
		prefix := opts.TagPrefix
		if prefix == "" {
			prefix = defaultTagP
		}
		tagName := prefix + verName
		res.GitTagName = tagName
		if opts.DryRun {
			res.Steps = append(res.Steps, fmt.Sprintf("Git tag %s (dry-run)", tagName))
		} else {
			if err := gitflow.CreateTag(ctx, repoInfo.RootDir, tagName, fmt.Sprintf("Release %s (%d)", verName, verCode)); err != nil {
				return err
			}
			res.Steps = append(res.Steps, fmt.Sprintf("Created Git tag %s", tagName))
		}
	}

	if opts.GitPush {
		if opts.DryRun {
			res.Steps = append(res.Steps, fmt.Sprintf("Git push branch %s and tag %s (dry-run)", repoInfo.Branch, res.GitTagName))
			return nil
		}
		if _, err := opts.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
			Title:       "Push Git Commit and Tag to Remote",
			TargetApp:   opts.PackageName,
			Warning:     "This pushes local commits and release tags to origin.",
			Details:     []output.KeyValue{{Key: "Branch", Value: repoInfo.Branch}, {Key: "Tag", Value: res.GitTagName}},
			PromptLabel: "Push to origin now?",
		}); err != nil {
			return err
		}
		if err := gitflow.Push(ctx, repoInfo.RootDir, "origin", repoInfo.Branch, res.GitTagName); err != nil {
			return err
		}
		res.Steps = append(res.Steps, fmt.Sprintf("Pushed branch %s and tag %s to origin", repoInfo.Branch, res.GitTagName))
	}

	return nil
}

func runBundleSignerHook(ctx context.Context, opts Options, aabPath string) error {
	expanded := strings.ReplaceAll(opts.BundleSignerCmd, "{aab}", aabPath)
	shell := shellUnix
	flag := shellUnixC
	if runtime.GOOS == osWindows {
		shell = shellWin
		flag = shellWinC
	}
	cmd := exec.CommandContext(ctx, shell, flag, expanded)
	if opts.Project != nil {
		cmd.Dir = opts.Project.RootDir
	}
	cmd.Stdout = opts.Printer.Err
	cmd.Stderr = opts.Printer.Err
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run bundle_signer_cmd: %w", err)
	}
	return nil
}

func highestVersionCode(ctx context.Context, responses ...pishkhan.ReleaseResponse) int {
	_ = ctx
	highest := 0
	for _, r := range responses {
		if !r.Exists || r.Release == nil {
			continue
		}
		for _, pkg := range r.Release.Packages {
			if pkg.VersionCode > highest {
				highest = pkg.VersionCode
			}
		}
	}
	return highest
}

func effectiveArch(ctx context.Context, arch pishkhan.Architecture) pishkhan.Architecture {
	_ = ctx
	if arch == "" {
		return pishkhan.ArchitectureAll
	}
	return arch
}

func effectiveRollout(ctx context.Context, rollout int) int {
	_ = ctx
	if rollout <= 0 {
		return pishkhan.MaxRolloutPercentage
	}
	return rollout
}
