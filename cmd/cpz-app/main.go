package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/about"
	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/home"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/launch"
	"github.com/jmas/cpzos-apps/internal/openwith"
	"github.com/jmas/cpzos-apps/internal/pacman"
	"github.com/jmas/cpzos-apps/internal/roms"
	"github.com/jmas/cpzos-apps/internal/settings"
	"github.com/jmas/cpzos-apps/internal/store"
)

const (
	exitOK      = 0
	exitFailed  = 1
	exitUsage   = 2
	exitConfirm = 3
	exitUnknown = 4
	exitProtect = 5
	exitOffline = 6
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// configureAudio connects the ES8389 speaker amplifier. cpz-session does the
// same at session start, but an app update on an older image must also make the
// emulators audible without a reboot. amixer -c takes a card index, so read it
// from /proc/asound/cards instead of assuming card 0: the vc4 HDMI card can own
// that number. No shell is involved.
func audioCardIndex(cards []byte) string {
	for _, line := range strings.Split(string(cards), "\n") {
		if !strings.Contains(line, "ES8389") {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func configureAudio() {
	cards, err := os.ReadFile("/proc/asound/cards")
	if err != nil {
		return
	}
	index := audioCardIndex(cards)
	if index == "" {
		return
	}
	for _, args := range [][]string{
		{"-q", "-c", index, "sset", "DACL Playback Volume", "164"},
		{"-q", "-c", index, "sset", "DACR Playback Volume", "164"},
		{"-q", "-c", index, "sset", "Speaker Switch", "on"},
	} {
		_ = exec.Command("amixer", args...).Run()
	}
}

func run(args []string) int {
	tr := i18n.For(i18n.System())
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
	if args[0] != "version" {
		configureAudio()
	}
	switch args[0] {
	case "home":
		if len(args) > 1 {
			fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
			return exitUsage
		}
		return runTUI(home.New())
	case "apps":
		return runApps(args[1:], tr)
	case "settings":
		return runSettings(args[1:], tr)
	case "roms":
		return runRoms(args[1:], tr)
	case "open":
		return runOpen(args[1:], tr)
	case "run":
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
			return exitUsage
		}
		if err := launch.RunSupervised(args[1], args[2]); err != nil {
			fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
			return exitFailed
		}
		return exitOK
	case "exec":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
			return exitUsage
		}
		if err := launch.ExecSpec(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
			return exitFailed
		}
		return exitOK
	case "version":
		fmt.Println("cpz-app 0.4.3")
		return exitOK
	default:
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
}

func runTUI(m tea.Model) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "cpz-app: %v\n", r)
			code = exitFailed
		}
	}()
	launch.SaveTerminal()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "cpz-app: %v\n", err)
		return exitFailed
	}
	return exitOK
}

// --- apps ----------------------------------------------------------------

