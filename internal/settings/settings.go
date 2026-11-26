package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/about"
	"github.com/jmas/cpzos-apps/internal/backlight"
	"github.com/jmas/cpzos-apps/internal/bt"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/pacman"
	"github.com/jmas/cpzos-apps/internal/tui"
	"github.com/jmas/cpzos-apps/internal/wifi"
)

type view int

const (
	viewRoot view = iota
	viewLanguage
	viewNetwork
	viewWiFi
	viewWiFiSaved
	viewWiFiPassword
	viewBT
	viewBTDevice
	viewDetails
	viewDisplay
	viewBrightness
	viewPower
	viewAbout
	viewConfirm
	viewUpdating
	viewPowering
)

type aboutMsg struct{ fields []about.Field }

type updateMsg struct {
	out       string
	err       error
	cancelled bool
}

type brightnessMsg struct {
	percent int
	err     error
}

type powerMsg struct {
	action string
	out    string
	err    error
}

type displayMsg struct {
	out string
	err error
}

// Model is the Settings app: language, display, power, system update and the
// read-only About screen.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	root           tui.List
	language       tui.List
	network        tui.List
	display        tui.List
	brightnessList tui.List
	power          tui.List
	about          tui.List

	view  view
	data  []about.Field
	ready bool
	help  bool

	brightness int
	output     string
	wifiOn     bool
	btOn       bool
	netLoaded  bool

	wifiDevice string
	wifiNets   []wifi.Network
	wifiSaved  []string
	wifiErr    error

	btDevs  []bt.Device
	btError error

	wifiSavedList tui.List
	btDevice      tui.List
	wifi          tui.List
	bt            tui.List

	details []string
	input   tui.Input
	inputID string
	btMac   string

	confirm     tui.Confirm
	confirmFrom view
	pending     string
	workLabel   string

	status     string
	spinner    tui.Spinner
	cancel     context.CancelFunc
	cancelling bool
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "cpz")
}

// SetLanguage writes the per-user language file. It does not touch the
// platform default in /etc/cpz/language.
func SetLanguage(code string) error {
	lang := i18n.Normalize(code)
	if lang == "" {
		return fmt.Errorf("unsupported language: %s", code)
	}
	dir := configDir()
	if dir == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "language"), []byte(lang+"\n"), 0o644)
}

func readDisplay() map[string]string {
	values := map[string]string{}
	path := filepath.Join(configDir(), "display")
	b, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key == "" {
			continue
		}
		values[key] = strings.TrimSpace(value)
	}
	return values
}

func writeDisplay(values map[string]string) error {
	dir := configDir()
	if dir == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	return os.WriteFile(filepath.Join(dir, "display"), []byte(b.String()), 0o644)
}

func setDisplayValue(key, value string) error {
	values := readDisplay()
	values[key] = value
	return writeDisplay(values)
}

func loadBrightness() int {
	values := readDisplay()
	n, err := strconv.Atoi(values["brightness"])
	if err != nil {
		return 70
	}
	if n < 0 {
		n = 0
	}
	if n > 100 {
		n = 100
	}
	return (n + 5) / 10 * 10
}

func saveBrightness(percent int) error {
	return setDisplayValue("brightness", strconv.Itoa(percent))
}

// Output returns the saved display preference: auto, panel or hdmi.
func Output() string {
	value := readDisplay()["output"]
	switch value {
	case "panel", "hdmi":
		return value
	default:
		return "auto"
	}
}

// SetOutput saves the display preference. cpz-session applies it on the next
// session start.
func SetOutput(value string) error {
	switch value {
	case "auto", "panel", "hdmi":
	default:
		return fmt.Errorf("unsupported output: %s", value)
	}
	return setDisplayValue("output", value)
}

// Brightness returns the saved panel level (0–100, multiples of 10).
func Brightness() int { return loadBrightness() }

// SetBrightness applies a level through the cpz-ui helper and saves it.
func SetBrightness(percent int) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("brightness out of range: %d", percent)
	}
	if err := backlight.Set(percent); err != nil {
		return err
	}
	return saveBrightness(percent)
}

