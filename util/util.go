package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func FormatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	size := float64(bytes)

	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", size, units[unitIndex])
}

func FormatSpeed(bps float64) string {
	if bps == 0 {
		return "-- B/s"
	}

	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	unitIndex := 0

	for bps >= 1024 && unitIndex < len(units)-1 {
		bps /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", bps, units[unitIndex])
}

func GetConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = home
	}

	dir := filepath.Join(configDir, "adam")
	os.MkdirAll(dir, 0755)

	return dir
}

func GetStatePath(filename string) string {
	return filepath.Join(GetConfigDir(), filename)
}

func CleanupTempFiles(numWorkers int) {
	for i := 0; i < numWorkers; i++ {
		os.Remove(fmt.Sprintf("part_%d.tmp", i))
	}
}

func CleanupSession(filename string, numWorkers int) {
	os.Remove(GetStatePath(filename) + ".json")
	CleanupTempFiles(numWorkers)
}

func TruncateString(str string, maxLen int) string {
	if len(str) > maxLen {
		return str[0:maxLen-3] + "..."
	}
	return str
}
