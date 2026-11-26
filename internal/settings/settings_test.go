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

func updateMsgFrom(t *testing.T, cmd tea.Cmd) updateMsg {
	t.Helper()
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if u, ok := c().(updateMsg); ok {
				return u
			}
		}
	}
	if u, ok := msg.(updateMsg); ok {
		return u
	}
	t.Fatalf("update command returned %T", msg)
	return updateMsg{}
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

func rootIndex(m Model, id string) int {
	for i, it := range m.root.Items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func selectRoot(m Model, id string) Model {
	m.root.Cursor = rootIndex(m, id)
	return m
}

func TestRootListsSettings(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	ids := make([]string, 0, len(m.root.Items))
	for _, it := range m.root.Items {
		ids = append(ids, it.ID)
	}
	if got, want := strings.Join(ids, ","), "language,network,display,power,update,about"; got != want {
		t.Fatalf("root items = %q, want %q", got, want)
	}
}

func TestUpdateAsksFirstAndCancels(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := selectRoot(New(), "update")
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
	m := selectRoot(New(), "update")
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
	msg := updateMsgFrom(t, cmd)
	if msg.err != nil {
		t.Fatalf("update command: %v", msg.err)
	}
	if got, want := recordedArgs(t, logPath), "update-system\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
}

func TestUpdatingEscCancels(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	m.view = viewUpdating
	cancelled := false
	m.cancel = func() { cancelled = true }

	nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if !cancelled || !m.cancelling {
		t.Fatalf("cancelled = %v, cancelling = %v", cancelled, m.cancelling)
	}
	if out := m.View(); !strings.Contains(out, "Скасовую оновлення") {
		t.Fatalf("cancelling text missing:\n%s", out)
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

func TestLanguageSwitchWritesUserFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	m = selectRoot(m, "language")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewLanguage {
		t.Fatalf("view = %v, want language", m.view)
	}
	// English is the first row; the active row is still Ukrainian.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.lang != "en" || m.tr.Lang != "en" {
		t.Fatalf("language = %q/%q, want en", m.lang, m.tr.Lang)
	}
	b, err := os.ReadFile(filepath.Join(home, ".config", "cpz", "language"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "en" {
		t.Fatalf("language file = %q", b)
	}
	if item := m.root.Items[rootIndex(m, "language")]; item.Right != "en" {
		t.Fatalf("root language detail = %q", item.Right)
	}
}

func fakeBacklight(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	script := filepath.Join(dir, "backlight")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CPZ_BACKLIGHT_LOG\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_BACKLIGHT", script)
	t.Setenv("CPZ_BACKLIGHT_LOG", logPath)
	return logPath
}

func TestBrightnessAppliesAndSaves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	logPath := fakeBacklight(t)
	m := selectRoot(New(), "display")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewDisplay {
		t.Fatalf("view = %v, want display", m.view)
	}
	// Brightness is the first row in the Display menu.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewBrightness {
		t.Fatalf("view = %v, want brightness", m.view)
	}
	// The list starts at 100%.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("brightness returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if got, want := recordedArgs(t, logPath), "set\n100\n"; got != want {
		t.Fatalf("backlight args = %q, want %q", got, want)
	}
	if m.brightness != 100 {
		t.Fatalf("brightness = %d, want 100", m.brightness)
	}
	b, err := os.ReadFile(filepath.Join(home, ".config", "cpz", "display"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "brightness=100" {
		t.Fatalf("display file = %q", b)
	}
}

func TestPowerRebootCallsHelper(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	logPath := fakeHelper(t)
	m := selectRoot(New(), "power")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewPower {
		t.Fatalf("view = %v, want power", m.view)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewConfirm {
		t.Fatalf("view = %v, want confirm", m.view)
	}
	// No is focused; Right selects Yes.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = nm.(Model)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewPowering {
		t.Fatalf("view = %v, want powering", m.view)
	}
	if cmd == nil {
		t.Fatal("power returned no command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if pm, ok := c().(powerMsg); ok {
				msg = pm
				break
			}
		}
	}
	if pm, ok := msg.(powerMsg); !ok || pm.err != nil {
		t.Fatalf("power command returned %T (%v)", msg, pm.err)
	}
	if got, want := recordedArgs(t, logPath), "power\nreboot\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
	if out := m.View(); !strings.Contains(out, "Перезавантаження") {
		t.Fatalf("powering text missing:\n%s", out)
	}
}

func TestSubscreenEscReturnsToRoot(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	for _, v := range []view{viewLanguage, viewNetwork, viewDisplay, viewPower, viewAbout} {
		m := New()
		m.view = v
		nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = nm.(Model)
		if cmd != nil {
			t.Fatalf("view %v: Esc returned a command", v)
		}
		if m.view != viewRoot {
			t.Fatalf("view %v: Esc landed on %v", v, m.view)
		}
	}
}

func TestRootFrameAtPanelSize(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	m := New()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 45, Height: 13})
	m = nm.(Model)
	out := m.View()
	t.Log("\n" + out)
	for _, want := range []string{
		"Мова й регіон", "Мережа", "Екран", "Живлення", "Оновити систему", "Про систему",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("frame misses %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 45 {
			t.Fatalf("line over 45 columns: %q", line)
		}
	}
}