// New builds the root list.
func New() Model {
	lang := i18n.System()
	tr := i18n.For(lang)
	m := Model{
		lang:       lang,
		tr:         tr,
		width:      45,
		height:     13,
		brightness: loadBrightness(),
		output:     Output(),
	}
	m.buildRoot()
	m.buildLanguage()
	m.buildNetwork()
	m.buildWiFi()
	m.buildWiFiSaved()
	m.buildBT()
	m.buildBTDevice()
	m.buildDisplay()
	m.buildBrightness()
	m.buildPower()
	return m
}

// Init reads the system facts after the first frame.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return aboutMsg{fields: about.Load(ctx, m.lang)}
	}
}

func (m *Model) buildRoot() {
	m.root.SetItems([]tui.Item{
		{ID: "language", Name: m.tr.T("settings.language"), Right: m.lang},
		{ID: "network", Name: m.tr.T("settings.network"), Right: m.networkRight()},
		{ID: "display", Name: m.tr.T("settings.display"), Right: fmt.Sprintf("%d%%", m.brightness)},
		{ID: "power", Name: m.tr.T("settings.power")},
		{ID: "update", Name: m.tr.T("settings.update")},
		{ID: "about", Name: m.tr.T("settings.about")},
	})
	m.root.EnsureVisible(m.rows())
}

func (m Model) networkRight() string {
	if !m.netLoaded {
		return ""
	}
	state := m.tr.T("settings.network.off")
	if m.wifiOn {
		state = m.tr.T("settings.network.on")
	}
	return "Wi-Fi " + state
}

func (m *Model) buildLanguage() {
	m.language.SetItems([]tui.Item{
		{ID: "en", Name: "English", Right: "en", Marker: activeMarker(m.lang, "en")},
		{ID: "uk", Name: "Українська", Right: "uk", Marker: activeMarker(m.lang, "uk")},
	})
	m.language.EnsureVisible(m.rows())
}

func activeMarker(current, code string) string {
	if current == code {
		return "*"
	}
	return ""
}

func (m *Model) buildBrightness() {
	items := make([]tui.Item, 0, 11)
	for p := 100; p >= 0; p -= 10 {
		marker := ""
		if m.brightness == p {
			marker = "*"
		}
		items = append(items, tui.Item{
			ID:     strconv.Itoa(p),
			Name:   fmt.Sprintf("%d%%", p),
			Marker: marker,
		})
	}
	m.brightnessList.SetItems(items)
	m.brightnessList.EnsureVisible(m.rows())
}

func (m *Model) buildDisplay() {
	m.display.SetItems([]tui.Item{
		{ID: "brightness", Name: m.tr.T("settings.display.brightness"), Right: fmt.Sprintf("%d%%", m.brightness)},
		{ID: "output", Name: m.tr.T("settings.display.output"), Right: m.tr.T("settings.display.output_" + m.output)},
	})
	m.display.EnsureVisible(m.rows())
}

func (m *Model) buildPower() {
	m.power.SetItems([]tui.Item{
		{ID: "reboot", Name: m.tr.T("settings.power.restart")},
		{ID: "off", Name: m.tr.T("settings.power.off")},
	})
	m.power.EnsureVisible(m.rows())
}

