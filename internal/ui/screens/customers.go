package screens

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui"
	"github.com/rezkit/tui/internal/ui/components"
)

const customersTable = "customers"

// Customers lists customers with filtering, sorting and paging.
type Customers struct {
	svc    api.Service
	table  *components.Table
	filter api.CustomerFilter
	keys   listKeyMap
}

// NewCustomers creates the customers list screen.
func NewCustomers(svc api.Service) *Customers {
	c := &Customers{svc: svc, keys: listKeys()}
	cols := []components.Column{
		{Title: "Name", SortKey: api.SortFamilyName, Weight: 2, MinWidth: 16},
		{Title: "Email", SortKey: api.SortEmail, Weight: 3, MinWidth: 18},
		{Title: "Telephone", MinWidth: 16},
		{Title: "Location", SortKey: api.SortCity, Weight: 1, MinWidth: 12},
		{Title: "Created", SortKey: api.SortCreatedAt, MinWidth: 11},
	}
	c.table = components.NewTable(customersTable, cols, c.fetch, api.Sort{Field: api.SortFamilyName})
	c.keys.table = c.table.Keys
	return c
}

// listKeys are the filter bindings every list screen shares.
func listKeys() listKeyMap {
	return listKeyMap{
		Filter:      key.NewBinding(key.WithKeys("/", "f"), key.WithHelp("/", "filter")),
		ClearFilter: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "clear filter")),
	}
}

func (c *Customers) fetch(sort api.Sort, page api.PageRequest) func(context.Context) (components.TablePage, error) {
	q := api.CustomerQuery{Filter: c.filter, Sort: sort, Page: page}
	svc := c.svc
	return func(ctx context.Context) (components.TablePage, error) {
		res, err := svc.Customers(ctx, q)
		if err != nil {
			return components.TablePage{}, err
		}
		out := components.TablePage{Info: res.Info}
		for _, cu := range res.Items {
			out.IDs = append(out.IDs, cu.ID)
			out.Rows = append(out.Rows, table.Row{
				cu.Name(), cu.Email, orDash(cu.Telephone), orDash(cu.Location()), dateOrDash(cu.Created),
			})
		}
		return out, nil
	}
}

func (c *Customers) Init() tea.Cmd { return c.table.Reload() }

func (c *Customers) Title() string { return "Customers" }

func (c *Customers) CapturingInput() bool { return false }

func (c *Customers) ShortHelp() []key.Binding {
	t := c.keys.table
	return []key.Binding{t.Select, c.keys.Filter, t.Sort, t.Next, t.Prev}
}

func (c *Customers) FullHelp() [][]key.Binding {
	t := c.keys.table
	return [][]key.Binding{
		{t.Select, t.Next, t.Prev, t.Reload},
		{t.Sort, t.Reverse, c.keys.Filter, c.keys.ClearFilter},
	}
}

type customerFilterAppliedMsg struct{ filter api.CustomerFilter }

func (c *Customers) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.table.SetSize(msg.Width, max(msg.Height-1, 1)) // 1 line for the filter summary
		return c, nil
	case customerFilterAppliedMsg:
		c.filter = msg.filter
		return c, tea.Batch(ui.CloseModal, c.table.Reload())
	case components.RowSelectedMsg:
		if msg.TableID == customersTable {
			return c, ui.Push(NewCustomer(msg.RowID, c.svc))
		}
		return c, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, c.keys.Filter):
			return c, ui.OpenModal(newCustomerFilterModal(c.filter))
		case key.Matches(msg, c.keys.ClearFilter):
			c.filter = api.CustomerFilter{}
			return c, c.table.Reload()
		}
	}
	return c, c.table.Update(msg)
}

func (c *Customers) View(width, height int) string {
	f := c.filter
	parts := append(containsPart("given name", f.Given), containsPart("family name", f.Family)...)
	parts = append(parts, containsPart("email", f.Email)...)
	return filterLine(append(parts, dateParts(f.CreatedFrom, f.CreatedTo)...)) + "\n" + c.table.View()
}

// newCustomerFilterModal edits a CustomerFilter. The API filters each name
// part separately (and ANDs them), so there is no single "name" field.
func newCustomerFilterModal(f api.CustomerFilter) *formModal {
	given, family, email := f.Given, f.Family, f.Email
	dates := newDateRange(f.CreatedFrom, f.CreatedTo)
	submit := func() tea.Msg {
		out := api.CustomerFilter{
			Given: strings.TrimSpace(given), Family: strings.TrimSpace(family), Email: strings.TrimSpace(email)}
		out.CreatedFrom, out.CreatedTo = dates.bounds()
		return customerFilterAppliedMsg{filter: out}
	}
	return newFormModal("Filter customers", submit, append([]huh.Field{
		huh.NewInput().Title("Given name contains").Value(&given),
		huh.NewInput().Title("Family name contains").Value(&family),
		huh.NewInput().Title("Email contains").Value(&email),
	}, dates.fields()...)...)
}