func writeTool(t *testing.T, dir, name, body string) {
	t.Helper()
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestWiFiConnectWithPassword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	t.Setenv("CPZ_WIFI_DEVICE", "wlan0")
	logPath := filepath.Join(t.TempDir(), "cmd.log")
	t.Setenv("CPZ_CMD_LOG", logPath)
	dir := t.TempDir()
	writeTool(t, dir, "iwctl", `
printf '%s\n' "$*" >> "$CPZ_CMD_LOG"
case "$*" in
  "device wlan0 show") echo "Powered: yes" ;;
  "station wlan0 get-networks")
    printf '                              Network name  Security  Signal\n'
    printf '%s\n' '---------------------------------------------------------------------------'
    printf '                              HomeNet       psk       ****\n'
    printf '                              Cafe          open      **\n'
    ;;
  "known-networks list") : ;;
esac
exit 0
`)
	writeTool(t, dir, "bluetoothctl", `
printf '%s\n' "$*" >> "$CPZ_CMD_LOG"
case "$1" in
  show) echo "Powered: no" ;;
esac
exit 0
`)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	m := selectRoot(New(), "network")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewNetwork {
		t.Fatalf("view = %v, want network", m.view)
	}
	if cmd == nil {
		t.Fatal("opening Connections returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !m.netLoaded || !m.wifiOn || len(m.wifiNets) != 2 {
		t.Fatalf("network state: loaded %v on %v nets %d", m.netLoaded, m.wifiOn, len(m.wifiNets))
	}

	// Wi-Fi is the first row in Connections.
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewWiFi {
		t.Fatalf("view = %v, want wifi", m.view)
	}
	if cmd == nil {
		t.Fatal("opening Wi-Fi returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if got, want := len(m.wifi.Items), 5; got != want {
		t.Fatalf("wifi rows = %d, want %d", got, want)
	}

	// toggle, scan, HomeNet, Cafe, saved
	m.wifi.Cursor = 2
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewWiFiPassword {
		t.Fatalf("view = %v, want password", m.view)
	}
	if cmd != nil {
		t.Fatal("password view returned a command")
	}
	for _, r := range "secret" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewWiFi {
		t.Fatalf("view = %v, want wifi", m.view)
	}
	if cmd == nil {
		t.Fatal("connect returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !strings.Contains(m.status, "Підключено") {
		t.Fatalf("status = %q", m.status)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "--passphrase secret station wlan0 connect HomeNet") {
		t.Fatalf("connect was not called with the passphrase:\n%s", log)
	}
}

func TestBluetoothPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	t.Setenv("CPZ_WIFI_DEVICE", "wlan0")
	logPath := filepath.Join(t.TempDir(), "cmd.log")
	t.Setenv("CPZ_CMD_LOG", logPath)
	dir := t.TempDir()
	writeTool(t, dir, "iwctl", `
printf '%s\n' "$*" >> "$CPZ_CMD_LOG"
case "$*" in
  "device wlan0 show") echo "Powered: no" ;;
  "known-networks list") : ;;
esac
exit 0
`)
	writeTool(t, dir, "bluetoothctl", `
printf '%s\n' "$*" >> "$CPZ_CMD_LOG"
case "$1" in
  show) echo "Powered: yes" ;;
  devices) echo "Device AA:BB:CC:DD:EE:FF Phone" ;;
  pair) echo "Pairing successful" ;;
esac
exit 0
`)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	m := selectRoot(New(), "network")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !m.btOn || len(m.btDevs) != 1 {
		t.Fatalf("bluetooth state: on %v devices %d", m.btOn, len(m.btDevs))
	}

	// Bluetooth is the second row in Connections.
	m.network.Cursor = 1
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewBT {
		t.Fatalf("view = %v, want bt", m.view)
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if got, want := len(m.bt.Items), 3; got != want {
		t.Fatalf("bt rows = %d, want %d", got, want)
	}

	// toggle, scan, Phone
	m.bt.Cursor = 2
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewBTDevice {
		t.Fatalf("view = %v, want bt device", m.view)
	}
	// Pair is the first action.
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("pair returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !strings.Contains(m.status, "Готово") {
		t.Fatalf("status = %q", m.status)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "pair AA:BB:CC:DD:EE:FF") {
		t.Fatalf("pair was not called:\n%s", log)
	}
}

func TestDisplayOutputCyclesAndApplies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	logPath := fakeHelper(t)
	m := selectRoot(New(), "display")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewDisplay {
		t.Fatalf("view = %v, want display", m.view)
	}
	// Brightness is first; Output is second.
	m.display.Cursor = 1
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("output returned no command")
	}
	if m.output != "panel" {
		t.Fatalf("output = %q, want panel", m.output)
	}
	if got, want := Output(), "panel"; got != want {
		t.Fatalf("saved output = %q, want %q", got, want)
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !strings.Contains(m.status, "застосовано") {
		t.Fatalf("status = %q", m.status)
	}
	if got, want := recordedArgs(t, logPath), "display\napply\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
}

func TestWiFiCanBeTurnedOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "uk")
	t.Setenv("CPZ_WIFI_DEVICE", "wlan0")
	logPath := filepath.Join(t.TempDir(), "cmd.log")
	t.Setenv("CPZ_CMD_LOG", logPath)
	dir := t.TempDir()
	writeTool(t, dir, "iwctl", `
printf '%s\n' "$*" >> "$CPZ_CMD_LOG"
case "$*" in
  "device wlan0 show") echo "Powered: yes" ;;
  "device wlan0 list") : ;;
  "station wlan0 get-networks") : ;;
  "known-networks list") : ;;
esac
exit 0
`)
	writeTool(t, dir, "bluetoothctl", `exit 0`)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	m := selectRoot(New(), "network")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open Wi-Fi
	m = nm.(Model)
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	if !m.wifiOn {
		t.Fatal("fake radio should start on")
	}
	m.wifi.Cursor = 0 // "Turn Wi-Fi off"
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("toggle off returned no command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(Model)
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "device wlan0 set-property Powered off") {
		t.Fatalf("power off was not called:\n%s", log)
	}
}
