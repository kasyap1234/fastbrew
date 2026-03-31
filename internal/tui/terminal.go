package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// TerminalCapabilities detects and reports terminal features
type TerminalCapabilities struct {
	// Basic capabilities
	IsTerminal    bool
	Width         int
	Height        int
	SupportsColor bool

	// Feature support
	SupportsUnicode    bool
	SupportsTrueColor  bool
	Supports256Color   bool
	SupportsHyperlinks bool

	// Environment info
	TermProgram string
	TermEnv     string
}

// DetectTerminalCapabilities probes the terminal for supported features.
// It gracefully degrades when features are unavailable.
func DetectTerminalCapabilities() TerminalCapabilities {
	caps := TerminalCapabilities{
		IsTerminal:    false,
		SupportsColor: false,
	}

	// Check if stdout is a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return caps
	}
	caps.IsTerminal = true

	// Get terminal size
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil {
		caps.Width = width
		caps.Height = height
	}

	// Detect color support
	caps.TermEnv = os.Getenv("TERM")
	caps.TermProgram = os.Getenv("TERM_PROGRAM")

	// Check for color support via environment
	colorTerm := os.Getenv("COLORTERM")
	switch strings.ToLower(colorTerm) {
	case "truecolor", "24bit":
		caps.SupportsTrueColor = true
		caps.SupportsColor = true
	case "256color":
		caps.Supports256Color = true
		caps.SupportsColor = true
	}

	// Fallback: check TERM environment variable
	if !caps.SupportsColor {
		termLower := strings.ToLower(caps.TermEnv)
		if strings.Contains(termLower, "256") || strings.Contains(termLower, "color") {
			caps.Supports256Color = true
			caps.SupportsColor = true
		}
	}

	// Check for specific terminal programs with known capabilities
	switch caps.TermProgram {
	case "iTerm.app":
		caps.SupportsTrueColor = true
		caps.SupportsColor = true
		caps.SupportsUnicode = true
		caps.SupportsHyperlinks = true
	case "Apple_Terminal":
		caps.SupportsColor = true
		caps.SupportsUnicode = true
	case "vscode":
		caps.SupportsTrueColor = true
		caps.SupportsColor = true
		caps.SupportsUnicode = true
	}

	// Check for unicode support
	if lang := os.Getenv("LANG"); strings.Contains(strings.ToLower(lang), "utf-8") ||
		strings.Contains(strings.ToLower(lang), "utf8") {
		caps.SupportsUnicode = true
	}

	// Check for NO_COLOR (user preference to disable colors)
	if os.Getenv("NO_COLOR") != "" {
		caps.SupportsColor = false
		caps.SupportsTrueColor = false
		caps.Supports256Color = false
	}

	// Force color via environment (override detection)
	if force := os.Getenv("FORCE_COLOR"); force != "" {
		caps.SupportsColor = true
		if level, err := strconv.Atoi(force); err == nil {
			switch level {
			case 1:
				caps.SupportsColor = true
			case 2:
				caps.Supports256Color = true
			case 3:
				caps.SupportsTrueColor = true
			}
		}
	}

	return caps
}

// CanUseAltScreen returns true if the terminal supports alternate screen buffer
func (c TerminalCapabilities) CanUseAltScreen() bool {
	return c.IsTerminal && c.Width >= 20 && c.Height >= 10
}

// CanUseFancyUI returns true if the terminal supports rich UI features
func (c TerminalCapabilities) CanUseFancyUI() bool {
	return c.IsTerminal && c.SupportsColor && c.Width >= 40 && c.Height >= 15
}

// GetRecommendedSpinner returns an appropriate spinner style based on capabilities
func (c TerminalCapabilities) GetRecommendedSpinner() string {
	if c.SupportsUnicode {
		return "Dot" // Unicode spinner
	}
	return "Line" // ASCII fallback
}

// GetRecommendedStyles returns lipgloss style options adjusted for terminal
func (c TerminalCapabilities) GetRecommendedStyles() StyleOptions {
	opts := StyleOptions{
		UseUnicode:    c.SupportsUnicode,
		UseColor:      c.SupportsColor,
		UseTrueColor:  c.SupportsTrueColor,
		UseHyperlinks: c.SupportsHyperlinks,
	}

	// Adjust for small terminals
	if c.Width < 60 {
		opts.CompactMode = true
	}
	if c.Height < 20 {
		opts.ReducePadding = true
	}

	return opts
}

// StyleOptions contains terminal-aware styling preferences
type StyleOptions struct {
	UseUnicode     bool
	UseColor       bool
	UseTrueColor   bool
	UseHyperlinks  bool
	CompactMode    bool
	ReducePadding  bool
}

// String returns a human-readable summary of terminal capabilities
func (c TerminalCapabilities) String() string {
	if !c.IsTerminal {
		return "Not a terminal (piped output)"
	}

	parts := []string{
		fmt.Sprintf("%dx%d", c.Width, c.Height),
	}

	if c.SupportsTrueColor {
		parts = append(parts, "truecolor")
	} else if c.Supports256Color {
		parts = append(parts, "256color")
	} else if c.SupportsColor {
		parts = append(parts, "basic color")
	}

	if c.SupportsUnicode {
		parts = append(parts, "unicode")
	}

	if c.TermProgram != "" {
		parts = append(parts, c.TermProgram)
	}

	return strings.Join(parts, ", ")
}

// CheckTerminal returns an error if the terminal is unsuitable for TUI
func CheckTerminal() error {
	caps := DetectTerminalCapabilities()

	if !caps.IsTerminal {
		return fmt.Errorf("stdout is not a terminal - TUI requires an interactive terminal")
	}

	if caps.Width < 40 || caps.Height < 10 {
		return fmt.Errorf("terminal too small (%dx%d) - minimum 40x10 required", caps.Width, caps.Height)
	}

	return nil
}
