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

type TickMsg struct{}

func StartDownloadCmd(downloadFn func() tea.Msg) tea.Cmd {
	return downloadFn
}
