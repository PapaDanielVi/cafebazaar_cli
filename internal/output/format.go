package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	bytesPerKB = 1024
	bytesPerMB = 1024 * 1024
	bytesPerGB = 1024 * 1024 * 1024
)

// KeyValue represents a single labeled metadata row rendered in cards or plain output.
type KeyValue struct {
	Key   string
	Value string
}

// Printer handles all terminal, Persian, JSON, YAML, and headless plain output.
type Printer struct {
	Out   io.Writer
	Err   io.Writer
	In    io.Reader
	Mode  RuntimeMode
	Theme Theme
}

// NewPrinter constructs a Printer configured for mode.
func NewPrinter(ctx context.Context, in io.Reader, out, errOut io.Writer, mode RuntimeMode) *Printer {
	return &Printer{
		In:    in,
		Out:   out,
		Err:   errOut,
		Mode:  mode,
		Theme: NewTheme(ctx, mode.Color),
	}
}

// Text formats Persian text for terminal display when RTL shaping is enabled.
func (p *Printer) Text(ctx context.Context, s string) string {
	return FormatDisplayText(ctx, s, p.Mode.RTL)
}

// PrintStructured outputs data as UTF-8 JSON or YAML when a machine format is active.
func (p *Printer) PrintStructured(ctx context.Context, data any) (bool, error) {
	switch p.Mode.Format {
	case FormatJSON:
		return true, p.PrintJSON(ctx, data)
	case FormatYAML:
		return true, p.PrintYAML(ctx, data)
	default:
		return false, nil
	}
}

// PrintJSON writes pretty-printed UTF-8 JSON without HTML escaping.
func (p *Printer) PrintJSON(ctx context.Context, data any) error {
	_ = ctx
	enc := json.NewEncoder(p.Out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	return nil
}

// PrintYAML writes UTF-8 YAML output.
func (p *Printer) PrintYAML(ctx context.Context, data any) error {
	_ = ctx
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode yaml output: %w", err)
	}
	if _, err := p.Out.Write(encoded); err != nil {
		return fmt.Errorf("write yaml output: %w", err)
	}
	return nil
}

// PrintCard renders a titled key-value summary card in UI mode or plain lines in headless mode.
func (p *Printer) PrintCard(ctx context.Context, title string, items []KeyValue) error {
	if !p.Theme.Enabled {
		if title != "" {
			if _, err := fmt.Fprintf(p.Out, "== %s ==\n", p.Text(ctx, title)); err != nil {
				return fmt.Errorf("write plain header: %w", err)
			}
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(p.Out, "%s: %s\n", item.Key, p.Text(ctx, item.Value)); err != nil {
				return fmt.Errorf("write plain row: %w", err)
			}
		}
		return nil
	}

	cardWidth := p.cardWidth(ctx)
	var b strings.Builder
	if title != "" {
		b.WriteString(p.Theme.Header.Render(p.Text(ctx, title)))
		b.WriteByte('\n')
	}
	maxKey := 0
	for _, item := range items {
		if len(item.Key) > maxKey {
			maxKey = len(item.Key)
		}
	}
	labelColWidth := maxKey + 3
	// Reserve 6 columns for left/right rounded borders and horizontal padding (2 each side + 2 borders).
	valueMaxRunes := cardWidth - labelColWidth - 6
	if valueMaxRunes < 24 {
		valueMaxRunes = 24
	}
	continuationIndent := strings.Repeat(" ", labelColWidth)

	for idx, item := range items {
		pad := strings.Repeat(" ", maxKey-len(item.Key)+2)
		wrapped := WrapRunes(ctx, item.Value, valueMaxRunes)
		b.WriteString(p.Theme.Label.Render(item.Key + ":"))
		b.WriteString(pad)
		b.WriteString(p.renderCardValueLine(ctx, wrapped[0]))
		for _, extraLine := range wrapped[1:] {
			b.WriteByte('\n')
			b.WriteString(continuationIndent)
			b.WriteString(p.renderCardValueLine(ctx, extraLine))
		}
		if idx < len(items)-1 {
			b.WriteByte('\n')
		}
	}
	if _, err := fmt.Fprintln(p.Out, p.Theme.Card.Render(b.String())); err != nil {
		return fmt.Errorf("write styled card: %w", err)
	}
	return nil
}

