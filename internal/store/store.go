package store

import (
	"context"
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
)

const updateRow = "__update__"

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
	viewList view = iota
	viewDetail
	viewConfirm
	viewWorking
)

type category struct {
	ID    string
	Label string
}

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

// Model is Apps Manager.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	catalogPath string
	idx         *catalog.Index
	all         []apps.App
	cats        []category
	cat         int
	query       string

	list   tui.List
	search tui.Input
	help   bool
	status string

	view      view
	detail    apps.App
	actions   tui.List
	confirm   tui.Confirm
	pending   actionKind
	working   string
	returnTo  view
	refreshID string
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
	m.confirm.Question = ""
	m.reload()
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
		out, err := pacman.Run(ctx, "refresh-catalog", "")
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
	m.buildCategories()
	m.applyFilter()
}

func (m *Model) buildCategories() {
	current := ""
	if len(m.cats) > 0 && m.cat < len(m.cats) {
		current = m.cats[m.cat].ID
	}
	m.cats = []category{
		{ID: "", Label: m.tr.T("apps.category.all")},
		{ID: "__installed", Label: m.tr.T("apps.category.installed")},
		{ID: "__system", Label: m.tr.T("apps.category.system")},
	}
	seen := map[string]bool{}
	for _, a := range m.all {
		cat := apps.MainCategory(a.Categories)
		if cat == "" || seen[cat] || cat == "System" {
			continue
		}
		seen[cat] = true
		m.cats = append(m.cats, category{ID: cat, Label: categoryLabel(m.tr, cat)})
	}
	sort.SliceStable(m.cats[3:], func(i, j int) bool {
		return m.cats[3+i].Label < m.cats[3+j].Label
	})
	m.cat = 0
	for i, c := range m.cats {
		if c.ID == current {
			m.cat = i
			break
		}
	}
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

func (m *Model) visibleApps() []apps.App {
	if len(m.cats) == 0 {
		return nil
	}
	cat := m.cats[m.cat].ID
	out := make([]apps.App, 0, len(m.all))
	for _, a := range m.all {
		switch cat {
		case "":
		case "__installed":
			if !a.Installed {
				continue
			}
		case "__system":
			if !a.System {
				continue
			}
		default:
			if apps.MainCategory(a.Categories) != cat {
				continue
			}
		}
		if !apps.Match(a, m.query, m.lang) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (m *Model) applyFilter() {
	prev, _ := m.list.Selected()
	list := m.visibleApps()
	items := make([]tui.Item, 0, len(list)+1)
	cat := ""
	if len(m.cats) > 0 {
		cat = m.cats[m.cat].ID
	}
	if cat == "" && m.query == "" {
		right := m.tr.T("apps.row.never")
		if !m.idx.Generated.IsZero() {
			right = m.tr.T("apps.row.update_at", "date", m.idx.Generated.Local().Format("2006-01-02"))
		}
		items = append(items, tui.Item{ID: updateRow, Name: m.tr.T("apps.row.update"), Right: right})
	}
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
	for i, it := range m.list.Items {
		if it.ID == id {
			m.list.Cursor = i
			m.list.EnsureVisible(m.listRows())
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

func (m *Model) listRows() int {
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
		m.working = ""
		m.view = m.returnTo
		id := m.detail.ID
		m.reload()
		if msg.err != nil {
			line := firstLine(msg.out)
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
		if id != "" {
			if a, ok := m.find(id); ok {
				m.detail = a
				m.buildActions()
			}
			m.selectID(id)
		}
		m.status = m.tr.T("apps.status.done")
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
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
	case viewConfirm:
		return m.handleConfirm(msg)
	case viewDetail:
		return m.handleDetail(msg)
	default:
		return m.handleList(msg)
	}
}

func (m Model) handleList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.search.Active {
		switch tui.ActionOf(msg, true) {
		case tui.ActionBack:
			m.search.Clear()
			m.search.Active = false
			m.query = ""
			m.applyFilter()
			return m, nil
		case tui.ActionConfirm, tui.ActionNextField:
			m.search.Active = false
			m.query = m.search.Value
			m.applyFilter()
			return m, nil
		}
		if m.search.Handle(msg) {
			m.query = m.search.Value
			m.applyFilter()
		}
		return m, nil
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
	case tui.ActionLeft:
		m.cat = (m.cat - 1 + len(m.cats)) % len(m.cats)
		m.applyFilter()
	case tui.ActionRight:
		m.cat = (m.cat + 1) % len(m.cats)
		m.applyFilter()
	case tui.ActionNextField:
		m.search.Active = true
	case tui.ActionHelp:
		m.help = true
	case tui.ActionBack, tui.ActionQuit:
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
	if item.ID == updateRow {
		m.pending = actUpdate
		m.returnTo = viewList
		m.confirm.Open(m.tr.T("apps.confirm.update"))
		m.view = viewConfirm
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
		m.view = viewList
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
			m.view = viewList
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
			m.returnTo = viewList
			m.confirm.Open(m.tr.T("apps.confirm.remove", "name", m.detail.Name))
			m.view = viewConfirm
		}
	}
	return m, nil
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	a := m.detail
	argv, err := launch.Expand(a.Exec, a.Name, a.ID, a.Icon, a.Path)
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
		return m, nil
	}
	spec := launch.Spec{Argv: argv, Name: a.Name, ID: a.ID, Path: a.Path}
	if a.Terminal {
		err = launch.NewTab(spec)
	} else {
		err = launch.Start(spec)
	}
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
	}
	return m, nil
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
		case actUpdate:
			verb = "refresh-catalog"
		}
		if verb == "" {
			return opMsg{kind: actNone}
		}
		if kind == actUpdate {
			pkg = ""
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
		s.Body = m.detailBody()
	case viewConfirm:
		s.Body = m.confirm.Render(tui.YesNo{Yes: m.tr.T("common.yes"), No: m.tr.T("common.no")})
	case viewWorking:
		s.Body = []string{m.working}
	default:
		rows := m.listRows()
		cat := m.cats[m.cat]
		s.Context = fmt.Sprintf("%s  %d/%d", cat.Label, len(m.visibleApps()), len(m.all))
		var body []string
		if m.search.Active {
			body = append(body, tui.NormalStyle.Render(m.search.Render(m.width)))
		}
		body = append(body, m.list.Render(m.width, rows)...)
		s.Body = body
	}
	return s.Render()
}

func (m Model) detailBody() []string {
	a := m.detail
	var lines []string
	if a.Comment != "" {
		wrapped := wrap(a.Comment, m.width)
		if len(wrapped) > 2 {
			wrapped = wrapped[:2]
		}
		lines = append(lines, wrapped...)
	}
	lines = append(lines, "")
	if cat := apps.MainCategory(a.Categories); cat != "" {
		lines = append(lines, m.tr.T("apps.detail.categories")+": "+categoryLabel(m.tr, cat))
	}
	if a.Package != "" {
		lines = append(lines, m.tr.T("apps.detail.package")+": "+a.Package)
	}
	if a.Version != "" {
		lines = append(lines, m.tr.T("apps.detail.version")+": "+a.Version)
	}
	if a.Size > 0 {
		lines = append(lines, m.tr.T("apps.detail.size")+": "+humanSize(a.Size))
	}
	state := m.tr.T("apps.detail.available")
	if a.Installed {
		state = m.tr.T("apps.detail.installed")
	}
	lines = append(lines, state)
	if a.System {
		lines = append(lines, m.tr.T("apps.detail.system"))
	}
	lines = append(lines, "")
	acts := m.actions.Render(m.width, len(m.actions.Items))
	lines = append(lines, acts...)
	return lines
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
