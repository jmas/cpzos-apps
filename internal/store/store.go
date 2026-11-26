package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/launch"
	"github.com/jmas/cpzos-apps/internal/pacman"
	"github.com/jmas/cpzos-apps/internal/tui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const (
	itemUpdate   = "__update__"
	catInstalled = "__installed"
)

type actionKind int

const (
	actNone actionKind = iota
	actOpen
	actInstall
	actRemove
	actUpdate
)

type view int

const (
	viewMenu view = iota
	viewCategory
	viewDetail
	viewConfirm
	viewWorking
)

type refreshMsg struct {
	err error
	out string
}

type opMsg struct {
	kind actionKind
	out  string
	err  error
}

type ownerMsg struct {
	id      string
	pkg     string
	version string
}

type launchedMsg struct {
	name string
	proc *launch.Process
	err  error
}

type launchDoneMsg struct {
	name string
	err  error
}

// Model is Apps Manager.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	catalogPath string
	idx         *catalog.Index
	all         []apps.App
	category    string
	query       string

	menu   tui.List
	list   tui.List
	search tui.Input
	help   bool
	status string

	view     view
	detail   apps.App
	actions  tui.List
	confirm  tui.Confirm
	pending  actionKind
	working  string
	returnTo view

	// A graphical app owns the tab while it starts. Refuse another launch
	// until it exits; the list behind it stays live but cannot spawn a second
	// client over the first.
	startingID   string
	startingName string
}

// New builds the model from the installed catalog, synchronously; it reads
// only local files.
func New() Model {
	lang := i18n.System()
	tr := i18n.For(lang)
	m := Model{
		lang:        lang,
		tr:          tr,
		width:       45,
		height:      13,
		catalogPath: catalogEnv(),
		idx:         &catalog.Index{Version: 1},
	}
	m.search.Prompt = tr.T("apps.search")
	m.reload()
	m.view = viewMenu
	m.refreshMenu()
	return m
}

func catalogEnv() string {
	if p := os.Getenv("CPZ_CATALOG"); p != "" {
		return p
	}
	return catalog.DefaultPath
}

// Init starts the best-effort catalog refresh after the first frame.
func (m Model) Init() tea.Cmd {
	if os.Getenv("CPZ_NO_REFRESH") != "" {
		return nil
	}
	return refreshCmd()
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		out, err := pacman.Run(ctx, "refresh-catalog")
		return refreshMsg{err: err, out: out}
	}
}

func (m *Model) reload() {
	entries, _ := apps.LoadAll()
	if idx, err := catalog.Load(m.catalogPath); err == nil {
		m.idx = idx
	} else {
		m.idx = &catalog.Index{Version: 1}
	}
	m.all = apps.Build(m.lang, entries, m.idx)
	m.refreshMenu()
	if m.view == viewCategory {
		m.applyCategoryFilter()
	}
}

func (m *Model) refreshMenu() {
	prev, _ := m.menu.Selected()
	m.menu.SetItems(m.menuItems())
	if prev.ID != "" {
		for i, it := range m.menu.Items {
			if it.ID == prev.ID {
				m.menu.Cursor = i
				m.menu.EnsureVisible(m.menuRows())
				break
			}
		}
	}
}

func (m *Model) menuItems() []tui.Item {
	items := make([]tui.Item, 0, 8)
	right := m.tr.T("apps.row.never")
	if !m.idx.Generated.IsZero() {
		right = m.tr.T("apps.row.update_at", "date", m.idx.Generated.Local().Format("2006-01-02"))
	}
	items = append(items, tui.Item{ID: itemUpdate, Name: m.tr.T("apps.row.update"), Right: right})

	installed := 0
	counts := map[string]int{}
	for _, a := range m.all {
		if a.Installed {
			installed++
		}
		if cat := apps.MainCategory(a.Categories); cat != "" {
			counts[cat]++
		}
	}
	items = append(items, tui.Item{
		ID:    catInstalled,
		Name:  m.tr.T("apps.category.installed"),
		Right: fmt.Sprintf("%d", installed),
	})

	cats := make([]string, 0, len(counts))
	for cat := range counts {
		cats = append(cats, cat)
	}
	coll := collate.New(language.Make(m.lang))
	sort.Slice(cats, func(i, j int) bool {
		return coll.CompareString(categoryLabel(m.tr, cats[i]), categoryLabel(m.tr, cats[j])) < 0
	})
	for _, cat := range cats {
		items = append(items, tui.Item{
			ID:    cat,
			Name:  categoryLabel(m.tr, cat),
			Right: fmt.Sprintf("%d", counts[cat]),
		})
	}
	return items
}

func (m *Model) enterCategory(id string) {
	m.category = id
	m.query = ""
	m.search.Clear()
	m.search.Active = false
	m.view = viewCategory
	m.status = ""
	m.list.Cursor = 0
	m.list.Offset = 0
	m.applyCategoryFilter()
}

