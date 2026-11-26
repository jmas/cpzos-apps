package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func isQuit(cmd tea.Cmd) bool {
	return cmd != nil &&
		reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func fakeHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	helper := filepath.Join(dir, "app-install")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CPZ_ARGS_LOG\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_APP_INSTALL", helper)
	t.Setenv("CPZ_ARGS_LOG", logPath)
	return logPath
}

func recordedArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRootListsUpdateAndAbout(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	ids := make([]string, 0, len(m.root.Items))
	for _, it := range m.root.Items {
		ids = append(ids, it.ID)
	}
	if got, want := strings.Join(ids, ","), "update,about"; got != want {
		t.Fatalf("root items = %q, want %q", got, want)
	}
}

func TestUpdateAsksFirstAndCancels(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewConfirm {
		t.Fatalf("view = %v, want confirm", m.view)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if cmd != nil {
		t.Fatalf("cancel returned a command")
	}
	if m.view != viewRoot {
		t.Fatalf("view = %v, want root", m.view)
	}
}

func TestUpdateYesRunsTheHelper(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	logPath := fakeHelper(t)
	m := New()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	// The dialog starts on No; Right selects Yes.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = nm.(Model)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewUpdating {
		t.Fatalf("view = %v, want updating", m.view)
	}
	if cmd == nil {
		t.Fatal("update returned no command")
	}
	if out := m.View(); !strings.Contains(out, "Оновлюю систему…") {
		t.Fatalf("working text missing:\n%s", out)
	}
	msg, ok := cmd().(updateMsg)
	if !ok {
		t.Fatal("update command returned a non-updateMsg")
	}
	if msg.err != nil {
		t.Fatalf("update command: %v", msg.err)
	}
	if got, want := recordedArgs(t, logPath), "update-system\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
}

func TestUpdateSuccessShowsRestartNote(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	nm, _ := m.Update(updateMsg{})
	m = nm.(Model)
	if m.view != viewRoot {
		t.Fatalf("view = %v, want root", m.view)
	}
	if m.ready {
		t.Fatal("About should reload after an update")
	}
	if out := m.View(); !strings.Contains(out, "Оновлено; перезапустіть UI") {
		t.Fatalf("success status missing:\n%s", out)
	}
}

func TestUpdateFailureShowsReason(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	nm, _ := m.Update(updateMsg{
		out: ":: Synchronizing package databases...\nerror: failed retrieving file 'core.db'\n",
		err: errors.New("exit 1"),
	})
	m = nm.(Model)
	if m.view != viewRoot {
		t.Fatalf("view = %v, want root", m.view)
	}
	if !strings.Contains(m.status, "Не вдалося оновити:") ||
		!strings.Contains(m.status, "failed retrieving file") {
		t.Fatalf("failure status = %q", m.status)
	}
}

func TestUpdateBlocksKeysUntilDone(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	m.view = viewUpdating
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("Esc returned a command while updating")
	}
	if m.view != viewUpdating {
		t.Fatalf("view = %v, want updating", m.view)
	}
}

func TestEscExitsRootAndReturnsFromAbout(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
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
