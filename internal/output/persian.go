package output

import (
	"context"
	"strings"
	"unicode"
)

const (
	runeZWNJ = '\u200c'
	runeZWJ  = '\u200d'
	runeLam  = '\u0644'

	truncationEllipsis = "..."
)

type glyphForms struct {
	isolated rune
	final    rune
	initial  rune
	medial   rune
	dualJoin bool
}

// persianGlyphs maps Persian and Arabic script codepoints to Unicode Presentation Forms-B.
var persianGlyphs = map[rune]glyphForms{
	'\u0621': {isolated: '\uFE80', final: '\uFE80'},
	'\u0622': {isolated: '\uFE81', final: '\uFE82'},
	'\u0623': {isolated: '\uFE83', final: '\uFE84'},
	'\u0624': {isolated: '\uFE85', final: '\uFE86'},
	'\u0625': {isolated: '\uFE87', final: '\uFE88'},
	'\u0626': {isolated: '\uFE89', final: '\uFE8A', initial: '\uFE8B', medial: '\uFE8C', dualJoin: true},
	'\u0627': {isolated: '\uFE8D', final: '\uFE8E'},
	'\u0628': {isolated: '\uFE8F', final: '\uFE90', initial: '\uFE91', medial: '\uFE92', dualJoin: true},
	'\u0629': {isolated: '\uFE93', final: '\uFE94'},
	'\u062A': {isolated: '\uFE95', final: '\uFE96', initial: '\uFE97', medial: '\uFE98', dualJoin: true},
	'\u062B': {isolated: '\uFE99', final: '\uFE9A', initial: '\uFE9B', medial: '\uFE9C', dualJoin: true},
	'\u062C': {isolated: '\uFE9D', final: '\uFE9E', initial: '\uFE9F', medial: '\uFEA0', dualJoin: true},
	'\u062D': {isolated: '\uFEA1', final: '\uFEA2', initial: '\uFEA3', medial: '\uFEA4', dualJoin: true},
	'\u062E': {isolated: '\uFEA5', final: '\uFEA6', initial: '\uFEA7', medial: '\uFEA8', dualJoin: true},
	'\u062F': {isolated: '\uFEA9', final: '\uFEAA'},
	'\u0630': {isolated: '\uFEAB', final: '\uFEAC'},
	'\u0631': {isolated: '\uFEAD', final: '\uFEAE'},
	'\u0632': {isolated: '\uFEAF', final: '\uFEB0'},
	'\u0633': {isolated: '\uFEB1', final: '\uFEB2', initial: '\uFEB3', medial: '\uFEB4', dualJoin: true},
	'\u0634': {isolated: '\uFEB5', final: '\uFEB6', initial: '\uFEB7', medial: '\uFEB8', dualJoin: true},
	'\u0635': {isolated: '\uFEB9', final: '\uFEBA', initial: '\uFEBB', medial: '\uFEBC', dualJoin: true},
	'\u0636': {isolated: '\uFEBD', final: '\uFEBE', initial: '\uFEBF', medial: '\uFEC0', dualJoin: true},
	'\u0637': {isolated: '\uFEC1', final: '\uFEC2', initial: '\uFEC3', medial: '\uFEC4', dualJoin: true},
	'\u0638': {isolated: '\uFEC5', final: '\uFEC6', initial: '\uFEC7', medial: '\uFEC8', dualJoin: true},
	'\u0639': {isolated: '\uFEC9', final: '\uFECA', initial: '\uFECB', medial: '\uFECC', dualJoin: true},
	'\u063A': {isolated: '\uFECD', final: '\uFECE', initial: '\uFECF', medial: '\uFED0', dualJoin: true},
	'\u0640': {isolated: '\u0640', final: '\u0640', initial: '\u0640', medial: '\u0640', dualJoin: true},
	'\u0641': {isolated: '\uFED1', final: '\uFED2', initial: '\uFED3', medial: '\uFED4', dualJoin: true},
	'\u0642': {isolated: '\uFED5', final: '\uFED6', initial: '\uFED7', medial: '\uFED8', dualJoin: true},
	'\u0643': {isolated: '\uFED9', final: '\uFEDA', initial: '\uFEDB', medial: '\uFEDC', dualJoin: true},
	'\u0644': {isolated: '\uFEDD', final: '\uFEDE', initial: '\uFEDF', medial: '\uFEE0', dualJoin: true},
	'\u0645': {isolated: '\uFEE1', final: '\uFEE2', initial: '\uFEE3', medial: '\uFEE4', dualJoin: true},
	'\u0646': {isolated: '\uFEE5', final: '\uFEE6', initial: '\uFEE7', medial: '\uFEE8', dualJoin: true},
	'\u0647': {isolated: '\uFEE9', final: '\uFEEA', initial: '\uFEEB', medial: '\uFEEC', dualJoin: true},
	'\u0648': {isolated: '\uFEED', final: '\uFEEE'},
	'\u0649': {isolated: '\uFEEF', final: '\uFEF0'},
	'\u064A': {isolated: '\uFEF1', final: '\uFEF2', initial: '\uFEF3', medial: '\uFEF4', dualJoin: true},
	// Persian-specific letters: پ چ ژ گ ک ی.
	'\u067E': {isolated: '\uFB56', final: '\uFB57', initial: '\uFB58', medial: '\uFB59', dualJoin: true},
	'\u0686': {isolated: '\uFB7A', final: '\uFB7B', initial: '\uFB7C', medial: '\uFB7D', dualJoin: true},
	'\u0698': {isolated: '\uFB8A', final: '\uFB8B'},
	'\u06A9': {isolated: '\uFB8E', final: '\uFB8F', initial: '\uFB90', medial: '\uFB91', dualJoin: true},
	'\u06AF': {isolated: '\uFB92', final: '\uFB93', initial: '\uFB94', medial: '\uFB95', dualJoin: true},
	'\u06CC': {isolated: '\uFBFC', final: '\uFBFD', initial: '\uFBFE', medial: '\uFBFF', dualJoin: true},
}

