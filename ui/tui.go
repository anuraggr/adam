package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"adam/util"

	tea "github.com/charmbracelet/bubbletea"
)

const defaultRows = 8 // rows are fixed for now

// Bubble tea model
type Model struct {
	mu             sync.RWMutex
	width          int
	height         int
	rows           int
	cols           int
	chunks         int // rows * cols
	chunkStatus    []bool
	bytesTotal     int64
	speed          float64
	done           bool
	err            error
	fileName       string
	startTime      time.Time
	timeRemaining  time.Duration
	progressMu     sync.RWMutex
	workerProgress map[int]*WorkerProgress // worker id -> progress

	// Pause state
	paused   bool
	pauseCh  chan struct{}
	pauseMu  sync.RWMutex
	quitMode QuitMode
}

type WorkerProgress struct {
	Start    int64
	End      int64
	Received int64
}

func New(fileName string, totalSize int64) *Model {
	return &Model{
		fileName:       fileName,
		bytesTotal:     totalSize,
		rows:           defaultRows,
		startTime:      time.Now(),
		workerProgress: make(map[int]*WorkerProgress),
		pauseCh:        make(chan struct{}),
	}
}

// register workers byte range
func (m *Model) RegisterWorker(id int, start, end int64) {
	m.progressMu.Lock()
	defer m.progressMu.Unlock()
	m.workerProgress[id] = &WorkerProgress{
		Start: start,
		End:   end,
	}
}

// update workers downloaded bytes
func (m *Model) UpdateWorkerProgress(id int, received int64) {
	m.progressMu.Lock()
	defer m.progressMu.Unlock()
	if wp, ok := m.workerProgress[id]; ok {
		wp.Received = received
	}
}

func (m *Model) Pause() {
	m.pauseMu.Lock()
	defer m.pauseMu.Unlock()
	if !m.paused {
		m.paused = true
		m.pauseCh = make(chan struct{})
	}
}

func (m *Model) Resume() {
	m.pauseMu.Lock()
	defer m.pauseMu.Unlock()
	if m.paused {
		m.paused = false
		close(m.pauseCh)
	}
}

func (m *Model) IsPaused() bool {
	m.pauseMu.RLock()
	defer m.pauseMu.RUnlock()
	return m.paused
}

func (m *Model) WaitIfPaused() {
	m.pauseMu.RLock()
	if m.paused {
		ch := m.pauseCh
		m.pauseMu.RUnlock()
		<-ch // block until resumed
		return
	}
	m.pauseMu.RUnlock()
}

func (m *Model) SetQuitMode(mode QuitMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quitMode = mode
}

func (m *Model) GetQuitMode() QuitMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.quitMode
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tea.ClearScreen,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

func (m *Model) recalculateGrid() {
	if m.width < 10 {
		m.cols = 10
	} else {
		m.cols = m.width - 4 // margin for border
	}

	m.chunks = m.rows * m.cols
	m.chunkStatus = make([]bool, m.chunks)
}

func (m *Model) updateChunksFromWorkers() {
	if m.bytesTotal == 0 || m.chunks == 0 {
		return
	}

	m.progressMu.RLock()
	defer m.progressMu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.chunkStatus {
		m.chunkStatus[i] = false
	}

	for _, wp := range m.workerProgress {
		if wp == nil {
			continue
		}

		workerStartChunk := int((wp.Start * int64(m.chunks)) / m.bytesTotal)
		workerEndChunk := int((wp.End * int64(m.chunks)) / m.bytesTotal)

		workerTotalBytes := wp.End - wp.Start + 1
		if workerTotalBytes <= 0 {
			continue
		}

		completedChunks := int((wp.Received * int64(workerEndChunk-workerStartChunk+1)) / workerTotalBytes)

		for i := 0; i < completedChunks && workerStartChunk+i <= workerEndChunk; i++ {
			chunkIdx := workerStartChunk + i
			if chunkIdx >= 0 && chunkIdx < m.chunks {
				m.chunkStatus[chunkIdx] = true
			}
		}
	}
}

func (m *Model) TotalReceived() int64 {
	m.progressMu.RLock()
	defer m.progressMu.RUnlock()

	var total int64
	for _, wp := range m.workerProgress {
		if wp != nil {
			total += wp.Received
		}
	}
	return total
}