func runApps(args []string, tr *i18n.Translator) int {
	if len(args) == 0 {
		return runTUI(store.New())
	}
	switch args[0] {
	case "list":
		return appsList(args[1:], tr)
	case "install", "remove", "update":
		return appsOp(args[0], args[1:], tr)
	default:
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
}

type appJSON struct {
	ID         string   `json:"id"`
	Package    string   `json:"package"`
	Name       string   `json:"name"`
	Comment    string   `json:"comment"`
	Version    string   `json:"version"`
	Installed  bool     `json:"installed"`
	System     bool     `json:"system"`
	Categories []string `json:"categories"`
}

type listJSON struct {
	Schema int       `json:"schema"`
	Apps   []appJSON `json:"apps"`
}

func loadAll(lang string) []apps.App {
	entries, _ := apps.LoadAll()
	path := os.Getenv("CPZ_CATALOG")
	if path == "" {
		path = catalog.DefaultPath
	}
	idx, err := catalog.Load(path)
	if err != nil {
		idx = &catalog.Index{Version: 1}
	}
	return apps.Build(lang, entries, idx)
}

func appsList(args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	installed := fs.Bool("installed", false, "only installed apps")
	available := fs.Bool("available", false, "only not installed apps")
	category := fs.String("category", "", "main category")
	query := fs.String("query", "", "search text")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(reorder(args, map[string]bool{"category": true, "query": true})); err != nil {
		return exitUsage
	}
	list := loadAll(tr.Lang)
	var out []apps.App
	for _, a := range list {
		if *installed && !a.Installed {
			continue
		}
		if *available && a.Installed {
			continue
		}
		if *category != "" && apps.MainCategory(a.Categories) != *category {
			continue
		}
		if !apps.Match(a, *query, tr.Lang) {
			continue
		}
		out = append(out, a)
	}
	if *asJSON {
		payload := listJSON{Schema: 1, Apps: make([]appJSON, 0, len(out))}
		for _, a := range out {
			payload.Apps = append(payload.Apps, appJSON{
				ID: a.ID, Package: a.Package, Name: a.Name, Comment: a.Comment,
				Version: a.Version, Installed: a.Installed, System: a.System,
				Categories: a.Categories,
			})
		}
		return writeJSON(payload, exitOK)
	}
	for _, a := range out {
		mark := " "
		if a.Installed {
			mark = "*"
		}
		fmt.Printf("%s %s\t%s\t%s\n", mark, a.Name, a.Package, a.Version)
	}
	return exitOK
}

type opJSON struct {
	Schema  int    `json:"schema"`
	Action  string `json:"action"`
	Package string `json:"package"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func appsOp(action string, args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet(action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	yes := fs.Bool("yes", false, "skip the confirmation")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(reorder(args, nil)); err != nil {
		return exitUsage
	}

	pkg := ""
	name := action
	if action != "update" {
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
			return exitUsage
		}
		arg := fs.Arg(0)
		var err error
		pkg, err = resolvePackage(tr.Lang, arg)
		if err != nil {
			fmt.Fprintln(os.Stderr, tr.T("cli.unknown", "name", arg))
			return exitUnknown
		}
		name = pkg
	}

	if !*yes {
		if !isTTY() || *asJSON {
			fmt.Fprintln(os.Stderr, tr.T("cli.needs_yes"))
			return exitConfirm
		}
		label := tr.T("apps.actions." + action)
		if action == "update" {
			label = tr.T("apps.row.update")
		}
		fmt.Fprintf(os.Stderr, "%s ", tr.T("cli.confirm", "action", label, "name", name))
		var answer string
		fmt.Scanln(&answer)
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(os.Stderr, tr.T("cli.cancelled"))
			return exitOK
		}
	}

	timeout := 10 * time.Minute
	if action == "update" {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var out string
	var err error
	if action == "update" {
		// A command is an explicit request: upgrade the system and installed
		// apps, then refresh the catalog.
		out, err = pacman.Run(ctx, "update-all")
	} else {
		out, err = pacman.Run(ctx, action, pkg)
	}
	code := exitOK
	if err != nil {
		code = mapExit(pacman.Kind(err))
	}
	if *asJSON {
		res := opJSON{Schema: 1, Action: action, Package: pkg, OK: err == nil}
		if err != nil {
			res.Message = pacman.Reason(out)
			if res.Message == "" {
				res.Message = err.Error()
			}
		}
		return writeJSON(res, code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", reasonOr(out, err)))
		return code
	}
	if out != "" {
		fmt.Println(out)
	}
	fmt.Println(tr.T("cli.done"))
	return exitOK
}

func resolvePackage(lang, arg string) (string, error) {
	if !strings.HasSuffix(arg, ".desktop") {
		if !validName(arg) {
			return "", errors.New("invalid package name")
		}
		return arg, nil
	}
	entries, _ := apps.LoadAll()
	for _, e := range entries {
		if e.ID != arg {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if pkg := pacman.Owner(ctx, e.Path); pkg != "" {
			return pkg, nil
		}
	}
	path := os.Getenv("CPZ_CATALOG")
	if path == "" {
		path = catalog.DefaultPath
	}
	idx, err := catalog.Load(path)
	if err == nil {
		for _, a := range idx.Apps {
			if a.ID == arg {
				return a.Package, nil
			}
		}
	}
	return "", errors.New("unknown desktop id")
}

// reorder moves interspersed flags before positional arguments so the standard
// flag package accepts "install pkg --yes". valueFlags consumes one value.
func reorder(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue
		}
		if valueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && strings.ContainsRune("@._+-", r):
		default:
			return false
		}
	}
	return true
}

func mapExit(code int) int {
	switch code {
	case 3:
		return exitConfirm
	case 4:
		return exitUnknown
	case 5:
		return exitProtect
	case 6:
		return exitOffline
	case 2:
		return exitUsage
	default:
		return exitFailed
	}
}

// --- open-with -----------------------------------------------------------

func runOpen(args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	with := fs.String("with", "", "desktop id of the handler")
	list := fs.Bool("list", false, "list handlers instead of opening")
	if err := fs.Parse(reorder(args, map[string]bool{"with": true})); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
	path, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
		return exitFailed
	}
	if *list {
		for _, h := range openwith.Candidates(path, tr.Lang) {
			mark := " "
			if h.Default {
				mark = "*"
			}
			fmt.Printf("%s %s\t%s\n", mark, h.ID, h.Name)
		}
		return exitOK
	}
	var handler openwith.Handler
	var ok bool
	if *with != "" {
		handler, ok = openwith.Find(*with, path, tr.Lang)
	} else {
		handler, ok = openwith.Default(path, tr.Lang)
	}
	if !ok {
		fmt.Fprintln(os.Stderr, tr.T("media.no_handler"))
		return exitUnknown
	}
	spec, err := openwith.Spec(handler, path, tr.Lang)
	if err != nil {
		fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
		return exitFailed
	}
	if spec.Terminal {
		if err := launch.NewTab(spec); err != nil {
			fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
			return exitFailed
		}
		return exitOK
	}
	if _, err := launch.Start(spec); err != nil {
		fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", err.Error()))
		return exitFailed
	}
	return exitOK
}

// --- roms ----------------------------------------------------------------

func runRoms(args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet("roms", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	system := fs.String("system", "", "nes, snes, genesis, gb or gba")
	if err := fs.Parse(reorder(args, map[string]bool{"system": true})); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || (*system != "" && !roms.Valid(*system)) {
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
	return runTUI(roms.New(*system))
}

// --- settings ------------------------------------------------------------

func runSettings(args []string, tr *i18n.Translator) int {
	if len(args) == 0 {
		return runTUI(settings.New())
	}
	switch args[0] {
	case "about":
		return settingsAbout(args[1:], tr)
	case "update":
		return settingsUpdate(args[1:], tr)
	default:
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
}

func settingsAbout(args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet("about", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fields := about.Load(ctx, tr.Lang)
	if *asJSON {
		m := map[string]string{"schema": "1"}
		for _, f := range fields {
			m[strings.TrimPrefix(f.Key, "about.")] = f.Value
		}
		return writeJSON(m, exitOK)
	}
	for _, f := range fields {
		fmt.Printf("%s: %s\n", tr.T(f.Key), f.Value)
	}
	return exitOK
}

func settingsUpdate(args []string, tr *i18n.Translator) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	yes := fs.Bool("yes", false, "skip the confirmation")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(reorder(args, nil)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, tr.T("cli.usage"))
		return exitUsage
	}
	if !*yes {
		if !isTTY() || *asJSON {
			fmt.Fprintln(os.Stderr, tr.T("cli.needs_yes"))
			return exitConfirm
		}
		fmt.Fprintf(os.Stderr, "%s ", tr.T("cli.confirm_update"))
		var answer string
		fmt.Scanln(&answer)
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(os.Stderr, tr.T("cli.cancelled"))
			return exitOK
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	out, err := pacman.Run(ctx, "update-system")
	code := exitOK
	if err != nil {
		code = mapExit(pacman.Kind(err))
	}
	if *asJSON {
		res := opJSON{Schema: 1, Action: "update", OK: err == nil}
		if err != nil {
			res.Message = pacman.Reason(out)
			if res.Message == "" {
				res.Message = err.Error()
			}
		}
		return writeJSON(res, code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, tr.T("cli.error", "error", reasonOr(out, err)))
		return code
	}
	if out != "" {
		fmt.Println(out)
	}
	fmt.Println(tr.T("cli.done"))
	return exitOK
}

// --- helpers -------------------------------------------------------------

func writeJSON(v any, code int) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailed
	}
	fmt.Println(string(b))
	return code
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func reasonOr(out string, err error) string {
	if line := pacman.Reason(out); line != "" {
		return line
	}
	return err.Error()
}
