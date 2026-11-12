package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui"
	"github.com/rezkit/tui/internal/ui/components"
)

const bookingsTable = "bookings"

// Bookings lists bookings with filtering, sorting and paging.
type Bookings struct {
	svc    api.Service
	table  *components.Table
	filter api.BookingFilter
	keys   listKeyMap
	width  int
	height int
}

type listKeyMap struct {
	Filter, ClearFilter key.Binding
	table               components.TableKeys
}

// NewBookings creates the bookings list screen.
func NewBookings(svc api.Service) *Bookings {
	b := &Bookings{
		svc:  svc,
		keys: listKeys(),
	}
	cols := []components.Column{
		// Only columns the API can sort by server-side get a SortKey.
		{Title: "Reference", SortKey: api.SortReference, MinWidth: 10},
		{Title: "Status", MinWidth: 11},
		{Title: "Description", SortKey: api.SortDescription, Weight: 3, MinWidth: 14},
		{Title: "Customer", Weight: 2, MinWidth: 14},
		{Title: "Start", MinWidth: 11},
		{Title: "Total", MinWidth: 14, Right: true},
		{Title: "Created", SortKey: api.SortCreatedAt, MinWidth: 11},
	}
	b.table = components.NewTable(bookingsTable, cols, b.fetch, api.Sort{Field: api.SortCreatedAt, Desc: true})
	b.keys.table = b.table.Keys
	return b
}

// fetch runs on the UI goroutine and snapshots the filter for the loader.
func (b *Bookings) fetch(sort api.Sort, page api.PageRequest) func(context.Context) (components.TablePage, error) {
	q := api.BookingQuery{Filter: b.filter, Sort: sort, Page: page}
	return func(ctx context.Context) (components.TablePage, error) { return b.load(ctx, q) }
}

func (b *Bookings) load(ctx context.Context, q api.BookingQuery) (components.TablePage, error) {
	res, err := b.svc.Bookings(ctx, q)
	if err != nil {
		return components.TablePage{}, err
	}
	out := components.TablePage{Info: res.Info}
	for _, bk := range res.Items {
		out.IDs = append(out.IDs, bk.ID)
		out.Rows = append(out.Rows, table.Row{
			bk.Reference, api.StatusLabel(bk.Status), bk.Description, bk.Customer,
			dateOrDash(bk.StartDate), api.FormatMoney(bk.Total), dateOrDash(bk.CreatedAt),
		})
	}
	return out, nil
}

func (b *Bookings) Init() tea.Cmd { return b.table.Reload() }

func (b *Bookings) Title() string { return "Bookings" }

func (b *Bookings) CapturingInput() bool { return false }

func (b *Bookings) ShortHelp() []key.Binding {
	return []key.Binding{b.keys.table.Select, b.keys.Filter, b.keys.table.Sort, b.keys.table.Next, b.keys.table.Prev}
}

func (b *Bookings) FullHelp() [][]key.Binding {
	t := b.keys.table
	return [][]key.Binding{
		{t.Select, t.Next, t.Prev, t.Reload},
		{t.Sort, t.Reverse, b.keys.Filter, b.keys.ClearFilter},
	}
}

func (b *Bookings) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width, b.height = msg.Width, msg.Height
		b.table.SetSize(msg.Width, max(msg.Height-1, 1)) // 1 line for the filter summary
		return b, nil
	case bookingFilterAppliedMsg:
		b.filter = msg.filter
		return b, tea.Batch(ui.CloseModal, b.table.Reload())
	case components.RowSelectedMsg:
		if msg.TableID == bookingsTable {
			return b, ui.Push(NewBooking(msg.RowID, b.svc.Booking))
		}
		return b, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, b.keys.Filter):
			return b, ui.OpenModal(newBookingFilterModal(b.filter))
		case key.Matches(msg, b.keys.ClearFilter):
			b.filter = api.BookingFilter{}
			return b, b.table.Reload()
		}
	}
	return b, b.table.Update(msg)
}

func (b *Bookings) View(width, height int) string {
	return b.filterSummary() + "\n" + b.table.View()
}

func (b *Bookings) filterSummary() string {
	f := b.filter
	parts := containsPart("reference", f.Reference)
	if f.Status != "" {
		parts = append(parts, "status "+api.StatusLabel(f.Status))
	}
	return filterLine(append(parts, dateParts(f.CreatedFrom, f.CreatedTo)...))
}

const dateLayout = "2006-01-02"

func dateOrDash(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return api.Civil(t).Format(dateLayout)
}

type bookingFilterAppliedMsg struct{ filter api.BookingFilter }

func newBookingFilterModal(f api.BookingFilter) *formModal {
	ref, status, dates := f.Reference, f.Status, newDateRange(f.CreatedFrom, f.CreatedTo)
	statuses := []huh.Option[string]{huh.NewOption("Any", "")}
	for _, s := range api.BookingStatuses {
		statuses = append(statuses, huh.NewOption(api.StatusLabel(s), s))
	}
	submit := func() tea.Msg {
		out := api.BookingFilter{Reference: strings.TrimSpace(ref), Status: status}
		out.CreatedFrom, out.CreatedTo = dates.bounds()
		return bookingFilterAppliedMsg{filter: out}
	}
	return newFormModal("Filter bookings", submit, append([]huh.Field{
		huh.NewInput().Title("Reference contains").Value(&ref),
		huh.NewSelect[string]().Title("Status").Options(statuses...).Value(&status),
	}, dates.fields()...)...)
}

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, strings.TrimSpace(s), time.Local)
}

func validDate(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if _, err := parseDate(s); err != nil {
		return fmt.Errorf("use YYYY-MM-DD")
	}
	return nil
}

// filterKeyMap makes esc cancel the form; ctrl+c is reserved for quitting.
func filterKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	return km
}
