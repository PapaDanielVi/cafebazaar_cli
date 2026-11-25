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
	progressBarWidth        = 24
	ciMilestoneStepPct      = 25
	progressRenderInterval  = 80 * time.Millisecond
	spinnerInterval         = 90 * time.Millisecond
	spinnerElapsedThreshold = 2 * time.Second
	ansiClearToEOL          = "\x1b[K"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// FormatDuration formats a duration into a compact readable string such as 1m 24s or 12s.
func FormatDuration(ctx context.Context, d time.Duration) string {
	_ = ctx
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return "<1s"
	}
	totalSecs := int64(d.Round(time.Second) / time.Second)
	hours := totalSecs / 3600
	mins := (totalSecs % 3600) / 60
	secs := totalSecs % 60
	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %02dm %02ds", hours, mins, secs)
	case mins > 0:
		return fmt.Sprintf("%dm %02ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// UploadProgress creates a progress callback showing percentage, speed, remaining ETA, and post-upload server processing status.
func (p *Printer) UploadProgress(ctx context.Context, label string) (httpclient.ProgressFunc, func(context.Context)) {
	start := time.Now()
	var mu sync.Mutex
	var writeMu sync.Mutex
	lastMilestone := 0
	lastRender := time.Time{}
	completedNotified := false
	renderedInteractive := false

	var waitDone chan struct{}
	var waitWG sync.WaitGroup

	clearSeq := ""
	if p.Theme.Enabled {
		clearSeq = ansiClearToEOL
	}

	startServerWait := func() {
		if !p.Mode.Interactive || waitDone != nil {
			return
		}
		waitStart := time.Now()
		waitDone = make(chan struct{})
		waitWG.Add(1)
		go func(doneCh <-chan struct{}) {
			defer waitWG.Done()
			ticker := time.NewTicker(spinnerInterval)
			defer ticker.Stop()
			idx := 0
			for {
				frame := spinnerFrames[idx%len(spinnerFrames)]
				if p.Theme.Enabled {
					frame = p.Theme.Success.Render(frame)
				}
				elapsed := FormatDuration(ctx, time.Since(waitStart))
				writeMu.Lock()
				_, err := fmt.Fprintf(p.Err, "\r%s Waiting for CafeBazaar to inspect and process package (%s)%s", frame, elapsed, clearSeq)
				writeMu.Unlock()
				if err != nil {
					return
				}
				idx++
				select {
				case <-doneCh:
					writeMu.Lock()
					if _, err := fmt.Fprintf(p.Err, "\r%s", clearSeq); err != nil {
						writeMu.Unlock()
						return
					}
					writeMu.Unlock()
					return
				case <-ctx.Done():
					writeMu.Lock()
					if _, err := fmt.Fprintf(p.Err, "\r%s", clearSeq); err != nil {
						writeMu.Unlock()
						return
					}
					writeMu.Unlock()
					return
				case <-ticker.C:
				}
			}
		}(waitDone)
	}

	onProgress := func(cbCtx context.Context, sent, total int64) {
		mu.Lock()
		defer mu.Unlock()
		if total <= 0 {
			return
		}
		if sent > total {
			sent = total
		}
		pct := min(int((sent*100)/total), 100)

		if !p.Mode.Interactive || p.Mode.Verbose {
			milestone := (pct / ciMilestoneStepPct) * ciMilestoneStepPct
			if milestone > lastMilestone && milestone > 0 {
				lastMilestone = milestone
				p.errMu.Lock()
				_, err := fmt.Fprintf(p.Err, "%s: %d%% (%s / %s)\n", label, milestone, HumanBytes(cbCtx, sent), HumanBytes(cbCtx, total))
				p.errMu.Unlock()
				if err != nil {
					return
				}
			}
			if sent >= total && !completedNotified {
				completedNotified = true
				p.errMu.Lock()
				if _, err := fmt.Fprintf(p.Err, "%s: upload complete, waiting for CafeBazaar server response...\n", label); err != nil {
					p.errMu.Unlock()
					return
				}
				p.errMu.Unlock()
			}
			return
		}

		now := time.Now()
		if sent < total && !lastRender.IsZero() && now.Sub(lastRender) < progressRenderInterval {
			return
		}
		lastRender = now
		renderedInteractive = true

		filled := min((pct*progressBarWidth)/100, progressBarWidth)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)
		styledBar := bar
		if p.Theme.Enabled {
			styledBar = p.Theme.Success.Render(bar)
		}

		elapsed := now.Sub(start)
		stats := buildTransferStats(cbCtx, sent, total, elapsed)

		writeMu.Lock()
		_, err := fmt.Fprintf(p.Err, "\r%s [%s] %3d%% %s / %s%s%s", label, styledBar, pct, HumanBytes(cbCtx, sent), HumanBytes(cbCtx, total), stats, clearSeq)
		if err == nil && sent >= total && !completedNotified {
			completedNotified = true
			_, err = fmt.Fprintln(p.Err)
		}
		writeMu.Unlock()
		if err != nil {
			return
		}

		if sent >= total {
			startServerWait()
		}
	}

	finish := func(finCtx context.Context) {
		_ = finCtx
		mu.Lock()
		doneCh := waitDone
		waitDone = nil
		hadInteractive := renderedInteractive
		alreadyNewlined := completedNotified
		mu.Unlock()

		if doneCh != nil {
			close(doneCh)
			waitWG.Wait()
			return
		}
		if p.Mode.Interactive && hadInteractive && !alreadyNewlined {
			writeMu.Lock()
			if _, err := fmt.Fprintln(p.Err); err != nil {
				writeMu.Unlock()
				return
			}
			writeMu.Unlock()
		}
	}

	return onProgress, finish
}

