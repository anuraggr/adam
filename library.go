package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"adam/util"
)

func listSessions(filter string) {
	var sessions []*DownloadState
	var sources []string

	switch filter {
	case "ongoing":
		sources = []string{util.GetOngoingDir()}
	case "complete":
		sources = []string{util.GetCompleteDir()}
	default: // "all"
		sources = []string{util.GetOngoingDir(), util.GetCompleteDir()}
	}

	for _, dir := range sources {
		sessions = append(sessions, loadSessionsFromDir(dir)...)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	fmt.Printf("%-3s | %-25s | %-10s | %-8s | %s\n", "ID", "File Name", "Size", "Progress", "Status")
	fmt.Println(strings.Repeat("-", 80))

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
		statusLabel := "Ongoing"
		if percent >= 100 {
			status = "Done"
			statusLabel = "Complete"
		}

		fmt.Printf("%-3d | %-25s | %-10s | %-8s | %s\n",
			i+1,
			util.TruncateString(state.Filename, 25),
			util.FormatBytes(state.TotalSize),
			status,
			statusLabel,
		)
	}
}

func loadSessionsFromDir(dir string) []*DownloadState {
	var sessions []*DownloadState

	files, err := os.ReadDir(dir)
	if err != nil {
		return sessions
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			baseName := file.Name()[:len(file.Name())-5]
			path := filepath.Join(dir, baseName)
			state, err := LoadState(path)
			if err == nil {
				sessions = append(sessions, state)
			}
		}
	}

	return sessions
}

func updateSessionUrl(targetFile string, newUrl string) {
	statePath := util.GetStatePath(targetFile)

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
	fmt.Printf("OLD: %s\n", util.TruncateString(state.URL, 50))
	fmt.Printf("NEW: %s\n", util.TruncateString(newUrl, 50))

	state.URL = newUrl
	err = SaveState(statePath, state)
	if err != nil {
		fmt.Println("Error saving state:", err)
	} else {
		fmt.Println("Success! Run 'adam resume " + targetFile + "' to continue.")
	}
}
