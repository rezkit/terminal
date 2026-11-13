package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui/components"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// Routes lets the app build screens without importing the screens package.
type Routes struct {
	// WorkspacePicker builds the workspace selection screen.
	WorkspacePicker func() Screen
	// Home builds the landing screen for a selected workspace.
	Home func(api.Workspace) Screen
}

// App is the root Bubble Tea model.
type App struct {
	routes    Routes
	stack     []Screen
	modal     Modal
	workspace api.Workspace
	user      string

	width, height int
	status        StatusMsg
	help          help.Model
	keys          globalKeys

	// API activity: requests in flight (loads can overlap) and the duration
	// of the last one to finish (zero until one has).
	inFlight    int
	lastRequest time.Duration
	spinner     spinner.Model
}

type globalKeys struct {
	Quit, Back, Workspace, Help key.Binding
}

func newGlobalKeys() globalKeys {
	return globalKeys{
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Workspace: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "workspace")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	}
}

// NewApp creates the root model starting at the given screen.
func NewApp(routes Routes, start Screen, user string) *App {
	h := help.New()
	h.Styles = help.DefaultDarkStyles()
	return &App{routes: routes, stack: []Screen{start}, user: user, help: h, keys: newGlobalKeys(),
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
}

func (a *App) top() Screen { return a.stack[len(a.stack)-1] }

func (a *App) Init() tea.Cmd { return a.top().Init() }

// bodySize is the area available to screens (minus header, status and help).
func (a *App) bodySize() (int, int) {
	return a.width, max(a.height-a.chromeHeight(), 1)
}

func (a *App) chromeHeight() int {
	return lipgloss.Height(a.headerView()) + lipgloss.Height(a.footerView())
}

func (a *App) sizeMsg() tea.WindowSizeMsg {
	w, h := a.bodySize()
	return tea.WindowSizeMsg{Width: w, Height: h}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.help.SetWidth(msg.Width)
		return a, a.updateTop(a.sizeMsg())

	case PushMsg:
		a.stack = append(a.stack, msg.Screen)
		return a, a.initTop()

	case PopMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
			return a, a.updateTop(a.sizeMsg())
		}
		return a, nil

	case ResetMsg:
		a.stack = []Screen{msg.Screen}
		a.modal = nil
		return a, a.initTop()

	case OpenModalMsg:
		a.modal = msg.Modal
		return a, msg.Modal.Init()

	case CloseModalMsg:
		a.modal = nil
		return a, nil

	case StatusMsg:
		a.status = msg
		return a, nil

	case RequestStartedMsg:
		a.inFlight++
		if a.inFlight == 1 {
			return a, a.spinner.Tick
		}
		return a, nil

	case RequestFinishedMsg:
		a.inFlight = max(a.inFlight-1, 0)
		a.lastRequest = msg.Elapsed
		return a, nil

	case WorkspaceSelectedMsg:
		a.workspace = msg.Workspace
		a.status = StatusMsg{Text: "Workspace: " + msg.Workspace.Name}
		if a.routes.Home != nil {
			return a, Reset(a.routes.Home(msg.Workspace))
		}
		return a, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		if a.modal != nil {
			var cmd tea.Cmd
			a.modal, cmd = a.modal.Update(msg)
			return a, cmd
		}
		if !a.top().CapturingInput() {
			if cmd, handled := a.handleGlobalKey(msg); handled {
				return a, cmd
			}
		}
		return a, a.updateTop(msg)
	}

	// Everything else (async results, spinner ticks, ...) goes to the modal
	// and to every screen on the stack, so background loads for screens
	// below the top still land. Screens drop responses they did not ask for.
	var cmds []tea.Cmd
	if tick, ok := msg.(spinner.TickMsg); ok && a.inFlight > 0 {
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(tick)
		cmds = append(cmds, cmd)
	}
	if a.modal != nil {
		var cmd tea.Cmd
		a.modal, cmd = a.modal.Update(msg)
		cmds = append(cmds, cmd)
	}
	for i := range a.stack {
		var cmd tea.Cmd
		a.stack[i], cmd = a.stack[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

func (a *App) handleGlobalKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		return tea.Quit, true
	case key.Matches(msg, a.keys.Back):
		if len(a.stack) > 1 {
			return Pop, true
		}
	case key.Matches(msg, a.keys.Help):
		a.help.ShowAll = !a.help.ShowAll
		return a.updateTop(a.sizeMsg()), true
	case key.Matches(msg, a.keys.Workspace):
		if a.routes.WorkspacePicker != nil && a.workspace.ID != "" {
			return Push(a.routes.WorkspacePicker()), true
		}
	}
	return nil, false
}

