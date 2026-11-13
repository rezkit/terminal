package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/components"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// BookingLoader fetches one booking.
type BookingLoader func(ctx context.Context, id string) (api.BookingDetail, error)

type bookingTab int

const (
	tabOverview bookingTab = iota
	tabPassengers
	tabReservations
	tabPayments
	tabNotes
	tabCount
)

var tabTitles = [tabCount]string{"Overview", "Passengers", "Reservations", "Payments", "Notes"}

// Booking shows one booking: a fixed header card, a tab bar, and the active
// tab. The Reservations tab switches between an invoice table and an
// itinerary timeline that share one selection.
type Booking struct {
	id   string
	load BookingLoader

	detail  *api.BookingDetail
	err     error
	spinner spinner.Model
	vp      viewport.Model
	w, h    int

	tab       bookingTab
	itinerary bool            // reservations view: false = invoice
	expanded  map[string]bool // itinerary rows showing their legs, by reservation ID
	cursor    int             // selected reservation
	offset    int             // first visible reservation row
	keys      bookingKeys
}

type bookingKeys struct {
	Next, Prev, Jump, Up, Down, View, Expand, ExpandAll, Reload key.Binding
}

type bookingLoadedMsg struct {
	id     string
	detail api.BookingDetail
	err    error
}

// NewBooking creates the booking screen.
func NewBooking(id string, load BookingLoader) *Booking {
	return &Booking{
		id: id, load: load, expanded: map[string]bool{},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		vp:      viewport.New(),
		keys: bookingKeys{
			Next:      key.NewBinding(key.WithKeys("tab", "l"), key.WithHelp("tab/1-5", "switch tab")),
			Prev:      key.NewBinding(key.WithKeys("shift+tab", "h")),
			Jump:      key.NewBinding(key.WithKeys("1", "2", "3", "4", "5"), key.WithHelp("1-5", "jump to tab")),
			Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "select")),
			Down:      key.NewBinding(key.WithKeys("down", "j")),
			View:      key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "invoice/itinerary")),
			Expand:    key.NewBinding(key.WithKeys("enter", "space", "right"), key.WithHelp("enter", "expand")),
			ExpandAll: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "expand all")),
			Reload:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload")),
		},
	}
}

func (b *Booking) fetch() tea.Cmd {
	id, load := b.id, b.load
	return tea.Batch(b.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := load(ctx, id)
		return bookingLoadedMsg{id: id, detail: d, err: err}
	})
}

func (b *Booking) Init() tea.Cmd { return b.fetch() }

func (b *Booking) Title() string {
	if b.detail != nil {
		return "Booking " + b.detail.Reference
	}
	return "Booking"
}

func (b *Booking) CapturingInput() bool { return false }

func (b *Booking) ShortHelp() []key.Binding {
	ks := []key.Binding{b.keys.Next}
	if b.tab == tabReservations {
		ks = append(ks, b.keys.Up, b.keys.View)
		if b.itinerary {
			ks = append(ks, b.keys.Expand)
		}
	} else {
		ks = append(ks, key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓", "scroll")))
	}
	return append(ks, b.keys.Reload)
}

func (b *Booking) FullHelp() [][]key.Binding {
	if b.tab == tabReservations && b.itinerary {
		return [][]key.Binding{b.ShortHelp(), {b.keys.ExpandAll}}
	}
	return [][]key.Binding{b.ShortHelp()}
}

func (b *Booking) selected() (api.Reservation, bool) {
	if b.detail == nil || b.cursor >= len(b.detail.Reservations) {
		return api.Reservation{}, false
	}
	return b.detail.Reservations[b.cursor], true
}

// toggleAll expands every reservation with legs, or collapses all if any
// is already expanded.
func (b *Booking) toggleAll() {
	if b.detail == nil {
		return
	}
	any := false
	for _, open := range b.expanded {
		any = any || open
	}
	clear(b.expanded)
	if !any {
		for _, r := range b.detail.Reservations {
			if len(r.Legs) > 0 {
				b.expanded[r.ID] = true
			}
		}
	}
}

func (b *Booking) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.w, b.h = msg.Width, msg.Height
		b.refresh()
	case bookingLoadedMsg:
		if msg.id != b.id {
			return b, nil
		}
		b.err = msg.err
		if msg.err == nil {
			b.detail = &msg.detail
			b.cursor = min(b.cursor, max(len(msg.detail.Reservations)-1, 0))
		}
		b.refresh()
	case spinner.TickMsg:
		if b.detail != nil || b.err != nil {
			return b, nil
		}
		var cmd tea.Cmd
		b.spinner, cmd = b.spinner.Update(msg)
		return b, cmd
	case tea.KeyPressMsg:
		return b, b.handleKey(msg)
	}
	return b, nil
}

