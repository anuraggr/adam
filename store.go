package main

import (
	"encoding/json"
	"os"
)

type Part struct {
	ID            int   `json:"id"`
	Start         int64 `json:"start"`
	End           int64 `json:"end"`
	CurrentOffset int64 `json:"current_offset"`
	IsComplete    bool  `json:"is_complete"`
	// below fiels are non persistant
	Restarts  int   `json:"-"`
	LastBytes int64 `json:"-"`
}

type DownloadState struct {
	URL       string  `json:"url"`
	Filename  string  `json:"filename"`
	TotalSize int64   `json:"total_size"`
	Parts     []*Part `json:"parts"`
}

func SaveState(filename string, state *DownloadState) error {
	//conv struct to json format
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename+".json", data, 0644)
}

func LoadState(filename string) (*DownloadState, error) {
	data, err := os.ReadFile(filename + ".json")
	if err != nil {
		return nil, err
	}

	state := &DownloadState{}

	err = json.Unmarshal(data, state)
	if err != nil {
		return nil, err
	}

	return state, nil
}
