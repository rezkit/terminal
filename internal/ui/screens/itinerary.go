package screens

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/rezkit/tui/internal/api"
	"github.com/rezkit/tui/internal/ui/components"
	"github.com/rezkit/tui/internal/ui/theme"
)

const (
	itinStatusW = 12 // "Waitlisted" plus a gap
	itinPriceW  = 12
	itinMinTrak = 18 // narrower than this and a timeline is useless
)

// itinerary lays reservations out on a day axis: stays as bars from check-in
// to check-out, single-day items as markers, coloured by status.
type itinerary struct {
	labelW, trackW int
	first          time.Time // first day on the axis (civil date, UTC)
	days           int       // days on the axis
}

func newItinerary(rs []api.Reservation, width int) (itinerary, bool) {
	it := itinerary{labelW: min(max(width*30/100, 16), 42)}
	it.trackW = width - it.labelW - itinStatusW - itinPriceW - 1
	if it.trackW < itinMinTrak {
		return it, false
	}
	var last time.Time
	for _, r := range rs {
		s, e, ok := resDays(r)
		if !ok {
			continue
		}
		if it.first.IsZero() || s.Before(it.first) {
			it.first = s
		}
		if e.After(last) {
			last = e
		}
		for _, l := range r.Legs { // an overnight leg can land after the reservation ends
			if a := wallDay(l.Arrive); !l.Arrive.IsZero() && a.After(last) {
				last = a
			}
		}
	}
	if !it.first.IsZero() {
		it.days = dayDiff(it.first, last) + 1
	}
	return it, true
}

// resDays returns a reservation's first and last civil day; a missing or
// earlier end collapses to the start.
func resDays(r api.Reservation) (time.Time, time.Time, bool) {
	if r.Start.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	s := civilDay(r.Start)
	e := s
	if !r.End.IsZero() {
		if ce := civilDay(r.End); ce.After(s) {
			e = ce
		}
	}
	return s, e, true
}

func civilDay(t time.Time) time.Time {
	c := api.Civil(t)
	return time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, time.UTC)
}

func dayDiff(a, b time.Time) int { return int(math.Round(b.Sub(a).Hours() / 24)) }

// x maps a (fractional) day offset to a track column.
func (it itinerary) x(day float64) int {
	return min(int(day*float64(it.trackW)/float64(it.days)), it.trackW-1)
}

// axis is the header line: day labels thinned out to fit.
func (it itinerary) axis() string {
	pad := strings.Repeat(" ", it.labelW+itinStatusW)
	if it.days == 0 {
		return pad
	}
	perDay := float64(it.trackW) / float64(it.days)
	track := make([]string, 0, it.days)
	next := 0
	used := 0
	for d := range it.days {
		t := it.first.AddDate(0, 0, d)
		label := t.Format("Jan 2")
		switch {
		case perDay >= 6:
			label = t.Format("Mon")[:2] + " " + t.Format("2")
		case perDay >= 3 && t.Day() != 1 && d != 0:
			label = t.Format("2")
		}
		col := it.x(float64(d))
		if col < next || col+len(label) > it.trackW {
			continue
		}
		track = append(track, strings.Repeat(" ", col-used), label)
		used = col + len(label)
		next = used + 1
	}
	return pad + theme.Muted.Render(strings.Join(track, "")+strings.Repeat(" ", it.trackW-used)) +
		theme.Muted.Render(fmt.Sprintf("%*s", itinPriceW+1, "Price"))
}

// row renders one reservation; selected highlights its label.
// row renders one reservation; selected highlights its label. Reservations
// with transport legs get a ▸/▾ expander.
func (it itinerary) row(r api.Reservation, selected, expanded bool) string {
	gutter := "  "
	if len(r.Legs) > 0 {
		gutter = "▸ "
		if expanded {
			gutter = "▾ "
		}
	}
	w := it.labelW - 1
	var label string
	if selected {
		label = lipgloss.NewStyle().Background(theme.Selection).Foreground(theme.Text).Bold(true).
			Width(w).Render(components.Truncate(gutter+productLabel(r, true), w))
	} else {
		label = lipgloss.NewStyle().Width(w).Render(
			components.Truncate(theme.Accent2.Render(gutter)+productLabel(r, false), w))
	}
	status := lipgloss.NewStyle().Width(itinStatusW).Render(statusText(r.Status, false))
	return label + " " + status + it.track(r) + fmt.Sprintf("%*s", itinPriceW+1, priceOrDash(r.Price))
}

