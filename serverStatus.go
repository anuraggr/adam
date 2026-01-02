package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var ErrNoRangeSupport = errors.New("server does not support range requests")

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

	if resp.StatusCode == http.StatusOK {
		return 0, ErrNoRangeSupport
	}
	return 0, fmt.Errorf("server returned unexpected status: %s", resp.Status)
}
