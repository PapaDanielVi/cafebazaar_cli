package android

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	osWindows          = "windows"
	gradleWrapperWin   = "gradlew.bat"
	gradleFallbackExec = "gradle"
)

// RunGradleTask executes a Gradle build task inside projectRoot using gradlew when available.
func RunGradleTask(ctx context.Context, projectRoot, task string, stdout, stderr io.Writer) error {
	trimmedTask := strings.TrimSpace(task)
	if trimmedTask == "" {
		return errors.New("gradle task must not be empty")
	}

	bin := resolveGradleCommand(ctx, projectRoot)
	cmd := exec.CommandContext(ctx, bin, trimmedTask)
	cmd.Dir = projectRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run gradle task %q: %w", trimmedTask, err)
	}
	return nil
}

func resolveGradleCommand(ctx context.Context, projectRoot string) string {
	_ = ctx
	wrapper := FileGradleWrapper
	if runtime.GOOS == osWindows {
		wrapper = gradleWrapperWin
	}
	candidate := filepath.Join(projectRoot, wrapper)
	if _, err := exec.LookPath(candidate); err == nil {
		return candidate
	}
	return gradleFallbackExec
}
