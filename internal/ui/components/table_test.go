package components

import (
	"context"
	"testing"

	"github.com/rezkit/tui/internal/api"
)

func TestTableColumnsFitNarrowWidths(t *testing.T) {
	cols := []Column{
		{Title: "Reference", MinWidth: 10},
		{Title: "Status", MinWidth: 11},
		{Title: "Description", Weight: 3, MinWidth: 14},
		{Title: "Customer", Weight: 2, MinWidth: 14},
		{Title: "Start", MinWidth: 11},
		{Title: "Total", MinWidth: 14, Right: true},
		{Title: "Created", MinWidth: 11},
	}
	fetch := func(api.Sort, api.PageRequest) func(context.Context) (TablePage, error) { return nil }
	for _, w := range []int{60, 80, 100, 160} {
		tb := NewTable("t", cols, fetch, api.Sort{})
		tb.SetSize(w, 20)
		total := 0
		for i, c := range tb.inner.Columns() {
			total += c.Width + 2
			if cols[i].Right && c.Width < cols[i].MinWidth {
				t.Errorf("width %d: numeric column %q shrank to %d", w, cols[i].Title, c.Width)
			}
		}
		if total > w {
			t.Errorf("width %d: columns need %d", w, total)
		}
		if w == 160 && total < w-len(cols) { // spare room is shared out (rounding aside)
			t.Errorf("width 160: columns only use %d", total)
		}
	}
}
