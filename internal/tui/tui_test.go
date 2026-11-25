package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestDpadKeys(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want Action
	}{
		{key('f'), ActionUp},
		{key('а'), ActionUp}, // ua layout, physical F
		{key('x'), ActionDown},
		{key('ч'), ActionDown},
		{key('z'), ActionLeft},
		{key('я'), ActionLeft},
		{key('c'), ActionRight},
		{key('с'), ActionRight},
		{key('h'), ActionHelp},
		{tea.KeyMsg{Type: tea.KeyEnter}, ActionConfirm},
		{tea.KeyMsg{Type: tea.KeyEsc}, ActionBack},
		{tea.KeyMsg{Type: tea.KeyTab}, ActionNextField},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, ActionPrevField},
		{tea.KeyMsg{Type: tea.KeyUp}, ActionUp},
		{tea.KeyMsg{Type: tea.KeyDown}, ActionDown},
		{tea.KeyMsg{Type: tea.KeyLeft}, ActionLeft},
		{tea.KeyMsg{Type: tea.KeyRight}, ActionRight},
		{tea.KeyMsg{Type: tea.KeyPgUp}, ActionPageUp},
		{tea.KeyMsg{Type: tea.KeyPgDown}, ActionPageDown},
		{tea.KeyMsg{Type: tea.KeyHome}, ActionFirst},
		{tea.KeyMsg{Type: tea.KeyEnd}, ActionLast},
	}
	for _, c := range cases {
		if got := ActionOf(c.msg, false); got != c.want {
			t.Errorf("%q: got %v, want %v", c.msg.String(), got, c.want)
		}
	}
}

func TestTextModeKeepsLetters(t *testing.T) {
	for _, r := range []rune{'f', 'x', 'z', 'c', 'а', 'ч', 'я', 'с', 'h'} {
		if got := ActionOf(key(r), true); got != ActionNone {
			t.Errorf("%q in text mode: got %v", r, got)
		}
	}
	if got := ActionOf(tea.KeyMsg{Type: tea.KeyEsc}, true); got != ActionBack {
		t.Errorf("esc must stay back in text mode, got %v", got)
	}
	if got := ActionOf(tea.KeyMsg{Type: tea.KeyEnter}, true); got != ActionConfirm {
		t.Errorf("enter must stay confirm in text mode, got %v", got)
	}
}

func TestListFitsPanelWidth(t *testing.T) {
	l := List{Items: []Item{
		{ID: "a", Name: "Дуже довга назва додатка, яка не влізе", Right: "1.2.3", Marker: "*"},
		{ID: "b", Name: "btop"},
	}}
	for _, line := range l.Render(24, 2) {
		if w := Width(line); w != 24 {
			t.Errorf("row width %d, want 24: %q", w, line)
		}
	}
}

func TestTruncateUsesCells(t *testing.T) {
	if got := Truncate("абвгде", 4); got != "абв…" {
		t.Errorf("got %q", got)
	}
}
