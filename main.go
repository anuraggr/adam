package main

import (
	"fmt"
	"os"
	"path/filepath"
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
