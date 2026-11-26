package pacman

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Helper is the only privileged entry point. The package installs it at
// /usr/lib/cpz/app-install and sudoers allows it without a password.
const Helper = "/usr/lib/cpz/app-install"

// Error carries the helper's exit code.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("package operation failed (exit %d)", e.Code)
	}
	return e.Msg
}

// Kind of a helper failure, used for user-facing messages.
func Kind(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 1
}

// Run executes one helper verb. Empty arguments are omitted, so callers can
// pass an optional package or option without a branch.
func Run(ctx context.Context, verb string, extra ...string) (string, error) {
	path := os.Getenv("CPZ_APP_INSTALL")
	direct := path != ""
	if !direct {
		path = Helper
	}
	args := helperArgs(verb, extra...)

	var cmd *exec.Cmd
	if direct || os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, path, args...)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n", path}, args...)...)
	}
	// The helper (and pacman below it) is a child of sudo. Put the command in
	// its own process group so Esc can stop the whole transaction instead of
	// leaving pacman running with the database locked.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}
	cmd.WaitDelay = 10 * time.Second
	cmd.Env = append(os.Environ(), "LC_ALL=C")

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := tail(buf.String(), 2000)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, &Error{Code: ee.ExitCode(), Msg: out}
		}
		return out, err
	}
	return out, nil
}

func helperArgs(verb string, extra ...string) []string {
	args := make([]string, 0, len(extra)+1)
	args = append(args, verb)
	for _, a := range extra {
		if a != "" {
			args = append(args, a)
		}
	}
	return args
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// Query runs a read-only pacman command.
func Query(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pacman", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// Owner returns the package that owns a desktop file.
func Owner(ctx context.Context, path string) string {
	out, err := Query(ctx, "-Qoq", path)
	if err != nil {
		return ""
	}
	return firstLine(out)
}

// InstalledVersion returns the installed version of a package.
func InstalledVersion(ctx context.Context, pkg string) string {
	out, err := Query(ctx, "-Q", pkg)
	if err != nil {
		return ""
	}
	fields := strings.Fields(firstLine(out))
	if len(fields) >= 2 {
		return fields[1]
	}
	return ""
}

// Reason picks the line worth showing when a helper call fails. The helper
// returns the tail of the pacman output, so its first line is usually
// "resolving dependencies…" — useless. A line with "error:" is the reason (a
// bad mirror, a signature, a full disk); otherwise the last line is the
// closest thing to a summary.
func Reason(out string) string {
	last := ""
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.Contains(strings.ToLower(l), "error:") {
			return l
		}
		last = l
	}
	return last
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
