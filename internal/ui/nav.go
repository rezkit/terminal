// Package ui contains the Bubble Tea application: a root model that owns a
// stack of screens, an optional modal, and the status/help bars.
package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/terminal/internal/api"
)

// Screen is one full-page view on the navigation stack.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	// View renders the screen into the given body area.
	View(width, height int) string
	Title() string
	help.KeyMap
	// CapturingInput reports whether the screen is consuming typed text
	// (e.g. a filter prompt), in which case global keys are not handled.
	CapturingInput() bool
}

// Modal is a dialog drawn over the current screen. While open it receives
// all key input.
type Modal interface {
	Init() tea.Cmd
	Update(tea.Msg) (Modal, tea.Cmd)
	View(maxWidth, maxHeight int) string
	Title() string
}

// Navigation and app-level messages, returned as commands by screens.
type (
	PushMsg       struct{ Screen Screen }
	PopMsg        struct{}
	ResetMsg      struct{ Screen Screen } // replace the whole stack
	OpenModalMsg  struct{ Modal Modal }
	CloseModalMsg struct{}
	StatusMsg     struct {
		Text  string
		Error bool
	}
	// WorkspaceSelectedMsg is sent once the user picks a workspace.
	WorkspaceSelectedMsg struct{ Workspace api.Workspace }
	// LoggedInMsg is sent when authentication completes in the TUI.
	LoggedInMsg struct{}
)

// Cmd helpers.
func Push(s Screen) tea.Cmd      { return func() tea.Msg { return PushMsg{s} } }
func Pop() tea.Msg               { return PopMsg{} }
func Reset(s Screen) tea.Cmd     { return func() tea.Msg { return ResetMsg{s} } }
func OpenModal(m Modal) tea.Cmd  { return func() tea.Msg { return OpenModalMsg{m} } }
func CloseModal() tea.Msg        { return CloseModalMsg{} }
func Status(text string) tea.Cmd { return func() tea.Msg { return StatusMsg{Text: text} } }
func StatusErr(err error) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: err.Error(), Error: true} }
}
