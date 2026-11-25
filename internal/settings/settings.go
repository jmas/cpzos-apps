package settings

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/about"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/tui"
)

type view int

const (
	viewRoot view = iota
	viewAbout
)

type aboutMsg struct{ fields []about.Field }

// Model is the Settings app. Today it has one item: About system.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	root  tui.List
	view  view
	about tui.List
	data  []about.Field
	ready bool
	help  bool
}

// New builds the root list.
func New() Model {
	lang := i18n.System()
	tr := i18n.For(lang)
	m := Model{lang: lang, tr: tr, width: 45, height: 13}
	m.root.SetItems([]tui.Item{{ID: "about", Name: tr.T("settings.about")}})
	return m
}

// Init reads the system facts after the first frame.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return aboutMsg{fields: about.Load(ctx, m.lang)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case aboutMsg:
		m.data = msg.fields
		m.ready = true
		m.buildAbout()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) buildAbout() {
	items := make([]tui.Item, 0, len(m.data))
	for _, f := range m.data {
		items = append(items, tui.Item{ID: f.Key, Name: m.tr.T(f.Key), Right: f.Value})
	}
	m.about.SetItems(items)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionHelp, tui.ActionConfirm, tui.ActionQuit:
			m.help = false
		}
		return m, nil
	}
	rows := m.height - 1
	if rows < 1 {
		rows = 1
	}
	list := &m.root
	if m.view == viewAbout {
		list = &m.about
	}
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		list.Move(-1, rows)
	case tui.ActionDown:
		list.Move(1, rows)
	case tui.ActionPageUp:
		list.Move(-rows, rows)
	case tui.ActionPageDown:
		list.Move(rows, rows)
	case tui.ActionFirst:
		list.MoveFirst()
	case tui.ActionLast:
		list.MoveLast(rows)
	case tui.ActionBack:
		if m.view == viewAbout {
			m.view = viewRoot
		}
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		if m.view == viewRoot {
			m.view = viewAbout
		}
	}
	return m, nil
}

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("settings.title"),
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("help.title"), m.tr.T("help.body"))
		return s.Render()
	}
	rows := m.height - 1
	if m.view == viewAbout {
		s.Context = m.tr.T("settings.about")
		if !m.ready {
			s.Body = []string{tui.DimStyle.Render(m.tr.T("settings.loading"))}
		} else {
			s.Body = m.about.Render(m.width, rows)
		}
		return s.Render()
	}
	s.Body = m.root.Render(m.width, rows)
	return s.Render()
}
