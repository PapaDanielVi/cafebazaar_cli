package gitflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	gitBinary        = "git"
	defaultGitRemote = "origin"
)

var (
	// ErrNotGitRepo indicates that the working directory is not inside a Git repository.
	ErrNotGitRepo = errors.New("not inside a git repository")
)

// RepoInfo summarizes the local Git repository state for release automation.
type RepoInfo struct {
	RootDir    string `json:"root_dir" yaml:"root_dir"`
	Branch     string `json:"branch" yaml:"branch"`
	LatestTag  string `json:"latest_tag,omitempty" yaml:"latest_tag,omitempty"`
	HasTags    bool   `json:"has_tags" yaml:"has_tags"`
	Dirty      bool   `json:"dirty" yaml:"dirty"`
	DirtyCount int    `json:"dirty_count" yaml:"dirty_count"`
}

// Inspect checks whether dir is inside a Git repository and returns branch, tag, and status info.
func Inspect(ctx context.Context, dir string) (RepoInfo, error) {
	root, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoInfo{}, ErrNotGitRepo
	}

	branch, err := runGit(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		branch = ""
	}

	latestTag, tagErr := runGit(ctx, dir, "describe", "--tags", "--abbrev=0")
	if tagErr != nil {
		latestTag = ""
	}
	hasTags := latestTag != ""
	if !hasTags {
		if tagsOut, listErr := runGit(ctx, dir, "tag", "--sort=-creatordate"); listErr == nil && tagsOut != "" {
			hasTags = true
			if first, _, _ := strings.Cut(tagsOut, "\n"); strings.TrimSpace(first) != "" {
				latestTag = strings.TrimSpace(first)
			}
		}
	}

	statusOut, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return RepoInfo{}, fmt.Errorf("inspect git status: %w", err)
	}
	dirtyCount := 0
	if statusOut != "" {
		dirtyCount = len(strings.Split(statusOut, "\n"))
	}

	return RepoInfo{
		RootDir:    root,
		Branch:     branch,
		LatestTag:  latestTag,
		HasTags:    hasTags,
		Dirty:      dirtyCount > 0,
		DirtyCount: dirtyCount,
	}, nil
}

// CommitsSinceTag returns commit subject lines since tag (or recent commits if tag is empty).
func CommitsSinceTag(ctx context.Context, dir, tag string, maxCount int) ([]string, error) {
	if maxCount <= 0 {
		maxCount = 20
	}
	args := []string{"log", fmt.Sprintf("-n%d", maxCount), "--pretty=format:%s"}
	if strings.TrimSpace(tag) != "" {
		args = append(args, fmt.Sprintf("%s..HEAD", strings.TrimSpace(tag)))
	}
	out, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, fmt.Errorf("read git commit log: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	rawLines := strings.Split(out, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines, nil
}

// FormatSuggestedChangelog joins commit subjects into a concise multiline changelog draft.
func FormatSuggestedChangelog(ctx context.Context, commits []string) string {
	_ = ctx
	if len(commits) == 0 {
		return ""
	}
	lines := make([]string, 0, len(commits))
	for _, c := range commits {
		lines = append(lines, "- "+c)
	}
	return strings.Join(lines, "\n")
}

// CommitFiles stages the specified files and creates a Git commit with message.
func CommitFiles(ctx context.Context, dir string, files []string, message string) error {
	if len(files) == 0 {
		return errors.New("no files specified to commit")
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("commit message must not be empty")
	}
	addArgs := append([]string{"add", "--"}, files...)
	if _, err := runGit(ctx, dir, addArgs...); err != nil {
		return fmt.Errorf("stage files for commit: %w", err)
	}
	if _, err := runGit(ctx, dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("create git commit: %w", err)
	}
	return nil
}

// CreateTag creates an annotated Git tag pointing at HEAD.
func CreateTag(ctx context.Context, dir, tag, message string) error {
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag == "" {
		return errors.New("tag name must not be empty")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "Release " + trimmedTag
	}
	if _, err := runGit(ctx, dir, "tag", "-a", trimmedTag, "-m", msg); err != nil {
		return fmt.Errorf("create git tag %q: %w", trimmedTag, err)
	}
	return nil
}

// Push pushes branch and optional tag to remote.
func Push(ctx context.Context, dir, remote, branch, tag string) error {
	targetRemote := strings.TrimSpace(remote)
	if targetRemote == "" {
		targetRemote = defaultGitRemote
	}
	if strings.TrimSpace(branch) != "" {
		if _, err := runGit(ctx, dir, "push", targetRemote, strings.TrimSpace(branch)); err != nil {
			return fmt.Errorf("push branch %q: %w", branch, err)
		}
	}
	if strings.TrimSpace(tag) != "" {
		if _, err := runGit(ctx, dir, "push", targetRemote, strings.TrimSpace(tag)); err != nil {
			return fmt.Errorf("push tag %q: %w", tag, err)
		}
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gitBinary, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return "", fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), errText, err)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
