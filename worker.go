package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"adam/ui"
)

var ErrLinkExpired = errors.New("link expired")

func downloadPartWithRetry(url string, part *Part, model *ui.Model) error {
	filename := fmt.Sprintf("part_%d.tmp", part.ID)

	model.RegisterWorker(part.ID, part.Start, part.End)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := downloadChunk(url, part, filename, model)
		if err == nil {
			return nil
		}

		// don't retry if link expired
		if errors.Is(err, ErrLinkExpired) {
			return err
		}

		if attempt < maxRetries {
			time.Sleep(1 * time.Second) // backoff
		}
	}

	return fmt.Errorf("worker %d failed after %d retries", part.ID, maxRetries)
}

func downloadChunk(url string, part *Part, filename string, model *ui.Model) error {
	mode := os.O_CREATE | os.O_WRONLY
	startByte := part.Start

	// resume from curOffset if we have progress
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
					// start fresh if truncate faisl
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
			// start fresh, tmp doesnt match expected progress
			part.CurrentOffset = 0
			mode = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		}
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", startByte, part.End))
	req.Header.Set("User-Agent", "Adam/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
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

	buf := make([]byte, 32*1024)
	for {
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
