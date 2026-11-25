package tui

import "github.com/charmbracelet/lipgloss"

// Theme tokens follow the tmux status row (see docs/TUI-CONTRACT.md).
var (
	ColorFg      = lipgloss.Color("#cfcfcf")
	ColorDim     = lipgloss.Color("#8a8a8a")
	ColorFocusBg = lipgloss.Color("#d7d7d7")
	ColorFocusFg = lipgloss.Color("#101010")

	NormalStyle = lipgloss.NewStyle().Foreground(ColorFg)
	DimStyle    = lipgloss.NewStyle().Foreground(ColorDim)
	FocusStyle  = lipgloss.NewStyle().
			Background(ColorFocusBg).
			Foreground(ColorFocusFg)
)
