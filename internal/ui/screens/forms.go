package screens

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/theme"
)

// formModal is a huh form in a modal: submit builds the result message,
// esc cancels.
type formModal struct {
	title string
	form  *huh.Form
}

func newFormModal(title string, submit func() tea.Msg, fields ...huh.Field) *formModal {
	m := &formModal{title: title}
	m.form = huh.NewForm(huh.NewGroup(fields...)).
		WithShowHelp(true).WithTheme(huh.ThemeFunc(huh.ThemeCharm)).WithKeyMap(filterKeyMap())
	m.form.SubmitCmd = submit
	m.form.CancelCmd = ui.CloseModal
	return m
}

func (m *formModal) Init() tea.Cmd { return m.form.Init() }

func (m *formModal) Title() string { return m.title }

func (m *formModal) Update(msg tea.Msg) (ui.Modal, tea.Cmd) {
	model, cmd := m.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		m.form = f
	}
	return m, cmd
}

func (m *formModal) View(maxWidth, maxHeight int) string {
	m.form.WithWidth(min(maxWidth, 60))
	return m.form.View()
}

// dateRange holds the text of a pair of "created from/to" inputs.
type dateRange struct{ from, to string }

func newDateRange(from, to time.Time) *dateRange {
	r := &dateRange{}
	if !from.IsZero() {
		r.from = from.Format(dateLayout)
	}
	if !to.IsZero() {
		r.to = to.Format(dateLayout)
	}
	return r
}

func (r *dateRange) fields() []huh.Field {
	return []huh.Field{
		huh.NewInput().Title("Created from").Placeholder("YYYY-MM-DD").Value(&r.from).Validate(validDate),
		huh.NewInput().Title("Created to").Placeholder("YYYY-MM-DD").Value(&r.to).Validate(validDate),
	}
}

// bounds parses the range; the end is inclusive (the end of that day).
func (r *dateRange) bounds() (from, to time.Time) {
	from, _ = parseDate(r.from)
	if t, err := parseDate(r.to); err == nil {
		to = t.AddDate(0, 0, 1).Add(-time.Second)
	}
	return from, to
}

// dateParts describes a created range for a filter summary.
func dateParts(from, to time.Time) []string {
	var parts []string
	if !from.IsZero() {
		parts = append(parts, "created from "+from.Format(dateLayout))
	}
	if !to.IsZero() {
		parts = append(parts, "created to "+to.Format(dateLayout))
	}
	return parts
}

// filterLine is the one-line summary above a list.
func filterLine(parts []string) string {
	if len(parts) == 0 {
		return theme.Muted.Render("No filter · press / to filter")
	}
	return theme.Bold.Render("Filter: ") + strings.Join(parts, ", ") + theme.Muted.Render("  (x to clear)")
}

func containsPart(label, v string) []string {
	if v == "" {
		return nil
	}
	return []string{fmt.Sprintf("%s contains %q", label, v)}
}
