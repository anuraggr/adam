package main

import (
	"fmt"
	"io"
	"os"
)

func mergeParts(filename string, parts int) error {
	destFile, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer destFile.Close()

	for i := 0; i < parts; i++ {
		partFileName := fmt.Sprintf("%s.part_%d.tmp", filename, i)

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
