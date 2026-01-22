package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type SpeedMsg struct {
	BytesPerSec   float64
	TimeRemaining time.Duration
}

type DoneMsg struct{}

type ErrorMsg struct {
	Error error
}

type DebugMsg struct {
	Message string
}

type TickMsg struct{}

type QuitMode int

const (
	QuitModeNone QuitMode = iota
	QuitModeClean
	QuitModeSave
)

type QuitMsg struct {
	Mode QuitMode
}

func StartDownloadCmd(downloadFn func() tea.Msg) tea.Cmd {
	return downloadFn
}
