package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func downloadPartWithRetry(url string, id int, start, end int64) bool {
	filename := fmt.Sprintf("part_%d.tmp", id)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("[Worker %d] Attempt %d/%d: Bytes %d-%d\n", id, attempt, maxRetries, start, end)

		err := downloadChunk(url, start, end, filename)
		if err == nil {
			// success
			fmt.Printf("[Worker %d] Finished! Saved to %s\n", id, filename)
			return true
		}

		// failure
		fmt.Printf("[Worker %d] Error: %v. Retrying in 1s...\n", id, err)
		time.Sleep(1 * time.Second) // backoff
	}

	return false
}

func downloadChunk(url string, start, end int64, filename string) error {
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

	// write to file
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}
