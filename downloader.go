package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"adam/ui"
	"adam/util"

	tea "github.com/charmbracelet/bubbletea"
)

type DownloadConfig struct {
	NumWorkers int
	MaxRetries int
}

func DefaultConfig() DownloadConfig {
	return DownloadConfig{
		NumWorkers: 8,
		MaxRetries: 3,
	}
}

func RunDownload(config DownloadConfig, state *DownloadState, model *ui.Model, program *tea.Program) error {
	statePath := util.GetStatePath(state.Filename)

	var wg sync.WaitGroup
	var downloadErr error
	var errMu sync.Mutex

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
				timeRemaining = (state.TotalSize - currentBytes) / int64(speed)
			}
			program.Send(ui.SpeedMsg{BytesPerSec: speed, TimeRemaining: time.Duration(timeRemaining) * time.Second})

			if currentBytes >= state.TotalSize {
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

			err := downloadPartWithRetry(state.URL, p, model, config.MaxRetries)
			if err != nil {
				errMu.Lock()
				if downloadErr == nil {
					downloadErr = err
				}
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
		return downloadErr
	}

	// merge all
	err := mergeParts(state.Filename, len(state.Parts))
	if err != nil {
		program.Send(ui.ErrorMsg{Error: fmt.Errorf("merge failed: %v", err)})
		return err
	}

	util.CleanupSession(state.Filename, len(state.Parts))

	program.Send(ui.DoneMsg{})
	return nil
}
