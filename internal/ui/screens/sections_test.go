package screens

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/tui/internal/api"
)

func (h *harness) mustFit(width int) {
	h.t.Helper()
	for _, l := range strings.Split(h.view(), "\n") {
		if lipgloss.Width(l) > width {
			h.t.Fatalf("line wider than the terminal: %q", l)
		}
	}
}

func (h *harness) mustNotContain(subs ...string) {
	h.t.Helper()
	v := stripANSI(h.view())
	for _, s := range subs {
		if strings.Contains(v, s) {
			h.t.Fatalf("view should not contain %q:\n%s", s, v)
		}
	}
}

func TestSectionsKeepTheirState(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")
	h.mustContain("1 Bookings", "2 Customers", "3 Transactions", "Created ▼")

	h.key("2")
	h.mustContain("RezKit  Customers", "Name ▲", "Page 1/")
	h.key("n")
	h.mustContain("Page 2/")

	h.send(tea.KeyPressMsg{Code: tea.KeyTab})
	h.mustContain("RezKit  Transactions", "Amount")
	h.key("1")
	h.mustContain("Created ▼", "Page 1/")
	h.send(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	h.send(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	h.mustContain("Name ▲", "Page 2/") // customers kept its page
	h.mustFit(140)
}

func TestCustomerToBookingAndBack(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")
	h.key("2")
	h.key("/")
	h.mustContain("Filter customers", "Family name contains")
	h.key("esc")
	h.send(customerFilterAppliedMsg{filter: api.CustomerFilter{Family: "lovelace"}})
	h.mustContain(`family name contains "lovelace"`, "Ada Lovelace", "1–1 of 1")

	h.key("enter")
	h.mustContain("Customers › Dr Ada Lovelace", "1 Overview", "2 Bookings (", "CONTACT", "Customer since", "TRIPS")
	h.mustFit(140)

	h.key("2")
	h.mustContain("Reference", "Outstanding", "enter opens RZ-")
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	ref := selectedBookingRef(t, h.view())
	h.key("enter")
	h.mustContain("Dr Ada Lovelace › Booking "+ref, "1 Overview")
	h.key("esc")
	h.mustContain("2 Bookings (")
	if got := selectedBookingRef(t, h.view()); got != ref {
		t.Errorf("selection after returning = %s, want %s", got, ref)
	}
	h.key("esc")
	h.mustContain("Ada Lovelace", "Name ▲")
	h.mustNotContain("CONTACT")
}

func TestCustomerBookingsScrollAndWithheldNotes(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.send(tea.WindowSizeMsg{Width: 100, Height: 20})
	h.key("enter")
	h.key("2")
	h.send(customerFilterAppliedMsg{filter: api.CustomerFilter{Family: "liskov"}})
	h.key("enter")
	h.mustContain("3 Notes (–)")
	h.key("3")
	h.mustContain("Notes are not available (access denied)")

	h.key("2")
	h.mustContain("more · enter opens")
	for range 60 { // past the end: the cursor stops on the last booking
		h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	h.mustNotContain("more · enter opens")
	last := selectedBookingRef(t, h.view())
	h.key("enter")
	h.mustContain("Booking " + last)
	h.mustFit(100)
}

func TestTransactionOpensBookingPayments(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")
	h.key("3")
	h.mustContain("Created", "Booking", "Method", "Amount", "Page 1/")
	h.mustNotContain("▲", "▼")
	h.key("s")
	h.mustContain("can't be sorted")

	h.key("/")
	h.mustContain("Filter transactions", "Booking reference contains", "Currency")
	h.key("esc")
	h.send(transactionFilterAppliedMsg{filter: api.TransactionFilter{BookingRef: "RZ-10000"}})
	h.mustContain(`booking contains "RZ-10000"`, "RZ-10000")

	h.key("enter")
	h.mustContain("Transactions › Booking RZ-10000", "4 Payments", "Deposit", "Outstanding")
	h.key("esc")
	h.mustContain(`booking contains "RZ-10000"`)
	h.mustFit(140)
}

// selectedBookingRef reads the selected booking from the customer bookings
// tab's footer ("enter opens RZ-…").
func selectedBookingRef(t *testing.T, view string) string {
	t.Helper()
	_, after, ok := strings.Cut(stripANSI(view), "enter opens ")
	if !ok {
		t.Fatalf("no selected booking in view:\n%s", view)
	}
	return strings.Fields(after)[0]
}

func TestCustomerWithheldBookingsIsNotAllClear(t *testing.T) {
	h := newHarness(t, api.NewFake())
	h.key("enter")
	h.key("2")
	h.send(customerFilterAppliedMsg{filter: api.CustomerFilter{Family: "thompson"}})
	h.key("enter")
	h.mustContain("Ken Thompson", "2 Bookings (–)", "Not available (bookings withheld)",
		"Bookings are not available (access denied)")
	h.mustNotContain("Nothing outstanding", "0 bookings")
	h.key("2")
	h.mustContain("Bookings are not available (access denied)")
	h.key("enter") // nothing to open
	h.mustContain("2 Bookings (–)")
}
