package main

import (
	"fmt"
	"os"
	"path/filepath"

	"adam/ui"
	"adam/util"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: adam <url> | adam resume <filename> | adam ls")
		return
	}
	command := os.Args[1]
	var url string
	var isResume bool

	switch command {
	case "ls", "list":
		listSessions()
		return

	case "update":
		if len(os.Args) < 4 {
			fmt.Println("Usage: adam update <filename> <new_url>")
			return
		}
		updateSessionUrl(os.Args[2], os.Args[3])
		return

	case "resume":
		if len(os.Args) < 3 {
			fmt.Println("Usage: adam resume <filename>")
			return
		}
		isResume = true

	case "help":
		printHelp()
		return

	default:
		url = os.Args[1]
	}

	config := DefaultConfig()
	var state *DownloadState
	var totalSize int64
	var outFileName string
	var err error

	if isResume {
		outFileName = os.Args[2]
		statePath := util.GetStatePath(outFileName)

		state, err = LoadState(statePath)
		if err != nil {
			fmt.Printf("Error: No session found for '%s'\n", outFileName)
			return
		}
		url = state.URL
		totalSize = state.TotalSize
		config.NumWorkers = len(state.Parts)
		fmt.Printf("Resuming download: %s\n", outFileName)
	} else {
		// fresh download
		outFileName = filepath.Base(url)
		statePath := util.GetStatePath(outFileName)

		// remove any existing state and tmp files for this file
		util.CleanupSession(outFileName, 20) // 20 as max possible workers to clean

		totalSize, err = checkServerSupport(url)
		if err == ErrNoRangeSupport {
			fmt.Println("Server does not support range requests. Falling back to a single worker.")
			config.NumWorkers = 1
			err = nil
		}
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		state = &DownloadState{
			URL:       url,
			Filename:  outFileName,
			TotalSize: totalSize,
			Parts:     make([]*Part, config.NumWorkers),
		}

		chunkSize := totalSize / int64(config.NumWorkers)
		for i := 0; i < config.NumWorkers; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == config.NumWorkers-1 {
				end = totalSize - 1
			}

			state.Parts[i] = &Part{
				ID:    i,
				Start: start,
				End:   end,
			}
		}
		SaveState(statePath, state)
	}

	model := ui.New(outFileName, totalSize)
	program := tea.NewProgram(model, tea.WithAltScreen())

	go func() {
		RunDownload(config, state, model, program)
	}()

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}

	handleQuitMode(model.GetQuitMode(), state, config.NumWorkers)
}

func handleQuitMode(mode ui.QuitMode, state *DownloadState, numWorkers int) {
	switch mode {
	case ui.QuitModeClean:
		util.CleanupSession(state.Filename, numWorkers)
		fmt.Println("Download cancelled.")

	case ui.QuitModeSave:
		SaveState(util.GetStatePath(state.Filename), state)
		fmt.Printf("Download paused. Resume with: adam resume %s\n", state.Filename)
	}
}

func printHelp() {
	fmt.Println(`Adam - A fast download manager with resume support

Usage:
  adam <url>                   Start a new download
  adam resume <filename>       Resume a paused download
  adam update <file> <url>     Update the URL for a paused download
  adam ls                      List all download sessions
  adam help                    Show this help message

Keyboard shortcuts (during download):
  p     Pause download
  r     Resume download
  s     Save progress and quit
  q     Cancel and quit`)
}
