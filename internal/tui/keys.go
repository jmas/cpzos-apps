package tui

import tea "github.com/charmbracelet/bubbletea"

// Action is a navigation intent decoded from a key press.
type Action int

const (
	ActionNone Action = iota
	ActionUp
	ActionDown
	ActionLeft
	ActionRight
	ActionConfirm
	ActionBack
	ActionNextField
	ActionPrevField
	ActionPageUp
	ActionPageDown
	ActionFirst
	ActionLast
	ActionHelp
	ActionQuit
)

// ActionOf decodes the CPZ navigation model. On the Cardputer Zero the plain
// F/X/Z/C keys are the physical d-pad; on the ua layout they arrive as
// Cyrillic letters. Real arrow keys always work. In text mode letters must not
// be captured, so only layout-independent keys are returned.
func ActionOf(msg tea.KeyMsg, textMode bool) Action {
	switch msg.String() {
	case "esc":
		return ActionBack
	case "ctrl+c":
		return ActionQuit
	case "tab":
		return ActionNextField
	case "shift+tab":
		return ActionPrevField
	case "enter":
		return ActionConfirm
	case "pgup":
		return ActionPageUp
	case "pgdown":
		return ActionPageDown
	case "home":
		return ActionFirst
	case "end":
		return ActionLast
	}

	switch msg.String() {
	case "up":
		return ActionUp
	case "down":
		return ActionDown
	case "left":
		return ActionLeft
	case "right":
		return ActionRight
	}

	if textMode {
		return ActionNone
	}

	switch msg.String() {
	case "f", "а": // physical F
		return ActionUp
	case "x", "ч": // physical X
		return ActionDown
	case "z", "я": // physical Z
		return ActionLeft
	case "c", "с": // physical C
		return ActionRight
	case "h", "р", "?":
		return ActionHelp
	}
	return ActionNone
}
