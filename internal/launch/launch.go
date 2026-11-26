package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/jmas/cpzos-apps/internal/apps"
)

// ErrNoDocument is returned for %f/%u: this UI has no document source.
var ErrNoDocument = errors.New("entry needs a file or URL argument")

// savedTerm is the terminal state captured before Bubble Tea enters raw mode.
var savedTerm *term.State

// SaveTerminal records the normal terminal state; call before the TUI starts.
func SaveTerminal() {
	if st, err := term.GetState(int(os.Stdin.Fd())); err == nil {
		savedTerm = st
	}
}

// RestoreTerminal puts the terminal back before handing it to a child.
func RestoreTerminal() {
	if savedTerm != nil {
		_ = term.Restore(int(os.Stdin.Fd()), savedTerm)
	}
}

// Spec is an executable launch derived from a desktop entry.
type Spec struct {
	Argv     []string
	Name     string
	ID       string
	Path     string
	Terminal bool
	Env      []string
}

// FromEntry expands Exec for the CPZ session.
func FromEntry(e apps.Entry, lang string) (Spec, error) {
	argv, err := Expand(e.Exec(), e.Name(lang), e.ID, e.Icon(), e.Path)
	if err != nil {
		return Spec{}, err
	}
	return Spec{
		Argv:     argv,
		Name:     e.Name(lang),
		ID:       e.ID,
		Path:     e.Path,
		Terminal: e.Terminal(),
	}, nil
}

