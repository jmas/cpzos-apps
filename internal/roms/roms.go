// Package roms lists emulator ROMs under the user's home and mounted USB
// sticks and launches the right emulator for the selected system.
package roms

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/launch"
	"github.com/jmas/cpzos-apps/internal/media"
	"github.com/jmas/cpzos-apps/internal/tui"
)

// MednafenConfig is installed by cpz-system-apps. It maps the Cardputer keys
// to the emulated pads without using the Fn layer.
const MednafenConfig = "/usr/share/cpz/emulators/mednafen.cfg"

// System is one emulated console.
type System struct {
	ID       string
	Name     string
	Module   string
	Emulator string
	Exts     []string
}

// Systems is the curated order shown in Roms.
var Systems = []System{
	{ID: "nes", Name: "NES", Module: "nes", Emulator: "mednafen", Exts: []string{".nes", ".unf", ".unif"}},
	{ID: "snes", Name: "SNES", Module: "snes", Emulator: "mednafen", Exts: []string{".sfc", ".smc", ".swc", ".fig"}},
	{ID: "genesis", Name: "Sega Genesis", Module: "md", Emulator: "mednafen", Exts: []string{".md", ".gen", ".bin", ".smd"}},
	{ID: "gb", Name: "Game Boy", Module: "", Emulator: "mgba", Exts: []string{".gb", ".gbc"}},
	{ID: "gba", Name: "Game Boy Advance", Module: "", Emulator: "mgba", Exts: []string{".gba"}},
}

// Game is one ROM file.
type Game struct {
	Path   string
	Name   string
	System System
}

// Valid reports whether id names a known system.
func Valid(id string) bool {
	_, ok := ByID(id)
	return ok
}

// ByID returns a system by id.
func ByID(id string) (System, bool) {
	for _, s := range Systems {
		if s.ID == id {
			return s, true
		}
	}
	return System{}, false
}

// Scan finds ROMs under ~/roms and on mounted USB sticks.
func Scan() []Game {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return ScanIn(home, mediaRoots())
}