var lamAlefLigatures = map[rune][2]rune{
	'\u0622': {'\uFEF5', '\uFEF6'},
	'\u0623': {'\uFEF7', '\uFEF8'},
	'\u0625': {'\uFEF9', '\uFEFA'},
	'\u0627': {'\uFEFB', '\uFEFC'},
}

// ContainsPersian reports whether text contains any Persian or Arabic script rune.
func ContainsPersian(ctx context.Context, text string) bool {
	_ = ctx
	for _, r := range text {
		if isArabicScriptRune(ctx, r) {
			return true
		}
	}
	return false
}

// FormatDisplayText applies Persian glyph shaping and BiDi ordering when rtlEnabled is true.
func FormatDisplayText(ctx context.Context, text string, rtlEnabled bool) string {
	if !rtlEnabled || !ContainsPersian(ctx, text) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if ContainsPersian(ctx, line) {
			shaped := shapePersian(ctx, line)
			lines[i] = reorderBiDiLine(ctx, shaped)
		}
	}
	return strings.Join(lines, "\n")
}

// TruncateRunes truncates s to at most maxRunes visible runes without splitting UTF-8 or ZWNJ.
func TruncateRunes(ctx context.Context, s string, maxRunes int) string {
	_ = ctx
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	ellipsisRunes := []rune(truncationEllipsis)
	if maxRunes <= len(ellipsisRunes) {
		return string(runes[:maxRunes])
	}
	cut := maxRunes - len(ellipsisRunes)
	for cut > 0 && (runes[cut-1] == runeZWNJ || runes[cut-1] == runeZWJ || unicode.Is(unicode.Mn, runes[cut-1])) {
		cut--
	}
	return strings.TrimRightFunc(string(runes[:cut]), unicode.IsSpace) + truncationEllipsis
}

