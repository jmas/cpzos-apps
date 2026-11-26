package settings

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/bt"
	"github.com/jmas/cpzos-apps/internal/tui"
	"github.com/jmas/cpzos-apps/internal/wifi"
)

type networkMsg struct {
	wifiDev   string
	wifiOn    bool
	wifiNets  []wifi.Network
	wifiSaved []string
	wifiErr   error
	btOn      bool
	btDevs    []bt.Device
	btErr     error
}

type wifiMsg struct {
	dev   string
	on    bool
	nets  []wifi.Network
	saved []string
	err   error
}

type wifiActionMsg struct {
	action string
	out    string
	err    error
}

type btMsg struct {
	on   bool
	devs []bt.Device
	err  error
}

type btActionMsg struct {
	action string
	out    string
	err    error
}

type detailsMsg struct{ text string }

func networkStatusCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		msg := networkMsg{}
		msg.wifiDev, msg.wifiOn, msg.wifiNets, msg.wifiSaved, msg.wifiErr = wifi.Load(ctx)
		msg.btOn, msg.btErr = bt.Powered(ctx)
		if msg.btErr == nil && msg.btOn {
			msg.btDevs, msg.btErr = bt.Devices(ctx)
		}
		return msg
	}
}

func wifiLoadCmd(scan bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := wifiMsg{}
		if scan {
			if err := wifi.Scan(ctx); err != nil {
				msg.err = err
				return msg
			}
		}
		msg.dev, msg.on, msg.nets, msg.saved, msg.err = wifi.Load(ctx)
		if scan && msg.err == nil && msg.on && len(msg.nets) == 0 {
			// iwd can finish the scan a moment after the command returns.
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				msg.dev, msg.on, msg.nets, msg.saved, msg.err = wifi.Load(ctx)
			}
		}
		return msg
	}
}

func wifiActionCmd(action, dev, ssid, psk string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		var out string
		var err error
		switch action {
		case "connect":
			out, err = wifi.Connect(ctx, dev, ssid, psk)
		case "disconnect":
			out, err = wifi.Disconnect(ctx, dev)
		case "forget":
			out, err = wifi.Forget(ctx, ssid)
		}
		return wifiActionMsg{action: action, out: out, err: err}
	}
}

func wifiToggleCmd(dev string, on bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := wifi.SetPowered(ctx, dev, on)
		return wifiActionMsg{action: "toggle", out: out, err: err}
	}
}

func btToggleCmd(on bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := bt.SetPowered(ctx, on)
		return btActionMsg{action: "toggle", out: out, err: err}
	}
}

func btLoadCmd(scan bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		msg := btMsg{}
		msg.on, msg.err = bt.Powered(ctx)
		if msg.err != nil {
			return msg
		}
		if !msg.on {
			return msg
		}
		if scan {
			msg.devs, msg.err = bt.Scan(ctx)
		} else {
			msg.devs, msg.err = bt.Devices(ctx)
		}
		return msg
	}
}

func btActionCmd(action, mac string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var out string
		var err error
		switch action {
		case "pair":
			out, err = bt.Pair(ctx, mac)
		case "connect":
			out, err = bt.Connect(ctx, mac)
		case "trust":
			out, err = bt.Trust(ctx, mac)
		case "remove":
			out, err = bt.Remove(ctx, mac)
		}
		return btActionMsg{action: action, out: out, err: err}
	}
}

func detailsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return detailsMsg{text: wifi.Details(ctx)}
	}
}

func (m *Model) buildNetwork() {
	state := func(on bool) string {
		if !m.netLoaded {
			return "…"
		}
		if on {
			return m.tr.T("settings.network.on")
		}
		return m.tr.T("settings.network.off")
	}
	m.network.SetItems([]tui.Item{
		{ID: "wifi", Name: m.tr.T("settings.network.wifi"), Right: state(m.wifiOn)},
		{ID: "bt", Name: m.tr.T("settings.network.bt"), Right: state(m.btOn)},
		{ID: "details", Name: m.tr.T("settings.network.details")},
	})
	m.network.EnsureVisible(m.rows())
}

func (m *Model) buildWiFi() {
	items := make([]tui.Item, 0, len(m.wifiNets)+3)
	toggle := m.tr.T("settings.network.wifi_on")
	if m.wifiOn {
		toggle = m.tr.T("settings.network.wifi_off")
	}
	items = append(items,
		tui.Item{ID: "toggle", Name: toggle},
		tui.Item{ID: "scan", Name: m.tr.T("settings.network.scan")},
	)
	if m.wifiDevice == "" {
		items = append(items, tui.Item{ID: "info", Name: m.tr.T("settings.network.no_device"), Dim: true})
	} else {
		for _, n := range m.wifiNets {
			right := n.Signal
			if n.Security != "" {
				right = strings.TrimSpace(n.Signal + " " + n.Security)
			}
			marker := ""
			if n.Active {
				marker = "*"
			}
			items = append(items, tui.Item{
				ID:     "net:" + n.SSID,
				Name:   n.SSID,
				Right:  right,
				Marker: marker,
			})
		}
		if m.wifiOn && len(m.wifiNets) == 0 {
			items = append(items, tui.Item{
				ID:   "info",
				Name: m.tr.T("settings.network.no_networks"),
				Dim:  true,
			})
		}
		items = append(items, tui.Item{ID: "saved", Name: m.tr.T("settings.network.saved")})
	}
	m.wifi.SetItems(items)
	m.wifi.EnsureVisible(m.rows())
}

