package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"adam/ui"
)

var workerBytesReceived = make(map[int]*int64)

func downloadPartWithRetry(url string, id int, start, end int64, model *ui.Model) bool {
	filename := fmt.Sprintf("part_%d.tmp", id)

	var received int64
	workerBytesReceived[id] = &received

	model.RegisterWorker(id, start, end)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := downloadChunk(url, start, end, filename, id, model)
		if err == nil {
			return true
		}

		if attempt < maxRetries {
			time.Sleep(1 * time.Second) // backoff
		}
	}

	return false
}

func downloadChunk(url string, start, end int64, filename string, workerID int, model *ui.Model) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
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

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	var totalReceived int64

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			totalReceived += int64(n)

			model.UpdateWorkerProgress(workerID, totalReceived)

			if counter, ok := workerBytesReceived[workerID]; ok {
				atomic.StoreInt64(counter, totalReceived)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	return nil
}
