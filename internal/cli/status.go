package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newStatusCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	var variant string
	cmd := &cobra.Command{
		Use:   cmdUseStatus,
		Short: "Show combined local Android project info and live CafeBazaar release status",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			return runUnifiedStatus(c.Context(), rt, variant)
		},
	}
	cmd.Flags().StringVar(&variant, "variant", "", "Gradle build variant to inspect")
	registerFlagCompletion(ctx, cmd, "variant", completeGradleVariants(rt))
	return cmd
}

type statusSummary struct {
	PackageName string                   `json:"package_name" yaml:"package_name"`
	ResolvedBy  string                   `json:"resolved_by" yaml:"resolved_by"`
	LocalModule *android.AppModule       `json:"local_module,omitempty" yaml:"local_module,omitempty"`
	Git         *gitflow.RepoInfo        `json:"git,omitempty" yaml:"git,omitempty"`
	Published   pishkhan.ReleaseResponse `json:"published" yaml:"published"`
	Committed   pishkhan.ReleaseResponse `json:"committed" yaml:"committed"`
	Uncommitted pishkhan.ReleaseResponse `json:"uncommitted" yaml:"uncommitted"`
	Bundle      *pishkhan.BundleInfo     `json:"bundle,omitempty" yaml:"bundle,omitempty"`
}

func runUnifiedStatus(ctx context.Context, rt *AppRuntime, variant string) error {
	resolved, err := requireInitializedApp(ctx, rt, variant)
	if err != nil {
		return err
	}
	client, err := rt.newPishkhanClient(ctx, resolved.Token)
	if err != nil {
		return err
	}

	summary := statusSummary{
		PackageName: resolved.PackageName,
		ResolvedBy:  resolved.ResolvedBy,
	}

	if rt.Android != nil {
		if mod, ok := rt.Android.PrimaryModule(ctx, ""); ok {
			summary.LocalModule = &mod
		}
		if gitInfo, gitErr := gitflow.Inspect(ctx, rt.Android.RootDir); gitErr == nil {
			summary.Git = &gitInfo
		}
	}

	if err := rt.Printer.WithSpinner(ctx, "Fetching release status from CafeBazaar Pishkhan", func(spinCtx context.Context) error {
		var callErr error
		if summary.Published, callErr = client.LastPublishedRelease(spinCtx); callErr != nil {
			return callErr
		}
		if summary.Committed, callErr = client.LastCommittedRelease(spinCtx); callErr != nil {
			return callErr
		}
		if summary.Uncommitted, callErr = client.LastUncommittedRelease(spinCtx); callErr != nil {
			return callErr
		}
		if summary.Uncommitted.Exists {
			if bResp, bErr := client.BundleStatus(spinCtx); bErr == nil && bResp.Bundle.State != "" {
				bCopy := bResp.Bundle
				summary.Bundle = &bCopy
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if printed, err := rt.Printer.PrintStructured(ctx, summary); printed || err != nil {
		return err
	}

	items := []output.KeyValue{
		{Key: labelPackageName, Value: fallbackDash(ctx, summary.PackageName)},
		{Key: "Resolved By", Value: summary.ResolvedBy},
	}
	if summary.LocalModule != nil {
		items = append(items, output.KeyValue{
			Key:   "Local Gradle",
			Value: fmt.Sprintf("v%s (code %d) in %s", summary.LocalModule.VersionName, summary.LocalModule.VersionCode, summary.LocalModule.Name),
		})
	}
	if summary.Git != nil {
		gitVal := summary.Git.Branch
		if summary.Git.LatestTag != "" {
			gitVal += " (latest tag: " + summary.Git.LatestTag + ")"
		}
		if summary.Git.Dirty {
			gitVal += fmt.Sprintf(" [%d dirty]", summary.Git.DirtyCount)
		}
		items = append(items, output.KeyValue{Key: "Git", Value: gitVal})
	}

	items = append(items,
		output.KeyValue{Key: "Published", Value: formatReleaseSummary(ctx, rt.Printer, "PUBLISHED", summary.Published)},
		output.KeyValue{Key: "Committed", Value: formatReleaseSummary(ctx, rt.Printer, "COMMITTED", summary.Committed)},
		output.KeyValue{Key: "Uncommitted", Value: formatReleaseSummary(ctx, rt.Printer, "UNCOMMITTED", summary.Uncommitted)},
	)
	if summary.Bundle != nil {
		items = append(items, output.KeyValue{
			Key:   "AAB Bundle",
			Value: fmt.Sprintf("%s (%s)", summary.Bundle.State, pishkhan.DescribeBundleState(ctx, summary.Bundle.State)),
		})
	}

	return rt.Printer.PrintCard(ctx, "CafeBazaar App Status", items)
}

func formatReleaseSummary(ctx context.Context, p *output.Printer, badge string, resp pishkhan.ReleaseResponse) string {
	if !resp.Exists || resp.Release == nil {
		return p.Theme.RenderBadge(ctx, "NONE") + " No release"
	}
	rel := resp.Release
	pkgSummary := "0 packages"
	if len(rel.Packages) > 0 {
		first := rel.Packages[0]
		pkgSummary = fmt.Sprintf("v%s (code %d, %s)", first.VersionName, first.VersionCode, output.HumanBytes(ctx, first.APKSize))
		if len(rel.Packages) > 1 {
			pkgSummary += fmt.Sprintf(" +%d more", len(rel.Packages)-1)
		}
	}
	parts := []string{
		p.Theme.RenderBadge(ctx, badge),
		pkgSummary,
		fmt.Sprintf("rollout=%d%%", rel.StagedRolloutPercentage),
		fmt.Sprintf("auto_publish=%v", rel.AutoPublish),
	}
	summary := strings.Join(parts, "  ")
	if rel.ChangelogFA != nil {
		cleanFA := output.SanitizeInline(ctx, *rel.ChangelogFA)
		if cleanFA != "" {
			summary += "\nFA: " + output.TruncateRunes(ctx, cleanFA, 68)
		}
	}
	return summary
}
