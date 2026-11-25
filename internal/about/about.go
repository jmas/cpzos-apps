package about

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/pacman"
)

// Field is one read-only value on the About screen.
type Field struct {
	Key   string
	Value string
}

// Load reads the system facts without touching the network.
func Load(ctx context.Context, lang string) []Field {
	conf := readKV("/etc/cpz/platform.conf")
	out := readKV("/run/cpz/output")

	platform := conf["platform"]
	if platform == "" {
		platform = "?"
	}
	contract := conf["contract"]
	if contract == "" {
		contract = "?"
	}

	return []Field{
		{Key: "about.platform", Value: platform},
		{Key: "about.contract", Value: contract},
		{Key: "about.ui", Value: pkgVersion(ctx, "cpz-ui")},
		{Key: "about.apps", Value: pkgVersion(ctx, "cpz-system-apps")},
		{Key: "about.catalog", Value: catalogDate()},
		{Key: "about.board", Value: board()},
		{Key: "about.kernel", Value: kernel()},
		{Key: "about.hostname", Value: hostname()},
		{Key: "about.language", Value: lang},
		{Key: "about.output", Value: out["kind"]},
		{Key: "about.uptime", Value: uptime()},
		{Key: "about.memory", Value: memory()},
	}
}

func readKV(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func pkgVersion(ctx context.Context, pkg string) string {
	if v := pacman.InstalledVersion(ctx, pkg); v != "" {
		return v
	}
	return "?"
}

func catalogDate() string {
	b, err := os.ReadFile(catalog.DefaultPath)
	if err != nil {
		return "?"
	}
	var idx catalog.Index
	if err := json.Unmarshal(b, &idx); err != nil || idx.Generated.IsZero() {
		return "?"
	}
	return idx.Generated.Local().Format("2006-01-02")
}

func board() string {
	b, err := os.ReadFile("/proc/device-tree/model")
	if err != nil {
		return "?"
	}
	s := strings.TrimRight(string(b), "\x00\n ")
	if s == "" {
		return "?"
	}
	return s
}

func kernel() string {
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return "?"
}

func hostname() string {
	if s, err := os.Hostname(); err == nil && s != "" {
		return s
	}
	return "?"
}

func uptime() string {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "?"
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return "?"
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "?"
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func memory() string {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "?"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			break
		}
		return fmt.Sprintf("%d MB", kb/1024)
	}
	return "?"
}
