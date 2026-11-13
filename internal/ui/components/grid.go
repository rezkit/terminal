package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/rezkit/terminal/internal/ui/theme"
)

// Grid is a static, borderless table with a header rule, for detail views.
// Rows render one line each, unless a cell holds several lines ("\n"), when
// the row is as tall as its tallest cell. Cells never wrap: they are cut to
// fit instead.
type Grid struct {
	Headers  []string
	Rows     [][]string
	Right    map[int]bool // right-aligned columns
	Dim      map[int]bool // rows shown muted and struck through (cancelled, void)
	Selected int          // highlighted row, or -1
	Flex     int          // column truncated first when too wide, or -1
}

const gridPad = 2 // space after each column

// Render draws the grid at its natural width. When that is wider than width
// it narrows the Flex column first (to no less than 8), then the widest
// other text columns (to no less than 4); right-aligned columns keep their
// width as long as anything else can give. If the grid is still too wide
// after that, every line is cut to width.
func (g Grid) Render(width int) string {
	widths := make([]int, len(g.Headers))
	for c, h := range g.Headers {
		widths[c] = lipgloss.Width(h)
	}
	for _, r := range g.Rows {
		for c := range min(len(r), len(widths)) {
			widths[c] = max(widths[c], lipgloss.Width(r[c]))
		}
	}
	over := -width
	for _, w := range widths {
		over += w + gridPad
	}
	if over > 0 && g.Flex >= 0 && g.Flex < len(widths) {
		cut := min(over, max(widths[g.Flex]-8, 0))
		widths[g.Flex] -= cut
		over -= cut
	}
	for _, allowRight := range []bool{false, true} {
		for over > 0 {
			widest := -1
			for c, w := range widths {
				if (allowRight || !g.Right[c]) && w > 4 && (widest < 0 || w > widths[widest]) {
					widest = c
				}
			}
			if widest < 0 {
				break
			}
			widths[widest]--
			over--
		}
	}
	return ClampLines(g.table(widths).Render(), width, -1)
}

// fit cuts every line of s to w cells.
func fit(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = Truncate(l, w)
		}
	}
	return strings.Join(lines, "\n")
}

func (g Grid) table(widths []int) *table.Table {
	headers := make([]string, len(g.Headers))
	for c, h := range g.Headers {
		headers[c] = fit(h, widths[c])
	}
	rows := make([][]string, len(g.Rows))
	for i, r := range g.Rows {
		rows[i] = make([]string, len(r))
		for c, cell := range r {
			if c < len(widths) {
				cell = fit(cell, widths[c])
			}
			rows[i][c] = cell
		}
	}
	return table.New().
		// Wrap(false) would cut multi-line cells to their first line; cells
		// already fit their column, so nothing actually wraps.
		Headers(headers...).Rows(rows...).Wrap(true).
		Border(lipgloss.NormalBorder()).BorderStyle(theme.Rule).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderRow(false).BorderHeader(true).
		StyleFunc(func(r, c int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(gridPad)
			switch {
			case r == table.HeaderRow:
				s = s.Bold(true).Foreground(theme.Subtle)
			case r == g.Selected:
				s = s.Background(theme.Selection).Foreground(theme.Text).Bold(true)
			case g.Dim[r]:
				s = s.Foreground(theme.Subtle).Strikethrough(true)
			}
			if g.Right[c] {
				s = s.Align(lipgloss.Right)
			}
			return s
		})
}
