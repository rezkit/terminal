package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/rezkit/tui/internal/ui/theme"
)

// Grid is a static, borderless table with a header rule, for detail views.
// Rows render one line each (no wrapping).
type Grid struct {
	Headers  []string
	Rows     [][]string
	Right    map[int]bool // right-aligned columns
	Dim      map[int]bool // rows shown muted and struck through (cancelled, void)
	Selected int          // highlighted row, or -1
	Flex     int          // column truncated first when too wide, or -1
}

// Render draws the grid at its natural width, shrinking the Flex column (and
// then every column) to fit within width.
func (g Grid) Render(width int) string {
	out := g.render(-1)
	if over := lipgloss.Width(firstLine(out)) - width; over > 0 && g.Flex >= 0 {
		maxW := 0
		for _, r := range g.Rows {
			if g.Flex < len(r) {
				maxW = max(maxW, lipgloss.Width(r[g.Flex]))
			}
		}
		out = g.render(max(maxW-over, 8))
	}
	if lipgloss.Width(firstLine(out)) > width {
		return g.table(-1).Width(width).Render()
	}
	return out
}

func (g Grid) render(flexW int) string { return g.table(flexW).Render() }

func (g Grid) table(flexW int) *table.Table {
	rows := g.Rows
	if flexW > 0 {
		rows = make([][]string, len(g.Rows))
		for i, r := range g.Rows {
			rows[i] = append([]string(nil), r...)
			if g.Flex < len(r) {
				rows[i][g.Flex] = Truncate(r[g.Flex], flexW)
			}
		}
	}
	return table.New().
		Headers(g.Headers...).Rows(rows...).Wrap(false).
		Border(lipgloss.NormalBorder()).BorderStyle(theme.Rule).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderRow(false).BorderHeader(true).
		StyleFunc(func(r, c int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(2)
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

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