// initTop sizes a newly shown screen before initialising it, so its first
// load already knows how many rows fit.
func (a *App) initTop() tea.Cmd {
	sizeCmd := a.updateTop(a.sizeMsg())
	return tea.Batch(sizeCmd, a.top().Init())
}

func (a *App) updateTop(msg tea.Msg) tea.Cmd {
	i := len(a.stack) - 1
	var cmd tea.Cmd
	a.stack[i], cmd = a.stack[i].Update(msg)
	return cmd
}

func (a *App) headerView() string {
	crumbs := make([]string, 0, len(a.stack))
	for _, s := range a.stack {
		crumbs = append(crumbs, s.Title())
	}
	left := theme.Header.Render("RezKit") + theme.Crumb.Render(strings.Join(crumbs, " › "))
	right := ""
	if a.workspace.Name != "" {
		right = theme.Muted.Render(a.workspace.Name)
	}
	if a.user != "" {
		right += theme.Muted.Render("  " + a.user)
	}
	gap := max(a.width-lipgloss.Width(left)-lipgloss.Width(right)-1, 1)
	return left + strings.Repeat(" ", gap) + right
}

// helpKeys merges the screen's bindings with the global ones.
type helpKeys struct {
	screen  help.KeyMap
	global  globalKeys
	canBack bool
}

func (h helpKeys) globals() []key.Binding {
	if h.canBack {
		return []key.Binding{h.global.Back, h.global.Help, h.global.Quit}
	}
	return []key.Binding{h.global.Help, h.global.Quit}
}

func (h helpKeys) ShortHelp() []key.Binding {
	return append(h.screen.ShortHelp(), h.globals()...)
}

func (h helpKeys) FullHelp() [][]key.Binding {
	return append(h.screen.FullHelp(), append([]key.Binding{h.global.Workspace}, h.globals()...))
}

func (a *App) footerView() string {
	status := ""
	if a.status.Text != "" {
		st := theme.Muted
		if a.status.Error {
			st = theme.Error
		}
		status = st.Render(a.status.Text)
	}
	status = components.Spread(max(a.width, 1), status, a.activityView())
	return lipgloss.JoinVertical(lipgloss.Left, status, a.help.View(helpKeys{a.top(), a.keys, len(a.stack) > 1}))
}

// activityView is the right side of the status bar: a spinner while API
// requests are in flight, otherwise how long the last one took.
func (a *App) activityView() string {
	switch {
	case a.inFlight > 0:
		return a.spinner.View() + theme.Muted.Render(" Loading…")
	case a.lastRequest > 0:
		return theme.Muted.Render("Last request " + formatElapsed(a.lastRequest))
	}
	return ""
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func (a *App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "RezKit"
	if a.width == 0 {
		return v
	}
	w, h := a.bodySize()
	body := lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(a.top().View(w, h))
	screen := lipgloss.JoinVertical(lipgloss.Left, a.headerView(), body, a.footerView())

	if a.modal == nil {
		v.SetContent(screen)
		return v
	}
	box := a.modalView()
	x := max((a.width-lipgloss.Width(box))/2, 0)
	y := max((a.height-lipgloss.Height(box))/2, 0)
	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	)
	v.SetContent(comp.Render())
	return v
}

func (a *App) modalView() string {
	frameW, frameH := theme.ModalBox.GetFrameSize()
	maxW := max(min(a.width-4, 80)-frameW, 10)
	maxH := max(a.height-4-frameH-2, 3)
	content := theme.ModalTitle.Render(a.modal.Title()) + "\n" + a.modal.View(maxW, maxH)
	return theme.ModalBox.Render(content)
}