func (b *Booking) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, b.keys.Reload):
		b.err = nil
		return b.fetch()
	case key.Matches(msg, b.keys.Next):
		b.setTab((b.tab + 1) % tabCount)
	case key.Matches(msg, b.keys.Prev):
		b.setTab((b.tab + tabCount - 1) % tabCount)
	case key.Matches(msg, b.keys.Jump):
		b.setTab(bookingTab(msg.String()[0] - '1'))
	case b.tab == tabReservations && key.Matches(msg, b.keys.View):
		b.itinerary = !b.itinerary
	case b.tab == tabReservations && b.itinerary && key.Matches(msg, b.keys.Expand):
		if r, ok := b.selected(); ok && len(r.Legs) > 0 {
			b.expanded[r.ID] = !b.expanded[r.ID]
		}
	case b.tab == tabReservations && b.itinerary && key.Matches(msg, b.keys.ExpandAll):
		b.toggleAll()
	case b.tab == tabReservations && key.Matches(msg, b.keys.Up):
		b.cursor = max(b.cursor-1, 0)
	case b.tab == tabReservations && key.Matches(msg, b.keys.Down):
		if b.detail != nil {
			b.cursor = min(b.cursor+1, max(len(b.detail.Reservations)-1, 0))
		}
	case b.tab != tabReservations:
		var cmd tea.Cmd
		b.vp, cmd = b.vp.Update(msg)
		return cmd
	}
	return nil
}

func (b *Booking) setTab(t bookingTab) {
	if t == b.tab {
		return
	}
	b.tab = t
	b.vp.GotoTop()
	b.refresh()
}

// --- layout -------------------------------------------------------------------

func (b *Booking) compact() bool { return b.h < 16 }

// chrome is the fixed part above the tab content.
func (b *Booking) chrome() string {
	card := b.headerCard()
	if b.compact() {
		card = strings.SplitN(card, "\n", 2)[0]
	}
	return card + "\n" + b.tabBar()
}

func (b *Booking) bodySize() (int, int) {
	return b.w, max(b.h-lipgloss.Height(b.chrome()), 1)
}

// refresh re-renders scrollable tab content into the viewport.
func (b *Booking) refresh() {
	if b.detail == nil || b.w == 0 {
		return
	}
	w, h := b.bodySize()
	b.vp.SetWidth(w)
	b.vp.SetHeight(h)
	var content string
	switch b.tab {
	case tabOverview:
		content = b.overview(w)
	case tabPassengers:
		content = b.passengers(w)
	case tabPayments:
		content = b.payments(w)
	case tabNotes:
		content = notesView(b.detail.Notes, w)
	}
	b.vp.SetContent(components.ClampLines(content, w, -1))
}

func (b *Booking) View(width, height int) string {
	switch {
	case b.detail == nil && b.err != nil:
		return theme.Error.Render("Failed to load booking: " + b.err.Error())
	case b.detail == nil:
		return b.spinner.View() + " Loading…"
	}
	var body string
	if b.tab == tabReservations {
		_, h := b.bodySize()
		body = b.reservations(b.w, h)
	} else {
		body = b.vp.View()
	}
	out := b.chrome() + "\n" + body
	if b.err != nil { // a failed reload keeps the old data on screen
		out = theme.Error.Render("Reload failed: "+b.err.Error()) + "\n" + out
	}
	return components.ClampLines(out, width, height)
}

