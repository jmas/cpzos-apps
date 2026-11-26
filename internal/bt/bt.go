// Package bt drives bluez through bluetoothctl. Commands use argv arrays; no
// shell is involved.
package bt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// Device is one row from `bluetoothctl devices`.
type Device struct {
	MAC   string
	Name  string
	Named bool
}

func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bluetoothctl", args...)
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

// Powered reports whether the adapter is on.
func Powered(ctx context.Context) (bool, error) {
	out, err := run(ctx, "show")
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "Powered: yes"), nil
}

// SetPowered turns the adapter on or off.
func SetPowered(ctx context.Context, on bool) (string, error) {
	state := "off"
	if on {
		state = "on"
	}
	return run(ctx, "power", state)
}

// Scan runs a bounded discovery and returns the devices seen.
func Scan(ctx context.Context) ([]Device, error) {
	if _, err := run(ctx, "--time", "8", "scan", "on"); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return Devices(ctx)
}

// Devices lists known devices.
func Devices(ctx context.Context) ([]Device, error) {
	out, err := run(ctx, "devices")
	if err != nil {
		return nil, err
	}
	return ParseDevices(out), nil
}

// ParseDevices parses lines of the form "Device XX:XX:.. Name". bluez uses
// the address itself as the alias for devices that do not advertise a name;
// Named is false for those rows so the UI can avoid showing a MAC twice.
func ParseDevices(out string) []Device {
	devices := make([]Device, 0, 16)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Device ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{MAC: fields[1]}
		if len(fields) > 2 {
			d.Name = strings.Join(fields[2:], " ")
		} else {
			d.Name = d.MAC
		}
		d.Named = !addressAlias(d.Name, d.MAC)
		devices = append(devices, d)
	}
	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Named != devices[j].Named {
			return devices[i].Named
		}
		a := strings.ToLower(devices[i].Name)
		b := strings.ToLower(devices[j].Name)
		if a != b {
			return a < b
		}
		return devices[i].MAC < devices[j].MAC
	})
	return devices
}

// addressAlias reports whether name is the MAC in another separator style.
func addressAlias(name, mac string) bool {
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, ":", "")
		s = strings.ReplaceAll(s, "-", "")
		return s
	}
	n := norm(strings.TrimSpace(name))
	return n == "" || n == norm(mac)
}

// Info returns the detail block for one device.
func Info(ctx context.Context, mac string) string {
	out, _ := run(ctx, "info", mac)
	return out
}

// Pair pairs with a device. A phone may require its own confirmation.
func Pair(ctx context.Context, mac string) (string, error) {
	return run(ctx, "pair", mac)
}

// Connect connects to a paired device.
func Connect(ctx context.Context, mac string) (string, error) {
	return run(ctx, "connect", mac)
}

// Trust marks a device as trusted.
func Trust(ctx context.Context, mac string) (string, error) {
	return run(ctx, "trust", mac)
}

// Remove forgets a device.
func Remove(ctx context.Context, mac string) (string, error) {
	return run(ctx, "remove", mac)
}
