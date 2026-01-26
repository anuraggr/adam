package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"adam/ui"
)

var ErrLinkExpired = errors.New("link expired")
var ErrWorkerCancelled = errors.New("worker cancelled")

func downloadPartWithRetry(ctx context.Context, url string, part *Part, model *ui.Model, maxRetries int) error {
	filename := fmt.Sprintf("part_%d.tmp", part.ID)

	model.RegisterWorker(part.ID, part.Start, part.End)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := downloadChunk(ctx, url, part, filename, model)
		if err == nil {
			return nil
		}

		// don't retry if link expired or context cancelled
		if errors.Is(err, ErrLinkExpired) || errors.Is(err, ErrWorkerCancelled) {
			return err
		}

		if attempt < maxRetries {
			time.Sleep(1 * time.Second) // backoff
		}
	}

	return fmt.Errorf("worker %d failed after %d retries", part.ID, maxRetries)
}

func downloadChunk(ctx context.Context, url string, part *Part, filename string, model *ui.Model) error {
	mode := os.O_CREATE | os.O_WRONLY
	startByte := part.Start

	// we resume from current offset if we have progress
	if part.CurrentOffset > 0 {
		expectedSize := part.CurrentOffset
		info, err := os.Stat(filename)

		// check if temp matches expected progress
		if err == nil && info.Size() >= expectedSize {
			if part.CurrentOffset >= (part.End - part.Start + 1) {
				part.IsComplete = true
				return nil
			}

			if info.Size() > expectedSize {
				if truncErr := os.Truncate(filename, expectedSize); truncErr != nil {
					part.CurrentOffset = 0
					mode = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
				} else {
					startByte = part.Start + part.CurrentOffset
					mode = os.O_APPEND | os.O_WRONLY
				}
			} else {
				startByte = part.Start + part.CurrentOffset
				mode = os.O_APPEND | os.O_WRONLY
			}
		} else {
			// start fresh, temp doesn't match expected progress
			part.CurrentOffset = 0
			mode = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", startByte, part.End))
	req.Header.Set("User-Agent", "Adam/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ErrWorkerCancelled
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: update download link with 'adam update'", ErrLinkExpired)
	}

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned unexpected status: %s", resp.Status)
	}

	file, err := os.OpenFile(filename, mode, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	//Increased buffer size from 32kb to 128kb to
	//decrease the number of syscalls
	buf := make([]byte, 128*1024) // 128kb buffer

	for {
		// check if cancelled
		select {
		case <-ctx.Done():
			return ErrWorkerCancelled
		default:
		}

		// we have to check if paused before each read
		model.WaitIfPaused()

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			part.CurrentOffset += int64(n)

			model.UpdateWorkerProgress(part.ID, part.CurrentOffset)
		}

		if readErr == io.EOF {
			part.IsComplete = true
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	return nil
}
