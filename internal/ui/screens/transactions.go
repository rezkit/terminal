package screens

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/components"
)

const transactionsTable = "transactions"

// Transactions lists payments across all bookings. The API cannot sort
// transactions, so no column is sortable. Opening a row shows its booking's
// payments.
type Transactions struct {
	svc    api.Service
	table  *components.Table
	filter api.TransactionFilter
	keys   listKeyMap
}

// NewTransactions creates the transactions list screen.
func NewTransactions(svc api.Service) *Transactions {
	t := &Transactions{svc: svc, keys: listKeys()}
	cols := []components.Column{
		{Title: "Created", MinWidth: 17},
		{Title: "Reference", MinWidth: 10},
		{Title: "Booking", MinWidth: 10},
		{Title: "Customer", Weight: 2, MinWidth: 14},
		{Title: "Description", Weight: 2, MinWidth: 12},
		{Title: "Method", MinWidth: 13},
		{Title: "Status", MinWidth: 10, Format: statusText},
		{Title: "Amount", MinWidth: 12, Right: true},
	}
	t.table = components.NewTable(transactionsTable, cols, t.fetch, api.Sort{})
	t.table.Keys.Select.SetHelp("enter", "open booking")
	t.keys.table = t.table.Keys
	return t
}

func (t *Transactions) fetch(_ api.Sort, page api.PageRequest) func(context.Context) (components.TablePage, error) {
	q := api.TransactionQuery{Filter: t.filter, Page: page}
	svc := t.svc
	return func(ctx context.Context) (components.TablePage, error) {
		res, err := svc.Transactions(ctx, q)
		if err != nil {
			return components.TablePage{}, err
		}
		out := components.TablePage{Info: res.Info}
		for _, tx := range res.Items {
			// The row opens the booking, so its ID is the booking's.
			out.IDs = append(out.IDs, tx.BookingID)
			out.Rows = append(out.Rows, table.Row{
				stamp(tx.Created), orDash(tx.Reference), orDash(tx.BookingRef), orDash(tx.Customer),
				orDash(tx.Description), api.StatusLabel(tx.Category), tx.Status, priceOrDash(tx.Amount),
			})
		}
		return out, nil
	}
}

func (t *Transactions) Init() tea.Cmd { return t.table.Reload() }

func (t *Transactions) Title() string { return "Transactions" }

func (t *Transactions) CapturingInput() bool { return false }

func (t *Transactions) ShortHelp() []key.Binding {
	k := t.keys.table
	return []key.Binding{k.Select, t.keys.Filter, k.Next, k.Prev}
}

func (t *Transactions) FullHelp() [][]key.Binding {
	k := t.keys.table
	return [][]key.Binding{{k.Select, k.Next, k.Prev, k.Reload}, {t.keys.Filter, t.keys.ClearFilter}}
}

type transactionFilterAppliedMsg struct{ filter api.TransactionFilter }

func (t *Transactions) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.table.SetSize(msg.Width, max(msg.Height-1, 1)) // 1 line for the filter summary
		return t, nil
	case transactionFilterAppliedMsg:
		t.filter = msg.filter
		return t, tea.Batch(ui.CloseModal, t.table.Reload())
	case components.RowSelectedMsg:
		if msg.TableID != transactionsTable {
			return t, nil
		}
		if msg.RowID == "" {
			return t, ui.Status("This transaction has no booking to open")
		}
		return t, ui.Push(NewBooking(msg.RowID, t.svc.Booking).OnPayments())
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, t.table.Keys.Sort), key.Matches(msg, t.table.Keys.Reverse):
			return t, ui.Status("Transactions can't be sorted (the API returns them in a fixed order)")
		case key.Matches(msg, t.keys.Filter):
			return t, ui.OpenModal(newTransactionFilterModal(t.filter))
		case key.Matches(msg, t.keys.ClearFilter):
			t.filter = api.TransactionFilter{}
			return t, t.table.Reload()
		}
	}
	return t, t.table.Update(msg)
}

func (t *Transactions) View(width, height int) string {
	f := t.filter
	parts := append(containsPart("reference", f.Reference), containsPart("booking", f.BookingRef)...)
	if f.Currency != "" {
		parts = append(parts, "currency "+f.Currency)
	}
	return filterLine(append(parts, dateParts(f.CreatedFrom, f.CreatedTo)...)) + "\n" + t.table.View()
}

// newTransactionFilterModal edits a TransactionFilter. The API has no
// status filter for transactions.
func newTransactionFilterModal(f api.TransactionFilter) *formModal {
	ref, booking, currency := f.Reference, f.BookingRef, f.Currency
	dates := newDateRange(f.CreatedFrom, f.CreatedTo)
	submit := func() tea.Msg {
		out := api.TransactionFilter{Reference: strings.TrimSpace(ref), BookingRef: strings.TrimSpace(booking),
			Currency: strings.ToUpper(strings.TrimSpace(currency))}
		out.CreatedFrom, out.CreatedTo = dates.bounds()
		return transactionFilterAppliedMsg{filter: out}
	}
	return newFormModal("Filter transactions", submit, append([]huh.Field{
		huh.NewInput().Title("Reference contains").Value(&ref),
		huh.NewInput().Title("Booking reference contains").Value(&booking),
		huh.NewInput().Title("Currency").Placeholder("e.g. GBP").Value(&currency).Validate(validCurrency),
	}, dates.fields()...)...)
}

func validCurrency(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	bad := len(s) != 3
	for _, r := range s {
		bad = bad || !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z')
	}
	if bad {
		return fmt.Errorf("use a 3-letter code like GBP")
	}
	return nil
}