func (m *Model) leaveCategory() {
	m.view = viewMenu
	m.query = ""
	m.search.Clear()
	m.search.Active = false
	m.status = ""
}

func (m *Model) filteredApps() []apps.App {
	out := make([]apps.App, 0, len(m.all))
	for _, a := range m.all {
		if m.category == catInstalled {
			if !a.Installed {
				continue
			}
		} else if apps.MainCategory(a.Categories) != m.category {
			continue
		}
		if !apps.Match(a, m.query, m.lang) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (m *Model) applyCategoryFilter() {
	prev, _ := m.list.Selected()
	list := m.filteredApps()
	items := make([]tui.Item, 0, len(list))
	for _, a := range list {
		item := tui.Item{ID: a.ID, Name: a.Name, Right: a.Version}
		if a.Installed {
			item.Marker = "*"
		}
		items = append(items, item)
	}
	m.list.SetItems(items)
	if prev.ID != "" {
		m.selectID(prev.ID)
	}
	if m.query != "" && len(list) == 0 {
		m.status = m.tr.T("apps.no_matches")
	}
}

func (m *Model) selectID(id string) {
	m.list.Cursor = 0
	for i, it := range m.list.Items {
		if it.ID == id {
			m.list.Cursor = i
			m.list.EnsureVisible(m.categoryRows())
			return
		}
	}
}

func (m *Model) find(id string) (apps.App, bool) {
	for _, a := range m.all {
		if a.ID == id {
			return a, true
		}
	}
	return apps.App{}, false
}

func (m Model) menuRows() int {
	rows := m.height - 1
	if m.status != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) categoryRows() int {
	rows := m.height - 1
	if m.status != "" {
		rows--
	}
	if m.search.Active {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case refreshMsg:
		if msg.err != nil {
			if strings.Contains(msg.out, "lock") {
				m.status = m.tr.T("apps.status.busy")
			} else {
				m.status = m.tr.T("apps.status.offline")
			}
			return m, nil
		}
		m.reload()
		m.status = m.tr.T("apps.status.refreshed")
		return m, nil
	case launchedMsg:
		if msg.err != nil {
			m.startingID, m.startingName = "", ""
			if errors.Is(msg.err, launch.ErrRunning) {
				m.status = m.tr.T("apps.status.running", "name", msg.name)
			} else {
				m.status = m.tr.T("apps.status.failed", "error", msg.err.Error())
			}
			return m, nil
		}
		return m, waitLaunch(msg.proc)
	case launchDoneMsg:
		m.startingID, m.startingName = "", ""
		if msg.err != nil {
			m.status = m.tr.T("apps.status.failed", "error", msg.err.Error())
		} else if m.status == m.tr.T("apps.status.starting", "name", msg.name) {
			m.status = ""
		}
		return m, nil
	case ownerMsg:
		if m.view == viewDetail && m.detail.ID == msg.id {
			if m.detail.Package == "" {
				m.detail.Package = msg.pkg
			}
			if m.detail.Version == "" {
				m.detail.Version = msg.version
			}
			m.buildActions()
		}
		return m, nil
	case opMsg:
		id := m.detail.ID
		m.reload()
		m.working = ""
		if msg.err != nil {
			m.view = m.returnTo
			line := pacman.Reason(msg.out)
			if line == "" {
				line = msg.err.Error()
			}
			if strings.Contains(msg.out, "lock") {
				m.status = m.tr.T("apps.status.busy")
			} else {
				m.status = m.tr.T("apps.status.failed", "error", line)
			}
			return m, nil
		}
		switch msg.kind {
		case actUpdate:
			m.view = viewMenu
		case actRemove:
			if len(m.filteredApps()) == 0 {
				m.leaveCategory()
			} else {
				m.view = viewCategory
			}
		default:
			if a, ok := m.find(id); ok {
				m.detail = a
				m.buildActions()
			}
			m.view = viewDetail
		}
		m.status = m.tr.T("apps.status.done")
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == viewWorking {
		return m, nil
	}
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

	switch m.view {
	case viewMenu:
		return m.handleMenu(msg)
	case viewCategory:
		return m.handleCategory(msg)
	case viewConfirm:
		return m.handleConfirm(msg)
	case viewDetail:
		return m.handleDetail(msg)
	}
	return m, nil
}

func (m Model) handleMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := m.menuRows()
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		m.menu.Move(-1, visible)
	case tui.ActionDown:
		m.menu.Move(1, visible)
	case tui.ActionPageUp:
		m.menu.Move(-visible, visible)
	case tui.ActionPageDown:
		m.menu.Move(visible, visible)
	case tui.ActionFirst:
		m.menu.MoveFirst()
	case tui.ActionLast:
		m.menu.MoveLast(visible)
	case tui.ActionHelp:
		m.help = true
	case tui.ActionBack, tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		item, ok := m.menu.Selected()
		if !ok {
			return m, nil
		}
		if item.ID == itemUpdate {
			m.pending = actUpdate
			m.returnTo = viewMenu
			m.confirm.Open(m.tr.T("apps.confirm.update"))
			m.view = viewConfirm
			return m, nil
		}
		m.enterCategory(item.ID)
	}
	return m, nil
}

func (m Model) handleCategory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.search.Active {
		if r, ok := tui.PrintableRune(msg); ok && !tui.NavRune(r) {
			m.search.Active = true
			m.search.Value = string(r)
			m.query = m.search.Value
			m.applyCategoryFilter()
			return m, nil
		}
	}
	if m.search.Active {
		switch tui.ActionOf(msg, true) {
		case tui.ActionBack:
			m.search.Clear()
			m.search.Active = false
			m.query = ""
			m.applyCategoryFilter()
			return m, nil
		case tui.ActionConfirm, tui.ActionNextField:
			m.search.Active = false
			m.query = m.search.Value
			m.applyCategoryFilter()
			return m, nil
		}
		if m.search.Handle(msg) {
			m.query = m.search.Value
			m.applyCategoryFilter()
		}
		return m, nil
	}

	visible := m.categoryRows()
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
	case tui.ActionNextField:
		m.search.Active = true
	case tui.ActionHelp:
		m.help = true
	case tui.ActionBack:
		m.leaveCategory()
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		return m.openSelected()
	}
	return m, nil
}