func buildTransferStats(ctx context.Context, sent, total int64, elapsed time.Duration) string {
	secs := elapsed.Seconds()
	if secs < 0.05 || sent <= 0 {
		if sent < total {
			return " (ETA calculating...)"
		}
		return ""
	}
	bytesPerSec := float64(sent) / secs
	speedStr := HumanBytes(ctx, int64(bytesPerSec)) + "/s"
	if sent >= total {
		return fmt.Sprintf(" (%s, done in %s)", speedStr, FormatDuration(ctx, elapsed))
	}
	remainingBytes := float64(total - sent)
	remaining := time.Duration((remainingBytes / bytesPerSec) * float64(time.Second))
	return fmt.Sprintf(" (%s, ETA %s)", speedStr, FormatDuration(ctx, remaining))
}

// WithSpinner executes action with an animated emerald spinner and elapsed timer in interactive mode or a clean log line in CI or verbose mode.
func (p *Printer) WithSpinner(ctx context.Context, title string, action func(context.Context) error) error {
	if !p.Mode.Interactive || p.Mode.Verbose {
		if p.Mode.Verbose {
			if err := p.PrintDebug(ctx, title+"..."); err != nil {
				return err
			}
		} else if p.Mode.Format == FormatPlain || p.Mode.Format == FormatTable {
			p.errMu.Lock()
			_, err := fmt.Fprintf(p.Err, "%s...\n", title)
			p.errMu.Unlock()
			if err != nil {
				return fmt.Errorf("write progress status: %w", err)
			}
		}
		return action(ctx)
	}

	start := time.Now()
	clearSeq := ""
	if p.Theme.Enabled {
		clearSeq = ansiClearToEOL
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		idx := 0
		for {
			frame := spinnerFrames[idx%len(spinnerFrames)]
			if p.Theme.Enabled {
				frame = p.Theme.Success.Render(frame)
			}
			elapsed := time.Since(start)
			suffix := ""
			if elapsed >= spinnerElapsedThreshold {
				suffix = fmt.Sprintf(" (%s)", FormatDuration(ctx, elapsed))
			}
			if _, err := fmt.Fprintf(p.Err, "\r%s %s%s%s", frame, title, suffix, clearSeq); err != nil {
				return
			}
			idx++
			select {
			case <-done:
				if _, err := fmt.Fprintf(p.Err, "\r%s%s\r", strings.Repeat(" ", len(title)+16), clearSeq); err != nil {
					return
				}
				return
			case <-ctx.Done():
				if _, err := fmt.Fprintf(p.Err, "\r%s%s\r", strings.Repeat(" ", len(title)+16), clearSeq); err != nil {
					return
				}
				return
			case <-ticker.C:
			}
		}
	})

	err := action(ctx)
	close(done)
	wg.Wait()
	return err
}
