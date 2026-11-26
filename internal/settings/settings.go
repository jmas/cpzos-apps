package settings

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/about"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/pacman"
	"github.com/jmas/cpzos-apps/internal/tui"
)

type view int

const (
	viewRoot view = iota
	viewAbout
	viewConfirm
	viewUpdating
)

type aboutMsg struct{ fields []about.Field }

type updateMsg struct {
	out       string
	err       error
	cancelled bool
}

// Model is the Settings app: a system update and the read-only About screen.
type Model struct {
	lang   string
	tr     *i18n.Translator
	width  int
	height int

	root       tui.List
	view       view
	about      tui.List
	data       []about.Field
	ready      bool
	help       bool
	confirm    tui.Confirm
	status     string
	spinner    tui.Spinner
	cancel     context.CancelFunc
	cancelling bool
}

// New builds the root list.
func New() Model {
	lang := i18n.System()
	tr := i18n.For(lang)
	m := Model{lang: lang, tr: tr, width: 45, height: 13}
	m.root.SetItems([]tui.Item{
		{ID: "update", Name: tr.T("settings.update")},
		{ID: "about", Name: tr.T("settings.about")},
	})
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
	case tui.SpinnerTickMsg:
		if m.view == viewUpdating {
			m.spinner.Advance()
			return m, m.spinner.Tick()
		}
		return m, nil
	case updateMsg:
		m.view = viewRoot
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.cancelling = false
		if msg.cancelled {
			m.status = m.tr.T("settings.cancelled")
			return m, nil
		}
		if msg.err != nil {
			line := pacman.Reason(msg.out)
			if line == "" {
				line = msg.err.Error()
			}
			if strings.Contains(msg.out, "lock") {
				m.status = m.tr.T("settings.update_busy")
			} else {
				m.status = m.tr.T("settings.update_failed", "error", line)
			}
			return m, nil
		}
		// The installed versions may have changed; re-read About in the
		// background before the user opens it.
		m.ready = false
		m.status = m.tr.T("settings.updated")
		return m, m.Init()
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

func (m Model) rows() int {
	rows := m.height - 1
	if m.status != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == viewUpdating {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionQuit:
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
				m.cancelling = true
			}
		}
		return m, nil
	}
	if m.help {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack, tui.ActionHelp, tui.ActionConfirm, tui.ActionQuit:
			m.help = false
		}
		return m, nil
	}
	if m.view == viewConfirm {
		done, ok := m.confirm.Handle(tui.ActionOf(msg, false))
		if !done {
			return m, nil
		}
		if !ok {
			m.view = viewRoot
			return m, nil
		}
		m.status = ""
		m.view = viewUpdating
		m.spinner = tui.Spinner{}
		m.cancelling = false
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		m.cancel = cancel
		return m, tea.Batch(updateCmd(ctx), m.spinner.Tick())
	}
	if m.status != "" {
		m.status = ""
	}

	rows := m.rows()
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
		} else {
			return m, tea.Quit
		}
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		if m.view != viewRoot {
			return m, nil
		}
		item, ok := m.root.Selected()
		if !ok {
			return m, nil
		}
		if item.ID == "update" {
			m.confirm.Open(m.tr.T("settings.confirm.update"))
			m.view = viewConfirm
		} else {
			m.view = viewAbout
		}
	}
	return m, nil
}

func updateCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		out, err := pacman.Run(ctx, "update-system")
		return updateMsg{out: out, err: err, cancelled: ctx.Err() == context.Canceled}
	}
}

func (m Model) View() string {
	s := tui.Screen{
		Width:  m.width,
		Height: m.height,
		Title:  m.tr.T("settings.title"),
		Status: m.status,
	}
	if m.help {
		s.Body = tui.HelpBody(m.tr.T("help.title"), m.tr.T("help.body"))
		return s.Render()
	}
	switch m.view {
	case viewAbout:
		s.Context = m.tr.T("settings.about")
		if !m.ready {
			s.Body = []string{tui.DimStyle.Render(m.tr.T("settings.loading"))}
		} else {
			s.Body = m.about.Render(m.width, m.rows())
		}
	case viewConfirm:
		s.Body = m.confirm.Render(tui.YesNo{Yes: m.tr.T("common.yes"), No: m.tr.T("common.no")})
	case viewUpdating:
		label := m.tr.T("settings.updating")
		if m.cancelling {
			label = m.tr.T("settings.cancelling")
		}
		s.Body = []string{m.spinner.Render(label)}
		s.Status = m.tr.T("settings.cancel_hint")
	default:
		s.Body = m.root.Render(m.width, m.rows())
	}
	return s.Render()
}
