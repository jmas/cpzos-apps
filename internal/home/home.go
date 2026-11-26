package home

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/i18n"
	"github.com/jmas/cpzos-apps/internal/launch"
	"github.com/jmas/cpzos-apps/internal/media"
	"github.com/jmas/cpzos-apps/internal/openwith"
	"github.com/jmas/cpzos-apps/internal/tui"
)

// ownID keeps Home out of its own list.
const ownID = "cpz-app-home.desktop"

// launchedMsg is the result of spawning a graphical app off the event loop.
type launchedMsg struct {
	name string
	proc *launch.Process
	err  error
}

// launchDoneMsg says that a started graphical app exited.
type launchDoneMsg struct {
	name string
	err  error
}

type mediaChangedMsg struct{}

type unmountMsg struct{ err error }

type view int

const (
	viewApps view = iota
	viewMedia
	viewOpenWith
)

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

	media      []media.Entry
	mediaCh    <-chan struct{}
	mediaClose func()
	view       view

	mediaEntry   media.Entry
	mediaActions tui.List
	unmounting   bool

	handlers    []openwith.Handler
	handlerList tui.List

	// A graphical app owns the tab while it starts. Refuse another launch from
	// the same tab until it exits, even though this Home stays on screen until
	// the app maps its window.
	startingID   string
	startingName string
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
		// Home shows what the system really has: a catalog record alone is an
		// app that is not installed yet. Installing is the Applications screen's job, so
		// a user never lands on an app that cannot start.
		if !a.Installed || a.ID == ownID {
			continue
		}
		kept = append(kept, a)
	}
	m := Model{lang: lang, tr: tr, width: 45, height: 13, all: kept}
	m.search.Prompt = tr.T("home.search")
	m.media = media.List()
	m.mediaCh, m.mediaClose, _ = media.Watch()
	m.filter()
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
		return m, nil
	case mediaChangedMsg:
		m.media = media.List()
		if m.view == viewMedia || m.view == viewOpenWith {
			if !m.hasMedia(m.mediaEntry.Device) {
				m.view = viewApps
			} else {
				m.handlers = openwith.Candidates(m.mediaEntry.Mountpoint, m.lang)
			}
		}
		m.filter()
		return m, m.waitMedia()
	case unmountMsg:
		m.unmounting = false
		m.media = media.List()
		m.filter()
		m.view = viewApps
		if msg.err != nil {
			m.status = m.tr.T("media.unmount_failed", "error", msg.err.Error())
		} else {
			m.status = m.tr.T("media.unmounted")
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
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
	if m.unmounting {
		return m, nil
	}
	if m.view == viewMedia {
		return m.handleMediaKey(msg)
	}
	if m.view == viewOpenWith {
		return m.handleOpenWithKey(msg)
	}
	if m.status != "" {
		m.status = ""
	}
	if !m.search.Active {
		if r, ok := tui.PrintableRune(msg); ok && !tui.NavRune(r) {
			m.search.Active = true
			m.search.Value = string(r)
			m.filter()
			return m, nil
		}
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
		return m.open()
	}
	return m, nil
}

func (m *Model) open() (tea.Model, tea.Cmd) {
	if m.startingID != "" {
		m.status = m.tr.T("apps.status.busy_launch", "name", m.startingName)
		return m, nil
	}
	item, ok := m.list.Selected()
	if !ok {
		return m, nil
	}
	if strings.HasPrefix(item.ID, "media:") {
		return m.openMedia(item.ID), nil
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
		return m, nil
	}
	argv, err := launch.Expand(app.Exec, app.Name, app.ID, app.Icon, app.Path)
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
		return m, nil
	}
	spec := launch.Spec{Argv: argv, Name: app.Name, ID: app.ID, Path: app.Path}
	if !app.Terminal {
		// Doom and other graphical apps take seconds to map a window. Show the
		// launch before spawning so the key never looks dead, and keep it until
		// the app exits. While it is set, another Enter is refused instead of
		// starting a second app over the first.
		m.startingID, m.startingName = app.ID, app.Name
		m.status = m.tr.T("apps.status.starting", "name", app.Name)
		return m, func() tea.Msg {
			proc, err := launch.Start(spec)
			return launchedMsg{name: app.Name, proc: proc, err: err}
		}
	}
	if err := launch.Supervised(spec); err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
	}
	return m, nil
}

func (m Model) openMedia(id string) Model {
	dev := strings.TrimPrefix(id, "media:")
	for _, e := range m.media {
		if e.Device == dev {
			m.mediaEntry = e
			m.handlers = openwith.Candidates(e.Mountpoint, m.lang)
			m.mediaActions.SetItems([]tui.Item{
				{ID: "open", Name: m.tr.T("common.open")},
				{ID: "openwith", Name: m.tr.T("media.actions.open_with")},
				{ID: "unmount", Name: m.tr.T("media.actions.unmount")},
				{ID: "back", Name: m.tr.T("common.back")},
			})
			m.view = viewMedia
			m.status = ""
			return m
		}
	}
	return m
}