func (m Model) openSelected() (tea.Model, tea.Cmd) {
	item, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	app, ok := m.find(item.ID)
	if !ok {
		return m, nil
	}
	m.detail = app
	m.buildActions()
	m.view = viewDetail
	if app.Installed && app.Package == "" && app.Path != "" {
		return m, m.ownerCmd(app.ID, app.Path)
	}
	return m, nil
}

func (m *Model) ownerCmd(id, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pkg := pacman.Owner(ctx, path)
		ver := ""
		if pkg != "" {
			ver = pacman.InstalledVersion(ctx, pkg)
		}
		return ownerMsg{id: id, pkg: pkg, version: ver}
	}
}

func (m *Model) buildActions() {
	a := m.detail
	items := make([]tui.Item, 0, 3)
	if a.Installed {
		items = append(items, tui.Item{ID: "open", Name: m.tr.T("apps.actions.open")})
		if !a.System {
			items = append(items, tui.Item{ID: "remove", Name: m.tr.T("apps.actions.remove")})
		}
	} else {
		items = append(items, tui.Item{ID: "install", Name: m.tr.T("apps.actions.install")})
	}
	items = append(items, tui.Item{ID: "back", Name: m.tr.T("apps.actions.back")})
	m.actions.SetItems(items)
}

func (m Model) handleDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := len(m.actions.Items)
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		m.actions.Move(-1, visible)
	case tui.ActionDown:
		m.actions.Move(1, visible)
	case tui.ActionBack:
		m.view = viewCategory
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		item, ok := m.actions.Selected()
		if !ok {
			return m, nil
		}
		switch item.ID {
		case "back":
			m.view = viewCategory
		case "open":
			return m.openDetail()
		case "install":
			m.pending = actInstall
			m.returnTo = viewDetail
			m.confirm.Open(m.tr.T("apps.confirm.install", "name", m.detail.Name))
			m.view = viewConfirm
		case "remove":
			if m.detail.Package == "" {
				m.status = m.tr.T("apps.status.failed", "error", "unknown package")
				return m, nil
			}
			m.pending = actRemove
			m.returnTo = viewCategory
			m.confirm.Open(m.tr.T("apps.confirm.remove", "name", m.detail.Name))
			m.view = viewConfirm
		}
	}
	return m, nil
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	if m.startingID != "" {
		m.status = m.tr.T("apps.status.busy_launch", "name", m.startingName)
		return m, nil
	}
	a := m.detail
	argv, err := launch.Expand(a.Exec, a.Name, a.ID, a.Icon, a.Path)
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
		return m, nil
	}
	spec := launch.Spec{Argv: argv, Name: a.Name, ID: a.ID, Path: a.Path}
	if a.Terminal {
		if err := launch.NewTab(spec); err != nil {
			m.status = m.tr.T("apps.status.failed", "error", err.Error())
		}
		return m, nil
	}
	// A graphical app may take seconds to map its window. Say that it is
	// starting before spawning it, and keep the message until it exits. While
	// it is set, another Open is refused.
	m.startingID, m.startingName = a.ID, a.Name
	m.status = m.tr.T("apps.status.starting", "name", a.Name)
	return m, func() tea.Msg {
		proc, err := launch.Start(spec)
		return launchedMsg{name: a.Name, proc: proc, err: err}
	}
}

