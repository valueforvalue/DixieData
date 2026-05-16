package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	stressLogMu    sync.Mutex
	stressLogFile  *os.File
	stressLogPath  string
	originalStdout = os.Stdout
	originalStderr = os.Stderr
)

func configureStressLogging() error {
	path := strings.TrimSpace(os.Getenv("DIXIEDATA_STRESS_LOG"))
	if path == "" {
		return nil
	}

	stressLogMu.Lock()
	defer stressLogMu.Unlock()

	if stressLogFile != nil && stressLogPath == path {
		return nil
	}

	if stressLogFile != nil {
		_ = stressLogFile.Close()
		stressLogFile = nil
		stressLogPath = ""
		os.Stdout = originalStdout
		os.Stderr = originalStderr
		log.SetOutput(originalStderr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	stressLogFile = file
	stressLogPath = path
	os.Stdout = file
	os.Stderr = file
	log.SetOutput(file)
	_, _ = fmt.Fprintf(file, "[%s] stress logging enabled\n", time.Now().Format(time.RFC3339))
	return nil
}

func resetStressLoggingForTests() {
	stressLogMu.Lock()
	defer stressLogMu.Unlock()

	if stressLogFile != nil {
		_, _ = fmt.Fprintf(stressLogFile, "[%s] stress logging reset\n", time.Now().Format(time.RFC3339))
		_ = stressLogFile.Close()
	}
	stressLogFile = nil
	stressLogPath = ""
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	log.SetOutput(originalStderr)
}
