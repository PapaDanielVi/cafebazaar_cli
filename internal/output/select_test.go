package output_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	testCategoryTitle       = "CafeBazaar Category"
	testCategoryDescription = "Pick a category."
)

func buildCategoryTestOptions(ctx context.Context) []output.SelectOption {
	_ = ctx
	opts := make([]output.SelectOption, 0, 31)
	opts = append(opts,
		output.SelectOption{Label: "[20] آب و هوا - Weather (Apps)", Value: "20"},
		output.SelectOption{Label: "[11] ابزارها - Tools (Apps)", Value: "11"},
		output.SelectOption{Label: "[58] استراتژی - Strategy (Games)", Value: "58"},
	)
	for i := 4; i <= 30; i++ {
		opts = append(opts, output.SelectOption{
			Label: fmt.Sprintf("[%d] دسته نمونه %d - Sample Category %d", i+100, i, i),
			Value: fmt.Sprintf("%d", i+100),
		})
	}
	opts = append(opts, output.SelectOption{Label: "Enter another category ID manually...", Value: "__manual__"})
	return opts
}

func assertSelectModel(ctx context.Context, t *testing.T, model tea.Model) output.SelectModel {
	_ = ctx
	t.Helper()
	got, ok := model.(output.SelectModel)
	if !ok {
		t.Fatalf("expected output.SelectModel, got %T", model)
	}
	return got
}

func TestSelectModelBoundsHeightAndScrollsCleanly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name      string
		width     int
		height    int
		downSteps int
		wantValue string
	}{
		{
			name:      "short android studio terminal stays bounded",
			width:     80,
			height:    14,
			downSteps: 15,
			wantValue: "116",
		},
		{
			name:      "standard terminal stays bounded",
			width:     90,
			height:    24,
			downSteps: 2,
			wantValue: "58",
		},
		{
			name:      "very short terminal never overflows height",
			width:     60,
			height:    8,
			downSteps: 30,
			wantValue: "__manual__",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Color:       false,
				Format:      output.FormatTable,
			})
			opts := buildCategoryTestOptions(ctx)
			var m tea.Model = output.NewSelectModel(ctx, printer, testCategoryTitle, "Use Up/Down or type / to filter.", opts, tc.width, tc.height)

			for step := 0; step < tc.downSteps; step++ {
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) >= tc.height {
					t.Fatalf("step %d: rendered %d lines, must stay strictly below terminal height %d\n%s", step, len(lines), tc.height, view)
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
			}

			view := m.View()
			if lines := strings.Split(view, "\n"); len(lines) >= tc.height {
				t.Fatalf("final view rendered %d lines, must stay strictly below terminal height %d", len(lines), tc.height)
			}

			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("expected quit cmd on Enter")
			}
			finalModel := assertSelectModel(ctx, t, m)
			if !finalModel.Submitted(ctx) || finalModel.Cancelled(ctx) {
				t.Fatalf("expected submitted=true cancelled=false, got submitted=%v cancelled=%v", finalModel.Submitted(ctx), finalModel.Cancelled(ctx))
			}
			if got := finalModel.Chosen(ctx); got != tc.wantValue {
				t.Fatalf("got chosen=%q, want %q", got, tc.wantValue)
			}
			if finalModel.View() != "" {
				t.Fatalf("expected empty view after submit, got %q", finalModel.View())
			}
		})
	}
}

func TestSelectModelFilterAndCancel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var outBuf, errBuf bytes.Buffer
	printer := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, output.RuntimeMode{
		Interactive: true,
		Color:       false,
		Format:      output.FormatTable,
	})
	opts := buildCategoryTestOptions(ctx)

	var m tea.Model = output.NewSelectModel(ctx, printer, testCategoryTitle, testCategoryDescription, opts, 80, 14)

	// Start filtering with '/' and type "strategy".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "strategy" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	view := m.View()
	if !strings.Contains(view, "Strategy") || !strings.Contains(view, "Item 1/1") {
		t.Fatalf("expected filtered Strategy match, got:\n%s", view)
	}

	// Select filtered match with Enter.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected := assertSelectModel(ctx, t, m)
	if !selected.Submitted(ctx) || selected.Chosen(ctx) != "58" {
		t.Fatalf("expected Strategy ID 58, got submitted=%v chosen=%q", selected.Submitted(ctx), selected.Chosen(ctx))
	}

	// Verify Esc clears active filter first, then cancels on a second Esc.
	var m2 tea.Model = output.NewSelectModel(ctx, printer, testCategoryTitle, testCategoryDescription, opts, 80, 14)
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if assertSelectModel(ctx, t, m2).Cancelled(ctx) {
		t.Fatal("first Esc while filtering should clear filter instead of cancelling")
	}
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !assertSelectModel(ctx, t, m2).Cancelled(ctx) {
		t.Fatal("second Esc should cancel selection")
	}
}
