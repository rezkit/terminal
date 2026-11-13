package screens

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/terminal/internal/auth"
	"github.com/rezkit/terminal/internal/browser"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// LoginFunc runs the device login, calling show with the code to display.
type LoginFunc func(ctx context.Context, show func(auth.DeviceCode)) error

// Login shows the device code and waits for the user to approve it.
type Login struct {
	login LoginFunc
	next  func() ui.Screen // screen to show once logged in

	code    *auth.DeviceCode
	err     error
	cancel  context.CancelFunc
	events  chan tea.Msg
	spinner spinner.Model
	keys    struct{ Open, Retry key.Binding }
}

type deviceCodeMsg struct{ code auth.DeviceCode }
type loginDoneMsg struct{ err error }

// NewLogin creates the login screen.
func NewLogin(login LoginFunc, next func() ui.Screen) *Login {
	l := &Login{login: login, next: next, spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
	l.keys.Open = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open browser"))
	l.keys.Retry = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry"))
	return l
}

func (l *Login) start() tea.Cmd {
	if l.cancel != nil {
		l.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel, l.code, l.err = cancel, nil, nil
	events := make(chan tea.Msg, 2)
	l.events = events
	go func() {
		err := l.login(ctx, func(dc auth.DeviceCode) { events <- deviceCodeMsg{dc} })
		events <- loginDoneMsg{err}
	}()
	return tea.Batch(l.spinner.Tick, waitFor(events))
}

func waitFor(ch chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }

func (l *Login) Init() tea.Cmd { return l.start() }

func (l *Login) Title() string { return "Log in" }

func (l *Login) CapturingInput() bool { return false }

func (l *Login) ShortHelp() []key.Binding {
	if l.err != nil {
		return []key.Binding{l.keys.Retry}
	}
	return []key.Binding{l.keys.Open}
}

func (l *Login) FullHelp() [][]key.Binding { return [][]key.Binding{l.ShortHelp()} }

func (l *Login) url() string {
	if l.code == nil {
		return ""
	}
	if l.code.VerificationURIComplete != "" {
		return l.code.VerificationURIComplete
	}
	return l.code.VerificationURI
}

func (l *Login) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case deviceCodeMsg:
		l.code = &msg.code
		_ = browser.Open(l.url()) // best effort; the URL is also shown
		return l, waitFor(l.events)
	case loginDoneMsg:
		if msg.err != nil {
			if !errors.Is(msg.err, context.Canceled) {
				l.err = msg.err
			}
			return l, nil
		}
		return l, tea.Batch(func() tea.Msg { return ui.LoggedInMsg{} }, ui.Reset(l.next()))
	case spinner.TickMsg:
		if l.err != nil {
			return l, nil
		}
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(msg)
		return l, cmd
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, l.keys.Open) && l.code != nil:
			if err := browser.Open(l.url()); err != nil {
				return l, ui.StatusErr(fmt.Errorf("couldn't open a browser: %w", err))
			}
		case key.Matches(msg, l.keys.Retry) && l.err != nil:
			return l, l.start()
		}
	}
	return l, nil
}

func (l *Login) View(width, height int) string {
	var b strings.Builder
	b.WriteString(theme.Bold.Render("Log in to RezKit") + "\n\n")
	switch {
	case l.err != nil:
		b.WriteString(theme.Error.Render(l.err.Error()) + "\n\nPress r to try again.")
	case l.code == nil:
		b.WriteString(l.spinner.View() + " Requesting a login code…")
	default:
		b.WriteString("Open this page in your browser and confirm the code:\n\n")
		b.WriteString("  " + theme.Bold.Render(l.url()) + "\n\n")
		b.WriteString("  Code: " + theme.ModalTitle.Render(l.code.UserCode) + "\n")
		b.WriteString(l.spinner.View() + " Waiting for approval…  " +
			theme.Muted.Render("expires "+l.code.Expiry.Format("15:04")))
	}
	return b.String()
}
