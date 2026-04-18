package tui

import "github.com/charmbracelet/lipgloss"

var (
	stylePrimary   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleSubtle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleWarning   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleBold      = lipgloss.NewStyle().Bold(true)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(1, 2)
	styleDimLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleStepTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).MarginBottom(1)
)
