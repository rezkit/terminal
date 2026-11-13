package screens

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// Sections is the workspace home: the top-level lists (Bookings, Customers,
// Transactions) behind a one-line switcher. Each section keeps its state
// (page, sort, filter) while another is shown, and loads on first view.
type Sections struct {
	screens []ui.Screen
	started []bool
	active  int
	w, h    int
	keys    sectionKeys
}

type sectionKeys struct{ Next, Prev, Jump key.Binding }

// NewHome builds the standard sections for a workspace.
func NewHome(svc api.Service) *Sections {
	return NewSections(NewBookings(svc), NewCustomers(svc), NewTransactions(svc))
}

// NewSections hosts the given screens; the first is shown initially.
func NewSections(screens ...ui.Screen) *Sections {
	jump := make([]string, len(screens))
	for i := range screens {
		jump[i] = fmt.Sprint(i + 1)
	}
	return &Sections{
		screens: screens,
		started: make([]bool, len(screens)),
		keys: sectionKeys{
			Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp(fmt.Sprintf("tab/1-%d", len(screens)), "section")),
			Prev: key.NewBinding(key.WithKeys("shift+tab")),
			Jump: key.NewBinding(key.WithKeys(jump...)),
		},
	}
}

func (s *Sections) Init() tea.Cmd { return s.show(s.active) }

// show activates section i, loading it the first time it is seen.
func (s *Sections) show(i int) tea.Cmd {
	s.active = i
	if s.started[i] {
		return nil
	}
	s.started[i] = true
	return s.screens[i].Init()
}

func (s *Sections) Title() string { return s.screens[s.active].Title() }

func (s *Sections) CapturingInput() bool { return s.screens[s.active].CapturingInput() }

func (s *Sections) ShortHelp() []key.Binding {
	return append([]key.Binding{s.keys.Next}, s.screens[s.active].ShortHelp()...)
}

func (s *Sections) FullHelp() [][]key.Binding {
	return append(s.screens[s.active].FullHelp(), []key.Binding{s.keys.Next})
}

func (s *Sections) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
		return s, s.broadcast(tea.WindowSizeMsg{Width: msg.Width, Height: max(msg.Height-1, 1)})
	case tea.KeyPressMsg:
		if !s.CapturingInput() {
			n := len(s.screens)
			switch {
			case key.Matches(msg, s.keys.Next):
				return s, s.show((s.active + 1) % n)
			case key.Matches(msg, s.keys.Prev):
				return s, s.show((s.active + n - 1) % n)
			case key.Matches(msg, s.keys.Jump):
				return s, s.show(int(msg.String()[0] - '1'))
			}
		}
		var cmd tea.Cmd
		s.screens[s.active], cmd = s.screens[s.active].Update(msg)
		return s, cmd
	}
	// Results, ticks and modal outcomes go to every section: each drops
	// what it did not ask for, and background loads still land.
	return s, s.broadcast(msg)
}

func (s *Sections) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(s.screens))
	for i := range s.screens {
		s.screens[i], cmds[i] = s.screens[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (s *Sections) View(width, height int) string {
	parts := make([]string, len(s.screens))
	for i, sc := range s.screens {
		label := fmt.Sprintf("%d %s", i+1, sc.Title())
		if i == s.active {
			parts[i] = theme.TabOn.Render(label)
		} else {
			parts[i] = theme.Tab.Render(label)
		}
	}
	return strings.Join(parts, " ") + "\n" + s.screens[s.active].View(width, max(height-1, 1))
}