func (b *Booking) headerCard() string {
	d, w := b.detail, b.w
	title := theme.Bold.Render(d.Reference) + "  " + badge(d.Status)
	if d.Description != "" {
		title += "  " + d.Description
	}
	var facts []string
	if !d.Start.IsZero() {
		facts = append(facts, dateSpan(d.Start, d.End, "Mon 2 Jan 2006"))
		if !d.End.IsZero() {
			facts = append(facts, plural(dayDiff(civilDay(d.Start), civilDay(d.End)), "night", "nights"))
		}
	}
	facts = append(facts, plural(travelling(d), "traveller", "travellers"))

	who := "No customer"
	if c := d.Customer; c != nil {
		who = strings.Join(nonEmpty(c.Name(), c.Email, c.Telephone), " · ")
	}
	money := theme.Muted.Render("Total ") + api.FormatMoney(d.Total) +
		theme.Muted.Render("   Paid ") + api.FormatMoney(d.Paid) +
		theme.Muted.Render("   Outstanding ") + outstanding(d.Outstanding)
	tags := ""
	if len(d.Tags) > 0 {
		tags = theme.Muted.Render("Tags ") + strings.Join(d.Tags, ", ")
	}
	return strings.Join([]string{
		components.Spread(w, title, theme.Muted.Render(strings.Join(facts, " · "))),
		components.Spread(w, theme.Muted.Render(who), money),
		components.Spread(w, tags, paidBar(d.Paid, d.Total, 30)),
	}, "\n")
}

func (b *Booking) tabBar() string {
	d := b.detail
	counts := []string{"", count(len(d.Passengers), false), count(len(d.Reservations), false),
		count(len(d.Transactions), d.TransactionsDenied()), count(len(d.Notes), d.NotesDenied())}
	return tabBar(tabTitles[:], counts, int(b.tab), b.w)
}

func count(n int, denied bool) string {
	if denied {
		return " (–)"
	}
	return fmt.Sprintf(" (%d)", n)
}

// --- tabs -----------------------------------------------------------------------

func (b *Booking) overview(w int) string {
	d := b.detail
	left := heading("Booking", -1) + "\n" + kv(13,
		"Reference", d.Reference,
		"Status", badge(d.Status),
		"Description", orDash(d.Description),
		"Dates", dateSpan(d.Start, d.End, "Mon 2 Jan 2006"),
		"Currency", d.Currency,
		"Created", stamp(d.Created),
		"Updated", stamp(d.Updated))
	if c := d.Customer; c != nil {
		left += "\n\n" + heading("Customer", -1) + "\n" + kv(13,
			"Name", c.Name(), "Email", orDash(c.Email), "Telephone", orDash(c.Telephone),
			"Address", orDash(strings.Join(c.Address, ", ")))
	}

	attn := b.attention()
	right := heading("Needs attention", len(attn)) + "\n"
	if len(attn) == 0 {
		right += theme.Ok.Render("✓ ") + "Nothing outstanding"
	} else {
		right += strings.Join(attn, "\n")
	}
	right += "\n\n" + heading("Notes", -1) + "\n" + notesPreview(d.Notes, "5")
	return columns(left, right, w)
}

// attention lists what an agent should look at: unconfirmed reservations,
// cancelled passengers. Pending payments are not a concern: they are
// initialised but not yet resolved.
func (b *Booking) attention() []string {
	d := b.detail
	var out []string
	for _, r := range d.Reservations {
		if r.Status == "CONFIRMED" || r.Status == "CANCELLED" || r.Status == "ARCHIVED" {
			continue
		}
		msg := api.StatusLabel(r.Status)
		if r.StatusMessage != "" {
			msg += ": " + r.StatusMessage
		}
		out = append(out, theme.Warn.Render("▲ ")+theme.Bold.Render(orDash(r.Product))+theme.Muted.Render(" · ")+msg)
	}
	for _, p := range d.Passengers {
		if !p.Cancelled.IsZero() {
			out = append(out, theme.Muted.Render("● ")+p.Name()+theme.Muted.Render(" cancelled "+day(p.Cancelled)))
		}
	}
	return out
}

func (b *Booking) passengers(w int) string {
	d := b.detail
	if len(d.Passengers) == 0 {
		return theme.Muted.Render("No passengers")
	}
	g := components.Grid{Headers: []string{"Name", "Age", "Date of birth", "Email", "Reservations", "Status"},
		Right: map[int]bool{1: true, 4: true}, Dim: map[int]bool{}, Selected: -1, Flex: 3}
	for i, p := range d.Passengers {
		status := "Travelling"
		if !p.Cancelled.IsZero() {
			status = "Cancelled " + day(p.Cancelled)
			g.Dim[i] = true
		}
		age := "—"
		if a := p.AgeAt(d.Start); a >= 0 {
			age = fmt.Sprint(a)
		}
		n := 0
		for _, r := range d.Reservations {
			for _, id := range r.PassengerIDs {
				if id == p.ID {
					n++
				}
			}
		}
		g.Rows = append(g.Rows, []string{p.Name(), age, day(p.DateOfBirth), orDash(p.Email), fmt.Sprint(n), status})
	}
	return g.Render(w)
}

