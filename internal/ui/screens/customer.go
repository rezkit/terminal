package screens

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui"
	"github.com/rezkit/tui/internal/ui/components"
	"github.com/rezkit/tui/internal/ui/theme"
)

type customerTab int

const (
	custOverview customerTab = iota
	custBookings
	custNotes
	custTabCount
)

var custTabTitles = []string{"Overview", "Bookings", "Notes"}

// Customer shows one customer: a header card, then tabs for their details,
// their bookings (each opens the booking screen) and notes.
type Customer struct {
	id  string
	svc api.Service

	detail  *api.CustomerDetail
	err     error
	now     func() time.Time
	spinner spinner.Model
	vp      viewport.Model
	w, h    int

	tab    customerTab
	cursor int // selected booking
	offset int // first visible booking row
	keys   customerKeys
}

type customerKeys struct {
	Next, Prev, Jump, Up, Down, Open, Reload key.Binding
}

type customerLoadedMsg struct {
	id     string
	detail api.CustomerDetail
	err    error
}

// NewCustomer creates the customer screen.
func NewCustomer(id string, svc api.Service) *Customer {
	return &Customer{
		id: id, svc: svc, now: time.Now,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		vp:      viewport.New(),
		keys: customerKeys{
			Next:   key.NewBinding(key.WithKeys("tab", "l"), key.WithHelp("tab/1-3", "switch tab")),
			Prev:   key.NewBinding(key.WithKeys("shift+tab", "h")),
			Jump:   key.NewBinding(key.WithKeys("1", "2", "3")),
			Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "select")),
			Down:   key.NewBinding(key.WithKeys("down", "j")),
			Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open booking")),
			Reload: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload")),
		},
	}
}

func (c *Customer) fetch() tea.Cmd {
	id, svc := c.id, c.svc
	return tea.Batch(c.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := svc.Customer(ctx, id)
		return customerLoadedMsg{id: id, detail: d, err: err}
	})
}

func (c *Customer) Init() tea.Cmd { return c.fetch() }

func (c *Customer) Title() string {
	if c.detail != nil {
		return c.detail.Name()
	}
	return "Customer"
}

func (c *Customer) CapturingInput() bool { return false }

func (c *Customer) ShortHelp() []key.Binding {
	ks := []key.Binding{c.keys.Next}
	if c.tab == custBookings {
		ks = append(ks, c.keys.Up, c.keys.Open)
	} else {
		ks = append(ks, key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓", "scroll")))
	}
	return append(ks, c.keys.Reload)
}

func (c *Customer) FullHelp() [][]key.Binding { return [][]key.Binding{c.ShortHelp()} }

func (c *Customer) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.w, c.h = msg.Width, msg.Height
		c.refresh()
	case customerLoadedMsg:
		if msg.id != c.id {
			return c, nil
		}
		c.err = msg.err
		if msg.err == nil {
			c.detail = &msg.detail
			c.cursor = min(c.cursor, max(len(msg.detail.Bookings)-1, 0))
		}
		c.refresh()
	case spinner.TickMsg:
		if c.detail != nil || c.err != nil {
			return c, nil
		}
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		return c, cmd
	case tea.KeyPressMsg:
		return c, c.handleKey(msg)
	}
	return c, nil
}

func (c *Customer) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, c.keys.Reload):
		c.err = nil
		return c.fetch()
	case key.Matches(msg, c.keys.Next):
		c.setTab((c.tab + 1) % custTabCount)
	case key.Matches(msg, c.keys.Prev):
		c.setTab((c.tab + custTabCount - 1) % custTabCount)
	case key.Matches(msg, c.keys.Jump):
		c.setTab(customerTab(msg.String()[0] - '1'))
	case c.tab == custBookings && key.Matches(msg, c.keys.Up):
		c.cursor = max(c.cursor-1, 0)
	case c.tab == custBookings && key.Matches(msg, c.keys.Down):
		if c.detail != nil {
			c.cursor = min(c.cursor+1, max(len(c.detail.Bookings)-1, 0))
		}
	case c.tab == custBookings && key.Matches(msg, c.keys.Open):
		if c.detail != nil && c.cursor < len(c.detail.Bookings) {
			return ui.Push(NewBooking(c.detail.Bookings[c.cursor].ID, c.svc.Booking))
		}
	case c.tab != custBookings:
		var cmd tea.Cmd
		c.vp, cmd = c.vp.Update(msg)
		return cmd
	}
	return nil
}

func (c *Customer) setTab(t customerTab) {
	if t == c.tab {
		return
	}
	c.tab = t
	c.vp.GotoTop()
	c.refresh()
}

// --- layout -------------------------------------------------------------------

func (c *Customer) chrome() string {
	card := c.headerCard()
	if c.h < 14 {
		card = strings.SplitN(card, "\n", 2)[0]
	}
	counts := []string{"", count(len(c.detail.Bookings), c.detail.BookingsDenied()),
		count(len(c.detail.Notes), c.detail.NotesDenied())}
	return card + "\n" + tabBar(custTabTitles, counts, int(c.tab), c.w)
}

