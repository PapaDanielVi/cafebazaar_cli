package output

import (
	"context"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

const (
	colorBazaarGreen   = "#00B14F"
	colorEmeraldBright = "#34D399"
	colorAmber         = "#F59E0B"
	colorCrimson       = "#EF4444"
	colorSky           = "#38BDF8"
	colorSlateMuted    = "#94A3B8"
	colorBorder        = "#334155"
	colorWhite         = "#FFFFFF"
	colorDarkText      = "#0F172A"

	starFilled = "★"
	starEmpty  = "☆"
)

// Theme holds Lip Gloss styles tailored to the CafeBazaar palette.
type Theme struct {
	Enabled    bool
	Header     lipgloss.Style
	SubHeader  lipgloss.Style
	Label      lipgloss.Style
	Value      lipgloss.Style
	Muted      lipgloss.Style
	Success    lipgloss.Style
	Warning    lipgloss.Style
	Danger     lipgloss.Style
	Info       lipgloss.Style
	Card       lipgloss.Style
	DangerCard lipgloss.Style
	BadgeGreen lipgloss.Style
	BadgeAmber lipgloss.Style
	BadgeRed   lipgloss.Style
	BadgeBlue  lipgloss.Style
	BadgeMuted lipgloss.Style
}

// NewTheme builds a styled Theme when colorEnabled is true or a zero-ANSI Theme when false.
func NewTheme(ctx context.Context, colorEnabled bool) Theme {
	_ = ctx
	if !colorEnabled {
		plain := lipgloss.NewStyle()
		return Theme{
			Enabled:    false,
			Header:     plain,
			SubHeader:  plain,
			Label:      plain,
			Value:      plain,
			Muted:      plain,
			Success:    plain,
			Warning:    plain,
			Danger:     plain,
			Info:       plain,
			Card:       plain,
			DangerCard: plain,
			BadgeGreen: plain,
			BadgeAmber: plain,
			BadgeRed:   plain,
			BadgeBlue:  plain,
			BadgeMuted: plain,
		}
	}

	return Theme{
		Enabled:   true,
		Header:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorBazaarGreen)),
		SubHeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorEmeraldBright)),
		Label:     lipgloss.NewStyle().Foreground(lipgloss.Color(colorSlateMuted)),
		Value:     lipgloss.NewStyle().Bold(true),
		Muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(colorSlateMuted)),
		Success:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorBazaarGreen)),
		Warning:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAmber)),
		Danger:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorCrimson)),
		Info:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorSky)),
		Card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorBorder)).
			Padding(0, 2),
		DangerCard: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorCrimson)).
			Padding(0, 2),
		BadgeGreen: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorDarkText)).Background(lipgloss.Color(colorEmeraldBright)).Padding(0, 1),
		BadgeAmber: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorDarkText)).Background(lipgloss.Color(colorAmber)).Padding(0, 1),
		BadgeRed:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorCrimson)).Padding(0, 1),
		BadgeBlue:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorDarkText)).Background(lipgloss.Color(colorSky)).Padding(0, 1),
		BadgeMuted: lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorBorder)).Padding(0, 1),
	}
}

// HuhTheme returns a Charmbracelet Huh theme styled with CafeBazaar's emerald accent.
func HuhTheme(ctx context.Context) *huh.Theme {
	_ = ctx
	t := huh.ThemeCharm()
	green := lipgloss.Color(colorBazaarGreen)
	t.Focused.Title = t.Focused.Title.Foreground(green).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(green)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(green).Foreground(lipgloss.Color(colorWhite))
	return t
}

// RenderBadge renders a status pill appropriate for status.
func (t Theme) RenderBadge(ctx context.Context, status string) string {
	_ = ctx
	upper := strings.ToUpper(strings.TrimSpace(status))
	if !t.Enabled {
		return "[" + upper + "]"
	}
	switch upper {
	case "PUBLISHED", "DONE", "ACTIVE", "REPLIED", "OK", "CONNECTED", "READY":
		return t.BadgeGreen.Render(upper)
	case "COMMITTED", "PROCESSING", "SIGNED", "UPLOADED", "PENDING":
		return t.BadgeBlue.Render(upper)
	case "UNCOMMITTED", "DRAFT", "UNREPLIED", "WARNING", "STAGED":
		return t.BadgeAmber.Render(upper)
	case "ERROR", "FAILED", "MISSING", "DANGER":
		return t.BadgeRed.Render(upper)
	default:
		return t.BadgeMuted.Render(upper)
	}
}

// RenderStars formats a 1 to 5 star CafeBazaar rating cleanly.
func (t Theme) RenderStars(ctx context.Context, rate int) string {
	_ = ctx
	if rate < 0 {
		rate = 0
	}
	if rate > 5 {
		rate = 5
	}
	stars := strings.Repeat(starFilled, rate) + strings.Repeat(starEmpty, 5-rate)
	if !t.Enabled {
		return stars
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber)).Render(stars)
}

// RenderTable renders headers and rows using a rounded Lip Gloss table or plain tab-separated lines.
func (t Theme) RenderTable(ctx context.Context, headers []string, rows [][]string) string {
	_ = ctx
	if !t.Enabled {
		var b strings.Builder
		if len(headers) > 0 {
			b.WriteString(strings.Join(headers, "\t"))
			b.WriteByte('\n')
		}
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
		return strings.TrimRight(b.String(), "\n")
	}

	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder))).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			_ = col
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorBazaarGreen)).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	return tbl.Render()
}
