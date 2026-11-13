package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestGridFitsAndKeepsMultilineRows(t *testing.T) {
	g := Grid{
		Headers:  []string{"Product", "Element", "Type", "Price"},
		Right:    map[int]bool{3: true},
		Selected: -1,
		Flex:     0,
		Rows: [][]string{
			{"Airport transfer\nDriver meets you in arrivals with a name board", "Private car (up to 4)", "Transfer", "£146.99"},
			{"Tea ceremony", "—", "Activity", "£391.99"},
		},
	}
	for _, w := range []int{120, 60, 40} {
		lines := strings.Split(g.Render(w), "\n")
		if len(lines) != 2+3 { // header, rule, a two-line row and a one-line row
			t.Fatalf("width %d: %d lines:\n%s", w, len(lines), strings.Join(lines, "\n"))
		}
		for _, l := range lines {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line is %d wide: %q", w, lipgloss.Width(l), l)
			}
		}
		if !strings.Contains(lines[4], "£391.99") || !strings.HasPrefix(lines[3], "Driver") {
			t.Errorf("width %d: rows out of place:\n%s", w, strings.Join(lines, "\n"))
		}
	}
}

func TestGridNeverExceedsWidth(t *testing.T) {
	g := Grid{
		Headers:  []string{"Product", "Element", "Type", "Status", "Qty", "Dates", "Balance due", "Price"},
		Right:    map[int]bool{4: true, 7: true},
		Selected: 0,
		Flex:     0,
		Rows: [][]string{
			{"Riffelalp Resort\nSea view", "Double room, half board", "Accommodation", "CONFIRMED", "1", "1–8 Jun", "Mon 1 May", "£1,234.00"},
		},
	}
	// Narrower than the columns can shrink to: lines are cut, not wrapped.
	for _, w := range []int{40, 20} {
		lines := strings.Split(g.Render(w), "\n")
		if len(lines) != 4 {
			t.Fatalf("width %d: %d lines, want 4:\n%s", w, len(lines), strings.Join(lines, "\n"))
		}
		for _, l := range lines {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line is %d wide: %q", w, lipgloss.Width(l), l)
			}
		}
	}
}