func waitLaunch(p *launch.Process) tea.Cmd {
	return func() tea.Msg {
		return launchDoneMsg{name: p.Name(), err: p.Wait()}
	}
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	done, ok := m.confirm.Handle(tui.ActionOf(msg, false))
	if !done {
		return m, nil
	}
	if !ok {
		m.view = m.returnTo
		return m, nil
	}
	kind := m.pending
	m.view = viewWorking
	m.pending = actNone
	switch kind {
	case actUpdate:
		m.working = m.tr.T("apps.status.refreshing")
	case actInstall:
		m.working = m.tr.T("apps.status.installing", "name", m.detail.Name)
	case actRemove:
		m.working = m.tr.T("apps.status.removing", "name", m.detail.Name)
	}
	return m, opCmd(kind, m.detail.Package)
}

func opCmd(kind actionKind, pkg string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		verb := ""
		switch kind {
		case actInstall:
			verb = "install"
		case actRemove:
			verb = "remove"
		}
		if kind == actUpdate {
			// The user asked for a refresh: bypass the automatic 10-minute
			// rate limit in the helper.
			out, err := pacman.Run(ctx, "refresh-catalog", "--force")
			return opMsg{kind: kind, out: out, err: err}
		}
		if verb == "" {
			return opMsg{kind: actNone}
		}
		out, err := pacman.Run(ctx, verb, pkg)
		return opMsg{kind: kind, out: out, err: err}
	}
}

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("apps.title"),
		Status: m.status,
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("help.title"), m.tr.T("help.body"))
		return s.Render()
	}
	switch m.view {
	case viewDetail:
		s.Title = m.detail.Name
		s.Context = m.detail.Version
		rows := m.height - 1
		if m.status != "" {
			rows--
		}
		s.Body = m.detailBody(rows)
	case viewConfirm:
		s.Body = m.confirm.Render(tui.YesNo{Yes: m.tr.T("common.yes"), No: m.tr.T("common.no")})
	case viewWorking:
		s.Body = []string{m.working}
	case viewCategory:
		label := categoryLabel(m.tr, m.category)
		if m.category == catInstalled {
			label = m.tr.T("apps.category.installed")
		}
		s.Context = fmt.Sprintf("%s  %d", label, len(m.list.Items))
		rows := m.categoryRows()
		var body []string
		if m.search.Active {
			body = append(body, tui.NormalStyle.Render(m.search.Render(m.width)))
		}
		body = append(body, m.list.Render(m.width, rows)...)
		s.Body = body
	default:
		s.Body = m.menu.Render(m.width, m.menuRows())
	}
	return s.Render()
}

// detailBody keeps the action list on the last rows. Metadata that does not
// fit is dropped; an action hidden under the tmux status row is a bug.
func (m Model) detailBody(rows int) []string {
	a := m.detail
	acts := m.actions.Render(m.width, len(m.actions.Items))
	if rows < len(acts)+1 {
		rows = len(acts) + 1
	}
	metaLimit := rows - len(acts) - 1 // one blank gap above the actions
	if metaLimit < 1 {
		metaLimit = 1
	}

	var meta []string
	if a.Comment != "" {
		wrapped := wrap(a.Comment, m.width)
		if len(wrapped) > 2 {
			wrapped = wrapped[:2]
		}
		meta = append(meta, wrapped...)
	}
	var parts []string
	if cat := apps.MainCategory(a.Categories); cat != "" {
		parts = append(parts, categoryLabel(m.tr, cat))
	}
	if a.Version != "" {
		parts = append(parts, a.Version)
	}
	if a.Size > 0 {
		parts = append(parts, humanSize(a.Size))
	}
	if len(parts) > 0 {
		meta = append(meta, strings.Join(parts, " · "))
	}
	if a.Package != "" {
		meta = append(meta, m.tr.T("apps.detail.package")+": "+a.Package)
	}
	state := m.tr.T("apps.detail.available")
	if a.Installed {
		state = m.tr.T("apps.detail.installed")
	}
	if a.System {
		state += " · " + m.tr.T("apps.detail.system")
	}
	meta = append(meta, state)
	if len(meta) > metaLimit {
		meta = meta[:metaLimit]
	}

	body := append([]string{}, meta...)
	for len(body) < rows-len(acts) {
		body = append(body, "")
	}
	return append(body, acts...)
}

func categoryLabel(tr *i18n.Translator, cat string) string {
	if cat == "" {
		return ""
	}
	key := "cat." + cat
	if label := tr.T(key); label != key {
		return label
	}
	return cat
}

func humanSize(n int64) string {
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	}
	return fmt.Sprintf("%d kB", n/1024)
}

func wrap(s string, width int) []string {
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
			continue
		}
		if len([]rune(line))+1+len([]rune(w)) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
