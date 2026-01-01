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

const numWorkers = 5 // hardcode for now
const maxRetries = 3

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <url>")
		return
	}
	url := os.Args[1]

	outFileName := filepath.Base(url)

	totalSize, err := checkServerSupport(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	model := ui.New(outFileName, totalSize)
	program := tea.NewProgram(model, tea.WithAltScreen())

	var wg sync.WaitGroup
	var downloadErr error
	var errMu sync.Mutex

	go func() {
		chunkSize := totalSize / int64(numWorkers)
		// startTime := time.Now()

		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var lastBytes int64
			for range ticker.C {
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

		for i := range numWorkers {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == numWorkers-1 {
				end = totalSize - 1
			}

			wg.Add(1)
			go func(id int, s, e int64) {
				defer wg.Done()

				success := downloadPartWithRetry(url, id, s, e, model)
				if !success {
					errMu.Lock()
					downloadErr = fmt.Errorf("worker %d failed after %d retries", id, maxRetries)
					errMu.Unlock()
				}
			}(i, start, end)
		}

		wg.Wait()

		if downloadErr != nil {
			program.Send(ui.ErrorMsg{Error: downloadErr})
			return
		}

		// merge
		err := mergeParts(outFileName, numWorkers)
		if err != nil {
			program.Send(ui.ErrorMsg{Error: fmt.Errorf("merge failed: %v", err)})
			return
		}

		// elapsed := time.Since(startTime)
		// _ = elapsed // duration shown in TUI

		program.Send(ui.DoneMsg{})
	}()

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
