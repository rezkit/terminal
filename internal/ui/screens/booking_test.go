package screens

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/terminal/internal/api"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func money(n int64) *api.Money { return &api.Money{Minor: n, Currency: "GBP"} }

func TestItineraryLayouts(t *testing.T) {
	cases := map[string][]api.Reservation{
		"single day": {{Reference: "R1", Product: "Day trip", Status: "CONFIRMED", Start: d(2026, 7, 11), Price: money(100)}},
		"multi-month": {
			{Reference: "R1", Product: "Flight out", Status: "CONFIRMED", Start: d(2026, 5, 28)},
			{Reference: "R2", Product: "Long stay", Status: "PENDING", Start: d(2026, 5, 28), End: d(2026, 8, 30)},
			{Reference: "R3", Product: "Flight back", Status: "CONFIRMED", Start: d(2026, 8, 30)},
		},
		"undated and inverted": {
			{Reference: "R1", Product: "Open ticket", Status: "PENDING"},
			{Reference: "R2", Product: "Bad dates", Status: "ERROR", Start: d(2026, 7, 11), End: d(2026, 7, 9)},
		},
		"no dates at all": {{Reference: "R1", Product: "Voucher", Status: "CONFIRMED"}},
	}
	for name, rs := range cases {
		for _, w := range []int{80, 120, 200} {
			it, ok := newItinerary(rs, w)
			if !ok {
				t.Fatalf("%s at %d: reported too narrow", name, w)
			}
			lines := []string{it.axis()}
			for i, r := range rs {
				lines = append(lines, it.row(r, i == 0, false))
			}
			for _, l := range lines {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("%s at %d: line is %d wide: %q", name, w, lw, l)
				}
			}
			if name == "no dates at all" && !strings.Contains(lines[1], "no dates") {
				t.Errorf("undated row: %q", lines[1])
			}
		}
	}
	if _, ok := newItinerary(nil, 40); ok {
		t.Error("40 columns should be too narrow")
	}
}

