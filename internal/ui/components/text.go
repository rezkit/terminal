package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Truncate cuts s (which may contain ANSI styling) to w cells with an ellipsis.
func Truncate(s string, w int) string { return ansi.Truncate(s, max(w, 0), "…") }

// Spread places left and right on one line of exactly width w, truncating
// left (and dropping right if it alone doesn't fit).
func Spread(w int, left, right string) string {
	rw := lipgloss.Width(right)
	if rw+1 > w {
		return Truncate(left, w)
	}
	left = Truncate(left, w-rw-1)
	return left + strings.Repeat(" ", w-lipgloss.Width(left)-rw) + right
}

// ClampLines truncates every line of s to w cells and keeps at most h lines.
func ClampLines(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h >= 0 && len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = Truncate(l, w)
		}
	}
	return strings.Join(lines, "\n")
}
