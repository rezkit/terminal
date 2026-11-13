package screens

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/components"
)

// harness drives the App synchronously: commands are executed and their
// messages fed back. Commands that don't finish quickly (spinner ticks) are
// dropped, so the fake service must have zero latency.
type harness struct {
	t    *testing.T
	app  *ui.App
	quit bool
}

func newHarness(t *testing.T, svc api.Service) *harness {
	app := ui.NewApp(ui.Routes{
		WorkspacePicker: func() ui.Screen { return NewWorkspaces(svc, "", nil) },
		Home:            func(api.Workspace) ui.Screen { return NewHome(svc) },
	}, NewWorkspaces(svc, "", nil), "tester")
	h := &harness{t: t, app: app}
	h.run(app.Init())
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	return h
}

func (h *harness) send(msg tea.Msg) {
	_, cmd := h.app.Update(msg)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(30 * time.Millisecond):
		return // a timer (spinner tick); irrelevant to these tests
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		// tea.Sequence produces an unexported message type holding []tea.Cmd.
		if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
			for i := range v.Len() {
				h.run(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		h.send(msg)
	}
}

func (h *harness) key(s string) {
	var k tea.KeyPressMsg
	switch s {
	case "enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		k = tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		r := []rune(s)[0]
		k = tea.KeyPressMsg{Code: r, Text: s}
	}
	h.send(k)
}

func (h *harness) view() string { return h.app.View().Content }

func (h *harness) mustContain(subs ...string) {
	h.t.Helper()
	v := stripANSI(h.view())
	for _, s := range subs {
		if !strings.Contains(v, s) {
			h.t.Fatalf("view missing %q:\n%s", s, v)
		}
	}
}

func TestWorkspaceToBookingsFlow(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.mustContain("Select a workspace", "Alpine Adventures", "Coastal Cruises")

	h.key("enter")
	h.mustContain("Bookings", "Alpine Adventures", "Created ▼", "Page 1/8", "1–32 of 237") // 40 rows high: 32 per page

	// Paging forward and back.
	h.key("n")
	h.mustContain("Page 2/", "33–64 of 237")
	h.key("p")
	h.mustContain("Page 1/")

	// Cycling the sort column re-queries sorted by reference ascending.
	h.key("s")
	h.mustContain("Reference ▲", "RZ-10000")

	// Open a booking, then go back.
	h.key("enter")
	h.mustContain("Booking RZ-10000", "1 Overview", "NEEDS ATTENTION")
	h.mustNotContain("payment pending")
	h.key("esc")
	h.mustContain("Reference ▲")
}

func TestFilterModalCapturesKeysAndApplies(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")

	h.key("/")
	h.mustContain("Filter bookings", "Reference contains")
	h.key("q") // typed into the search box, must not quit
	if h.quit {
		t.Fatal("q inside the modal quit the app")
	}
	h.key("esc")
	if strings.Contains(h.view(), "Filter bookings") {
		t.Fatal("esc did not close the modal")
	}
	h.mustContain("No filter")
}

func TestQuitKey(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("q")
	if !h.quit {
		t.Fatal("q did not quit")
	}
}

func TestTableDropsStaleResponses(t *testing.T) {
	release := make(chan string, 2)
	fetch := func(sort api.Sort, page api.PageRequest) func(context.Context) (components.TablePage, error) {
		return func(ctx context.Context) (components.TablePage, error) {
			label := <-release
			return components.TablePage{Rows: []table.Row{{label}}, IDs: []string{label}, Info: api.PageInfo{Total: -1}}, nil
		}
	}
	tbl := components.NewTable("t", []components.Column{{Title: "Name"}}, fetch, api.Sort{})
	tbl.SetSize(40, 10)

	first := tbl.Reload()
	second := tbl.Reload()
	release <- "stale"
	release <- "fresh"
	// Deliver results in order: the first (stale) response must be ignored.
	for _, cmd := range []tea.Cmd{first, second} {
		for _, c := range cmd().(tea.BatchMsg) {
			done := make(chan tea.Msg, 1)
			go func() { done <- c() }()
			select {
			case m := <-done:
				tbl.Update(m)
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	v := tbl.View()
	if strings.Contains(v, "stale") || !strings.Contains(v, "fresh") {
		t.Fatalf("stale response not dropped:\n%s", v)
	}
}
