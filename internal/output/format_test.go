package output_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
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
