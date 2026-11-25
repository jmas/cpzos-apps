package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Item is one row: focus marker, name, right-hand state column.
type Item struct {
	ID     string
	Name   string
	Right  string
	Marker string
	Dim    bool
}

// List is a panel-sized list with a single focus.
type List struct {
	Items  []Item
	Cursor int
	Offset int
}

// SetItems replaces the rows and keeps the cursor in range.
func (l *List) SetItems(items []Item) {
	l.Items = items
	if l.Cursor >= len(items) {
		l.Cursor = len(items) - 1
	}
	if l.Cursor < 0 {
		l.Cursor = 0
	}
	l.Offset = 0
}

// Move moves the cursor and keeps it visible.
func (l *List) Move(delta, visible int) {
	if len(l.Items) == 0 {
		return
	}
	l.Cursor += delta
	if l.Cursor < 0 {
		l.Cursor = 0
	}
	if l.Cursor >= len(l.Items) {
		l.Cursor = len(l.Items) - 1
	}
	if visible < 1 {
		visible = 1
	}
	if l.Cursor < l.Offset {
		l.Offset = l.Cursor
	}
	if l.Cursor >= l.Offset+visible {
		l.Offset = l.Cursor - visible + 1
	}
}

func (l *List) MoveFirst() { l.Cursor = 0; l.Offset = 0 }

// EnsureVisible scrolls the window so the cursor is on screen.
func (l *List) EnsureVisible(visible int) {
	if visible < 1 {
		visible = 1
	}
	if l.Cursor < l.Offset {
		l.Offset = l.Cursor
	}
	if l.Cursor >= l.Offset+visible {
		l.Offset = l.Cursor - visible + 1
	}
	if l.Offset < 0 {
		l.Offset = 0
	}
}

func (l *List) MoveLast(visible int) {
	if len(l.Items) == 0 {
		return
	}
	l.Cursor = len(l.Items) - 1
	if visible < 1 {
		visible = 1
	}
	l.Offset = l.Cursor - visible + 1
	if l.Offset < 0 {
		l.Offset = 0
	}
}

// Selected returns the focused item, if any.
func (l *List) Selected() (Item, bool) {
	if l.Cursor < 0 || l.Cursor >= len(l.Items) {
		return Item{}, false
	}
	return l.Items[l.Cursor], true
}

// Render draws exactly visible rows at the given width.
func (l *List) Render(width, visible int) []string {
	rows := make([]string, 0, visible)
	if width < 3 || visible < 1 {
		return rows
	}
	end := l.Offset + visible
	if end > len(l.Items) {
		end = len(l.Items)
	}
	for i := l.Offset; i < end; i++ {
		item := l.Items[i]
		focused := i == l.Cursor
		rows = append(rows, renderRow(item, width, focused))
	}
	for len(rows) < visible {
		rows = append(rows, strings.Repeat(" ", width))
	}
	return rows
}

func renderRow(item Item, width int, focused bool) string {
	prefix := "  "
	if focused {
		prefix = "> "
	}
	right := Truncate(item.Right, width/2)
	if item.Marker != "" {
		if right != "" {
			right += " " + item.Marker
		} else {
			right = item.Marker
		}
	}
	rightW := runewidth.StringWidth(right)
	avail := width - 2 - 1 - rightW
	if right == "" {
		avail = width - 2
	}
	if avail < 3 {
		avail = 3
	}
	name := Truncate(item.Name, avail)
	row := prefix + name
	if right != "" {
		gap := width - runewidth.StringWidth(row) - rightW
		if gap < 1 {
			gap = 1
		}
		row += strings.Repeat(" ", gap) + right
	}
	row = Pad(Truncate(row, width), width)
	switch {
	case focused:
		return FocusStyle.Render(row)
	case item.Dim:
		return DimStyle.Render(row)
	default:
		return NormalStyle.Render(row)
	}
}

// Truncate cuts a plain string to w cells, with an ellipsis when needed.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// Pad right-pads a plain string to w cells.
func Pad(s string, w int) string {
	n := runewidth.StringWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// Fit truncates and pads a plain string to exactly w cells.
func Fit(s string, w int) string {
	return Pad(Truncate(s, w), w)
}

// Width is the display width of a possibly styled string.
func Width(s string) int { return lipgloss.Width(s) }