func (c *Customer) bodySize() (int, int) {
	return c.w, max(c.h-lipgloss.Height(c.chrome()), 1)
}

func (c *Customer) refresh() {
	if c.detail == nil || c.w == 0 {
		return
	}
	w, h := c.bodySize()
	c.vp.SetWidth(w)
	c.vp.SetHeight(h)
	var content string
	switch c.tab {
	case custOverview:
		content = c.overview(w)
	case custNotes:
		content = notesView(c.detail.Notes, w)
	}
	c.vp.SetContent(components.ClampLines(content, w, -1))
}

func (c *Customer) View(width, height int) string {
	switch {
	case c.detail == nil && c.err != nil:
		return theme.Error.Render("Failed to load customer: " + c.err.Error())
	case c.detail == nil:
		return c.spinner.View() + " Loading…"
	}
	body := c.vp.View()
	if c.tab == custBookings {
		_, h := c.bodySize()
		body = c.bookings(c.w, h)
	}
	out := c.chrome() + "\n" + body
	if c.err != nil {
		out = theme.Error.Render("Reload failed: "+c.err.Error()) + "\n" + out
	}
	return components.ClampLines(out, width, height)
}

func (c *Customer) headerCard() string {
	d, w := c.detail, c.w
	title := theme.Bold.Render(d.Name())
	if d.KnownAs != "" && d.KnownAs != d.Given {
		title += theme.Muted.Render(fmt.Sprintf("  “%s”", d.KnownAs))
	}
	since := theme.Muted.Render("Customer since " + api.Civil(d.Created).Format("Jan 2006"))
	contact := theme.Muted.Render(strings.Join(nonEmpty(d.Email, d.Telephone), " · "))

	var money string
	if !d.BookingsDenied() {
		total, owed := sumByCurrency(d.Bookings)
		money = theme.Muted.Render(plural(len(d.Bookings), "booking", "bookings"))
		if len(total) > 0 {
			money += theme.Muted.Render("   Total ") + strings.Join(total, " + ")
		}
		if len(owed) > 0 {
			money += theme.Muted.Render("   Outstanding ") + theme.Warn.Render(strings.Join(owed, " + "))
		}
	}
	tags := ""
	if len(d.Tags) > 0 {
		tags = theme.Muted.Render("Tags ") + strings.Join(d.Tags, ", ")
	}
	return strings.Join([]string{
		components.Spread(w, title, since),
		components.Spread(w, contact, money),
		tags,
	}, "\n")
}

// sumByCurrency totals bookings' value and outstanding balance per currency
// (cancelled bookings excluded), formatted; currencies are not converted.
func sumByCurrency(bs []api.CustomerBooking) (total, owed []string) {
	totals, outstanding := map[string]int64{}, map[string]int64{}
	var order []string
	for _, b := range bs {
		if b.Status == "CANCELLED" {
			continue
		}
		cur := b.Total.Currency
		if _, seen := totals[cur]; !seen {
			order = append(order, cur)
		}
		totals[cur] += b.Total.Minor
		outstanding[b.Outstanding.Currency] += b.Outstanding.Minor
	}
	slices.Sort(order)
	for _, cur := range order {
		total = append(total, api.FormatMoney(api.Money{Minor: totals[cur], Currency: cur}))
		if n := outstanding[cur]; n > 0 {
			owed = append(owed, api.FormatMoney(api.Money{Minor: n, Currency: cur}))
		}
	}
	return total, owed
}

// --- tabs -----------------------------------------------------------------------

func (c *Customer) overview(w int) string {
	d := c.detail
	dob := day(d.DateOfBirth)
	if p := (api.Passenger{DateOfBirth: d.DateOfBirth}); !d.DateOfBirth.IsZero() {
		dob += theme.Muted.Render(fmt.Sprintf("  (age %d)", p.AgeAt(c.now())))
	}
	address := "—"
	if len(d.Address) > 0 {
		address = strings.Join(d.Address, "\n")
	}
	left := heading("Contact", -1) + "\n" + kv(15,
		"Name", d.Name(),
		"Known as", orDash(d.KnownAs),
		"Email", orDash(d.Email),
		"Telephone", orDash(d.Telephone),
		"Date of birth", dob,
		"Sex", orDash(api.StatusLabel(d.Sex))) +
		"\n" + kvBlock(15, "Address", address) +
		"\n\n" + heading("Record", -1) + "\n" + kv(15,
		"Created", stamp(d.Created),
		"Updated", stamp(d.Updated))

	attn := c.attention()
	colW := w
	if w >= 90 {
		colW = (w - 4) / 2
	}
	right := heading("Trips", -1) + "\n" + c.trips(colW) +
		"\n\n" + heading("Needs attention", hiddenIf(d.BookingsDenied(), len(attn))) + "\n"
	const maxAttn = 8
	switch {
	case d.BookingsDenied(): // nothing to check, which is not the same as all clear
		right += theme.Muted.Render("Not available (bookings withheld)")
	case len(attn) == 0:
		right += theme.Ok.Render("✓ ") + "Nothing outstanding"
	case len(attn) > maxAttn:
		right += strings.Join(attn[:maxAttn-1], "\n") + "\n" +
			theme.Muted.Render(fmt.Sprintf("…and %d more (press 2)", len(attn)-maxAttn+1))
	default:
		right += strings.Join(attn, "\n")
	}
	right += "\n\n" + heading("Notes", -1) + "\n" + notesPreview(d.Notes, "3")
	return columns(left, right, w)
}

