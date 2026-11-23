package output

import (
	"context"
	"fmt"
	"strings"
)

// ConfirmRequest describes a dangerous or externally visible action requiring user approval.
type ConfirmRequest struct {
	Title       string
	TargetApp   string
	Warning     string
	Details     []KeyValue
	PromptLabel string
}

// ConfirmDangerousAction enforces explicit user confirmation (defaulting to No) before risky actions.
func (p *Printer) ConfirmDangerousAction(ctx context.Context, req ConfirmRequest) (bool, error) {
	if p.Mode.Yes {
		return true, nil
	}
	if !p.Mode.Interactive {
		return false, &ExitError{
			Code: ExitConfirmation,
			Err:  fmt.Errorf("%s: %w", req.Title, ErrConfirmationRequired),
		}
	}

	if err := p.renderDangerSummary(ctx, req); err != nil {
		return false, err
	}

	prompt := strings.TrimSpace(req.PromptLabel)
	if prompt == "" {
		prompt = "Proceed with this action?"
	}
	ok, err := p.Confirm(ctx, prompt, req.Warning, false)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, &ExitError{
			Code: ExitConfirmation,
			Err:  ErrActionCancelled,
		}
	}
	return true, nil
}

func (p *Printer) renderDangerSummary(ctx context.Context, req ConfirmRequest) error {
	var b strings.Builder
	title := req.Title
	if title == "" {
		title = "Confirm Dangerous Action"
	}

	if p.Theme.Enabled {
		b.WriteString(p.Theme.RenderBadge(ctx, "DANGER"))
		b.WriteByte(' ')
		b.WriteString(p.Theme.Danger.Render(p.Text(ctx, title)))
		b.WriteByte('\n')
	} else {
		b.WriteString("[DANGER] " + p.Text(ctx, title) + "\n")
	}

	if req.TargetApp != "" {
		b.WriteString(fmt.Sprintf("Target App: %s\n", req.TargetApp))
	}
	for _, d := range req.Details {
		b.WriteString(fmt.Sprintf("%s: %s\n", d.Key, AnchorBiDiLine(ctx, d.Value, p.Mode.RTL)))
	}
	if req.Warning != "" {
		b.WriteString("\n")
		if p.Theme.Enabled {
			b.WriteString(p.Theme.Warning.Render(p.Text(ctx, req.Warning)))
		} else {
			b.WriteString("Warning: " + p.Text(ctx, req.Warning))
		}
	}

	rendered := strings.TrimRight(b.String(), "\n")
	if p.Theme.Enabled {
		rendered = p.Theme.DangerCard.MaxWidth(p.cardWidth(ctx)).Render(rendered)
	}
	if _, err := fmt.Fprintln(p.Err, rendered); err != nil {
		return fmt.Errorf("write danger confirmation card: %w", err)
	}
	return nil
}
