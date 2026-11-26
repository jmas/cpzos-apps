// Package media lists USB filesystems mounted by cpz-automount and provides
// the small privileged call that unmounts them.
package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// StateDir is where cpz-automount records mounted filesystems.
func StateDir() string {
	if d := os.Getenv("CPZ_AUTOMOUNT_STATE"); d != "" {
		return d
	}
	return "/run/cpz-automount"
}

// UmountHelper is installed by cpz-system-apps. It validates the device and
// calls cpz-automount as root.
const UmountHelper = "/usr/lib/cpz/media-umount"

// Entry is one mounted USB filesystem.
type Entry struct {
	Device     string
	Mountpoint string
	Label      string
	Free       string
}

// List reads the automount state directory. An absent directory is empty.
func List() []Entry {
	ents, err := os.ReadDir(StateDir())
	if err != nil {
		return nil
	}
	out := make([]Entry, 0, len(ents))
	for _, de := range ents {
		name := de.Name()
		if de.IsDir() || !strings.HasSuffix(name, ".mountpoint") {
			continue
		}
		devName := strings.TrimSuffix(name, ".mountpoint")
		if devName == "" || strings.ContainsAny(devName, "/\\") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(StateDir(), name))
		if err != nil {
			continue
		}
		mnt := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
		if mnt == "" {
			continue
		}
		label := filepath.Base(mnt)
		if label == "" || label == "." || label == "/" {
			label = "media"
		}
		e := Entry{
			Device:     "/dev/" + devName,
			Mountpoint: mnt,
			Label:      label,
		}
		if free, ok := freeSpace(mnt); ok {
			e.Free = human(free)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

func freeSpace(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}

func human(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for x := n / unit; x >= unit && exp < 4; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTP"[exp])
}

// Unmount asks the privileged helper to unmount the filesystem.
func Unmount(ctx context.Context, e Entry) error {
	// Prefer the device; if the entry somehow has no device, the helper can
	// resolve the mounted directory with findmnt.
	target := e.Device
	if !strings.HasPrefix(target, "/dev/") || strings.Contains(target, "..") {
		target = e.Mountpoint
	}
	if target == "" || strings.Contains(target, "..") {
		return fmt.Errorf("no device or mountpoint for %s", e.Label)
	}
	helper := os.Getenv("CPZ_MEDIA_UMOUNT")
	if helper == "" {
		helper = UmountHelper
	}
	var cmd *exec.Cmd
	if os.Getenv("CPZ_MEDIA_DIRECT") != "" {
		cmd = exec.CommandContext(ctx, helper, target)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", "-n", helper, target)
	}
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", lastLine(msg))
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}