// legRows renders an expanded reservation's legs: route and service on the
// left, local departure–arrival times, a bar placed by the actual times, and
// the duration. A muted line lists seats when the API has them.
func (it itinerary) legRows(r api.Reservation, name func(passengerID string) string) []string {
	var out []string
	for i, l := range r.Legs {
		branch := "├ "
		if i == len(r.Legs)-1 {
			branch = "└ "
		}
		route := l.From.Label() + " → " + l.To.Label()
		label := theme.Rule.Render("  "+branch) + route
		if n := l.Number(); n != "" {
			label += theme.Muted.Render("  " + n)
		}
		label = lipgloss.NewStyle().Width(it.labelW - 1).Render(components.Truncate(label, it.labelW-1))
		times := l.Depart.Format("15:04") + "–" + l.Arrive.Format("15:04")
		dur := duration(l.Arrive.Sub(l.Depart))
		if days := dayDiff(wallDay(l.Depart), wallDay(l.Arrive)); days > 0 {
			dur += fmt.Sprintf(" +%dd", days)
		}
		out = append(out, label+" "+fmt.Sprintf("%-*s", itinStatusW, times)+it.legTrack(l)+
			theme.Muted.Render(fmt.Sprintf("%*s", itinPriceW+1, dur)))

		var seats []string
		class := ""
		for _, p := range l.Passengers {
			if p.Seat != "" {
				seats = append(seats, name(p.PassengerID)+" "+p.Seat)
			}
			if class == "" {
				class = p.Class
			}
		}
		if len(seats) > 0 || class != "" {
			info := strings.Join(nonEmpty(class, strings.Join(seats, ", ")), " · ")
			out = append(out, theme.Muted.Render(components.Truncate(
				strings.Repeat(" ", 6)+info, it.labelW+itinStatusW+it.trackW+itinPriceW+1)))
		}
	}
	return out
}

// legTrack draws the leg from departure to arrival at time-of-day precision.
func (it itinerary) legTrack(l api.Leg) string {
	if it.days == 0 || l.Depart.IsZero() {
		return strings.Repeat(" ", it.trackW)
	}
	from := it.x(it.pos(l.Depart))
	to := max(it.x(it.pos(l.Arrive)), from+1)
	to = min(to, it.trackW)
	return strings.Repeat(" ", from) + theme.Accent2.Render(strings.Repeat("━", to-from)) +
		strings.Repeat(" ", it.trackW-to)
}

// pos is t's (fractional) day offset on the axis, using t's own wall clock
// (stop-local time), clamped to the axis.
func (it itinerary) pos(t time.Time) float64 {
	frac := (float64(t.Hour()) + float64(t.Minute())/60) / 24
	p := float64(dayDiff(it.first, wallDay(t))) + frac
	return min(max(p, 0), float64(it.days)-0.01)
}

func duration(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < 0 {
		return "—"
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

func (it itinerary) track(r api.Reservation) string {
	s, e, ok := resDays(r)
	if !ok || it.days == 0 {
		return theme.Muted.Render(fmt.Sprintf("%-*s", it.trackW, "no dates"))
	}
	cells := make([]rune, it.trackW)
	for i := range cells {
		cells[i] = ' '
	}
	for d := range it.days { // a dot at the start of each day that has room
		if c := it.x(float64(d)); float64(it.trackW)/float64(it.days) >= 2 {
			cells[c] = '·'
		}
	}
	ds, de := dayDiff(it.first, s), dayDiff(it.first, e)
	var from, to int
	mark := '█'
	if ds == de { // single day: a marker mid-day
		from = it.x(float64(ds) + 0.5)
		to = from + 1
		mark = '◆'
	} else { // stay: check-in afternoon to check-out morning
		from, to = it.x(float64(ds)+0.5), it.x(float64(de)+0.5)
		to = max(to, from+1)
	}
	fill := lipgloss.NewStyle().Foreground(theme.StatusColor(r.Status))
	return theme.Rule.Render(string(cells[:from])) + fill.Render(strings.Repeat(string(mark), to-from)) +
		theme.Rule.Render(string(cells[to:]))
}

func priceOrDash(m *api.Money) string {
	if m == nil {
		return "—"
	}
	return api.FormatMoney(*m)
}

// wallDay is t's calendar day on its own wall clock (stop-local for legs).
func wallDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
