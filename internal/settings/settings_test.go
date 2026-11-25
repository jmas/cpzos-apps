package settings

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func isQuit(cmd tea.Cmd) bool {
	return cmd != nil &&
		reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func TestEscExitsRootAndReturnsFromAbout(t *testing.T) {
	m := New()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Fatal("Esc at the Settings root did not exit")
	}
	m.view = viewAbout
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if cmd != nil {
		t.Fatalf("Esc in About returned a command")
	}
	if m.view != viewRoot {
		t.Fatalf("view = %v, want root", m.view)
	}
}