func (m *Model) buildAbout() {
	items := make([]tui.Item, 0, len(m.data))
	for _, f := range m.data {
		items = append(items, tui.Item{ID: f.Key, Name: m.tr.T(f.Key), Right: f.Value})
	}
	m.about.SetItems(items)
	m.about.EnsureVisible(m.rows())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case aboutMsg:
		m.data = msg.fields
		m.ready = true
		m.buildAbout()
		return m, nil
	case brightnessMsg:
		if msg.err != nil {
			m.status = m.tr.T("settings.brightness_failed", "error", msg.err.Error())
			return m, nil
		}
		m.brightness = msg.percent
		if err := saveBrightness(m.brightness); err != nil {
			m.status = m.tr.T("settings.brightness_failed", "error", err.Error())
			return m, nil
		}
		m.buildRoot()
		m.buildDisplay()
		m.buildBrightness()
		m.status = m.tr.T("settings.brightness_applied", "value", strconv.Itoa(m.brightness))
		return m, nil
	case displayMsg:
		if msg.err != nil {
			m.status = m.tr.T("settings.display.failed", "error", reasonOr(msg.out, msg.err))
			return m, nil
		}
		m.status = m.tr.T("settings.display.applied")
		return m, nil
	case powerMsg:
		if msg.err != nil {
			m.view = viewRoot
			m.status = m.tr.T("settings.power_failed", "error", reasonOr(msg.out, msg.err))
			return m, nil
		}
		// The system is going down; keep the working screen until it does.
		return m, nil
	case networkMsg:
		m.applyNetworkMsg(msg)
		return m, nil
	case wifiMsg:
		m.applyWiFiMsg(msg)
		return m, nil
	case wifiActionMsg:
		m.applyWiFiAction(msg)
		if msg.err == nil {
			scan := msg.action == "connect" || (msg.action == "toggle" && !m.wifiOn)
			return m, wifiLoadCmd(scan)
		}
		return m, nil
	case btMsg:
		m.applyBTMsg(msg)
		return m, nil
	case btActionMsg:
		m.applyBTAction(msg)
		if msg.err == nil {
			return m, btLoadCmd(false)
		}
		return m, nil
	case detailsMsg:
		m.details = strings.Split(msg.text, "\n")
		m.view = viewDetails
		return m, nil
	case tui.SpinnerTickMsg:
		if m.view == viewUpdating || m.view == viewPowering {
			m.spinner.Advance()
			return m, m.spinner.Tick()
		}
		return m, nil
	case updateMsg:
		m.view = viewRoot
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.cancelling = false
		if msg.cancelled {
			m.status = m.tr.T("settings.cancelled")
			return m, nil
		}
		if msg.err != nil {
			line := pacman.Reason(msg.out)
			if line == "" {
				line = msg.err.Error()
			}
			if strings.Contains(msg.out, "lock") {
				m.status = m.tr.T("settings.update_busy")
			} else {
				m.status = m.tr.T("settings.update_failed", "error", line)
			}
			return m, nil
		}
		// The installed versions may have changed; re-read About in the
		// background before the user opens it.
		m.ready = false
		m.status = m.tr.T("settings.updated")
		return m, m.Init()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) rows() int {
	rows := m.height - 1
	if m.status != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == viewPowering {
		return m, nil
	}
	if m.view == viewUpdating {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionQuit:
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
				m.cancelling = true
			}
		}
		return m, nil
	}
	if m.help {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionHelp, tui.ActionConfirm, tui.ActionQuit:
			m.help = false
		}
		return m, nil
	}
	if m.view == viewConfirm {
		done, ok := m.confirm.Handle(tui.ActionOf(msg, false))
		if !done {
			return m, nil
		}
		if !ok {
			m.view = m.confirmFrom
			return m, nil
		}
		m.status = ""
		if strings.HasPrefix(m.pending, "wifi-forget:") {
			ssid := strings.TrimPrefix(m.pending, "wifi-forget:")
			m.view = viewWiFiSaved
			return m, wifiActionCmd("forget", m.wifiDevice, ssid, "")
		}
		if strings.HasPrefix(m.pending, "bt-remove:") {
			mac := strings.TrimPrefix(m.pending, "bt-remove:")
			m.view = viewBT
			return m, btActionCmd("remove", mac)
		}
		switch m.pending {
		case "invoke:update":
			m.view = viewUpdating
			m.spinner = tui.Spinner{}
			m.cancelling = false
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			m.cancel = cancel
			return m, tea.Batch(updateCmd(ctx), m.spinner.Tick())
		case "invoke:reboot", "invoke:off":
			action := strings.TrimPrefix(m.pending, "invoke:")
			m.view = viewPowering
			m.spinner = tui.Spinner{}
			m.workLabel = m.tr.T("settings.restarting")
			if action == "off" {
				m.workLabel = m.tr.T("settings.powering_off")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			m.cancel = cancel
			return m, tea.Batch(powerCmd(ctx, action), m.spinner.Tick())
		}
		return m, nil
	}
	if m.status != "" {
		m.status = ""
	}

	if m.view == viewWiFiPassword {
		switch tui.ActionOf(msg, true) {
		case tui.ActionBack:
			m.input = tui.Input{}
			m.view = viewWiFi
			return m, nil
		case tui.ActionConfirm:
			ssid := m.inputID
			psk := m.input.Value
			m.input = tui.Input{}
			m.view = viewWiFi
			if psk == "" {
				m.status = m.tr.T("settings.network.password_required")
				return m, nil
			}
			return m, wifiActionCmd("connect", m.wifiDevice, ssid, psk)
		}
		m.input.Handle(msg)
		return m, nil
	}
	if m.view == viewDetails {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack:
			m.view = viewNetwork
		case tui.ActionQuit:
			return m, tea.Quit
		}
		return m, nil
	}

	rows := m.rows()
	var list *tui.List
	switch m.view {
	case viewLanguage:
		list = &m.language
	case viewNetwork:
		list = &m.network
	case viewWiFi:
		list = &m.wifi
	case viewWiFiSaved:
		list = &m.wifiSavedList
	case viewBT:
		list = &m.bt
	case viewBTDevice:
		list = &m.btDevice
	case viewDisplay:
		list = &m.display
	case viewBrightness:
		list = &m.brightnessList
	case viewPower:
		list = &m.power
	case viewAbout:
		list = &m.about
	default:
		list = &m.root
	}
	if list == nil {
		return m, nil
	}
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		list.Move(-1, rows)
	case tui.ActionDown:
		list.Move(1, rows)
	case tui.ActionPageUp:
		list.Move(-rows, rows)
	case tui.ActionPageDown:
		list.Move(rows, rows)
	case tui.ActionFirst:
		list.MoveFirst()
	case tui.ActionLast:
		list.MoveLast(rows)
	case tui.ActionBack:
		switch m.view {
		case viewRoot:
			return m, tea.Quit
		case viewWiFi, viewBT, viewDetails:
			m.view = viewNetwork
		case viewWiFiSaved:
			m.view = viewWiFi
		case viewBTDevice:
			m.view = viewBT
		case viewBrightness:
			m.view = viewDisplay
		default:
			m.view = viewRoot
		}
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		return m.selectRow()
	}
	return m, nil
}

