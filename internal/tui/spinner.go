package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SpinnerTickMsg advances the spinner. The tick is 100 ms, the slowest redraw
// the TUI contract allows for a working screen.
type SpinnerTickMsg struct{}

var spinnerFrames = []string{"|", "/", "-", "\\"}

// Spinner is an ASCII progress indicator. It never draws a progress bar or a
// footer; the caller puts it in the single working body line.
type Spinner struct {
	Frame int
}

// Tick schedules the next frame.
func (s *Spinner) Tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// Advance moves to the next frame.
func (s *Spinner) Advance() {
	s.Frame = (s.Frame + 1) % len(spinnerFrames)
}

// Render returns the current frame and a label.
func (s Spinner) Render(label string) string {
	frame := spinnerFrames[0]
	if len(spinnerFrames) > 0 {
		frame = spinnerFrames[s.Frame%len(spinnerFrames)]
	}
	return frame + " " + label
}
