package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"adam/ui"

	tea "github.com/charmbracelet/bubbletea"
)

var numWorkers = 5 // hardcode for now
const maxRetries = 3

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <url>")
		return
	}
	url := os.Args[1]

	outFileName := filepath.Base(url)

	statePath := getStatePath(outFileName)

	state, err := LoadState(statePath)
	var totalSize int64

	if err != nil {
		totalSize, err = checkServerSupport(url)
		if err == ErrNoRangeSupport {
			fmt.Println("Server does not support range requests. Falling back to a single worker.")
			numWorkers = 1
			err = nil
		}
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		state = &DownloadState{
			URL:       url,
			Filename:  outFileName,
			TotalSize: totalSize,
			Parts:     make([]*Part, numWorkers),
		}

		chunkSize := totalSize / int64(numWorkers)
		for i := 0; i < numWorkers; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == numWorkers-1 {
				end = totalSize - 1
			}

			state.Parts[i] = &Part{
				ID:    i,
				Start: start,
				End:   end,
			}
		}
		SaveState(statePath, state)
	} else {
		totalSize = state.TotalSize
		numWorkers = len(state.Parts)
	}

	model := ui.New(outFileName, totalSize)
	program := tea.NewProgram(model, tea.WithAltScreen())

	var wg sync.WaitGroup
	var downloadErr error
	var errMu sync.Mutex

	go func() {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var lastBytes int64
			for range ticker.C {
				SaveState(statePath, state)
				currentBytes := model.TotalReceived()
				speed := float64(currentBytes-lastBytes) * 2 // bytes per second (500ms * 2)
				lastBytes = currentBytes

				var timeRemaining int64
				if speed > 0 {
					timeRemaining = (totalSize - currentBytes) / int64(speed)
				}
				program.Send(ui.SpeedMsg{BytesPerSec: speed, TimeRemaining: time.Duration(timeRemaining) * time.Second})

				if currentBytes >= totalSize {
					return
				}
			}
		}()

		for _, part := range state.Parts {
			if part.IsComplete {
				model.RegisterWorker(part.ID, part.Start, part.End)
				model.UpdateWorkerProgress(part.ID, part.End-part.Start+1)
				continue
			}

			wg.Add(1)
			go func(p *Part) {
				defer wg.Done()

				success := downloadPartWithRetry(url, p, model)
				if !success {
					errMu.Lock()
					downloadErr = fmt.Errorf("worker %d failed after %d retries", p.ID, maxRetries)
					errMu.Unlock()
				} else {
					p.IsComplete = true
				}
			}(part)
		}

		wg.Wait()
		SaveState(statePath, state)

		if downloadErr != nil {
			program.Send(ui.ErrorMsg{Error: downloadErr})
			return
		}

		err := mergeParts(outFileName, numWorkers)
		if err != nil {
			program.Send(ui.ErrorMsg{Error: fmt.Errorf("merge failed: %v", err)})
			return
		}

		os.Remove(outFileName + ".json")
		for i := 0; i < numWorkers; i++ {
			os.Remove(fmt.Sprintf("part_%d.tmp", i))
		}

		program.Send(ui.DoneMsg{})
	}()

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func getStatePath(filename string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = home
	}

	dir := filepath.Join(configDir, "adam")

	os.MkdirAll(dir, 0755)

	return filepath.Join(dir, filename)
}
