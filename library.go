package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func listSessions() {
	configDir, _ := os.UserConfigDir()
	adamDir := filepath.Join(configDir, "adam")

	files, err := os.ReadDir(adamDir)
	if err != nil {
		fmt.Println("No sessions found (or error reading directory).")
		return
	}

	fmt.Printf("%-3s | %-25s | %-10s | %-8s | %s\n", "ID", "File Name", "Size", "Progress", "Last Active")
	fmt.Println(strings.Repeat("-", 80))

	var sessions []*DownloadState

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			baseName := file.Name()[:len(file.Name())-5]
			path := filepath.Join(adamDir, baseName)
			state, err := LoadState(path)
			if err == nil {
				sessions = append(sessions, state)
			}
		}
	}

	// maybe sort by newest. might need lastUpdated field in json

	for i, state := range sessions {
		var downloaded int64 = 0
		for _, p := range state.Parts {
			downloaded += p.CurrentOffset
		}

		percent := 0.0
		if state.TotalSize > 0 {
			percent = float64(downloaded) / float64(state.TotalSize) * 100
		}

		status := fmt.Sprintf("%.1f%%", percent)
		if percent >= 100 {
			status = "Done"
		}

		fmt.Printf("%-3d | %-25s | %-10s | %-8s | %s\n",
			i+1,
			truncateString(state.Filename, 25),
			simpleFormatBytes(state.TotalSize),
			status,
			filepath.Base(state.Filename), // using filename as id for now
		)
	}
}

func updateSessionUrl(targetFile string, newUrl string) {
	statePath := getStatePath(targetFile)

	if _, err := os.Stat(statePath + ".json"); os.IsNotExist(err) {
		fmt.Printf("Error: Could not find session for '%s'\n", targetFile)
		fmt.Println("Tip: Use the exact filename from 'adam ls'")
		return
	}

	state, err := LoadState(statePath)
	if err != nil {
		fmt.Println("Error loading state:", err)
		return
	}

	fmt.Printf("Updating URL for %s...\n", state.Filename)
	fmt.Printf("OLD: %s\n", truncateString(state.URL, 50))
	fmt.Printf("NEW: %s\n", truncateString(newUrl, 50))

	state.URL = newUrl
	err = SaveState(statePath, state)
	if err != nil {
		fmt.Println("Error saving state:", err)
	} else {
		fmt.Println("Success! Run 'adam resume " + targetFile + "' to continue.")
	}
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num-3] + "..."
	}
	return str
}

func simpleFormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
