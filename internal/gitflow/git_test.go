package gitflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
)

func TestGitflowInspectCommitTagAndChangelog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	repoDir := t.TempDir()
	runCmd := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	runCmd("init")
	runCmd("config", "user.email", "test@example.com")
	runCmd("config", "user.name", "Test Dev")

	f1 := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(f1, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := gitflow.CommitFiles(ctx, repoDir, []string{"README.md"}, "feat: initial commit"); err != nil {
		t.Fatalf("commit initial: %v", err)
	}
	if err := gitflow.CreateTag(ctx, repoDir, "v1.0.0", "Release 1.0.0"); err != nil {
		t.Fatalf("create tag: %v", err)
	}

	if err := os.WriteFile(f1, []byte("hello v2"), 0o644); err != nil {
		t.Fatalf("update file: %v", err)
	}
	if err := gitflow.CommitFiles(ctx, repoDir, []string{"README.md"}, "fix: persian rendering"); err != nil {
		t.Fatalf("commit fix: %v", err)
	}

	info, err := gitflow.Inspect(ctx, repoDir)
	if err != nil {
		t.Fatalf("inspect repo: %v", err)
	}
	if info.LatestTag != "v1.0.0" || info.Dirty {
		t.Fatalf("unexpected repo info: %+v", info)
	}

	commits, err := gitflow.CommitsSinceTag(ctx, repoDir, info.LatestTag, 10)
	if err != nil {
		t.Fatalf("commits since tag: %v", err)
	}
	if len(commits) != 1 || commits[0] != "fix: persian rendering" {
		t.Fatalf("unexpected commits: %v", commits)
	}
	draft := gitflow.FormatSuggestedChangelog(ctx, commits)
	if draft != "- fix: persian rendering" {
		t.Fatalf("unexpected changelog draft: %q", draft)
	}
}
