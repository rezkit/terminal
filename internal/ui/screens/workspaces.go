package screens

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui"
	"github.com/rezkit/tui/internal/ui/theme"
)

// Workspaces lets the user pick an organization workspace.
type Workspaces struct {
	svc      api.Service
	onSelect func(api.Workspace) // e.g. persist as last-used
	preferID string              // auto-select this ID if present

	list    list.Model
	spinner spinner.Model
	loaded  bool
	err     error
	choose  key.Binding
}

type workspaceItem api.Workspace

func (w workspaceItem) Title() string { return w.Name }
func (w workspaceItem) Description() string {
	if w.Environment != "" {
		return w.Environment
	}
	return w.ID
}
func (w workspaceItem) FilterValue() string { return w.Name }

type workspacesLoadedMsg struct {
	items []api.Workspace
	err   error
}

// NewWorkspaces creates the picker. If preferID matches a workspace it is
// selected automatically (used to restore the last-used workspace).
func NewWorkspaces(svc api.Service, preferID string, onSelect func(api.Workspace)) *Workspaces {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a workspace"
	l.SetShowHelp(false) // the app renders help
	l.SetStatusBarItemName("workspace", "workspaces")
	return &Workspaces{
		svc: svc, onSelect: onSelect, preferID: preferID,
		list:    l,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		choose:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	}
}

func (w *Workspaces) Init() tea.Cmd {
	svc := w.svc
	return tea.Batch(w.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, err := svc.Workspaces(ctx)
		return workspacesLoadedMsg{items: items, err: err}
	})
}

func (w *Workspaces) Title() string { return "Workspaces" }

func (w *Workspaces) CapturingInput() bool { return w.list.SettingFilter() }

func (w *Workspaces) ShortHelp() []key.Binding {
	return []key.Binding{w.choose, w.list.KeyMap.Filter, w.list.KeyMap.CursorUp, w.list.KeyMap.CursorDown}
}

func (w *Workspaces) FullHelp() [][]key.Binding { return [][]key.Binding{w.ShortHelp()} }

func (w *Workspaces) selectWorkspace(ws api.Workspace) tea.Cmd {
	w.svc.SetWorkspace(ws)
	if w.onSelect != nil {
		w.onSelect(ws)
	}
	return func() tea.Msg { return ui.WorkspaceSelectedMsg{Workspace: ws} }
}

func (w *Workspaces) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.list.SetSize(msg.Width, msg.Height)
		return w, nil
	case workspacesLoadedMsg:
		w.loaded, w.err = true, msg.err
		if msg.err != nil {
			return w, nil
		}
		items := make([]list.Item, len(msg.items))
		for i, ws := range msg.items {
			items[i] = workspaceItem(ws)
			if ws.ID == w.preferID {
				w.preferID = ""
				return w, w.selectWorkspace(ws)
			}
		}
		if len(msg.items) == 1 {
			return w, w.selectWorkspace(msg.items[0])
		}
		return w, w.list.SetItems(items)
	case spinner.TickMsg:
		if w.loaded {
			return w, nil
		}
		var cmd tea.Cmd
		w.spinner, cmd = w.spinner.Update(msg)
		return w, cmd
	case tea.KeyPressMsg:
		if !w.list.SettingFilter() && key.Matches(msg, w.choose) {
			if it, ok := w.list.SelectedItem().(workspaceItem); ok {
				return w, w.selectWorkspace(api.Workspace(it))
			}
			return w, nil
		}
	}
	var cmd tea.Cmd
	w.list, cmd = w.list.Update(msg)
	return w, cmd
}

func (w *Workspaces) View(width, height int) string {
	switch {
	case w.err != nil:
		return theme.Error.Render("Failed to load workspaces: " + w.err.Error())
	case !w.loaded:
		return w.spinner.View() + " Loading workspaces…"
	case len(w.list.Items()) == 0:
		return theme.Muted.Render("You don't have access to any workspaces.")
	}
	return w.list.View()
}
