package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type stubScreen struct{}

func (stubScreen) Init() tea.Cmd                      { return nil }
func (s stubScreen) Update(tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (stubScreen) View(int, int) string               { return "" }
func (stubScreen) Title() string                      { return "Stub" }
func (stubScreen) ShortHelp() []key.Binding           { return nil }
func (stubScreen) FullHelp() [][]key.Binding          { return nil }
func (stubScreen) CapturingInput() bool               { return false }

// statusLine is the status bar (the line above the help bar), unstyled.
func statusLine(a *App) string {
	lines := strings.Split(a.footerView(), "\n")
	return ansi.Strip(lines[0])
}

func TestStatusBarActivity(t *testing.T) {
	a := NewApp(Routes{}, stubScreen{}, "")
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	a.Update(StatusMsg{Text: "Workspace: Alpine"})

	if got := statusLine(a); strings.TrimSpace(got) != "Workspace: Alpine" {
		t.Fatalf("before any request: %q", got)
	}

	if _, cmd := a.Update(RequestStartedMsg{}); cmd == nil {
		t.Fatal("first request should start the spinner")
	}
	if _, cmd := a.Update(RequestStartedMsg{}); cmd != nil {
		t.Fatal("overlapping request should not start a second spinner")
	}
	got := statusLine(a)
	if !strings.HasPrefix(got, "Workspace: Alpine") || !strings.HasSuffix(got, "Loading…") {
		t.Fatalf("loading: %q", got)
	}
	if w := ansi.StringWidth(got); w != 60 {
		t.Fatalf("status line width %d, want 60", w)
	}

	a.Update(RequestFinishedMsg{Elapsed: 120 * time.Millisecond})
	if got := statusLine(a); !strings.HasSuffix(got, "Loading…") {
		t.Fatalf("one request still in flight: %q", got)
	}
	a.Update(RequestFinishedMsg{Elapsed: 1340 * time.Millisecond})
	if got := statusLine(a); !strings.HasSuffix(got, "Last request 1.3s") {
		t.Fatalf("idle: %q", got)
	}
}

func TestFormatElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{
		142*time.Millisecond + 600*time.Microsecond: "142ms",
		999 * time.Millisecond:                      "999ms",
		1250 * time.Millisecond:                     "1.2s",
		12 * time.Second:                            "12.0s",
	} {
		if got := formatElapsed(d); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}