func TestItineraryStayBarCoversItsDays(t *testing.T) {
	rs := []api.Reservation{
		{Reference: "A", Product: "Hotel", Status: "CONFIRMED", Start: d(2026, 7, 11), End: d(2026, 7, 15)},
		{Reference: "B", Product: "Flight", Status: "CONFIRMED", Start: d(2026, 7, 19)},
	}
	it, _ := newItinerary(rs, 120)
	if it.days != 9 {
		t.Fatalf("days = %d, want 9", it.days)
	}
	bar := []rune(stripANSI(it.track(rs[0])))
	first, last := slices.Index(bar, '█'), -1
	for i, r := range bar {
		if r == '█' {
			last = i
		}
	}
	if first != it.x(0.5) || last != it.x(4.5)-1 { // check-in afternoon to check-out morning
		t.Errorf("bar spans %d..%d in %q", first, last, bar)
	}
	if !strings.Contains(stripANSI(it.track(rs[1])), "◆") {
		t.Error("single-day item has no marker")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestBookingScreenTabsAndViews(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter") // pick the first workspace
	h.key("enter") // open the first booking
	h.mustContain("1 Overview", "NEEDS ATTENTION", "CUSTOMER")
	h.key("3")
	h.mustContain("Invoice", "Balance due", "Total ")
	first := selectedRef(t, h.view())
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	picked := selectedRef(t, h.view())
	if picked == first {
		t.Fatalf("down did not move the selection (still %s)", picked)
	}
	h.key("v")
	h.mustContain("Itinerary", "◆")
	if got := selectedRef(t, h.view()); got != picked {
		t.Errorf("itinerary selection = %s, want %s", got, picked)
	}
	h.key("e") // expand every transport reservation
	h.mustContain("▾ ", "└ ", "LHR → ")
	h.key("e")
	if strings.Contains(h.view(), "└ ") {
		t.Error("e did not collapse the legs")
	}
	h.key("v")
	if got := selectedRef(t, h.view()); got != picked {
		t.Errorf("invoice selection after toggling back = %s, want %s", got, picked)
	}
	h.key("4")
	h.mustContain("Deposit")
	h.key("esc")
	h.mustContain("Page 1/")
	for _, l := range strings.Split(h.view(), "\n") {
		if lipgloss.Width(l) > 140 {
			t.Fatalf("line wider than the terminal: %q", l)
		}
	}
}

// selectedRef identifies the selected reservation by the product name that
// leads the strip under the list (the line above "Dates … Travellers").
func selectedRef(t *testing.T, view string) string {
	t.Helper()
	lines := strings.Split(stripANSI(view), "\n")
	for i, l := range lines {
		if i > 0 && strings.Contains(l, "Balance due") && strings.Contains(l, "Travellers") {
			name, _, _ := strings.Cut(strings.TrimSpace(lines[i-1]), "  ")
			return name
		}
	}
	t.Fatalf("no selected-reservation strip in view:\n%s", view)
	return ""
}

func TestItineraryLegs(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	london, _ := time.LoadLocation("Europe/London")
	dep := time.Date(2026, 7, 11, 21, 30, 0, 0, london)
	arr := dep.Add(14 * time.Hour).In(tokyo) // lands 12 Jul, 19:30 Tokyo time
	rs := []api.Reservation{
		{ID: "f", Reference: "R1", Product: "Flight", Status: "CONFIRMED", Start: d(2026, 7, 11),
			Legs: []api.Leg{{Name: "7", CarrierCode: "BA", From: api.Stop{Code: "LHR"}, To: api.Stop{Code: "KIX"},
				Depart: dep, Arrive: arr, Passengers: []api.LegPassenger{{PassengerID: "p1", Class: "Economy", Seat: "40A"}}}}},
	}
	for _, w := range []int{80, 120} {
		it, ok := newItinerary(rs, w)
		if !ok {
			t.Fatal("too narrow")
		}
		if it.days != 2 {
			t.Errorf("overnight leg should extend the axis to 2 days, got %d", it.days)
		}
		if row := stripANSI(it.row(rs[0], false, true)); !strings.HasPrefix(row, "▾ ") {
			t.Errorf("expanded row = %q", row)
		}
		lines := it.legRows(rs[0], func(string) string { return "Ada" })
		if len(lines) != 2 {
			t.Fatalf("leg lines = %q", lines)
		}
		leg := stripANSI(lines[0])
		for _, want := range []string{"LHR → KIX", "BA 7", "21:30–19:30", "14h 00m +1d", "━"} {
			if !strings.Contains(leg, want) {
				t.Errorf("leg line missing %q: %q", want, leg)
			}
		}
		if !strings.Contains(lines[1], "Economy · Ada 40A") {
			t.Errorf("seat line = %q", lines[1])
		}
		for _, l := range append(lines, it.axis()) {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line is %d wide", w, lipgloss.Width(l))
			}
		}
	}
}

func TestStatusText(t *testing.T) {
	for _, s := range []string{"PENDING", "CONFIRMED", "ERROR", "CANCELLED", "ON_HOLD"} {
		if got, want := stripANSI(statusText(s, false)), api.StatusLabel(s); got != want {
			t.Errorf("%s: %q, want %q", s, got, want)
		}
		if got := statusText(s, true); got != api.StatusLabel(s) {
			t.Errorf("%s: plain = %q", s, got)
		}
	}
	if c := statusText("CANCELLED", false); !strings.Contains(c, "\x1b[9") && !strings.Contains(c, ";9m") && !strings.Contains(c, ";9;") {
		t.Errorf("cancelled is not struck through: %q", c)
	}
	if c := statusText("PENDING", false); !strings.Contains(c, "237;237;237") {
		t.Errorf("pending should use the normal text colour: %q", c)
	}
	if c := statusText("CONFIRMED", false); !strings.Contains(c, "48;164;108") {
		t.Errorf("confirmed should be green: %q", c)
	}
}

// The list tables colour statuses, except on the highlighted row.
func TestBookingsListColoursStatus(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")
	view := h.view()
	if !strings.Contains(view, statusText("CONFIRMED", false)) {
		t.Fatal("no coloured Confirmed status in the bookings list")
	}
	h.mustNotContain("CONFIRMED", "ON_HOLD") // raw enum values never leak
	first := func() string {
		for _, l := range strings.Split(h.view(), "\n") {
			if strings.Contains(l, "RZ-") {
				return l
			}
		}
		return ""
	}
	if row := first(); strings.Count(row, "\x1b[") > 2 { // highlight only, no nested colours
		t.Errorf("highlighted row has nested styling: %q", row)
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	if row := first(); !strings.Contains(stripANSI(row), "RZ-") || strings.Count(row, "\x1b[") <= 2 && !strings.Contains(row, "\x1b[38;2;") {
		t.Errorf("first row was not restyled after moving off it: %q", row)
	}
}
