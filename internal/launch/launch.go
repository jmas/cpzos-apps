package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jmas/cpzos-apps/internal/apps"
)

// ErrNoDocument is returned for %f/%u: this UI has no document source.
var ErrNoDocument = errors.New("entry needs a file or URL argument")

// Spec is an executable launch derived from a desktop entry.
type Spec struct {
	Argv     []string
	Name     string
	ID       string
	Path     string
	Terminal bool
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
	cmd := exec.Command("tmux", "-L", "cpz", "new-window", "-n", windowName(spec.Name), self, "exec", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(file)
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Start launches a graphical client as a child of the session.
func Start(spec Spec) error {
	if len(spec.Argv) == 0 {
		return errors.New("empty command")
	}
	if _, err := exec.LookPath(spec.Argv[0]); err != nil {
		return err
	}
	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// Replace runs the app in the current window, after renaming the tab.
func Replace(spec Spec) error {
	if os.Getenv("TMUX") != "" {
		name := windowName(spec.Name)
		_ = exec.Command("tmux", "-L", "cpz", "rename-window", "--", name).Run()
	}
	return execArgv(spec.Argv)
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
