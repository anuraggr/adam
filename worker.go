package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"adam/ui"
)

func downloadPartWithRetry(url string, part *Part, model *ui.Model) bool {
	filename := fmt.Sprintf("part_%d.tmp", part.ID)

	model.RegisterWorker(part.ID, part.Start, part.End)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := downloadChunk(url, part, filename, model)
		if err == nil {
			return true
		}

		if attempt < maxRetries {
			time.Sleep(1 * time.Second) // backoff
		}
	}

	return false
}

func downloadChunk(url string, part *Part, filename string, model *ui.Model) error {
	mode := os.O_CREATE | os.O_WRONLY
	startByte := part.Start

	info, err := os.Stat(filename)
	if err == nil {
		//file exists
		downloadedSoFar := info.Size()

		if downloadedSoFar >= (part.End - part.Start + 1) {
			part.IsComplete = true
			return nil
		}

		startByte = part.Start + downloadedSoFar
		part.CurrentOffset = downloadedSoFar
		mode = os.O_APPEND | os.O_WRONLY
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
