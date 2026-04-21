package tui

import "github.com/charmbracelet/lipgloss"

//nolint:gochecknoglobals
var (
	stylePrimary   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleSubtle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleBold      = lipgloss.NewStyle().Bold(true)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(1, 2)
	styleDimLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleStepTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).MarginBottom(1)

	// Field label/description styles — change based on focus.
	styleLabelFocused   = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	styleLabelUnfocused = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	styleDescFocused    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleDescUnfocused  = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)
