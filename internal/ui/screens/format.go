package screens

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui/components"
	"github.com/rezkit/tui/internal/ui/theme"
)

// Formatting helpers shared by the detail screens.

func badge(status string) string {
	return lipgloss.NewStyle().Foreground(theme.StatusColor(status)).Render("● " + api.StatusLabel(status))
}

func heading(title string, n int) string {
	h := theme.Heading.Render(strings.ToUpper(title))
	if n >= 0 {
		h += theme.Muted.Render(fmt.Sprintf(" %d", n))
	}
	return h
}

func kv(labelW int, pairs ...string) string {
	var lines []string
	for i := 0; i+1 < len(pairs); i += 2 {
		lines = append(lines, theme.Muted.Width(labelW).Render(pairs[i])+pairs[i+1])
	}
	return strings.Join(lines, "\n")
}

func paidBar(paid, total api.Money, w int) string {
	w = max(w, 4)
	n, pct := 0, 0
	if total.Minor > 0 {
		n = int(min(max(int64(w)*paid.Minor/total.Minor, 0), int64(w)))
		pct = int(paid.Minor * 100 / total.Minor)
	}
	return lipgloss.NewStyle().Foreground(theme.Success).Render(strings.Repeat("█", n)) +
		theme.Rule.Render(strings.Repeat("░", w-n)) + theme.Muted.Render(fmt.Sprintf(" %d%% paid", pct))
}

func outstanding(m api.Money) string {
	if m.Minor > 0 {
		return theme.Warn.Render(api.FormatMoney(m))
	}
	return api.FormatMoney(m)
}

func travelling(d *api.BookingDetail) int {
	n := 0
	for _, p := range d.Passengers {
		if p.Cancelled.IsZero() {
			n++
		}
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2 Jan 2006 15:04")
}

func day(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return api.Civil(t).Format("2 Jan 2006")
}

// dateSpan formats a civil date range with the given layout.
func dateSpan(start, end time.Time, layout string) string {
	switch {
	case start.IsZero() && end.IsZero():
		return "—"
	case end.IsZero() || civilDay(end).Equal(civilDay(start)):
		return api.Civil(start).Format(layout)
	case start.IsZero():
		return "until " + api.Civil(end).Format(layout)
	}
	return api.Civil(start).Format(layout) + " → " + api.Civil(end).Format(layout)
}

// compactSpan is a table-friendly range: "Sat 11 Jul", "11–15 Jul", "30 Jun–2 Jul".
func compactSpan(start, end time.Time) string {
	if start.IsZero() {
		return "—"
	}
	s := api.Civil(start)
	if end.IsZero() || !civilDay(end).After(civilDay(start)) {
		return s.Format("Mon 2 Jan")
	}
	e := api.Civil(end)
	if s.Year() == e.Year() && s.Month() == e.Month() {
		return fmt.Sprintf("%d–%s", s.Day(), e.Format("2 Jan"))
	}
	return s.Format("2 Jan") + "–" + e.Format("2 Jan")
}

// tabBar renders numbered tabs ("1 Overview  2 Bookings (3)") with the
// active one highlighted, over a full-width rule. suffixes may be shorter
// than titles.
func tabBar(titles, suffixes []string, active, w int) string {
	parts := make([]string, len(titles))
	for i, title := range titles {
		label := fmt.Sprintf("%d %s", i+1, title)
		if i < len(suffixes) {
			label += suffixes[i]
		}
		if i == active {
			parts[i] = theme.TabOn.Render(label)
		} else {
			parts[i] = theme.Tab.Render(label)
		}
	}
	return "\n" + strings.Join(parts, " ") + "\n" + theme.Rule.Render(strings.Repeat("─", w))
}

// notesView lists notes in full; nil notes means the API withheld them.
func notesView(notes []api.Note, w int) string {
	switch {
	case notes == nil:
		return theme.Muted.Render("Notes are not available (access denied)")
	case len(notes) == 0:
		return theme.Muted.Render("No notes")
	}
	parts := make([]string, len(notes))
	for i, n := range notes {
		parts[i] = theme.Bold.Render(n.Title) + theme.Muted.Render("  "+stamp(n.Created)) + "\n" +
			lipgloss.NewStyle().Width(w).Render(n.Content)
	}
	return strings.Join(parts, "\n\n")
}

// notesPreview shows the first few notes for an overview, pointing at the
// Notes tab (key) for the rest.
func notesPreview(notes []api.Note, key string) string {
	switch {
	case notes == nil:
		return theme.Muted.Render("Not available (access denied)")
	case len(notes) == 0:
		return theme.Muted.Render("No notes")
	}
	var out string
	for i, n := range notes {
		if i == 2 {
			out += theme.Muted.Render(fmt.Sprintf("…and %d more (press %s)", len(notes)-2, key))
			break
		}
		out += theme.Bold.Render(n.Title) + "\n" + n.Content + "\n"
	}
	return out
}

// columns lays left and right side by side when there is room, otherwise
// stacks them.
func columns(left, right string, w int) string {
	if w < 90 {
		return left + "\n\n" + lipgloss.NewStyle().Width(w).Render(right)
	}
	half := (w - 4) / 2
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(half).Render(components.ClampLines(left, half, -1)), "    ",
		lipgloss.NewStyle().Width(half).Render(right))
}

// yearSpan is compactSpan with the year, for lists spanning years:
// "11–15 Jul 2026", "30 Dec 2026–2 Jan 2027".
func yearSpan(start, end time.Time) string {
	if start.IsZero() {
		return "—"
	}
	s := api.Civil(start)
	if end.IsZero() || !civilDay(end).After(civilDay(start)) {
		return s.Format("2 Jan 2006")
	}
	e := api.Civil(end)
	if s.Year() != e.Year() {
		return s.Format("2 Jan 2006") + "–" + e.Format("2 Jan 2006")
	}
	return compactSpan(start, end) + " " + e.Format("2006")
}
