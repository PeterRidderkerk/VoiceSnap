package main

import (
	"os"
	"voicesnap/internal/logger"
	"voicesnap/internal/singleinstance"
)

func main() {
	logger.Init()
	logger.Info("VoiceSnap Win7 starting...")

	lock, err := singleinstance.Acquire()
	if err != nil {
		logger.Info("Another instance is already running, exiting")
		os.Exit(0)
	}
	defer lock.Release()

	if err := RunApp(); err != nil {
		logger.Error("Application error: %v", err)
		os.Exit(1)
	}
}