func (m *Model) CompletedChunks() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, c := range m.chunkStatus {
		if c {
			count++
		}
	}
	return count
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.SetQuitMode(QuitModeClean)
			return m, tea.Quit
		case "s":
			m.SetQuitMode(QuitModeSave)
			return m, tea.Quit
		case "p":
			m.Pause()
			return m, nil
		case "r":
			m.Resume()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.mu.Lock()
		m.width = msg.Width
		m.height = msg.Height
		m.mu.Unlock()
		m.recalculateGrid()
		return m, tea.ClearScreen

	case SpeedMsg:
		m.mu.Lock()
		m.speed = msg.BytesPerSec
		m.timeRemaining = msg.TimeRemaining
		m.mu.Unlock()
		return m, nil

	case DoneMsg:
		m.mu.Lock()
		m.done = true
		for i := range m.chunkStatus {
			m.chunkStatus[i] = true
		}
		m.mu.Unlock()
		return m, tea.Quit

	case ErrorMsg:
		m.mu.Lock()
		m.err = msg.Error
		m.mu.Unlock()
		return m, nil

	case TickMsg:
		m.updateChunksFromWorkers()
		return m, tickCmd()
	}

	return m, nil
}

func (m *Model) View() string {
	m.mu.RLock()
	width := m.width
	chunks := m.chunks
	cols := m.cols
	chunkStatus := make([]bool, len(m.chunkStatus))
	copy(chunkStatus, m.chunkStatus)
	done := m.done
	err := m.err
	speed := m.speed
	fileName := m.fileName
	bytesTotal := m.bytesTotal
	startTime := m.startTime
	m.mu.RUnlock()

	if width == 0 || chunks == 0 {
		return "Initializing..."
	}

	var b strings.Builder

	// Title
	title := TitleStyle.Render(fmt.Sprintf(" 📥 Downloading: %s ", fileName))
	b.WriteString(title)
	b.WriteString("\n\n")

	// grid
	var gridBuilder strings.Builder
	for i := 0; i < chunks; i++ {
		if i < len(chunkStatus) && chunkStatus[i] {
			gridBuilder.WriteString(CompleteStyle.Render(CompleteChar))
		} else {
			gridBuilder.WriteString(IncompleteStyle.Render(IncompleteChar))
		}

		if (i+1)%cols == 0 && i < chunks-1 {
			gridBuilder.WriteString("\n")
		}
	}

	// border
	grid := BorderStyle.Width(cols + 2).Render(gridBuilder.String())
	b.WriteString(grid)
	b.WriteString("\n")

	received := m.TotalReceived()
	percent := float64(0)
	if bytesTotal > 0 {
		percent = float64(received) / float64(bytesTotal) * 100
	}
	elapsed := time.Since(startTime).Round(time.Second)

	percentStr := PercentStyle.Render(fmt.Sprintf("%.1f%%", percent))
	speedStr := SpeedStyle.Render(util.FormatSpeed(speed))
	receivedStr := util.FormatBytes(received)
	totalStr := util.FormatBytes(bytesTotal)
	timeRemainingStr := formatDuration(m.timeRemaining)

	stats := StatsStyle.Render(fmt.Sprintf(
		"Progress: %s (%s / %s) │ Speed: %s │ ETA: %s │ Elapsed: %s",
		percentStr, receivedStr, totalStr, speedStr, timeRemainingStr, elapsed,
	))
	b.WriteString(stats)
	b.WriteString("\n")

	if err != nil {
		b.WriteString(fmt.Sprintf("\n❌ Error: %v\n", err))
		b.WriteString("Press 'q' to quit\n")
	} else if done {
		b.WriteString("\n")
		b.WriteString(DoneStyle.Render("✅ Download complete!"))
		b.WriteString("\n")
	} else if m.IsPaused() {
		b.WriteString("\n")
		b.WriteString(PausedStyle.Render("⏸ PAUSED"))
		b.WriteString("\n")
		b.WriteString("Press 'r' to resume │ 's' to save & quit │ 'q' to cancel\n")
	} else {
		b.WriteString("\nPress 'p' to pause │ 's' to save & quit │ 'q' to cancel\n")
	}

	return b.String()
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "--:--"
	}

	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