func (m Model) handleMediaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := m.listRows()
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		m.mediaActions.Move(-1, visible)
	case tui.ActionDown:
		m.mediaActions.Move(1, visible)
	case tui.ActionPageUp:
		m.mediaActions.Move(-visible, visible)
	case tui.ActionPageDown:
		m.mediaActions.Move(visible, visible)
	case tui.ActionFirst:
		m.mediaActions.MoveFirst()
	case tui.ActionLast:
		m.mediaActions.MoveLast(visible)
	case tui.ActionBack:
		m.view = viewApps
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		item, ok := m.mediaActions.Selected()
		if !ok {
			return m, nil
		}
		switch item.ID {
		case "open":
			h, ok := openwith.Default(m.mediaEntry.Mountpoint, m.lang)
			if !ok {
				m.status = m.tr.T("media.no_handler")
				return m, nil
			}
			return m.launchHandler(h)
		case "openwith":
			m.view = viewOpenWith
			items := make([]tui.Item, 0, len(m.handlers))
			for _, h := range m.handlers {
				marker := ""
				if h.Default {
					marker = "*"
				}
				items = append(items, tui.Item{ID: h.ID, Name: h.Name, Marker: marker})
			}
			m.handlerList.SetItems(items)
			if len(items) == 0 {
				m.status = m.tr.T("media.no_handler")
			}
		case "unmount":
			return m.startUnmount()
		case "back":
			m.view = viewApps
		}
	}
	return m, nil
}

func (m Model) handleOpenWithKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.handlers) == 0 {
		switch tui.ActionOf(msg, false) {
		case tui.ActionBack:
			m.view = viewMedia
		case tui.ActionQuit:
			return m, tea.Quit
		}
		return m, nil
	}
	visible := m.listRows()
	switch tui.ActionOf(msg, false) {
	case tui.ActionUp:
		m.handlerList.Move(-1, visible)
	case tui.ActionDown:
		m.handlerList.Move(1, visible)
	case tui.ActionPageUp:
		m.handlerList.Move(-visible, visible)
	case tui.ActionPageDown:
		m.handlerList.Move(visible, visible)
	case tui.ActionFirst:
		m.handlerList.MoveFirst()
	case tui.ActionLast:
		m.handlerList.MoveLast(visible)
	case tui.ActionBack:
		m.view = viewMedia
	case tui.ActionHelp:
		m.help = true
	case tui.ActionQuit:
		return m, tea.Quit
	case tui.ActionConfirm:
		item, ok := m.handlerList.Selected()
		if !ok {
			return m, nil
		}
		for _, h := range m.handlers {
			if h.ID == item.ID {
				return m.launchHandler(h)
			}
		}
	}
	return m, nil
}

func (m Model) launchHandler(h openwith.Handler) (tea.Model, tea.Cmd) {
	if m.startingID != "" {
		m.status = m.tr.T("apps.status.busy_launch", "name", m.startingName)
		return m, nil
	}
	spec, err := openwith.Spec(h, m.mediaEntry.Mountpoint, m.lang)
	if err != nil {
		m.status = m.tr.T("apps.status.failed", "error", err.Error())
		return m, nil
	}
	if spec.Terminal {
		if err := launch.NewTab(spec); err != nil {
			m.status = m.tr.T("apps.status.failed", "error", err.Error())
			return m, nil
		}
		m.status = m.tr.T("media.opened")
		m.view = viewApps
		return m, nil
	}
	m.startingID, m.startingName = spec.ID, h.Name
	m.status = m.tr.T("apps.status.starting", "name", h.Name)
	return m, func() tea.Msg {
		proc, err := launch.Start(spec)
		return launchedMsg{name: h.Name, proc: proc, err: err}
	}
}

func (m Model) startUnmount() (tea.Model, tea.Cmd) {
	m.unmounting = true
	m.status = m.tr.T("media.unmounting")
	entry := m.mediaEntry
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return unmountMsg{err: media.Unmount(ctx, entry)}
	}
}

func (m Model) hasMedia(dev string) bool {
	for _, e := range m.media {
		if e.Device == dev {
			return true
		}
	}
	return false
}

func waitLaunch(p *launch.Process) tea.Cmd {
	return func() tea.Msg {
		return launchDoneMsg{name: p.Name(), err: p.Wait()}
	}
}

func (m *Model) filter() {
	q := strings.TrimSpace(m.search.Value)
	items := make([]tui.Item, 0, len(m.all)+len(m.media))
	for _, e := range m.media {
		if q != "" && !strings.Contains(strings.ToLower(e.Label), strings.ToLower(q)) {
			continue
		}
		right := m.tr.T("media.media")
		if e.Free != "" {
			right = m.tr.T("media.free", "size", e.Free)
		}
		items = append(items, tui.Item{ID: "media:" + e.Device, Name: e.Label, Right: right})
	}
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
	if m.view == viewMedia {
		s.Title = m.mediaEntry.Label
		if m.mediaEntry.Free != "" {
			s.Context = m.tr.T("media.free", "size", m.mediaEntry.Free)
		}
		s.Body = m.mediaActions.Render(m.width, m.listRows())
		return s.Render()
	}
	if m.view == viewOpenWith {
		s.Title = m.tr.T("media.open_with")
		s.Context = m.mediaEntry.Label
		if len(m.handlers) == 0 {
			s.Body = []string{tui.DimStyle.Render(m.tr.T("media.no_handler"))}
		} else {
			s.Body = m.handlerList.Render(m.width, m.listRows())
		}
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