func (b *Booking) payments(w int) string {
	d := b.detail
	if d.TransactionsDenied() {
		return theme.Muted.Render("Payments are not available (access denied)")
	}
	summary := kv(13, "Total", api.FormatMoney(d.Total), "Paid", api.FormatMoney(d.Paid),
		"Outstanding", outstanding(d.Outstanding)) + "\n" + paidBar(d.Paid, d.Total, min(40, w-10))
	if len(d.Transactions) == 0 {
		return theme.Muted.Render("No payments") + "\n\n" + summary
	}
	g := components.Grid{Headers: []string{"Date", "Reference", "Description", "Method", "Status", "Amount"},
		Right: map[int]bool{5: true}, Dim: map[int]bool{}, Selected: -1, Flex: 2}
	for i, t := range d.Transactions {
		if t.Status == "VOID" {
			g.Dim[i] = true
		}
		g.Rows = append(g.Rows, []string{stamp(t.Created), orDash(t.Reference), orDash(t.Description),
			api.StatusLabel(t.Category), statusText(t.Status, false), priceOrDash(t.Amount)})
	}
	return g.Render(w) + "\n\n" + summary
}

// reservations renders the view switch, the invoice or itinerary list
// (windowed around the cursor), totals, and the selected item's details.
// Each reservation is a block of lines: one row, plus its legs when
// expanded in the itinerary.
func (b *Booking) reservations(w, h int) string {
	d := b.detail
	inv, itin := theme.TabOn, theme.Tab
	hint := "v switch view"
	if b.itinerary {
		inv, itin = theme.Tab, theme.TabOn
		hint = "enter expand · e expand all · " + hint
	}
	switcher := components.Spread(w, inv.Render("Invoice")+" "+itin.Render("Itinerary"), theme.Muted.Render(hint))
	if len(d.Reservations) == 0 {
		return switcher + "\n\n" + theme.Muted.Render("No reservations")
	}

	var head []string
	blocks := make([][]string, len(d.Reservations))
	if b.itinerary {
		it, ok := newItinerary(d.Reservations, w)
		if !ok {
			return switcher + "\n\n" + theme.Muted.Render("Too narrow for the itinerary: widen the terminal or press v.")
		}
		head = []string{it.axis()}
		name := func(id string) string {
			if p, ok := d.Passenger(id); ok {
				return p.Given
			}
			return "?"
		}
		for i, r := range d.Reservations {
			open := b.expanded[r.ID] && len(r.Legs) > 0
			blocks[i] = []string{it.row(r, i == b.cursor, open)}
			if open {
				blocks[i] = append(blocks[i], it.legRows(r, name)...)
			}
		}
	} else {
		lines := strings.Split(b.invoice(w), "\n")
		head, lines = lines[:2], lines[2:]
		for i := range blocks {
			n := invoiceRowHeight(d.Reservations[i])
			blocks[i], lines = lines[:n], lines[n:]
		}
	}

	totals := theme.Muted.Render("Total ") + api.FormatMoney(d.Total) +
		theme.Muted.Render("   Outstanding ") + outstanding(d.Outstanding)
	note := ""
	for _, r := range d.Reservations {
		if r.Status == "CANCELLED" {
			note = theme.Muted.Render("Cancelled items are not included in the total")
			break
		}
	}
	footer := components.Spread(w, note, totals)
	detail := b.selectedStrip(w)

	// Window over lines so the selected block is visible (its top line at
	// least, when it is taller than the window).
	var lines []string
	selStart, selEnd := 0, 0
	for i, blk := range blocks {
		if i == b.cursor {
			selStart, selEnd = len(lines), len(lines)+len(blk)
		}
		lines = append(lines, blk...)
	}
	avail := max(h-1-len(head)-1-lipgloss.Height(detail)-1, 1)
	if selEnd > b.offset+avail {
		b.offset = selEnd - avail
	}
	if selStart < b.offset {
		b.offset = selStart
	}
	b.offset = max(min(b.offset, len(lines)-avail), 0)
	visible := lines[b.offset:min(b.offset+avail, len(lines))]
	if more := len(lines) - b.offset - len(visible); more > 0 {
		footer = components.Spread(w, theme.Muted.Render(fmt.Sprintf("↓ %d more", more)), totals)
	}
	return strings.Join([]string{switcher, strings.Join(head, "\n"), strings.Join(visible, "\n"),
		footer, detail}, "\n")
}

