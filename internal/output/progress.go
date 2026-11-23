package output

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	progressBarWidth   = 24
	ciMilestoneStepPct = 25
	spinnerInterval    = 90 * time.Millisecond
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// UploadProgress creates a progress callback suitable for interactive terminals and CI logs.
func (p *Printer) UploadProgress(ctx context.Context, label string) (httpclient.ProgressFunc, func(context.Context)) {
	_ = ctx
	start := time.Now()
	var mu sync.Mutex
	lastMilestone := 0
	lastRender := time.Time{}

	onProgress := func(cbCtx context.Context, sent, total int64) {
		mu.Lock()
		defer mu.Unlock()
		if total <= 0 {
			return
		}
		pct := int((sent * 100) / total)
		if pct > 100 {
			pct = 100
		}

		if !p.Mode.Interactive {
			milestone := (pct / ciMilestoneStepPct) * ciMilestoneStepPct
			if milestone > lastMilestone && milestone > 0 {
				lastMilestone = milestone
				if _, err := fmt.Fprintf(p.Err, "%s: %d%% (%s / %s)\n", label, milestone, HumanBytes(cbCtx, sent), HumanBytes(cbCtx, total)); err != nil {
					return
				}
			}
			return
		}

		now := time.Now()
		if pct < 100 && now.Sub(lastRender) < 80*time.Millisecond {
			return
		}
		lastRender = now

		filled := (pct * progressBarWidth) / 100
		if filled > progressBarWidth {
			filled = progressBarWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)
		elapsed := now.Sub(start).Seconds()
		speed := ""
		if elapsed > 0.1 {
			speed = fmt.Sprintf(" (%s/s)", HumanBytes(cbCtx, int64(float64(sent)/elapsed)))
		}
		styledBar := bar
		if p.Theme.Enabled {
			styledBar = p.Theme.Success.Render(bar)
		}
		if _, err := fmt.Fprintf(p.Err, "\r%s [%s] %3d%% %s / %s%s", label, styledBar, pct, HumanBytes(cbCtx, sent), HumanBytes(cbCtx, total), speed); err != nil {
			return
		}
	}

	finish := func(finCtx context.Context) {
		_ = finCtx
		if p.Mode.Interactive {
			if _, err := fmt.Fprintln(p.Err); err != nil {
				return
			}
		}
	}

	return onProgress, finish
}

// WithSpinner executes action with an animated emerald spinner in interactive mode or a clean log line in CI mode.
func (p *Printer) WithSpinner(ctx context.Context, title string, action func(context.Context) error) error {
	if !p.Mode.Interactive {
		if p.Mode.Format == FormatPlain || p.Mode.Format == FormatTable {
			if _, err := fmt.Fprintf(p.Err, "%s...\n", title); err != nil {
				return fmt.Errorf("write progress status: %w", err)
			}
		}
		return action(ctx)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		idx := 0
		for {
			frame := spinnerFrames[idx%len(spinnerFrames)]
			if p.Theme.Enabled {
				frame = p.Theme.Success.Render(frame)
			}
			if _, err := fmt.Fprintf(p.Err, "\r%s %s", frame, title); err != nil {
				return
			}
			idx++
			select {
			case <-done:
				if _, err := fmt.Fprintf(p.Err, "\r%s\r", strings.Repeat(" ", len(title)+6)); err != nil {
					return
				}
				return
			case <-ctx.Done():
				if _, err := fmt.Fprintf(p.Err, "\r%s\r", strings.Repeat(" ", len(title)+6)); err != nil {
					return
				}
				return
			case <-ticker.C:
			}
		}
	}()

	err := action(ctx)
	close(done)
	wg.Wait()
	return err
}