func shapePersian(ctx context.Context, s string) string {
	runes := []rune(s)
	out := make([]rune, 0, len(runes))

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == runeZWNJ || r == runeZWJ {
			continue
		}

		// Handle Lam + Alef ligatures cleanly.
		if r == runeLam {
			nextIdx := nextNonCombiningIndex(ctx, runes, i+1)
			if nextIdx >= 0 {
				if lig, ok := lamAlefLigatures[runes[nextIdx]]; ok {
					prevConnects := prevConnectsLeft(ctx, runes, i-1)
					if prevConnects {
						out = append(out, lig[1])
					} else {
						out = append(out, lig[0])
					}
					i = nextIdx
					continue
				}
			}
		}

		forms, isPersian := persianGlyphs[r]
		if !isPersian {
			out = append(out, r)
			continue
		}

		prevConnects := prevConnectsLeft(ctx, runes, i-1)
		nextConnects := forms.dualJoin && nextConnectsRight(ctx, runes, i+1)

		switch {
		case prevConnects && nextConnects && forms.medial != 0:
			out = append(out, forms.medial)
		case prevConnects && forms.final != 0:
			out = append(out, forms.final)
		case nextConnects && forms.initial != 0:
			out = append(out, forms.initial)
		default:
			out = append(out, forms.isolated)
		}
	}

	return string(out)
}

func nextNonCombiningIndex(ctx context.Context, runes []rune, start int) int {
	_ = ctx
	for i := start; i < len(runes); i++ {
		if unicode.Is(unicode.Mn, runes[i]) {
			continue
		}
		return i
	}
	return -1
}

func prevConnectsLeft(ctx context.Context, runes []rune, start int) bool {
	_ = ctx
	for i := start; i >= 0; i-- {
		r := runes[i]
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == runeZWNJ {
			return false
		}
		if r == runeZWJ {
			return true
		}
		forms, ok := persianGlyphs[r]
		return ok && forms.dualJoin
	}
	return false
}

func nextConnectsRight(ctx context.Context, runes []rune, start int) bool {
	_ = ctx
	for i := start; i < len(runes); i++ {
		r := runes[i]
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == runeZWNJ {
			return false
		}
		if r == runeZWJ {
			return true
		}
		_, ok := persianGlyphs[r]
		return ok
	}
	return false
}

func reorderBiDiLine(ctx context.Context, shaped string) string {
	runes := []rune(shaped)
	reversed := make([]rune, len(runes))
	for i, r := range runes {
		reversed[len(runes)-1-i] = mirrorBracket(ctx, r)
	}

	// Restore embedded LTR runs (ASCII words, digits, versions, package names) to left-to-right order.
	i := 0
	for i < len(reversed) {
		if !isLTRRunChar(ctx, reversed[i]) {
			i++
			continue
		}
		start := i
		for i < len(reversed) && (isLTRRunChar(ctx, reversed[i]) || isLTRBridgeChar(ctx, reversed, i)) {
			i++
		}
		end := i
		for end > start && !isLTRRunChar(ctx, reversed[end-1]) {
			end--
		}
		if end-start > 1 {
			reverseRuneSlice(ctx, reversed[start:end])
		}
	}

	return string(reversed)
}

func isArabicScriptRune(ctx context.Context, r rune) bool {
	_ = ctx
	return (r >= 0x0600 && r <= 0x06FF) ||
		(r >= 0x0750 && r <= 0x077F) ||
		(r >= 0xFB50 && r <= 0xFDFF) ||
		(r >= 0xFE70 && r <= 0xFEFF)
}

func isLTRRunChar(ctx context.Context, r rune) bool {
	_ = ctx
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		(r >= '\u06F0' && r <= '\u06F9') ||
		(r >= '\u0660' && r <= '\u0669')
}

func isLTRBridgeChar(ctx context.Context, runes []rune, idx int) bool {
	r := runes[idx]
	if r != '.' && r != '_' && r != '-' && r != '/' && r != ':' && r != '@' && r != '+' && r != ' ' {
		return false
	}
	if idx+1 < len(runes) && isLTRRunChar(ctx, runes[idx+1]) {
		return true
	}
	return false
}

func mirrorBracket(ctx context.Context, r rune) rune {
	_ = ctx
	switch r {
	case '(':
		return ')'
	case ')':
		return '('
	case '[':
		return ']'
	case ']':
		return '['
	case '{':
		return '}'
	case '}':
		return '{'
	default:
		return r
	}
}

func reverseRuneSlice(ctx context.Context, s []rune) {
	_ = ctx
	for l, r := 0, len(s)-1; l < r; l, r = l+1, r-1 {
		s[l], s[r] = s[r], s[l]
	}
}
