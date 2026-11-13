// Package screens contains the application's screens.
package screens

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// DetailLoader fetches one resource by ID.
type DetailLoader func(ctx context.Context, id string) (api.Detail, error)

// Detail shows a single resource as labelled sections in a scrollable view.
// It is resource-agnostic: any api.Detail renders the same way.
type Detail struct {
	id, kind string
	load     DetailLoader

	detail  *api.Detail
	err     error
	vp      viewport.Model
	spinner spinner.Model
	reload  key.Binding
}

type detailLoadedMsg struct {
	id     string
	detail api.Detail
	err    error
}

// NewDetail creates a detail screen for the resource kind ("Booking", ...).
func NewDetail(id, kind string, load DetailLoader) *Detail {
	return &Detail{
		id: id, kind: kind, load: load,
		vp:      viewport.New(),
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		reload:  key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload")),
	}
}

func (d *Detail) fetch() tea.Cmd {
	id, load := d.id, d.load
	return tea.Batch(d.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		det, err := load(ctx, id)
		return detailLoadedMsg{id: id, detail: det, err: err}
	})
}

func (d *Detail) Init() tea.Cmd { return d.fetch() }

func (d *Detail) Title() string {
	if d.detail != nil {
		return d.detail.Title
	}
	return d.kind
}

func (d *Detail) CapturingInput() bool { return false }

func (d *Detail) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
		d.reload,
	}
}

func (d *Detail) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }

func (d *Detail) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.vp.SetWidth(msg.Width)
		d.vp.SetHeight(msg.Height)
		d.render()
		return d, nil
	case detailLoadedMsg:
		if msg.id != d.id {
			return d, nil
		}
		d.err = msg.err
		if msg.err == nil {
			d.detail = &msg.detail
		}
		d.render()
		return d, nil
	case spinner.TickMsg:
		if d.detail != nil || d.err != nil {
			return d, nil
		}
		var cmd tea.Cmd
		d.spinner, cmd = d.spinner.Update(msg)
		return d, cmd
	case tea.KeyPressMsg:
		if key.Matches(msg, d.reload) {
			d.err = nil
			return d, d.fetch()
		}
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(msg)
		return d, cmd
	}
	return d, nil
}

func (d *Detail) render() {
	if d.detail == nil {
		return
	}
	var b strings.Builder
	b.WriteString(theme.Bold.Render(d.detail.Title))
	b.WriteString("\n")
	for _, s := range d.detail.Sections {
		b.WriteString(theme.Section.Render(s.Title))
		b.WriteString("\n")
		for _, f := range s.Fields {
			b.WriteString(theme.Label.Render(f.Label))
			b.WriteString(f.Value)
			b.WriteString("\n")
		}
	}
	d.vp.SetContent(b.String())
}

func (d *Detail) View(width, height int) string {
	switch {
	case d.err != nil:
		return theme.Error.Render("Failed to load " + strings.ToLower(d.kind) + ": " + d.err.Error())
	case d.detail == nil:
		return d.spinner.View() + " Loading…"
	}
	return d.vp.View()
}