func (p *Printer) renderCardValueLine(ctx context.Context, line string) string {
	if ContainsPersian(ctx, line) {
		// Keep Persian lines in regular weight so Arabic fallback fonts do not drift past borders.
		return AnchorBiDiLine(ctx, line, p.Mode.RTL)
	}
	return p.Theme.Value.Render(p.Text(ctx, line))
}

func (p *Printer) cardWidth(ctx context.Context) int {
	_ = ctx
	w := p.Mode.Width
	if w <= 0 {
		w = defaultTerminalWidth
	}
	available := w - 2
	if available > maxCardWidth {
		return maxCardWidth
	}
	if available < minTerminalWidth {
		return minTerminalWidth
	}
	return available
}

// PrintTable prints a styled table or tab-delimited rows with Persian-aware cell formatting.
func (p *Printer) PrintTable(ctx context.Context, headers []string, rows [][]string) error {
	formatted := make([][]string, len(rows))
	for i, row := range rows {
		formatted[i] = make([]string, len(row))
		for j, cell := range row {
			formatted[i][j] = p.Text(ctx, cell)
		}
	}
	rendered := p.Theme.RenderTable(ctx, headers, formatted)
	if _, err := fmt.Fprintln(p.Out, rendered); err != nil {
		return fmt.Errorf("write table: %w", err)
	}
	return nil
}

// PrintSuccess prints a formatted success status line.
func (p *Printer) PrintSuccess(ctx context.Context, msg string) error {
	prefix := "[OK]"
	if p.Theme.Enabled {
		prefix = p.Theme.Success.Render("✓")
	}
	if _, err := fmt.Fprintf(p.Out, "%s %s\n", prefix, p.Text(ctx, msg)); err != nil {
		return fmt.Errorf("write success message: %w", err)
	}
	return nil
}

// PrintWarning prints a formatted warning status line to stderr.
func (p *Printer) PrintWarning(ctx context.Context, msg string) error {
	prefix := "[WARN]"
	if p.Theme.Enabled {
		prefix = p.Theme.Warning.Render("!")
	}
	if _, err := fmt.Fprintf(p.Err, "%s %s\n", prefix, p.Text(ctx, msg)); err != nil {
		return fmt.Errorf("write warning message: %w", err)
	}
	return nil
}

// PrintInfo prints an informational line.
func (p *Printer) PrintInfo(ctx context.Context, msg string) error {
	prefix := "[INFO]"
	if p.Theme.Enabled {
		prefix = p.Theme.Info.Render("•")
	}
	if _, err := fmt.Fprintf(p.Out, "%s %s\n", prefix, p.Text(ctx, msg)); err != nil {
		return fmt.Errorf("write info message: %w", err)
	}
	return nil
}

// PrintError prints a formatted error message to stderr.
func (p *Printer) PrintError(ctx context.Context, msg string) error {
	prefix := "[ERROR]"
	if p.Theme.Enabled {
		prefix = p.Theme.Danger.Render("✗")
	}
	if _, err := fmt.Fprintf(p.Err, "%s %s\n", prefix, p.Text(ctx, msg)); err != nil {
		return fmt.Errorf("write error message: %w", err)
	}
	return nil
}

// HumanBytes formats a byte count into a readable string such as 12.4 MB.
func HumanBytes(ctx context.Context, n int64) string {
	_ = ctx
	switch {
	case n >= bytesPerGB:
		return fmt.Sprintf("%.2f GB", float64(n)/bytesPerGB)
	case n >= bytesPerMB:
		return fmt.Sprintf("%.2f MB", float64(n)/bytesPerMB)
	case n >= bytesPerKB:
		return fmt.Sprintf("%.1f KB", float64(n)/bytesPerKB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
