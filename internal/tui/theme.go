// Package tui implements the Bubble Tea front-end documented by the Python
// assistant_tui.py. Layout, palette (Catppuccin Frappé), and keybindings are
// reproduced for parity.
package tui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Frappé subset used by the original curses TUI.
var (
	colorBase     = lipgloss.Color("#303446")
	colorSurface0 = lipgloss.Color("#414559")
	colorText     = lipgloss.Color("#c6d0f5")
	colorBlue     = lipgloss.Color("#8caaee")
	colorGreen    = lipgloss.Color("#a6d189")
	colorYellow   = lipgloss.Color("#e5c890")
	colorRed      = lipgloss.Color("#e78284")
	colorMauve    = lipgloss.Color("#ca9ee6")
)

// Style presets used across the views; lipgloss styles are immutable so we
// can share these globally.
var (
	styleTopBar = lipgloss.NewStyle().
			Foreground(colorBase).
			Background(colorBlue).
			Bold(true)

	styleStatusOK = lipgloss.NewStyle().Foreground(colorGreen)
	styleStatusErr = lipgloss.NewStyle().Foreground(colorRed)
	styleHint     = lipgloss.NewStyle().Foreground(colorYellow)
	styleEmpty    = lipgloss.NewStyle().Foreground(colorYellow)
	styleMuted    = lipgloss.NewStyle().Foreground(colorSurface0)
	styleText     = lipgloss.NewStyle().Foreground(colorText)
	stylePrompt   = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)

	stylePanelUnfocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSurface0).
				Padding(0, 1)

	stylePanelFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorMauve).
				Padding(0, 1)

	styleSelected = lipgloss.NewStyle().
			Foreground(colorBase).
			Background(colorMauve).
			Bold(true)

	styleDone = lipgloss.NewStyle().
			Foreground(colorSurface0).
			Strikethrough(true)

	styleOverlayBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMauve).
			Padding(1, 2)

	styleOverlayTitle = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
)
