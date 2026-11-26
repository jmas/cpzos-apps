// Package wifi drives iwd through iwctl, the same backend the CPZ image
// already ships. Commands use argv arrays; no shell is involved.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// ErrNoDevice means the kernel has no wireless interface.
var ErrNoDevice = errors.New("no Wi-Fi device")

// Network is one row from `iwctl station <dev> get-networks`.
type Network struct {
	SSID     string
	Security string
	Signal   string
	Active   bool
}

// Device returns the first wireless interface, or "".
func Device() string {
	if d := os.Getenv("CPZ_WIFI_DEVICE"); d != "" {
		return d
	}
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	for _, m := range matches {
		return filepath.Base(filepath.Dir(m))
	}
	return ""
}

func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "iwctl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(stripANSI(string(out)))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, errors.New(text)
	}
	return text, nil
}

// Load returns the device, radio state, visible networks and saved SSIDs.
// It does not scan; call Scan first and then List.
func Load(ctx context.Context) (device string, powered bool, networks []Network, saved []string, err error) {
	device = Device()
	if device == "" {
		return "", false, nil, nil, ErrNoDevice
	}
	powered, err = Powered(ctx, device)
	if err != nil {
		return device, false, nil, nil, err
	}
	if !powered {
		return device, false, nil, nil, nil
	}
	networks, err = List(ctx, device)
	if err != nil {
		return device, true, nil, saved, err
	}
	saved, _ = Saved(ctx)
	return device, true, networks, saved, nil
}

// Powered reports the iwd radio state. `device show` and `device list` format
// the state differently across iwd versions, so accept both.
func Powered(ctx context.Context, device string) (bool, error) {
	out, err := run(ctx, "device", device, "show")
	if err == nil {
		if on, ok := parsePoweredShow(out); ok {
			return on, nil
		}
	}
	list, lerr := run(ctx, "device", "list")
	if lerr == nil {
		if on, ok := parsePoweredList(list, device); ok {
			return on, nil
		}
	}
	if err != nil {
		return false, err
	}
	if lerr != nil {
		return false, lerr
	}
	return false, fmt.Errorf("cannot read the Wi-Fi power state")
}

func parsePoweredShow(out string) (bool, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if strings.TrimSuffix(field, ":") != "Powered" || i+1 >= len(fields) {
				continue
			}
			value := strings.Trim(fields[i+1], ":")
			return value == "yes" || value == "on", true
		}
	}
	return false, false
}

func parsePoweredList(out, device string) (bool, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != device {
			continue
		}
		return fields[2] == "on", true
	}
	return false, false
}

// SetPowered turns the radio on or off without stopping the iwd service.
// Powering off an associated station is not portable across iwd versions, so
// disconnect first; an already-disconnected station is not an error.
func SetPowered(ctx context.Context, device string, on bool) (string, error) {
	state := "off"
	if on {
		state = "on"
	} else {
		_, _ = Disconnect(ctx, device)
	}
	return run(ctx, "device", device, "set-property", "Powered", state)
}

// Scan asks iwd for a fresh scan and waits for results to settle.
func Scan(ctx context.Context) error {
	device := Device()
	if device == "" {
		return ErrNoDevice
	}
	if _, err := run(ctx, "station", device, "scan"); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return nil
	}
}

// List returns the networks iwd currently knows about.
func List(ctx context.Context, device string) ([]Network, error) {
	out, err := run(ctx, "station", device, "get-networks")
	if err != nil {
		return nil, err
	}
	return ParseNetworks(out), nil
}

// ParseNetworks parses the iwctl table. Two or more spaces separate columns;
// a single space can be part of an SSID.
func ParseNetworks(out string) []Network {
	networks := make([]Network, 0, 16)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimLeft(line, " \t")
		active := strings.HasPrefix(line, ">")
		if active {
			line = strings.TrimLeft(strings.TrimPrefix(line, ">"), " \t")
		}
		if line == "" || strings.HasPrefix(line, "Network name") ||
			strings.HasPrefix(line, "Available networks") ||
			strings.HasPrefix(line, "No networks available") ||
			strings.HasPrefix(line, "No networks found") ||
			strings.HasPrefix(line, "---") {
			continue
		}
		fields := splitColumns(line)
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		n := Network{SSID: fields[0], Active: active}
		if len(fields) > 1 {
			n.Security = fields[1]
		}
		if len(fields) > 2 {
			n.Signal = fields[2]
		}
		networks = append(networks, n)
	}
	return networks
}

func splitColumns(line string) []string {
	var fields []string
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		start := i
		for i < len(line) {
			if line[i] == ' ' || line[i] == '\t' {
				j := i
				for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
					j++
				}
				if j-i >= 2 {
					break
				}
				i = j
				continue
			}
			i++
		}
		fields = append(fields, line[start:i])
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
	}
	return fields
}

// Saved lists the networks iwd will reconnect to.
func Saved(ctx context.Context) ([]string, error) {
	out, err := run(ctx, "known-networks", "list")
	if err != nil {
		return nil, err
	}
	var saved []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "Network name") ||
			strings.HasPrefix(name, "Known networks") ||
			strings.HasPrefix(name, "---") {
			continue
		}
		saved = append(saved, name)
	}
	return saved, nil
}

// Connect joins a network. A passphrase is passed with --passphrase, which is
// the non-interactive iwctl interface; the value is never written to disk.
func Connect(ctx context.Context, device, ssid, passphrase string) (string, error) {
	args := []string{"station", device, "connect", ssid}
	if passphrase != "" {
		args = append([]string{"--passphrase", passphrase}, args...)
	}
	return run(ctx, args...)
}

// Disconnect leaves the current network.
func Disconnect(ctx context.Context, device string) (string, error) {
	return run(ctx, "station", device, "disconnect")
}

// Forget removes a saved network.
func Forget(ctx context.Context, ssid string) (string, error) {
	return run(ctx, "known-networks", ssid, "forget")
}

// IsSaved reports whether iwd has a stored profile for the SSID.
func IsSaved(saved []string, ssid string) bool {
	for _, s := range saved {
		if s == ssid {
			return true
		}
	}
	return false
}

// Secure reports whether the network needs a passphrase.
func Secure(n Network) bool {
	switch strings.ToLower(n.Security) {
	case "", "open", "--":
		return false
	}
	return true
}

// Details returns the address and interface summary for the Connections page.
func Details(ctx context.Context) string {
	var b strings.Builder
	if out, err := exec.CommandContext(ctx, "ip", "-brief", "address").Output(); err == nil {
		b.Write(out)
	}
	if out, err := exec.CommandContext(ctx, "iw", "dev").Output(); err == nil && len(out) > 0 {
		b.WriteString("\n")
		b.Write(out)
	}
	return strings.TrimSpace(b.String())
}
