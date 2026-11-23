package output_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/charmbracelet/lipgloss"
)

func TestConfirmDangerousActionBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        output.RuntimeMode
		stdin       string
		wantOK      bool
		wantErr     error
		wantDetails bool
	}{
		{
			name:   "non-interactive refuses without yes",
			mode:   output.RuntimeMode{Interactive: false, Yes: false, Format: output.FormatPlain},
			wantOK: false,
			wantErr: output.ErrConfirmationRequired,
		},
		{
			name:   "non-interactive allows with yes",
			mode:   output.RuntimeMode{Interactive: false, Yes: true, Format: output.FormatPlain},
			wantOK: true,
		},
		{
			name:        "interactive empty enter defaults to no",
			mode:        output.RuntimeMode{Interactive: true, Yes: false, Format: output.FormatPlain},
			stdin:       "\n",
			wantOK:      false,
			wantErr:     output.ErrActionCancelled,
			wantDetails: true,
		},
		{
			name:        "interactive explicit yes confirms",
			mode:        output.RuntimeMode{Interactive: true, Yes: false, Format: output.FormatPlain},
			stdin:       "y\n",
			wantOK:      true,
			wantDetails: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var outBuf bytes.Buffer
			var errBuf bytes.Buffer
			p := output.NewPrinter(ctx, strings.NewReader(tc.stdin), &outBuf, &errBuf, tc.mode)

			ok, err := p.ConfirmDangerousAction(ctx, output.ConfirmRequest{
				Title:     "Delete Uncommitted Release",
				TargetApp: "com.example.shop",
				Warning:   "All uploaded APKs will be deleted.",
				Details:   []output.KeyValue{{Key: "Packages", Value: "1"}},
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err=%v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("got ok=%v, want %v", ok, tc.wantOK)
			}
			if tc.wantDetails && !strings.Contains(errBuf.String(), "com.example.shop") {
				t.Fatalf("expected target app in warning card, got %q", errBuf.String())
			}
		})
	}
}

func TestJSONPreservesRawPersianUTF8(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	mode := output.ResolveMode(ctx, output.ModeInput{
		FlagOutput: output.FormatJSON,
		Env:        config.EnvSettings{CI: true},
		Stdin:      strings.NewReader(""),
		Stdout:     &outBuf,
	})
	if mode.Interactive || mode.RTL {
		t.Fatalf("expected non-interactive and rtl=false for JSON CI mode, got %+v", mode)
	}

	p := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, mode)
	persian := "تغییرات نسخه جدید با پشتیبانی فارسی"
	if err := p.PrintJSON(ctx, map[string]string{"changelog_fa": persian}); err != nil {
		t.Fatalf("print json: %v", err)
	}
	if !strings.Contains(outBuf.String(), persian) {
		t.Fatalf("expected raw UTF-8 Persian string in JSON, got %q", outBuf.String())
	}
}

func TestPrintCardWithPersianKeepsClosedBoxAndWraps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var outBuf, errBuf bytes.Buffer
	p := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, output.RuntimeMode{
		Interactive: true,
		Color:       true,
		Format:      output.FormatTable,
		RTL:         false,
		Width:       72,
	})

	val := p.Theme.RenderBadge(ctx, "PUBLISHED") + "  v1.7 (code 8)\nFA: رفع مشکل ضبط صدا و هماهنگ کردن فایل صوتی در اندروید با کیفیت بهتر"
	if err := p.PrintCard(ctx, "CafeBazaar App Status", []output.KeyValue{
		{Key: "Package Name", Value: "ir.negarotion.studio"},
		{Key: "Published", Value: val},
	}); err != nil {
		t.Fatalf("print card: %v", err)
	}

	rendered := outBuf.String()
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected wrapped multi-line card, got:\n%s", rendered)
	}
	if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[0], "╮") {
		t.Fatalf("expected closed top border, got %q", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "╰") || !strings.Contains(last, "╯") {
		t.Fatalf("expected closed bottom border, got %q", last)
	}
	for i := 1; i < len(lines)-1; i++ {
		if !strings.Contains(lines[i], "│") || !strings.HasSuffix(strings.TrimSpace(lines[i]), "│") {
			t.Fatalf("expected closed right border on row %d, got %q", i, lines[i])
		}
	}
	if w := lipgloss.Width("\u200F\u200E"); w != 0 {
		t.Fatalf("expected zero display width for RLM+LRM marks, got %d", w)
	}
}

func TestFormatDurationAndUploadProgressETA(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	durTests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "0s"},
		{name: "subsecond", d: 400 * time.Millisecond, want: "<1s"},
		{name: "seconds", d: 18 * time.Second, want: "18s"},
		{name: "minutes", d: 84 * time.Second, want: "1m 24s"},
		{name: "hours", d: 3665 * time.Second, want: "1h 01m 05s"},
	}
	for _, tc := range durTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := output.FormatDuration(ctx, tc.d); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	// Verify interactive upload progress renders percentage, ETA, and clean post-upload wait.
	var outBuf, errBuf bytes.Buffer
	p := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, output.RuntimeMode{
		Interactive: true,
		Color:       false,
		Format:      output.FormatTable,
	})
	onProg, finish := p.UploadProgress(ctx, "Uploading app.apk")
	time.Sleep(60 * time.Millisecond)
	onProg(ctx, 5*1024*1024, 10*1024*1024)
	time.Sleep(90 * time.Millisecond)
	onProg(ctx, 10*1024*1024, 10*1024*1024)
	time.Sleep(100 * time.Millisecond)
	finish(ctx)

	got := errBuf.String()
	if !strings.Contains(got, "50%") || !strings.Contains(got, "ETA") {
		t.Fatalf("expected percentage and ETA in interactive upload progress, got %q", got)
	}
	if !strings.Contains(got, "100%") || !strings.Contains(got, "Waiting for CafeBazaar") {
		t.Fatalf("expected 100%% and post-upload processing status, got %q", got)
	}
}
