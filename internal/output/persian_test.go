package output_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
)

func TestPersianShapingAndBiDi(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name       string
		input      string
		rtlEnabled bool
		wantEqual  bool
		contains   string
	}{
		{
			name:       "logical utf8 preserved when rtl off",
			input:      "چیزهای باحال جدید اضافه شد",
			rtlEnabled: false,
			wantEqual:  true,
		},
		{
			name:       "english text untouched even when rtl on",
			input:      "Release 1.2.3 published",
			rtlEnabled: true,
			wantEqual:  true,
		},
		{
			name:       "persian text shaped and valid utf8 when rtl on",
			input:      "پژوهش گنجینه کافه\u200cبازار نسخه 2.4.1",
			rtlEnabled: true,
			contains:   "2.4.1",
		},
		{
			name:       "mixed persian and english keeps english token readable",
			input:      "API Key نامعتبر است.",
			rtlEnabled: true,
			contains:   "API Key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := output.FormatDisplayText(ctx, tc.input, tc.rtlEnabled)
			if !utf8.ValidString(got) {
				t.Fatalf("result is not valid utf8: %q", got)
			}
			if tc.wantEqual && got != tc.input {
				t.Fatalf("got %q, want unchanged %q", got, tc.input)
			}
			if tc.contains != "" && !strings.Contains(got, tc.contains) {
				t.Fatalf("expected %q to contain LTR token %q", got, tc.contains)
			}
			if tc.rtlEnabled && output.ContainsPersian(ctx, tc.input) && strings.ContainsRune(got, '\u200c') {
				t.Fatalf("expected ZWNJ to be consumed during shaping, got %q", got)
			}
		})
	}
}

func TestTruncateRunesRespectsUTF8AndZWNJ(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	text := "می\u200cشود برنامه جدید منتشر کرد"
	got := output.TruncateRunes(ctx, text, 6)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated string is invalid utf8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	if strings.ContainsRune(got, '\u200c') {
		t.Fatalf("expected no trailing ZWNJ before ellipsis, got %q", got)
	}
}
