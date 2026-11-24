package output

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
)

// SelectOption represents a single selectable item in an interactive picker.
type SelectOption struct {
	Label string
	Value string
}

// Select displays an interactive Huh picker and returns the chosen value.
func (p *Printer) Select(ctx context.Context, title, description string, options []SelectOption) (string, error) {
	if !p.Mode.Interactive {
		return "", ErrNonInteractive
	}
	if len(options) == 0 {
		return "", fmt.Errorf("no options available for %q", title)
	}

	// Use simple line reading when In is not a real TTY (for deterministic tests).
	if !isTerminalReader(ctx, p.In) {
		return p.promptLineSelect(ctx, title, options)
	}

	huhOpts := make([]huh.Option[string], 0, len(options))
	for _, opt := range options {
		huhOpts = append(huhOpts, huh.NewOption(p.Text(ctx, opt.Label), opt.Value))
	}

	selected := options[0].Value
	field := huh.NewSelect[string]().
		Title(p.Text(ctx, title)).
		Description(p.Text(ctx, description)).
		Options(huhOpts...).
		Value(&selected)

	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(HuhTheme(ctx)).
		WithInput(p.In).
		WithOutput(p.Err)

	if err := form.RunWithContext(ctx); err != nil {
		return "", fmt.Errorf("select %q: %w", title, err)
	}
	return selected, nil
}

// Input prompts for a single-line string value (masked when secret is true).
func (p *Printer) Input(ctx context.Context, title, description, placeholder, defaultValue string, secret bool) (string, error) {
	if !p.Mode.Interactive {
		return "", ErrNonInteractive
	}

	// Use simple line reading when In is not a real TTY (for deterministic tests).
	if !isTerminalReader(ctx, p.In) {
		return p.promptLineInput(ctx, title, defaultValue)
	}

	value := defaultValue
	field := huh.NewInput().
		Title(p.Text(ctx, title)).
		Description(p.Text(ctx, description)).
		Placeholder(placeholder).
		Value(&value)
	if secret {
		field = field.EchoMode(huh.EchoModePassword)
	}

	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(HuhTheme(ctx)).
		WithInput(p.In).
		WithOutput(p.Err)

	if err := form.RunWithContext(ctx); err != nil {
		return "", fmt.Errorf("prompt %q: %w", title, err)
	}
	return strings.TrimSpace(value), nil
}

// MultilineInput prompts for multiline UTF-8 text (such as Persian or English changelogs).
func (p *Printer) MultilineInput(ctx context.Context, title, description, defaultValue string) (string, error) {
	if !p.Mode.Interactive {
		return "", ErrNonInteractive
	}

	value := defaultValue
	field := huh.NewText().
		Title(p.Text(ctx, title)).
		Description(p.Text(ctx, description)).
		Value(&value)

	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(HuhTheme(ctx)).
		WithInput(p.In).
		WithOutput(p.Err)

	if err := form.RunWithContext(ctx); err != nil {
		return "", fmt.Errorf("prompt multiline %q: %w", title, err)
	}
	return strings.TrimSpace(value), nil
}

// Confirm prompts the user with a yes/no choice.
func (p *Printer) Confirm(ctx context.Context, title, description string, defaultYes bool) (bool, error) {
	if !p.Mode.Interactive {
		return false, ErrNonInteractive
	}

	// Use simple line reading when In is not a real TTY (for deterministic tests).
	if !isTerminalReader(ctx, p.In) {
		return p.promptLineConfirm(ctx, title, defaultYes)
	}

	confirmed := defaultYes
	field := huh.NewConfirm().
		Title(p.Text(ctx, title)).
		Description(p.Text(ctx, description)).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed)

	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(HuhTheme(ctx)).
		WithInput(p.In).
		WithOutput(p.Err)

	if err := form.RunWithContext(ctx); err != nil {
		return false, fmt.Errorf("confirm %q: %w", title, err)
	}
	return confirmed, nil
}

func (p *Printer) readLine(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("prompt cancelled: %w", err)
	}
	if p.lineReader == nil {
		p.lineReader = bufio.NewReader(p.In)
	}
	line, err := p.lineReader.ReadString('\n')
	return line, err
}

func (p *Printer) promptLineSelect(ctx context.Context, title string, options []SelectOption) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("select cancelled: %w", err)
	}
	if _, err := fmt.Fprintln(p.Err, title); err != nil {
		return "", fmt.Errorf("write select title: %w", err)
	}
	for i, opt := range options {
		if _, err := fmt.Fprintf(p.Err, "  %d) %s\n", i+1, opt.Label); err != nil {
			return "", fmt.Errorf("write select option: %w", err)
		}
	}
	if _, err := fmt.Fprintf(p.Err, "Select [1-%d]: ", len(options)); err != nil {
		return "", fmt.Errorf("write select prompt: %w", err)
	}
	line, err := p.readLine(ctx)
	if err != nil && len(line) == 0 {
		return options[0].Value, nil
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return options[0].Value, nil
	}
	if idx, convErr := strconv.Atoi(trimmed); convErr == nil && idx >= 1 && idx <= len(options) {
		return options[idx-1].Value, nil
	}
	for _, opt := range options {
		if strings.EqualFold(opt.Value, trimmed) {
			return opt.Value, nil
		}
	}
	return "", fmt.Errorf("invalid selection %q for %q", trimmed, title)
}

func (p *Printer) promptLineInput(ctx context.Context, title, defaultValue string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("input cancelled: %w", err)
	}
	promptText := title
	if strings.TrimSpace(defaultValue) != "" {
		promptText = fmt.Sprintf("%s [%s]", title, defaultValue)
	}
	if _, err := fmt.Fprintf(p.Err, "%s: ", promptText); err != nil {
		return "", fmt.Errorf("write input prompt: %w", err)
	}
	line, err := p.readLine(ctx)
	if err != nil && len(line) == 0 {
		return strings.TrimSpace(defaultValue), nil
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return strings.TrimSpace(defaultValue), nil
	}
	return trimmed, nil
}

func (p *Printer) promptLineConfirm(ctx context.Context, title string, defaultYes bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("confirm cancelled: %w", err)
	}
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	if _, err := fmt.Fprintf(p.Err, "%s %s: ", title, hint); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}
	line, err := p.readLine(ctx)
	if err != nil && len(line) == 0 {
		return defaultYes, nil
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	switch ans {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultYes, nil
	}
}
