// Package components holds reusable UI building blocks.
package components

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui/theme"
)

// Column describes one table column.
type Column struct {
	Title    string
	SortKey  string // API sort field; empty = not sortable
	Weight   int    // share of spare width (default 1)
	MinWidth int
	Right    bool // right-align (numbers, money)
}

// TablePage is one page of rows, with a parallel slice of row IDs.
type TablePage struct {
	Rows []table.Row
	IDs  []string
	Info api.PageInfo
}

// FetchFunc is called on the UI goroutine to snapshot any query state
// (filters etc.) and returns the loader, which runs inside a tea.Cmd.
type FetchFunc func(sort api.Sort, page api.PageRequest) func(ctx context.Context) (TablePage, error)

// RowSelectedMsg is emitted when the user presses enter on a row.
type RowSelectedMsg struct {
	TableID string
	RowID   string
}

type pageLoadedMsg struct {
	tableID string
	reqID   int
	page    TablePage
	err     error
}

// TableKeys are the table's own bindings (row movement comes from bubbles).
type TableKeys struct {
	Select, Next, Prev, Sort, Reverse, Reload key.Binding
}

// DefaultTableKeys returns the standard bindings.
func DefaultTableKeys() TableKeys {
	return TableKeys{
		Select:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Next:    key.NewBinding(key.WithKeys("n", "pgdown", "right"), key.WithHelp("n/→", "next page")),
		Prev:    key.NewBinding(key.WithKeys("p", "pgup", "left"), key.WithHelp("p/←", "prev page")),
		Sort:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort column")),
		Reverse: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sort direction")),
		Reload:  key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload")),
	}
}

// Table is a server-paginated, server-sorted data grid. Sorting and paging
// always re-query; nothing is sorted client-side over a partial page.
type Table struct {
	id       string
	columns  []Column
	fetch    FetchFunc
	pageSize int
	Keys     TableKeys

	inner   table.Model
	spinner spinner.Model
	sort    api.Sort

	// cursors[i] is the cursor that loads page i (cursors[0] == ""), and
	// offsets[i] the number of rows before page i. Pages can differ in size
	// (the terminal may be resized), so offsets are tracked, not computed.
	cursors []string
	offsets []int
	pageIdx int
	page    TablePage

	reqID   int
	loading bool
	err     error
	width   int
	height  int
}

// NewTable creates a table. id distinguishes tables in messages.
func NewTable(id string, cols []Column, fetch FetchFunc, defaultSort api.Sort) *Table {
	t := &Table{
		id:       id,
		columns:  cols,
		fetch:    fetch,
		pageSize: 25,
		Keys:     DefaultTableKeys(),
		sort:     defaultSort,
		cursors:  []string{""},
		offsets:  []int{0},
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).
		BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(theme.Subtle)
	styles.Selected = styles.Selected.Foreground(theme.Text).Background(theme.Accent).Bold(false)
	t.inner = table.New(table.WithFocused(true), table.WithStyles(styles))
	t.syncColumns()
	return t
}

// Sort returns the current sort.
func (t *Table) Sort() api.Sort { return t.sort }

// PageNumber is the 1-based current page.
func (t *Table) PageNumber() int { return t.pageIdx + 1 }

// Loading reports whether a request is in flight.
func (t *Table) Loading() bool { return t.loading }

// Reload discards paging state and loads the first page (e.g. after the
// filter changed).
func (t *Table) Reload() tea.Cmd {
	t.cursors = []string{""}
	t.offsets = []int{0}
	t.pageIdx = 0
	return t.load()
}

func (t *Table) load() tea.Cmd {
	t.reqID++
	t.loading = true
	t.err = nil
	id, req, sort := t.reqID, api.PageRequest{Size: t.pageSize, Cursor: t.cursors[t.pageIdx]}, t.sort
	load, tableID := t.fetch(sort, req), t.id
	return tea.Batch(t.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		page, err := load(ctx)
		return pageLoadedMsg{tableID: tableID, reqID: id, page: page, err: err}
	})
}

// SetSize sets the area the table (including its footer line) occupies.
func (t *Table) SetSize(width, height int) {
	t.width, t.height = width, height
	// Header row + border take 2 lines; the footer takes 1.
	t.inner.SetHeight(max(height-1, 3))
	t.inner.SetWidth(width)
	t.syncColumns()
	t.pageSize = max(height-3, 5) // rows that fit; applies from the next load
}

func (t *Table) syncColumns() {
	cols := make([]table.Column, len(t.columns))
	totalWeight, fixed := 0, 0
	for _, c := range t.columns {
		totalWeight += max(c.Weight, 1)
		fixed += max(c.MinWidth, len(c.Title)+2) + 2 // cell padding
	}
	spare := max(t.width-fixed, 0)
	widths := make([]int, len(t.columns))
	for i, c := range t.columns {
		widths[i] = max(c.MinWidth, len(c.Title)+2) + spare*max(c.Weight, 1)/max(totalWeight, 1)
	}
	t.fitWidths(widths)
	for i, c := range t.columns {
		title := c.Title
		if c.SortKey != "" && c.SortKey == t.sort.Field {
			if t.sort.Desc {
				title += " ▼"
			} else {
				title += " ▲"
			}
		}
		w := widths[i]
		if c.Right {
			title = padLeft(title, w)
		}
		cols[i] = table.Column{Title: title, Width: w}
	}
	t.inner.SetColumns(cols)
	t.inner.SetRows(t.displayRows())
}

