package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	selectDefaultHeight  = 24
	selectMaxVisibleRows = 12
	selectChromeRows     = 5
	selectWidthMargin    = 8
	selectMinLabelWidth  = 16
	errWriteSelectFormat = "write selection summary: %w"
)

// SelectModel implements a bounded, filterable Bubble Tea option picker.
type SelectModel struct {
	ctx         context.Context
	printer     *Printer
	title       string
	description string
	all         []SelectOption
	filtered    []SelectOption
	cursor      int
	scrollTop   int
	width       int
	height      int
	filterText  string
	filtering   bool
	chosen      string
	submitted   bool
	cancelled   bool
}

// NewSelectModel creates a bounded interactive select model for the given options and terminal size.
func NewSelectModel(
	ctx context.Context,
	printer *Printer,
	title string,
	description string,
	options []SelectOption,
	width int,
	height int,
) SelectModel {
	if width <= 0 {
		width = defaultTerminalWidth
	}
	if height <= 0 {
		height = selectDefaultHeight
	}
	copied := make([]SelectOption, len(options))
	copy(copied, options)
	return SelectModel{
		ctx:         ctx,
		printer:     printer,
		title:       title,
		description: description,
		all:         copied,
		filtered:    copied,
		width:       width,
		height:      height,
	}
}

// Chosen returns the selected option value once submitted.
func (m SelectModel) Chosen(ctx context.Context) string {
	_ = ctx
	return m.chosen
}

// Submitted reports whether the user confirmed a selection.
func (m SelectModel) Submitted(ctx context.Context) bool {
	_ = ctx
	return m.submitted
}

// Cancelled reports whether the user cancelled the selection prompt.
func (m SelectModel) Cancelled(ctx context.Context) bool {
	_ = ctx
	return m.cancelled
}

// Init implements tea.Model.
func (m SelectModel) Init() tea.Cmd {
	return nil
}

func (m SelectModel) visibleRows(ctx context.Context) int {
	_ = ctx
	maxFit := max(m.height-selectChromeRows, 1)
	avail := min(maxFit, selectMaxVisibleRows)
	return min(len(m.filtered), avail)
}

func (m *SelectModel) ensureCursorVisible(ctx context.Context) {
	vis := m.visibleRows(ctx)
	if vis <= 0 || len(m.filtered) == 0 {
		m.cursor = 0
		m.scrollTop = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+vis {
		m.scrollTop = m.cursor - vis + 1
	}
	maxTop := max(0, len(m.filtered)-vis)
	if m.scrollTop > maxTop {
		m.scrollTop = maxTop
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
}

func (m *SelectModel) applyFilter(ctx context.Context) {
	query := strings.ToLower(strings.TrimSpace(m.filterText))
	if query == "" {
		m.filtered = m.all
		m.ensureCursorVisible(ctx)
		return
	}
	matches := make([]SelectOption, 0, len(m.all))
	for _, opt := range m.all {
		if strings.Contains(strings.ToLower(opt.Label), query) ||
			strings.Contains(strings.ToLower(opt.Value), query) {
			matches = append(matches, opt)
		}
	}
	m.filtered = matches
	m.cursor = 0
	m.scrollTop = 0
	m.ensureCursorVisible(ctx)
}

// Update implements tea.Model.
func (m SelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.ensureCursorVisible(m.ctx)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit

		case tea.KeyEsc:
			if m.filtering || m.filterText != "" {
				m.filtering = false
				m.filterText = ""
				m.applyFilter(m.ctx)
				return m, nil
			}
			m.cancelled = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) == 0 {
				return m, nil
			}
			m.chosen = m.filtered[m.cursor].Value
			m.submitted = true
			return m, tea.Quit

		case tea.KeyUp:
			m.moveCursor(m.ctx, -1)
			return m, nil

		case tea.KeyDown:
			m.moveCursor(m.ctx, 1)
			return m, nil

		case tea.KeyPgUp:
			if len(m.filtered) > 0 {
				m.cursor = max(0, m.cursor-m.visibleRows(m.ctx))
				m.ensureCursorVisible(m.ctx)
			}
			return m, nil

		case tea.KeyPgDown:
			if len(m.filtered) > 0 {
				m.cursor = min(len(m.filtered)-1, m.cursor+m.visibleRows(m.ctx))
				m.ensureCursorVisible(m.ctx)
			}
			return m, nil

		case tea.KeyHome:
			m.cursor = 0
			m.ensureCursorVisible(m.ctx)
			return m, nil

		case tea.KeyEnd:
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
				m.ensureCursorVisible(m.ctx)
			}
			return m, nil

		case tea.KeyBackspace:
			if m.filtering && len(m.filterText) > 0 {
				runes := []rune(m.filterText)
				m.filterText = string(runes[:len(runes)-1])
				m.applyFilter(m.ctx)
			}
			return m, nil

		case tea.KeyRunes, tea.KeySpace:
			txt := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				txt = " "
			}
			if !m.filtering {
				switch txt {
				case "k":
					m.moveCursor(m.ctx, -1)
					return m, nil
				case "j":
					m.moveCursor(m.ctx, 1)
					return m, nil
				case "/":
					m.filtering = true
					return m, nil
				case "q":
					m.cancelled = true
					return m, tea.Quit
				}
			}
			if m.filtering {
				m.filterText += txt
				m.applyFilter(m.ctx)
				return m, nil
			}
		}
	}
	return m, nil
}

