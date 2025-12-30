package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const numWorkers = 4 //hardcode for now
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
	fmt.Printf("File Size: %d bytes. Starting %d workers...\n", totalSize, numWorkers)

	var wg sync.WaitGroup

	chunkSize := totalSize / int64(numWorkers)

	startTime := time.Now()

	for i := 0; i < numWorkers; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == numWorkers-1 {
			end = totalSize - 1
		}
		wg.Add(1)

		go func(id int, s, e int64) {
			defer wg.Done()

			success := downloadPartWithRetry(url, id, s, e)
			if !success {
				fmt.Printf("[Manager] Worker %d failed after %d retries. Download incomplete.\n", id, maxRetries)
			}
		}(i, start, end)
	}

	wg.Wait()
	fmt.Printf("\nAll threads finished in %s\n", time.Since(startTime))

	err = mergeParts(outFileName, numWorkers)
	if err != nil {
		fmt.Printf("Merge failed: %v\n", err)
		return
	}

	fmt.Printf("Done! Saved to: %s\n", outFileName)

}

func mergeParts(filename string, parts int) error {
	destFile, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer destFile.Close()

	for i := 0; i < parts; i++ {
		partFileName := fmt.Sprintf("part_%d.tmp", i)

		partFile, err := os.Open(partFileName)
		if err != nil {
			return err
		}

		fmt.Printf("Merging %s...\n", partFileName)
		_, err = io.Copy(destFile, partFile)
		partFile.Close()

		if err != nil {
			return err
		}
		os.Remove(partFileName)
	}
	return nil
}

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

func checkServerSupport(url string) (int64, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "Adam/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		parts := strings.Split(resp.Header.Get("Content-Range"), "/")
		if len(parts) == 2 {
			size, _ := strconv.ParseInt(parts[1], 10, 64)
			return size, nil
		}
	}
	return 0, fmt.Errorf("server does not support parallel downloads")
}