// kvBlock is a kv pair whose value may span lines, aligned under itself.
func kvBlock(labelW int, label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, theme.Muted.Width(labelW).Render(label), value)
}

// trips summarises the trip under way or next, and the most recent past one,
// each truncated to w.
func (c *Customer) trips(w int) string {
	d := c.detail
	if d.BookingsDenied() {
		return theme.Muted.Render("Bookings are not available (access denied)")
	}
	today := civilDay(c.now())
	var next, last *api.CustomerBooking
	for i := range d.Bookings { // newest first, so the last upcoming one seen is the soonest
		b := &d.Bookings[i]
		if b.Start.IsZero() || b.Status == "CANCELLED" {
			continue
		}
		end := b.End
		if end.IsZero() {
			end = b.Start
		}
		switch {
		case !civilDay(end).Before(today):
			next = b
		case last == nil:
			last = b
		}
	}
	label := "Next"
	if next != nil && !civilDay(next.Start).After(today) {
		label = "Travelling now"
	}
	line := func(label string, b *api.CustomerBooking) string {
		v := "—"
		if b != nil {
			v = theme.Bold.Render(b.Reference) + "  " + compactSpan(b.Start, b.End) + theme.Muted.Render("  "+b.Description)
		}
		return components.Truncate(kv(15, label, v), w)
	}
	return line(label, next) + "\n" + line("Last", last)
}

// attention lists bookings with money owed or an unsettled status.
func (c *Customer) attention() []string {
	var out []string
	for _, b := range c.detail.Bookings {
		switch {
		case b.Status == "CANCELLED" || b.Status == "ARCHIVED":
		case b.Outstanding.Minor > 0:
			out = append(out, theme.Warn.Render("▲ ")+theme.Bold.Render(b.Reference)+" "+
				api.FormatMoney(b.Outstanding)+" outstanding"+theme.Muted.Render(" · "+api.StatusLabel(b.Status)))
		case b.Status != "CONFIRMED":
			out = append(out, theme.Warn.Render("▲ ")+theme.Bold.Render(b.Reference)+" "+api.StatusLabel(b.Status))
		}
	}
	return out
}

// bookings is the selectable list of the customer's bookings, windowed
// around the cursor.
func (c *Customer) bookings(w, h int) string {
	d := c.detail
	switch {
	case d.BookingsDenied():
		return theme.Muted.Render("Bookings are not available (access denied)")
	case len(d.Bookings) == 0:
		return theme.Muted.Render("No bookings")
	}
	g := components.Grid{
		Headers:  []string{"Reference", "Status", "Description", "Dates", "Created", "Total", "Outstanding"},
		Right:    map[int]bool{5: true, 6: true},
		Dim:      map[int]bool{},
		Selected: c.cursor,
		Flex:     2,
	}
	for i, b := range d.Bookings {
		status := badge(b.Status)
		if i == c.cursor { // nested colours would cut through the highlight
			status = "● " + api.StatusLabel(b.Status)
		}
		if b.Status == "CANCELLED" {
			g.Dim[i] = true
		}
		g.Rows = append(g.Rows, []string{b.Reference, status, orDash(b.Description),
			yearSpan(b.Start, b.End), day(b.Created), api.FormatMoney(b.Total), outstandingCell(b, i == c.cursor)})
	}
	lines := strings.Split(g.Render(w), "\n")
	head, rows := lines[:2], lines[2:]

	avail := max(h-len(head)-1, 1)
	if c.cursor >= c.offset+avail {
		c.offset = c.cursor - avail + 1
	}
	c.offset = max(min(c.offset, c.cursor, len(rows)-avail), 0)
	visible := rows[c.offset:min(c.offset+avail, len(rows))]
	footer := theme.Muted.Render("enter opens " + d.Bookings[c.cursor].Reference)
	if more := len(rows) - c.offset - len(visible); more > 0 {
		footer = theme.Muted.Render(fmt.Sprintf("↓ %d more · ", more)) + footer
	}
	return strings.Join(head, "\n") + "\n" + strings.Join(visible, "\n") + "\n" + footer
}

func outstandingCell(b api.CustomerBooking, selected bool) string {
	if selected || b.Status == "CANCELLED" {
		return api.FormatMoney(b.Outstanding)
	}
	return outstanding(b.Outstanding)
}

// hiddenIf is n, or -1 (no count shown) when the data was withheld.
func hiddenIf(denied bool, n int) int {
	if denied {
		return -1
	}
	return n
}