func (m *SelectModel) moveCursor(ctx context.Context, delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = len(m.filtered) - 1
	} else if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	m.ensureCursorVisible(ctx)
}

// View implements tea.Model.
func (m SelectModel) View() string {
	if m.submitted || m.cancelled {
		return ""
	}

	maxLabelRunes := m.width - selectWidthMargin
	if maxLabelRunes < selectMinLabelWidth {
		maxLabelRunes = max(1, m.width-2)
	}

	var b strings.Builder
	title := TruncateRunes(m.ctx, SanitizeInline(m.ctx, m.printer.Text(m.ctx, m.title)), maxLabelRunes)
	if m.printer.Theme.Enabled {
		b.WriteString(m.printer.Theme.Header.Render(title))
	} else {
		b.WriteString(title)
	}
	b.WriteByte('\n')

	if m.filtering || m.filterText != "" {
		filterLine := TruncateRunes(m.ctx, SanitizeInline(m.ctx, fmt.Sprintf("Filter: %s_", m.filterText)), maxLabelRunes)
		if m.printer.Theme.Enabled {
			b.WriteString(m.printer.Theme.Info.Render(filterLine))
		} else {
			b.WriteString(filterLine)
		}
		b.WriteByte('\n')
	} else if strings.TrimSpace(m.description) != "" {
		desc := TruncateRunes(m.ctx, SanitizeInline(m.ctx, m.printer.Text(m.ctx, m.description)), maxLabelRunes)
		if m.printer.Theme.Enabled {
			b.WriteString(m.printer.Theme.Muted.Render(desc))
		} else {
			b.WriteString(desc)
		}
		b.WriteByte('\n')
	}

	vis := m.visibleRows(m.ctx)
	if len(m.filtered) == 0 {
		noMatch := TruncateRunes(m.ctx, "  (no matching options; press Esc to clear filter)", maxLabelRunes)
		if m.printer.Theme.Enabled {
			b.WriteString(m.printer.Theme.Warning.Render(noMatch))
		} else {
			b.WriteString(noMatch)
		}
		b.WriteByte('\n')
	} else {
		end := min(len(m.filtered), m.scrollTop+vis)
		selectedStyle := lipgloss.NewStyle()
		if m.printer.Theme.Enabled {
			selectedStyle = m.printer.Theme.Success
		}
		for i := m.scrollTop; i < end; i++ {
			label := TruncateRunes(m.ctx, SanitizeInline(m.ctx, m.printer.Text(m.ctx, m.filtered[i].Label)), maxLabelRunes)
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("> " + label))
			} else {
				b.WriteString("  ")
				b.WriteString(label)
			}
			b.WriteByte('\n')
		}
	}

	footer := fmt.Sprintf(
		"Item %d/%d  |  Up/Down move  |  / filter  |  Enter select  |  Esc cancel",
		min(m.cursor+1, len(m.filtered)),
		len(m.filtered),
	)
	if len(m.filtered) == 0 {
		footer = "0 matches  |  Backspace edit  |  Esc clear filter"
	}
	footer = TruncateRunes(m.ctx, footer, maxLabelRunes)
	if m.printer.Theme.Enabled {
		b.WriteString(m.printer.Theme.Muted.Render(footer))
	} else {
		b.WriteString(footer)
	}
	return b.String()
}

func (p *Printer) runInteractiveSelect(ctx context.Context, title, description string, options []SelectOption) (string, error) {
	width, height := detectTerminalSize(ctx, p.Err, p.In)
	model := NewSelectModel(ctx, p, title, description, options, width, height)

	prog := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(p.In),
		tea.WithOutput(p.Err),
	)
	finalModel, err := prog.Run()
	if err != nil {
		return "", fmt.Errorf("select %q: %w", title, err)
	}
	res, ok := finalModel.(SelectModel)
	if !ok {
		return "", fmt.Errorf("select %q: unexpected model state", title)
	}
	if res.cancelled || !res.submitted {
		return "", &ExitError{Code: ExitConfirmation, Err: ErrActionCancelled}
	}

	chosenLabel := res.chosen
	for _, opt := range options {
		if opt.Value == res.chosen {
			chosenLabel = opt.Label
			break
		}
	}
	summary := fmt.Sprintf("%s: %s", title, p.Text(ctx, chosenLabel))
	if p.Theme.Enabled {
		if _, writeErr := fmt.Fprintf(p.Err, "%s %s\n", p.Theme.Success.Render("✓"), summary); writeErr != nil {
			return "", fmt.Errorf(errWriteSelectFormat, writeErr)
		}
	} else {
		if _, writeErr := fmt.Fprintf(p.Err, "Selected %s\n", summary); writeErr != nil {
			return "", fmt.Errorf(errWriteSelectFormat, writeErr)
		}
	}
	return res.chosen, nil
}

func detectTerminalSize(ctx context.Context, w io.Writer, r io.Reader) (int, int) {
	_ = ctx
	for _, candidate := range []any{w, r} {
		f, ok := candidate.(*os.File)
		if !ok {
			continue
		}
		width, height, err := term.GetSize(int(f.Fd()))
		if err != nil {
			continue
		}
		if width < minTerminalWidth {
			width = minTerminalWidth
		}
		if height <= 0 {
			height = selectDefaultHeight
		}
		return width, height
	}
	return defaultTerminalWidth, selectDefaultHeight
}