func (m Model) selectRow() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewRoot:
		item, ok := m.root.Selected()
		if !ok {
			return m, nil
		}
		switch item.ID {
		case "language":
			m.view = viewLanguage
		case "network":
			m.view = viewNetwork
			m.status = m.tr.T("settings.network.reading")
			return m, networkStatusCmd()
		case "display":
			m.view = viewDisplay
		case "power":
			m.view = viewPower
		case "update":
			m.confirmFrom = viewRoot
			m.pending = "invoke:update"
			m.confirm.Open(m.tr.T("settings.confirm.update"))
			m.view = viewConfirm
		case "about":
			m.view = viewAbout
		}
	case viewLanguage:
		item, ok := m.language.Selected()
		if !ok {
			return m, nil
		}
		if err := SetLanguage(item.ID); err != nil {
			m.status = m.tr.T("settings.language_failed", "error", err.Error())
			return m, nil
		}
		m.lang = item.ID
		m.tr = i18n.For(m.lang)
		m.buildRoot()
		m.buildLanguage()
		m.buildNetwork()
		m.buildWiFi()
		m.buildWiFiSaved()
		m.buildBT()
		m.buildBTDevice()
		m.buildDisplay()
		m.buildBrightness()
		m.buildPower()
		m.buildAbout()
		m.status = m.tr.T("settings.language_applied")
	case viewNetwork:
		item, ok := m.network.Selected()
		if !ok {
			return m, nil
		}
		switch item.ID {
		case "wifi":
			m.view = viewWiFi
			m.status = m.tr.T("settings.network.scanning")
			return m, wifiLoadCmd(true)
		case "bt":
			m.view = viewBT
			m.status = m.tr.T("settings.network.reading")
			return m, btLoadCmd(false)
		case "details":
			m.status = m.tr.T("settings.network.reading")
			return m, detailsCmd()
		}
	case viewWiFi:
		item, ok := m.wifi.Selected()
		if !ok {
			return m, nil
		}
		switch {
		case item.ID == "toggle":
			return m, wifiToggleCmd(m.wifiDevice, !m.wifiOn)
		case item.ID == "scan":
			m.status = m.tr.T("settings.network.scanning")
			return m, wifiLoadCmd(true)
		case item.ID == "saved":
			m.view = viewWiFiSaved
			m.buildWiFiSaved()
			return m, nil
		case strings.HasPrefix(item.ID, "net:"):
			n, ok := m.wifiSelectedSSID()
			if !ok {
				return m, nil
			}
			if n.Active {
				return m, wifiActionCmd("disconnect", m.wifiDevice, n.SSID, "")
			}
			if wifi.Secure(n) && !wifi.IsSaved(m.wifiSaved, n.SSID) {
				m.input = tui.Input{
					Prompt: m.tr.T("settings.network.password"),
					Active: true,
					Secret: true,
				}
				m.inputID = n.SSID
				m.view = viewWiFiPassword
				return m, nil
			}
			return m, wifiActionCmd("connect", m.wifiDevice, n.SSID, "")
		}
	case viewWiFiSaved:
		item, ok := m.wifiSavedList.Selected()
		if !ok || !strings.HasPrefix(item.ID, "saved:") {
			return m, nil
		}
		ssid := strings.TrimPrefix(item.ID, "saved:")
		m.confirmFrom = viewWiFiSaved
		m.pending = "wifi-forget:" + ssid
		m.confirm.Open(m.tr.T("settings.network.confirm_forget", "name", ssid))
		m.view = viewConfirm
	case viewBT:
		item, ok := m.bt.Selected()
		if !ok {
			return m, nil
		}
		switch {
		case item.ID == "toggle":
			return m, btToggleCmd(!m.btOn)
		case item.ID == "scan":
			m.status = m.tr.T("settings.network.scanning")
			return m, btLoadCmd(true)
		case strings.HasPrefix(item.ID, "bt:"):
			d, ok := m.btSelected()
			if !ok {
				return m, nil
			}
			m.btMac = d.MAC
			m.buildBTDevice()
			m.view = viewBTDevice
			return m, nil
		}
	case viewBTDevice:
		item, ok := m.btDevice.Selected()
		if !ok {
			return m, nil
		}
		if item.ID == "remove" {
			m.confirmFrom = viewBTDevice
			m.pending = "bt-remove:" + m.btMac
			m.confirm.Open(m.tr.T("settings.network.confirm_remove"))
			m.view = viewConfirm
			return m, nil
		}
		return m, btActionCmd(item.ID, m.btMac)
	case viewDisplay:
		item, ok := m.display.Selected()
		if !ok {
			return m, nil
		}
		if item.ID == "brightness" {
			m.view = viewBrightness
			return m, nil
		}
		if item.ID == "output" {
			next := "auto"
			switch m.output {
			case "auto":
				next = "panel"
			case "panel":
				next = "hdmi"
			}
			if err := SetOutput(next); err != nil {
				m.status = m.tr.T("settings.display.failed", "error", err.Error())
				return m, nil
			}
			m.output = next
			m.buildDisplay()
			m.status = m.tr.T("settings.display.applying")
			return m, displayApplyCmd()
		}
	case viewBrightness:
		item, ok := m.brightnessList.Selected()
		if !ok {
			return m, nil
		}
		percent, err := strconv.Atoi(item.ID)
		if err != nil {
			return m, nil
		}
		return m, brightnessCmd(percent)
	case viewPower:
		item, ok := m.power.Selected()
		if !ok {
			return m, nil
		}
		m.confirmFrom = viewPower
		m.pending = "invoke:" + item.ID
		question := m.tr.T("settings.confirm.restart")
		if item.ID == "off" {
			question = m.tr.T("settings.confirm.poweroff")
		}
		m.confirm.Open(question)
		m.view = viewConfirm
	}
	return m, nil
}