// fitWidths narrows columns when their minimums overflow the table: first
// the flexible text columns (Weight > 1), then other text columns down to
// their title, then titles are cut. Right-aligned (numeric) columns keep
// their width. Within a pass the widest column gives way first.
func (t *Table) fitWidths(widths []int) {
	if t.width <= 0 {
		return
	}
	over := -t.width
	for _, w := range widths {
		over += w + 2 // cell padding
	}
	passes := []func(c Column) (int, bool){
		func(c Column) (int, bool) { return max(len(c.Title), 8), c.Weight > 1 },
		func(c Column) (int, bool) { return max(len(c.Title), 6), true },
		func(c Column) (int, bool) { return 4, true },
	}
	for _, pass := range passes {
		for over > 0 {
			widest := -1
			for i, c := range t.columns {
				floor, ok := pass(c)
				if ok && !c.Right && widths[i] > floor && (widest < 0 || widths[i] > widths[widest]) {
					widest = i
				}
			}
			if widest < 0 {
				break
			}
			widths[widest]--
			over--
		}
	}
}

// displayRows returns the current page with right-aligned cells padded to
// their column width (the bubbles table only left-aligns).
func (t *Table) displayRows() []table.Row {
	cols := t.inner.Columns()
	rows := make([]table.Row, len(t.page.Rows))
	for r, row := range t.page.Rows {
		rows[r] = slices.Clone(row)
		for i, c := range t.columns {
			if c.Right && i < len(row) && i < len(cols) {
				rows[r][i] = padLeft(row[i], cols[i].Width)
			}
		}
	}
	return rows
}

func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-lipgloss.Width(s), 0)) + s
}

func (t *Table) nextSortColumn() {
	var sortable []string
	for _, c := range t.columns {
		if c.SortKey != "" {
			sortable = append(sortable, c.SortKey)
		}
	}
	if len(sortable) == 0 {
		return
	}
	next := sortable[0]
	for i, k := range sortable {
		if k == t.sort.Field {
			next = sortable[(i+1)%len(sortable)]
		}
	}
	t.sort = api.Sort{Field: next}
}

// Update handles keys and load results.
func (t *Table) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case pageLoadedMsg:
		if msg.tableID != t.id || msg.reqID != t.reqID {
			return nil // stale or not ours
		}
		t.loading = false
		if msg.err != nil {
			t.err = msg.err
			return nil
		}
		t.page = msg.page
		t.inner.SetRows(t.displayRows())
		t.inner.SetCursor(0)
		if next := msg.page.Info.NextCursor; next != "" {
			t.cursors = append(t.cursors[:t.pageIdx+1], next)
			t.offsets = append(t.offsets[:t.pageIdx+1], t.offsets[t.pageIdx]+len(msg.page.Rows))
		} else {
			t.cursors = t.cursors[:t.pageIdx+1]
			t.offsets = t.offsets[:t.pageIdx+1]
		}
		return nil

	case spinner.TickMsg:
		if !t.loading {
			return nil
		}
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return cmd

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, t.Keys.Select):
			if i := t.inner.Cursor(); i >= 0 && i < len(t.page.IDs) {
				id := t.page.IDs[i]
				return func() tea.Msg { return RowSelectedMsg{TableID: t.id, RowID: id} }
			}
			return nil
		case key.Matches(msg, t.Keys.Next):
			if t.pageIdx+1 < len(t.cursors) && !t.loading {
				t.pageIdx++
				return t.load()
			}
			return nil
		case key.Matches(msg, t.Keys.Prev):
			if t.pageIdx > 0 && !t.loading {
				t.pageIdx--
				return t.load()
			}
			return nil
		case key.Matches(msg, t.Keys.Sort):
			t.nextSortColumn()
			t.syncColumns()
			return t.Reload()
		case key.Matches(msg, t.Keys.Reverse):
			if t.sort.Field != "" {
				t.sort.Desc = !t.sort.Desc
				t.syncColumns()
				return t.Reload()
			}
			return nil
		case key.Matches(msg, t.Keys.Reload):
			return t.load()
		}
		var cmd tea.Cmd
		t.inner, cmd = t.inner.Update(msg)
		return cmd
	}
	return nil
}

// View renders the grid and a one-line footer.
func (t *Table) View() string {
	var body string
	switch {
	case t.err != nil && len(t.page.Rows) == 0:
		body = theme.Error.Render("Failed to load: " + t.err.Error())
	case t.loading && len(t.page.Rows) == 0:
		body = t.spinner.View() + " Loading…"
	case len(t.page.Rows) == 0:
		body = theme.Muted.Render("No results.")
	default:
		body = t.inner.View()
	}
	body = lipgloss.NewStyle().Height(max(t.height-1, 1)).MaxHeight(max(t.height-1, 1)).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, body, t.footer())
}

func (t *Table) footer() string {
	parts := []string{fmt.Sprintf("Page %d", t.pageIdx+1)}
	if total := t.page.Info.Total; total >= 0 && len(t.page.Rows) > 0 {
		start := t.offsets[t.pageIdx] + 1
		remaining := max(total-(start-1+len(t.page.Rows)), 0)
		pages := t.pageIdx + 1 + (remaining+t.pageSize-1)/t.pageSize
		parts[0] = fmt.Sprintf("Page %d/%d", t.pageIdx+1, pages)
		parts = append(parts, fmt.Sprintf("%d–%d of %d", start, start+len(t.page.Rows)-1, total))
	}
	if t.page.Info.HasNext() {
		parts = append(parts, "more →")
	}
	if t.loading {
		parts = append(parts, t.spinner.View()+" loading")
	}
	if t.err != nil && len(t.page.Rows) > 0 {
		return theme.Error.Render(strings.Join(append(parts, "error: "+t.err.Error()), " · "))
	}
	return theme.Muted.Render(strings.Join(parts, " · "))
}
