package home

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/launch"
	"github.com/jmas/cpzos-apps/internal/tui"
)

// ownID keeps Home out of its own list.
const ownID = "cpz-app-home.desktop"

// Model is the list shown in a new tab. It replaces its window with the chosen
// app, so a tab is the app the user picked.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	all    []apps.App
	list   tui.List
	search tui.Input
	help   bool
	status string
}

// New loads the installed entries; call it after the first frame budget allows
// (it only reads local desktop files).
func New() Model {
	lang := i18n.System()
	tr := i18n.For(lang)
	entries, _ := apps.LoadAll()
	path := os.Getenv("CPZ_CATALOG")
	if path == "" {
		path = catalog.DefaultPath
	}
	idx, err := catalog.Load(path)
	if err != nil {
		idx = &catalog.Index{Version: 1}
	}
	all := apps.Build(lang, entries, idx)
	kept := all[:0]
	for _, a := range all {
		if a.ID != ownID {
			kept = append(kept, a)
		}
	}
	m := Model{lang: lang, tr: tr, width: 45, height: 13, all: kept}
	m.search.Prompt = tr.T("home.search")
	m.filter()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
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
	if m.search.Active {
		switch tui.ActionOf(msg, true) {
		case tui.ActionBack:
			m.search.Clear()
			m.search.Active = false
			m.filter()
			return m, nil
		case tui.ActionConfirm, tui.ActionNextField:
			m.search.Active = false
			return m, nil
		}
		if m.search.Handle(msg) {
			m.filter()
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
	case tui.ActionNextField:
		m.search.Active = true
	case tui.ActionHelp:
		m.help = true
	case tui.ActionBack, tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		m.open()
	}
	return m, nil
}

func (m *Model) open() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	var app apps.App
	found := false
	for _, a := range m.all {
		if a.ID == item.ID {
			app, found = a, true
			break
		}
	}
	if !found || app.Exec == "" {
		return
	}
	argv, err := launch.Expand(app.Exec, app.Name, app.ID, app.Icon, app.Path)
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
		return
	}
	spec := launch.Spec{Argv: argv, Name: app.Name, ID: app.ID, Path: app.Path}
	if !app.Terminal {
		if err := launch.Start(spec); err != nil {
			m.status = m.tr.T("apps.status.failed", "error", err.Error())
		}
		return
	}
	if err := launch.Supervised(spec); err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
	}
}

func (m *Model) filter() {
	q := strings.TrimSpace(m.search.Value)
	items := make([]tui.Item, 0, len(m.all))
	for _, a := range m.all {
		if !apps.Match(a, q, m.lang) {
			continue
		}
		items = append(items, tui.Item{
			ID:    a.ID,
			Name:  a.Name,
			Right: categoryLabel(m.tr, a.Categories),
		})
	}
	m.list.SetItems(items)
	if len(items) == 0 && q != "" {
		m.status = m.tr.T("apps.no_matches")
	}
}

func categoryLabel(tr *i18n.Translator, cats []string) string {
	cat := apps.MainCategory(cats)
	if cat == "" {
		return ""
	}
	key := "cat." + cat
	if label := tr.T(key); label != key {
		return label
	}
	return cat
}

func (m Model) listRows() int {
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

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("home.title"),
		Status: m.status,
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("help.title"), m.tr.T("help.body"))
		return s.Render()
	}
	rows := m.listRows()
	var body []string
	if m.search.Active {
		body = append(body, tui.NormalStyle.Render(m.search.Render(m.width)))
	}
	body = append(body, m.list.Render(m.width, rows)...)
	s.Body = body
	return s.Render()
}
