package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"adam/ui"
	"adam/util"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	speedCheckInterval     = 3 * time.Second
	minMeanSpeedForRestart = 100 * 1024 // 100 KB/s
	slowWorkerThreshold    = 0.3
	maxWorkerRestarts      = 5
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

type workerControl struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func RunDownload(config DownloadConfig, state *DownloadState, model *ui.Model, program *tea.Program) error {
	statePath := util.GetStatePath(state.Filename)

	var wg sync.WaitGroup
	var downloadErr error
	var errMu sync.Mutex

	workerCtx := make(map[int]*workerControl)
	var ctxMu sync.RWMutex

	done := make(chan struct{})

	// Speed and state routine
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var lastBytes int64
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
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
		}
	}()

	// worker performance routine
	go func() {
		ticker := time.NewTicker(speedCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				checkAndRestartSlowWorkers(state, workerCtx, &ctxMu, model, config, &wg, &downloadErr, &errMu, program)
			}
		}
	}()

	// init the workers
	for _, part := range state.Parts {
		if part.IsComplete {
			model.RegisterWorker(part.ID, part.Start, part.End)
			model.UpdateWorkerProgress(part.ID, part.End-part.Start+1)
			continue
		}

		// lastbyte is for speed tracking
		part.LastBytes = part.CurrentOffset

		ctx, cancel := context.WithCancel(context.Background())
		ctxMu.Lock()
		workerCtx[part.ID] = &workerControl{ctx: ctx, cancel: cancel}
		ctxMu.Unlock()

		wg.Add(1)
		go runWorker(ctx, state.URL, part, model, config, &wg, &downloadErr, &errMu)
	}

	wg.Wait()
	close(done)
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

	util.CompleteSession(state.Filename, len(state.Parts))

	program.Send(ui.DoneMsg{})
	return nil
}

func runWorker(ctx context.Context, url string, part *Part, model *ui.Model, config DownloadConfig, wg *sync.WaitGroup, downloadErr *error, errMu *sync.Mutex) {
	defer wg.Done()

	err := downloadPartWithRetry(ctx, url, part, model, config.MaxRetries)
	if err != nil {
		if errors.Is(err, ErrWorkerCancelled) {
			return
		}
		errMu.Lock()
		if *downloadErr == nil {
			*downloadErr = err
		}
		if errors.Is(err, ErrLinkExpired) {
			*downloadErr = err
		}
		errMu.Unlock()
	} else {
		part.IsComplete = true
	}
}

func checkAndRestartSlowWorkers(state *DownloadState, workerCtx map[int]*workerControl, ctxMu *sync.RWMutex, model *ui.Model, config DownloadConfig, wg *sync.WaitGroup, downloadErr *error, errMu *sync.Mutex, program *tea.Program) {
	var speeds []float64
	var totalSpeed float64
	var activeWorkers []*Part

	for _, part := range state.Parts {
		if part.IsComplete {
			continue
		}

		speed := float64(part.CurrentOffset-part.LastBytes) / speedCheckInterval.Seconds()
		part.LastBytes = part.CurrentOffset

		if speed >= 0 {
			speeds = append(speeds, speed)
			totalSpeed += speed
			activeWorkers = append(activeWorkers, part)
		}
	}

	if len(activeWorkers) == 0 {
		return
	}

	meanSpeed := totalSpeed / float64(len(activeWorkers))

	// we only restart if mean speed is above threshold
	if meanSpeed < minMeanSpeedForRestart {
		return
	}

	threshold := meanSpeed * slowWorkerThreshold

	program.Send(ui.DebugMsg{Message: fmt.Sprintf("Speed check: mean=%.1f KB/s, threshold=%.1f KB/s", meanSpeed/1024, threshold/1024)})

	for i, part := range activeWorkers {
		if speeds[i] < threshold && part.Restarts < maxWorkerRestarts {
			// cancel and restart this worker
			ctxMu.RLock()
			ctrl := workerCtx[part.ID]
			ctxMu.RUnlock()

			if ctrl != nil {
				ctrl.cancel()
			}

			part.Restarts++

			program.Send(ui.DebugMsg{Message: fmt.Sprintf("Restarting worker %d (%.1f KB/s < %.1f KB/s) [restart %d/%d]", part.ID, speeds[i]/1024, threshold/1024, part.Restarts, maxWorkerRestarts)})

			// start new worker
			ctx, cancel := context.WithCancel(context.Background())
			ctxMu.Lock()
			workerCtx[part.ID] = &workerControl{ctx: ctx, cancel: cancel}
			ctxMu.Unlock()

			wg.Add(1)
			go runWorker(ctx, state.URL, part, model, config, wg, downloadErr, errMu)
		}
	}
}