func (b *Booking) invoice(w int) string {
	d := b.detail
	g := components.Grid{
		Headers:  []string{"Product", "Element", "Type", "Status", "Qty", "Dates", "Balance due", "Price"},
		Right:    map[int]bool{4: true, 7: true},
		Dim:      map[int]bool{},
		Selected: b.cursor,
		Flex:     0,
	}
	for i, r := range d.Reservations {
		status := statusText(r.Status, i == b.cursor) // plain when highlighted
		if r.Status == "CANCELLED" {
			g.Dim[i] = true
		}
		name := orDash(r.Product)
		if desc := description(r); desc != "" { // second line, in grey (plain when highlighted)
			if i != b.cursor {
				desc = theme.Muted.Render(desc)
			}
			name += "\n" + desc
		}
		g.Rows = append(g.Rows, []string{name, orDash(r.Element),
			orDash(r.Category), status,
			fmt.Sprint(r.Quantity), compactSpan(r.Start, r.End), day(r.BalanceDue), priceOrDash(r.Price)})
	}
	return g.Render(w)
}

// selectedStrip summarises the selected reservation under the list.
func (b *Booking) selectedStrip(w int) string {
	d := b.detail
	r := d.Reservations[b.cursor]
	line1 := theme.Bold.Render(orDash(r.Product))
	if r.Element != "" {
		line1 += theme.Muted.Render(" · ") + r.Element
	}
	if desc := description(r); desc != "" {
		line1 += theme.Muted.Render("  " + desc)
	}
	line1 += "  " + badge(r.Status)
	if len(r.Legs) > 0 {
		stops := []string{r.Legs[0].From.Label()}
		for _, l := range r.Legs {
			stops = append(stops, l.To.Label())
		}
		line1 += theme.Muted.Render("  " + strings.Join(stops, " → "))
	}
	if r.StatusMessage != "" {
		line1 += theme.Warn.Render("  " + r.StatusMessage)
	}
	var names []string
	for _, id := range r.PassengerIDs {
		if p, ok := d.Passenger(id); ok {
			names = append(names, p.Name())
		}
	}
	line2 := theme.Muted.Render("Dates ") + dateSpan(r.Start, r.End, "Mon 2 Jan 2006") +
		theme.Muted.Render("   Balance due ") + day(r.BalanceDue) +
		theme.Muted.Render("   Travellers ") + orDash(strings.Join(names, ", "))
	line3 := theme.Muted.Render("Type ") + orDash(r.Category) +
		theme.Muted.Render("   Element ") + orDash(r.Element) +
		theme.Muted.Render("   Group ") + orDash(r.Group) +
		theme.Muted.Render("   Supplier ") + orDash(r.Supplier)
	return theme.Rule.Render(strings.Repeat("─", w)) + "\n" +
		components.Truncate(line1, w) + "\n" + components.Truncate(line2, w) + "\n" + components.Truncate(line3, w)
}

// productLabel is the reservation's product name and element ("Hotel ·
// Double room") with its description in subtle text. plain drops the
// styling (for highlighted rows, where nested colours would cut through the
// selection background).
func productLabel(r api.Reservation, plain bool) string {
	name, sep, desc := orDash(r.Product), " · ", ""
	if d := description(r); d != "" {
		desc = "  " + d
	}
	if !plain {
		sep = theme.Muted.Render(sep)
		if desc != "" {
			desc = theme.Muted.Render(desc)
		}
	}
	if r.Element != "" {
		name += sep + r.Element
	}
	return name + desc
}

// OnPayments opens the screen on the Payments tab (e.g. from a transaction).
func (b *Booking) OnPayments() *Booking {
	b.tab = tabPayments
	return b
}

// invoiceRowHeight is the lines a reservation takes in the invoice: its
// name, plus its description underneath when it has one.
func invoiceRowHeight(r api.Reservation) int {
	if description(r) != "" {
		return 2
	}
	return 1
}

// description is the reservation's free-text description on one line, so
// line breaks in it can't make rows taller than the layout expects.
func description(r api.Reservation) string {
	return strings.Join(strings.Fields(r.Description), " ")
}