// ScanIn is Scan with an explicit home and media roots. Tests use it. Each
// root is searched directly (<root>/nes) and under roms/ (<root>/roms/nes),
// because a disk may be prepared either way.
func ScanIn(home string, media []string) []Game {
	roots := []string{filepath.Join(home, "roms")}
	for _, m := range media {
		roots = append(roots, m, filepath.Join(m, "roms"))
	}

	seen := map[string]bool{}
	var out []Game
	add := func(path string, sys System) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, Game{
			Path:   abs,
			Name:   strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			System: sys,
		})
	}
	for _, root := range roots {
		for _, sys := range Systems {
			dir := filepath.Join(root, sys.ID)
			_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && sys.matches(path) {
					add(path, sys)
				}
				return nil
			})
		}
		// A disk may also carry ROMs directly in its root.
		if ents, err := os.ReadDir(root); err == nil {
			for _, de := range ents {
				if de.IsDir() {
					continue
				}
				path := filepath.Join(root, de.Name())
				if sys, ok := systemFor(path); ok {
					add(path, sys)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Counts returns the number of ROMs for each system.
func Counts(games []Game) map[string]int {
	out := map[string]int{}
	for _, g := range games {
		out[g.System.ID]++
	}
	return out
}

// ForSystem returns the games of one system.
func ForSystem(games []Game, id string) []Game {
	out := make([]Game, 0, len(games))
	for _, g := range games {
		if g.System.ID == id {
			out = append(out, g)
		}
	}
	return out
}

func systemFor(path string) (System, bool) {
	for _, s := range Systems {
		if s.matches(path) {
			return s, true
		}
	}
	return System{}, false
}

func (s System) matches(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, want := range s.Exts {
		if ext == want {
			return true
		}
	}
	return false
}

// mgbaBindings maps the Cardputer d-pad (F/X/Z/C) and the face/shoulder keys
// to the GBA pad. mGBA otherwise uses the arrow keys, which are Fn-only here.
// SDL scancodes, not key symbols: the physical keys are the same on the us
// and ua layouts, while their Unicode symbols are not.
var mgbaBindings = []string{
	"gba.input.SDLK.keyUp=9",      // F
	"gba.input.SDLK.keyDown=27",   // X
	"gba.input.SDLK.keyLeft=29",   // Z
	"gba.input.SDLK.keyRight=6",   // C
	"gba.input.SDLK.keyA=14",      // K
	"gba.input.SDLK.keyB=13",      // J
	"gba.input.SDLK.keyL=24",      // U
	"gba.input.SDLK.keyR=12",      // I
	"gba.input.SDLK.keyStart=40",  // Enter
	"gba.input.SDLK.keySelect=42", // Backspace
}

// Argv returns the emulator command for a ROM.
func (s System) Argv(path string) []string {
	if s.Emulator == "mgba" {
		argv := []string{"mgba", "-f"}
		for _, binding := range mgbaBindings {
			argv = append(argv, "-C", binding)
		}
		return append(argv, path)
	}
	return []string{"mednafen", "-ovconfig", MednafenConfig, "-force_module", s.Module, path}
}

// Env is the SDL environment used by both emulators.
func Env() []string {
	return []string{"SDL_VIDEODRIVER=wayland", "SDL_AUDIODRIVER=alsa"}
}

// Available reports whether the emulator for the system is installed.
func (s System) Available() bool {
	_, err := exec.LookPath(s.Emulator)
	return err == nil
}

// Dir returns the preferred directory for a system's ROMs.
func Dir(home, id string) string { return filepath.Join(home, "roms", id) }

func mediaRoots() []string {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	// cpz-automount mounts under /media/<label> and records the path.
	for _, e := range media.List() {
		add(e.Mountpoint)
	}
	userName := os.Getenv("USER")
	if userName == "" {
		if u, err := user.Current(); err == nil {
			userName = u.Username
		}
	}
	if userName != "" {
		for _, base := range []string{"/run/media", "/media"} {
			matches, _ := filepath.Glob(filepath.Join(base, userName, "*"))
			for _, m := range matches {
				add(m)
			}
		}
	}
	// Other applications and older images use /media/<label> directly.
	matches, _ := filepath.Glob("/media/*")
	for _, m := range matches {
		add(m)
	}
	return out
}

// --- model ---------------------------------------------------------------

type view int

const (
	viewSystems view = iota
	viewGames
)

type launchedMsg struct {
	name string
	proc *launch.Process
	err  error
}

type launchDoneMsg struct {
	name string
	err  error
}

type mediaChangedMsg struct{}

// Model is the Roms screen.
type Model struct {
	tr     *i18n.Translator
	width  int
	height int

	systems []System
	all     []Game
	system  string // selected system, empty on the system list
	filter  bool   // started with --system

	list   tui.List
	help   bool
	status string

	starting   string
	view       view
	mediaCh    <-chan struct{}
	mediaClose func()
}

// New scans the disks and builds the screen. An empty system starts on the
// system list; a known system starts on that system's ROMs.
func New(system string) Model {
	tr := i18n.For(i18n.System())
	m := Model{
		tr:      tr,
		width:   45,
		height:  13,
		systems: Systems,
		all:     Scan(),
		view:    viewSystems,
	}
	if sys, ok := ByID(system); ok {
		m.system = sys.ID
		m.filter = true
		m.view = viewGames
	}
	m.mediaCh, m.mediaClose, _ = media.Watch()
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd { return m.waitMedia() }

func (m Model) waitMedia() tea.Cmd {
	if m.mediaCh == nil {
		return nil
	}
	return func() tea.Msg {
		if _, ok := <-m.mediaCh; !ok {
			return nil
		}
		return mediaChangedMsg{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.EnsureVisible(m.listRows())
		return m, nil
	case mediaChangedMsg:
		m.all = Scan()
		m.refresh()
		return m, m.waitMedia()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case launchedMsg:
		if msg.err != nil {
			m.starting = ""
			m.status = m.tr.T("apps.status.failed", "error", msg.err.Error())
			return m, nil
		}
		return m, waitLaunch(msg.proc)
	case launchDoneMsg:
		m.starting = ""
		if msg.err != nil {
			m.status = m.tr.T("apps.status.failed", "error", msg.err.Error())
		} else if strings.HasPrefix(m.status, m.tr.T("roms.starting", "name", msg.name)) {
			m.status = ""
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionHelp, tui.ActionConfirm, tui.ActionQuit:
			m.help = false
		}
		return m, nil
	}
	if m.status != "" {
		m.status = ""
	}
	visible := m.listRows()
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		m.list.Move(-1, visible)
	case tui.ActionDown:
		m.list.Move(1, visible)
	case tui.ActionPageUp:
		m.list.Move(-visible, visible)
	case tui.ActionPageDown:
		m.list.Move(visible, visible)
	case tui.ActionFirst:
		m.list.MoveFirst()
	case tui.ActionLast:
		m.list.MoveLast(visible)
	case tui.ActionHelp:
		m.help = true
	case tui.ActionBack:
		if m.view == viewGames && !m.filter {
			m.view = viewSystems
			m.system = ""
			m.refresh()
			return m, nil
		}
		return m, tea.Quit
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		return m.open()
	}
	return m, nil
}

func (m *Model) open() (tea.Model, tea.Cmd) {
	item, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	if m.view == viewSystems {
		sys, ok := ByID(item.ID)
		if !ok {
			return m, nil
		}
		m.system = sys.ID
		m.view = viewGames
		m.refresh()
		return m, nil
	}
	if m.starting != "" {
		m.status = m.tr.T("apps.status.busy_launch", "name", m.starting)
		return m, nil
	}
	g, ok := m.game(item.ID)
	if !ok {
		return m, nil
	}
	if !g.System.Available() {
		m.status = m.tr.T("roms.missing", "system", g.System.Name)
		return m, nil
	}
	spec := launch.Spec{
		Argv:     g.System.Argv(g.Path),
		Name:     g.Name,
		ID:       "cpz-emulator",
		Path:     g.Path,
		Terminal: false,
		Env:      Env(),
	}
	m.starting = g.Name
	m.status = m.tr.T("roms.starting", "name", g.Name)
	return m, func() tea.Msg {
		proc, err := launch.Start(spec)
		return launchedMsg{name: g.Name, proc: proc, err: err}
	}
}

func waitLaunch(p *launch.Process) tea.Cmd {
	return func() tea.Msg {
		return launchDoneMsg{name: p.Name(), err: p.Wait()}
	}
}

func (m *Model) refresh() {
	if m.view == viewSystems {
		counts := Counts(m.all)
		items := make([]tui.Item, 0, len(m.systems))
		for _, s := range m.systems {
			n := counts[s.ID]
			items = append(items, tui.Item{
				ID:    s.ID,
				Name:  s.Name,
				Right: strconv.Itoa(n),
				Dim:   n == 0,
			})
		}
		m.list.SetItems(items)
		return
	}
	games := ForSystem(m.all, m.system)
	sys, _ := ByID(m.system)
	items := make([]tui.Item, 0, len(games))
	for _, g := range games {
		items = append(items, tui.Item{ID: g.Path, Name: g.Name, Right: sys.Name})
	}
	m.list.SetItems(items)
}

func (m Model) game(path string) (Game, bool) {
	for _, g := range m.all {
		if g.Path == path {
			return g, true
		}
	}
	return Game{}, false
}

func (m Model) listRows() int {
	rows := m.height - 1
	if m.status != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("roms.title"),
		Status: m.status,
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("roms.help.title"), m.tr.T("roms.help.body"))
		return s.Render()
	}
	rows := m.listRows()
	if m.view == viewSystems {
		s.Body = m.list.Render(m.width, rows)
		return s.Render()
	}
	sys, _ := ByID(m.system)
	games := ForSystem(m.all, m.system)
	s.Context = fmt.Sprintf("%s  %d", sys.Name, len(games))
	if len(games) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "~"
		}
		line := m.tr.T("roms.empty", "dir", Dir(home, m.system))
		s.Body = append(s.Body, tui.NormalStyle.Render(tui.Fit(line, m.width)))
	}
	s.Body = append(s.Body, m.list.Render(m.width, rows)...)
	return s.Render()
}