func (m *Model) buildWiFiSaved() {
	items := make([]tui.Item, 0, len(m.wifiSaved))
	for _, ssid := range m.wifiSaved {
		items = append(items, tui.Item{ID: "saved:" + ssid, Name: ssid})
	}
	if len(items) == 0 {
		items = append(items, tui.Item{ID: "info", Name: m.tr.T("settings.network.no_saved"), Dim: true})
	}
	m.wifiSavedList.SetItems(items)
	m.wifiSavedList.EnsureVisible(m.rows())
}

func (m *Model) buildBT() {
	items := make([]tui.Item, 0, len(m.btDevs)+3)
	toggle := m.tr.T("settings.network.bt_on")
	if m.btOn {
		toggle = m.tr.T("settings.network.bt_off")
	}
	items = append(items,
		tui.Item{ID: "toggle", Name: toggle},
		tui.Item{ID: "scan", Name: m.tr.T("settings.network.scan")},
	)
	if len(m.btDevs) == 0 {
		items = append(items, tui.Item{ID: "info", Name: m.tr.T("settings.network.no_devices"), Dim: true})
	} else {
		for _, d := range m.btDevs {
			name := d.Name
			if !d.Named {
				name = m.tr.T("settings.network.unknown_device")
			}
			items = append(items, tui.Item{ID: "bt:" + d.MAC, Name: name, Right: d.MAC})
		}
	}
	m.bt.SetItems(items)
	m.bt.EnsureVisible(m.rows())
}

func (m *Model) buildBTDevice() {
	items := []tui.Item{
		{ID: "pair", Name: m.tr.T("settings.network.pair")},
		{ID: "connect", Name: m.tr.T("settings.network.connect")},
		{ID: "trust", Name: m.tr.T("settings.network.trust")},
		{ID: "remove", Name: m.tr.T("settings.network.remove")},
	}
	m.btDevice.SetItems(items)
	m.btDevice.EnsureVisible(m.rows())
}

func (m *Model) applyNetworkMsg(msg networkMsg) {
	m.wifiDevice = msg.wifiDev
	m.wifiOn = msg.wifiOn
	m.wifiNets = msg.wifiNets
	m.wifiSaved = msg.wifiSaved
	m.btOn = msg.btOn
	m.btDevs = msg.btDevs
	m.netLoaded = true
	if msg.wifiErr != nil && msg.wifiErr != wifi.ErrNoDevice {
		m.status = m.tr.T("settings.network.failed", "error", msg.wifiErr.Error())
	} else if msg.btErr != nil {
		m.status = m.tr.T("settings.network.failed", "error", msg.btErr.Error())
	} else {
		m.status = ""
	}
	m.buildRoot()
	m.buildNetwork()
	m.buildWiFi()
	m.buildWiFiSaved()
	m.buildBT()
}

func (m *Model) applyWiFiMsg(msg wifiMsg) {
	if msg.err != nil && msg.err != wifi.ErrNoDevice {
		m.status = m.tr.T("settings.network.failed", "error", msg.err.Error())
		return
	}
	m.wifiDevice = msg.dev
	m.wifiOn = msg.on
	m.wifiNets = msg.nets
	m.wifiSaved = msg.saved
	m.netLoaded = true
	m.status = ""
	m.buildRoot()
	m.buildNetwork()
	m.buildWiFi()
	m.buildWiFiSaved()
}

func (m *Model) applyWiFiAction(msg wifiActionMsg) {
	if msg.err != nil {
		m.status = m.tr.T("settings.network.failed", "error", msg.err.Error())
		return
	}
	switch msg.action {
	case "connect":
		m.status = m.tr.T("settings.network.connected")
	case "disconnect":
		m.status = m.tr.T("settings.network.disconnected")
	case "forget":
		m.status = m.tr.T("settings.network.forgotten")
		m.view = viewWiFiSaved
	case "toggle":
		m.status = m.tr.T("settings.network.done")
	}
}

func (m *Model) applyBTMsg(msg btMsg) {
	if msg.err != nil {
		m.status = m.tr.T("settings.network.failed", "error", msg.err.Error())
		return
	}
	m.btOn = msg.on
	m.btDevs = msg.devs
	m.netLoaded = true
	m.status = ""
	m.buildRoot()
	m.buildNetwork()
	m.buildBT()
}

func (m *Model) applyBTAction(msg btActionMsg) {
	if msg.err != nil {
		m.status = m.tr.T("settings.network.failed", "error", msg.err.Error())
		return
	}
	if msg.action == "remove" {
		m.view = viewBT
	}
	m.status = m.tr.T("settings.network.done")
}

func (m *Model) wifiSelectedSSID() (wifi.Network, bool) {
	item, ok := m.wifi.Selected()
	if !ok || !strings.HasPrefix(item.ID, "net:") {
		return wifi.Network{}, false
	}
	ssid := strings.TrimPrefix(item.ID, "net:")
	for _, n := range m.wifiNets {
		if n.SSID == ssid {
			return n, true
		}
	}
	return wifi.Network{}, false
}

func (m *Model) btSelected() (bt.Device, bool) {
	item, ok := m.bt.Selected()
	if !ok || !strings.HasPrefix(item.ID, "bt:") {
		return bt.Device{}, false
	}
	mac := strings.TrimPrefix(item.ID, "bt:")
	for _, d := range m.btDevs {
		if d.MAC == mac {
			return d, true
		}
	}
	return bt.Device{}, false
}

func (m Model) networkTitle() string { return m.tr.T("settings.network") }

func (m Model) wifiPrompt() string { return m.tr.T("settings.network.password") }

func (m Model) detailLines(rows int) []string {
	if len(m.details) == 0 {
		return []string{tui.DimStyle.Render(m.tr.T("settings.loading"))}
	}
	if rows < 1 {
		rows = 1
	}
	lines := m.details
	if len(lines) > rows {
		lines = lines[:rows]
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, tui.Fit(line, m.width))
	}
	return out
}
