// Package theme holds shared Lip Gloss styles.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	Accent  = lipgloss.Color("#7D56F4")
	Subtle  = lipgloss.Color("#6C6C6C")
	Danger  = lipgloss.Color("#E5484D")
	Success = lipgloss.Color("#30A46C")
	Text    = lipgloss.Color("#EDEDED")

	Header = lipgloss.NewStyle().Bold(true).Foreground(Text).Background(Accent).Padding(0, 1)
	Crumb  = lipgloss.NewStyle().Foreground(Subtle).Padding(0, 1)
	Muted  = lipgloss.NewStyle().Foreground(Subtle)
	Error  = lipgloss.NewStyle().Foreground(Danger)
	Ok     = lipgloss.NewStyle().Foreground(Success)
	Bold   = lipgloss.NewStyle().Bold(true)
	Label  = lipgloss.NewStyle().Foreground(Subtle).Width(14)

	ModalBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(1, 2)
	ModalTitle = lipgloss.NewStyle().Bold(true).Foreground(Accent).MarginBottom(1)
	Section    = lipgloss.NewStyle().Bold(true).Foreground(Accent).MarginTop(1)
)

var (
	Warning   = lipgloss.Color("#F5A524")
	Faint     = lipgloss.Color("#444444")
	Selection = lipgloss.Color("#3D2E7C")
	Link      = lipgloss.Color("#8FB8FF")

	Warn    = lipgloss.NewStyle().Foreground(Warning)
	Accent2 = lipgloss.NewStyle().Foreground(Link)
	Rule    = lipgloss.NewStyle().Foreground(Faint)
	Heading = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Tab     = lipgloss.NewStyle().Foreground(Subtle).Padding(0, 1)
	TabOn   = lipgloss.NewStyle().Bold(true).Foreground(Text).Background(Accent).Padding(0, 1)
)

// StatusColor colours booking, reservation and payment status enums.
func StatusColor(status string) color.Color {
	switch status {
	case "CONFIRMED":
		return Success
	case "PENDING":
		return Text
	case "PROCESSING", "ON_HOLD", "WAITLISTED", "CANCELLING", "AUTHORIZED":
		return Warning
	case "ERROR", "FAILED":
		return Danger
	}
	return Subtle // cancelled, archived, void, refunded
}
