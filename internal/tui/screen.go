package tui

import (
	"strings"
)

// Screen is the shared chrome: one title row, the body, one transient status.
// There are no borders, no footer hints and no scrollbars.
type Screen struct {
	Width   int
	Height  int
	Title   string
	Context string
	Body    []string
	Status  string
}

// Render assembles the screen to exactly Width columns.
func (s Screen) Render() string {
	if s.Width < 1 || s.Height < 1 {
		return ""
	}
	lines := make([]string, 0, s.Height)

	title := Title(s.Title, s.Context, s.Width)
	lines = append(lines, title)

	bodyRows := s.Height - 1
	if s.Status != "" && s.Height > 1 {
		bodyRows--
	}
	if bodyRows < 0 {
		bodyRows = 0
	}
	for i := 0; i < bodyRows; i++ {
		if i < len(s.Body) {
			lines = append(lines, Pad(s.Body[i], s.Width))
		} else {
			lines = append(lines, strings.Repeat(" ", s.Width))
		}
	}
	if s.Status != "" && s.Height > 1 {
		lines = append(lines, DimStyle.Render(Fit(s.Status, s.Width)))
	}
	return strings.Join(lines, "\n")
}

// Title renders a left title and a right context on one dim row.
func Title(title, context string, width int) string {
	if context == "" {
		return TitleStyle.Render(Fit(title, width))
	}
	ctx := Truncate(context, width/2)
	leftW := width - Width(ctx) - 1
	if leftW < 1 {
		leftW = 1
	}
	line := Fit(title, leftW) + " " + ctx
	return TitleStyle.Render(Fit(line, width))
}

// TitleStyle is the dim single header row.
var TitleStyle = DimStyle

// Confirm is a full-screen yes/no dialog. The default answer is No.
type Confirm struct {
	Question string
	Yes      bool
	Active   bool
}

// Open shows the dialog with No focused.
func (c *Confirm) Open(question string) {
	c.Question = question
	c.Yes = false
	c.Active = true
}

// Handle returns done=true when the dialog closed and ok=true for Yes.
func (c *Confirm) Handle(a Action) (done, ok bool) {
	if !c.Active {
		return false, false
	}
	switch a {
	case ActionUp, ActionDown, ActionLeft, ActionRight:
		c.Yes = !c.Yes
	case ActionBack, ActionQuit:
		c.Active = false
		return true, false
	case ActionConfirm:
		c.Active = false
		return true, c.Yes
	}
	return false, false
}

// Render returns the dialog body.
func (c *Confirm) Render(t YesNo) []string {
	no := "  " + t.No
	yes := "  " + t.Yes
	noStyle, yesStyle := NormalStyle, NormalStyle
	if c.Yes {
		yes = "> " + t.Yes
		yesStyle = FocusStyle
	} else {
		no = "> " + t.No
		noStyle = FocusStyle
	}
	return []string{
		c.Question,
		"",
		noStyle.Render(no),
		yesStyle.Render(yes),
	}
}

// HelpBody renders a translated help overlay.
func HelpBody(title, body string) []string {
	lines := []string{DimStyle.Render(title), ""}
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, NormalStyle.Render(line))
	}
	return lines
}

// YesNo is a translated pair for the confirmation dialog.
type YesNo struct {
	Yes string
	No  string
}
