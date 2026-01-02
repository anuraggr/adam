package main

import (
	"errors"
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
		fmt.Println("Usage: adam <url> | adam resume <filename> | adam ls")
		return
	}
	command := os.Args[1]
	var url string
	var isResume bool

	switch command {
	case "ls", "list":
		listSessions()
		return

	case "update":
		if len(os.Args) < 4 {
			fmt.Println("Usage: adam update <filename> <new_url>")
			return
		}
		updateSessionUrl(os.Args[2], os.Args[3])
		return

	case "resume":
		if len(os.Args) < 3 {
			fmt.Println("Usage: adam resume <filename>")
			return
		}
		isResume = true

	default:
		url = os.Args[1]
	}

	var state *DownloadState
	var totalSize int64
	var outFileName string
	var statePath string
	var err error

	if isResume {
		outFileName = os.Args[2]
		statePath = getStatePath(outFileName)

		state, err = LoadState(statePath)
		if err != nil {
			fmt.Printf("Error: No session found for '%s'\n", outFileName)
			return
		}
		url = state.URL
		totalSize = state.TotalSize
		numWorkers = len(state.Parts)
		fmt.Printf("Resuming download: %s\n", outFileName)
	} else {
		//fresh download
		outFileName = filepath.Base(url)
		statePath = getStatePath(outFileName)

		//remove any existing state and tmp for this file
		os.Remove(statePath + ".json")
		for i := 0; i < 10; i++ { //10 for now. might need to check and delete later
			os.Remove(fmt.Sprintf("part_%d.tmp", i))
		}

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

				err := downloadPartWithRetry(url, p, model)
				if err != nil {
					errMu.Lock()
					if downloadErr == nil {
						downloadErr = err
					}
					// check if link expired
					if errors.Is(err, ErrLinkExpired) {
						downloadErr = err
					}
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
