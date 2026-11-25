package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Input is a one-line filter field. It has no cursor movement: backspace and
// typing are enough on a 45-column panel.
type Input struct {
	Prompt string
	Value  string
	Active bool
}

// Handle processes a key while the field has focus.
func (i *Input) Handle(msg tea.KeyMsg) bool {
	if !i.Active {
		return false
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if i.Value != "" {
			r := []rune(i.Value)
			i.Value = string(r[:len(r)-1])
			return true
		}
	case tea.KeyCtrlU:
		if i.Value != "" {
			i.Value = ""
			return true
		}
	case tea.KeySpace:
		i.Value += " "
		return true
	case tea.KeyRunes:
		var b strings.Builder
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				b.WriteRune(r)
			}
		}
		if b.Len() > 0 {
			i.Value += b.String()
			return true
		}
	}
	return false
}

// Clear empties the field.
func (i *Input) Clear() { i.Value = "" }

// Render draws the field on one line.
func (i *Input) Render(width int) string {
	text := i.Prompt + ": " + i.Value
	if i.Value == "" {
		text = i.Prompt + ":"
	}
	if i.Active {
		text += "_"
	}
	return Fit(text, width)
}
