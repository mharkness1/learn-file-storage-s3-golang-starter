package main

import (
	"fmt"
	"os/exec"
)

func processVideoForFastStart(filePath string) (string, error) {
	outputFilepath := fmt.Sprintf("%s.processing", filePath)

	command := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilepath)
	err := command.Run()
	if err != nil {
		return "", fmt.Errorf("failed to executute command: %v", err)
	}

	return outputFilepath, nil
}
