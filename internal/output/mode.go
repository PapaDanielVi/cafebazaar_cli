package output

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"golang.org/x/term"
)

const (
	// FormatTable renders styled terminal tables and cards.
	FormatTable = "table"
	// FormatPlain renders plain line-oriented text with no ANSI styling.
	FormatPlain = "plain"
	// FormatJSON renders indented UTF-8 JSON without HTML escaping.
	FormatJSON = "json"
	// FormatYAML renders UTF-8 YAML.
	FormatYAML = "yaml"

	// RTLAuto automatically detects terminal BiDi/shaping support.
	RTLAuto = "auto"
	// RTLOn forces Persian glyph shaping and BiDi reordering for terminal display.
	RTLOn = "on"
	// RTLOff keeps raw logical UTF-8 text without shaping or reordering.
	RTLOff = "off"

	// ExitOK indicates successful command execution.
	ExitOK = 0
	// ExitGeneral indicates an unspecified runtime error.
	ExitGeneral = 1
	// ExitUsage indicates invalid CLI flags or arguments.
	ExitUsage = 2
	// ExitAuth indicates missing or invalid CafeBazaar credentials.
	ExitAuth = 3
	// ExitConfirmation indicates a dangerous action was declined or lacked --yes in CI.
	ExitConfirmation = 4
	// ExitAPI indicates a CafeBazaar API or bundle processing failure.
	ExitAPI = 5
)

var (
	// ErrConfirmationRequired indicates a dangerous command was invoked non-interactively without --yes.
	ErrConfirmationRequired = errors.New("confirmation required for dangerous action; re-run interactively or pass --yes")
	// ErrActionCancelled indicates the user declined a confirmation prompt.
	ErrActionCancelled = errors.New("action cancelled by user")
	// ErrNonInteractive indicates an interactive prompt cannot run in headless or CI mode.
	ErrNonInteractive = errors.New("interactive input is unavailable in headless or CI mode; provide required flags or environment variables")
)

// ExitError wraps an error with a deterministic process exit code.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the wrapped error message.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error for errors.Is and errors.As checks.
func (e *ExitError) Unwrap() error {
	return e.Err
}

// ModeInput supplies CLI flags, environment variables, and IO streams for runtime mode detection.
type ModeInput struct {
	FlagOutput  string
	FlagNoUI    bool
	FlagPlain   bool
	FlagNoColor bool
	FlagYes     bool
	FlagVerbose bool
	FlagRTL     string
	ConfigRTL   string
	Env         config.EnvSettings
	Stdin       io.Reader
	Stdout      io.Writer
	LookupEnv   func(string) (string, bool)
}

const (
	defaultTerminalWidth = 88
	minTerminalWidth     = 48
	maxCardWidth         = 100
)

// RuntimeMode controls how bazaar renders output, handles Persian text, and prompts the user.
type RuntimeMode struct {
	Interactive bool
	Color       bool
	Format      string
	RTL         bool
	Yes         bool
	Verbose     bool
	Width       int
}

// ResolveMode determines interactive UI vs headless CI/CD mode, color, format, and Persian RTL shaping.
func ResolveMode(ctx context.Context, in ModeInput) RuntimeMode {
	_ = ctx
	lookup := in.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	format := strings.ToLower(strings.TrimSpace(in.FlagOutput))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(in.Env.Output))
	}
	if in.FlagPlain || in.Env.Plain {
		if format == "" || format == FormatTable {
			format = FormatPlain
		}
	}
	if format == "" {
		format = FormatTable
	}

	stdoutTTY := isTerminalWriter(ctx, in.Stdout)
	stdinTTY := isTerminalReader(ctx, in.Stdin)

	headless := in.FlagNoUI ||
		in.FlagPlain ||
		in.Env.NoUI ||
		in.Env.Plain ||
		in.Env.CI ||
		format == FormatJSON ||
		format == FormatYAML ||
		format == FormatPlain ||
		!stdoutTTY ||
		!stdinTTY

	interactive := !headless
	color := interactive && !in.FlagNoColor && !in.Env.NoColor

	rtlPref := strings.ToLower(strings.TrimSpace(in.FlagRTL))
	if rtlPref == "" {
		rtlPref = strings.ToLower(strings.TrimSpace(in.Env.RTL))
	}
	if rtlPref == "" {
		rtlPref = strings.ToLower(strings.TrimSpace(in.ConfigRTL))
	}
	if rtlPref == "" {
		rtlPref = RTLAuto
	}

	rtl := false
	if format != FormatJSON && format != FormatYAML && stdoutTTY && !in.FlagPlain && !in.Env.Plain && !in.Env.CI {
		switch rtlPref {
		case RTLOn:
			rtl = true
		case RTLOff:
			rtl = false
		default:
			rtl = !hasNativeBiDiTerminal(ctx, lookup)
		}
	}

	return RuntimeMode{
		Interactive: interactive,
		Color:       color,
		Format:      format,
		RTL:         rtl,
		Yes:         in.FlagYes || in.Env.Yes,
		Verbose:     in.FlagVerbose || in.Env.Verbose,
		Width:       detectTerminalWidth(ctx, in.Stdout),
	}
}

func detectTerminalWidth(ctx context.Context, w io.Writer) int {
	_ = ctx
	f, ok := w.(*os.File)
	if !ok {
		return defaultTerminalWidth
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return defaultTerminalWidth
	}
	if width < minTerminalWidth {
		return minTerminalWidth
	}
	return width
}

func hasNativeBiDiTerminal(ctx context.Context, lookup func(string) (string, bool)) bool {
	_ = ctx
	nativeEnvKeys := []string{
		"WEZTERM_EXECUTABLE",
		"WT_SESSION",
		"KONSOLE_VERSION",
		"VTE_VERSION",
		"GHOSTTY_RESOURCES_DIR",
	}
	for _, key := range nativeEnvKeys {
		if val, ok := lookup(key); ok && strings.TrimSpace(val) != "" {
			return true
		}
	}
	if termProg, ok := lookup("TERM_PROGRAM"); ok {
		lower := strings.ToLower(strings.TrimSpace(termProg))
		if lower == "apple_terminal" || lower == "wezterm" || lower == "ghostty" {
			return true
		}
	}
	return false
}

func isTerminalWriter(ctx context.Context, w io.Writer) bool {
	_ = ctx
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func isTerminalReader(ctx context.Context, r io.Reader) bool {
	_ = ctx
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