func brightnessCmd(percent int) tea.Cmd {
	return func() tea.Msg {
		return brightnessMsg{percent: percent, err: backlight.Set(percent)}
	}
}

func powerCmd(ctx context.Context, action string) tea.Cmd {
	return func() tea.Msg {
		out, err := pacman.Run(ctx, "power", action)
		return powerMsg{action: action, out: out, err: err}
	}
}

func displayApplyCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := pacman.Run(ctx, "display", "apply")
		return displayMsg{out: out, err: err}
	}
}

func updateCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		out, err := pacman.Run(ctx, "update-system")
		return updateMsg{out: out, err: err, cancelled: ctx.Err() == context.Canceled}
	}
}

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("settings.title"),
		Status: m.status,
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("help.title"), m.tr.T("help.body"))
		return s.Render()
	}
	switch m.view {
	case viewLanguage:
		s.Context = m.tr.T("settings.language")
		s.Body = m.language.Render(m.width, m.rows())
	case viewNetwork:
		s.Context = m.tr.T("settings.network")
		s.Body = m.network.Render(m.width, m.rows())
	case viewWiFi:
		s.Context = m.tr.T("settings.network.wifi")
		s.Body = m.wifi.Render(m.width, m.rows())
	case viewWiFiSaved:
		s.Context = m.tr.T("settings.network.saved")
		s.Body = m.wifiSavedList.Render(m.width, m.rows())
	case viewWiFiPassword:
		s.Context = m.tr.T("settings.network.wifi")
		s.Body = []string{m.input.Render(m.width)}
		s.Status = m.tr.T("settings.network.password_hint")
	case viewBT:
		s.Context = m.tr.T("settings.network.bt")
		s.Body = m.bt.Render(m.width, m.rows())
	case viewBTDevice:
		s.Context = m.btMac
		s.Body = m.btDevice.Render(m.width, m.rows())
	case viewDetails:
		s.Context = m.tr.T("settings.network.details")
		s.Body = m.detailLines(m.rows())
	case viewDisplay:
		s.Context = m.tr.T("settings.display")
		s.Body = m.display.Render(m.width, m.rows())
	case viewBrightness:
		s.Context = m.tr.T("settings.display.brightness")
		s.Body = m.brightnessList.Render(m.width, m.rows())
	case viewPower:
		s.Context = m.tr.T("settings.power")
		s.Body = m.power.Render(m.width, m.rows())
	case viewAbout:
		s.Context = m.tr.T("settings.about")
		if !m.ready {
			s.Body = []string{tui.DimStyle.Render(m.tr.T("settings.loading"))}
		} else {
			s.Body = m.about.Render(m.width, m.rows())
		}
	case viewConfirm:
		s.Body = m.confirm.Render(tui.YesNo{Yes: m.tr.T("common.yes"), No: m.tr.T("common.no")})
	case viewUpdating:
		label := m.tr.T("settings.updating")
		if m.cancelling {
			label = m.tr.T("settings.cancelling")
		}
		s.Body = []string{m.spinner.Render(label)}
		s.Status = m.tr.T("settings.cancel_hint")
	case viewPowering:
		s.Body = []string{m.spinner.Render(m.workLabel)}
	default:
		s.Body = m.root.Render(m.width, m.rows())
	}
	return s.Render()
}

func reasonOr(out string, err error) string {
	if line := pacman.Reason(out); line != "" {
		return line
	}
	return err.Error()
}