// Expand parses an Exec line and expands the supported field codes.
func Expand(execLine, name, id, icon, path string) ([]string, error) {
	parts, err := split(execLine)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range parts {
		switch p {
		case "%f", "%F", "%u", "%U":
			return nil, ErrNoDocument
		case "%i":
			if icon != "" {
				out = append(out, "--icon", icon)
			}
		case "%c":
			out = append(out, name)
		case "%k":
			out = append(out, path)
		case "%%":
			out = append(out, "%")
		case "%d", "%D", "%n", "%N", "%v", "%m":
			// Deprecated codes carry no value in a desktop file.
		default:
			if strings.Contains(p, "%") && len(p) > 1 {
				return nil, fmt.Errorf("unsupported field code in Exec: %s", p)
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 || out[0] == "" {
		return nil, errors.New("empty Exec")
	}
	return out, nil
}

// split follows the Desktop Entry quoting rules closely enough for Exec.
func split(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	quoted := false
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"':
			quoted = !quoted
		case ' ', '\t':
			if quoted {
				cur.WriteRune(r)
				continue
			}
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quoted {
		return nil, errors.New("unterminated quote in Exec")
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

// NewTab opens a terminal app in a new tmux window. The command is passed as a
// spec file so tmux never has to quote an argument list.
func NewTab(spec Spec) error {
	if len(spec.Argv) == 0 {
		return errors.New("empty command")
	}
	if _, err := exec.LookPath(spec.Argv[0]); err != nil {
		return err
	}
	file, err := writeSpec(spec.Argv)
	if err != nil {
		return err
	}
	if os.Getenv("TMUX") == "" {
		_ = os.Remove(file)
		cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Start()
	}
	self, err := selfPath()
	if err != nil {
		_ = os.Remove(file)
		return err
	}
	cmd := exec.Command("tmux", "-L", "cpz", "new-window", "-n", windowName(spec.Name), self, "run", file, windowName(spec.Name))
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(file)
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Process is a started graphical client. The caller must call Wait to reap
// it; until then it is still running, which is what a launching indicator
// needs to know.
type Process struct {
	name string
	cmd  *exec.Cmd
	log  *os.File
	lock *runLock
}

// Name is the desktop-entry name used in messages.
func (p *Process) Name() string { return p.name }

// Start launches a graphical client as a child of the session. It must not
// consume the Home terminal, so stdin is detached; output goes to a log file.
// A failed start is returned at once, but the app runs on: Wait reports the
// exit and shows a failure in the tmux message line.
//
// A graphical entry is single-instance across tabs. The lock is passed to the
// child as an extra file, so it survives the launcher but is released when the
// app exits.
func Start(spec Spec) (*Process, error) {
	if len(spec.Argv) == 0 {
		return nil, errors.New("empty command")
	}
	if _, err := exec.LookPath(spec.Argv[0]); err != nil {
		return nil, err
	}
	lock, err := claim(spec.ID)
	if err != nil {
		return nil, err
	}
	logFile, err := os.CreateTemp(specDir(), "app.")
	if err != nil {
		lock.release()
		return nil, err
	}
	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(), spec.Env...)
	if lock != nil {
		cmd.ExtraFiles = []*os.File{lock.file}
	}
	if err := cmd.Start(); err != nil {
		lock.release()
		logFile.Close()
		os.Remove(logFile.Name())
		return nil, err
	}
	return &Process{name: spec.Name, cmd: cmd, log: logFile, lock: lock}, nil
}

// Wait blocks until the app exits. A clean exit clears the log; a failure is
// reported in the tmux message line and returned with the log's last line so
// the launcher can replace its launching status with a useful reason.
func (p *Process) Wait() error {
	runErr := p.cmd.Wait()
	p.lock.release()
	_ = p.log.Close()
	if runErr == nil {
		_ = os.Remove(p.log.Name())
		return nil
	}
	cause := failureCause(p.log.Name(), runErr)
	notifyFailure(p.name, cause)
	return errors.New(cause)
}

func failureCause(logPath string, runErr error) string {
	line := ""
	if b, err := os.ReadFile(logPath); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				line = l
			}
		}
	}
	if line == "" && runErr != nil {
		line = runErr.Error()
	}
	return line
}

func notifyFailure(name, cause string) {
	msg := "App failed: " + name
	if cause != "" {
		msg += ": " + cause
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	msg = strings.ReplaceAll(msg, "#", "##")
	// A leading space keeps a log line that starts with "-" from being parsed
	// as a tmux option.
	_ = exec.Command("tmux", "-L", "cpz", "display-message", "-d", "8000", " "+msg).Run()
}

// Supervised runs the app in the current window and returns to Home when it
// exits. The supervisor is a separate cpz-app process with no input loop, so
// the app owns the terminal and the tab never closes into a black screen.
func Supervised(spec Spec) error {
	if len(spec.Argv) == 0 {
		return errors.New("empty command")
	}
	if _, err := exec.LookPath(spec.Argv[0]); err != nil {
		return err
	}
	file, err := writeSpec(spec.Argv)
	if err != nil {
		return err
	}
	self, err := selfPath()
	if err != nil {
		_ = os.Remove(file)
		return err
	}
	fmt.Print("\x1b[?1049l\x1b[?25h")
	RestoreTerminal()
	return syscall.Exec(self, []string{self, "run", file, windowName(spec.Name)}, os.Environ())
}

// RunSupervised waits for the app and starts Home again in the same window.
func RunSupervised(file, name string) error {
	argv, err := ReadSpec(file)
	if err != nil || len(argv) == 0 {
		if err == nil {
			err = errors.New("empty spec")
		}
		fmt.Fprintf(os.Stderr, "cpz-app: %v\n", err)
		time.Sleep(1500 * time.Millisecond)
	} else {
		renameWindow(name)
		state, _ := term.GetState(int(os.Stdin.Fd()))
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "\n%s\n", err)
			time.Sleep(1500 * time.Millisecond)
		}
		if state != nil {
			_ = term.Restore(int(os.Stdin.Fd()), state)
		}
	}
	renameWindow("Home")
	self, err := selfPath()
	if err != nil {
		return err
	}
	return syscall.Exec(self, []string{self, "home"}, os.Environ())
}

func renameWindow(name string) {
	if os.Getenv("TMUX") == "" {
		return
	}
	_ = exec.Command("tmux", "-L", "cpz", "rename-window", "--", windowName(name)).Run()
}

func windowName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 24 {
		s = string(r[:24])
	}
	if strings.TrimSpace(s) == "" {
		return "App"
	}
	return s
}

func selfPath() (string, error) {
	if p, err := exec.LookPath("cpz-app"); err == nil {
		return p, nil
	}
	return os.Executable()
}

func specDir() string {
	if d := os.Getenv("CPZ_RUN_DIR"); d != "" {
		return d
	}
	if st, err := os.Stat("/run/cpz"); err == nil && st.IsDir() {
		return "/run/cpz"
	}
	return os.TempDir()
}

func writeSpec(argv []string) (string, error) {
	f, err := os.CreateTemp(specDir(), "launch.")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if _, err := f.WriteString(strings.Join(argv, "\x00")); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	return name, f.Close()
}

// ReadSpec reads and removes an argv file written by NewTab.
func ReadSpec(path string) ([]string, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("spec path must be absolute")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	if len(b) == 0 {
		return nil, errors.New("empty spec")
	}
	return strings.Split(string(b), "\x00"), nil
}

// ExecSpec replaces the current process with the spec's argv.
func ExecSpec(path string) error {
	argv, err := ReadSpec(path)
	if err != nil {
		return err
	}
	return execArgv(argv)
}

func execArgv(argv []string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	// Bubble Tea runs in the alternate screen; leave it before handing the
	// terminal to the app so a plain shell starts on the normal screen.
	fmt.Print("\x1b[?1049l\x1b[?25h")
	return syscall.Exec(bin, argv, os.Environ())
}
