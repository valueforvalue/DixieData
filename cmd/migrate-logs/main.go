// migrate-logs is a one-shot CLI that runs the appshell log
// migration on a real .dixiedata directory without launching the
// app. Useful for verifying the migration against the user's live
// data dir after upgrading, before the next app launch.
//
// Usage: go run ./cmd/migrate-logs -data-dir <path>
//
// By default uses appdata.DefaultDir() (the same path the app uses).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

func main() {
	dataDir := flag.String("data-dir", appdata.DefaultDir(), "Path to the DixieData data directory (default: same as app)")
	flag.Parse()

	fmt.Printf("migrate-logs: data dir = %s\n", *dataDir)

	oldLogs := *dataDir + string(os.PathSeparator) + "logs"
	if _, err := os.Stat(oldLogs); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("nothing to migrate: old logs dir does not exist")
			return
		}
		fmt.Fprintf(os.Stderr, "stat old logs dir: %v\n", err)
		os.Exit(1)
	}

	newLogs := appdata.LogsRoot(*dataDir)
	fmt.Printf("migrate-logs: %s -> %s\n", oldLogs, newLogs)

	entries, err := os.ReadDir(oldLogs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read old logs: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(newLogs, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create new logs: %v\n", err)
		os.Exit(1)
	}

	moved := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		oldPath := oldLogs + string(os.PathSeparator) + entry.Name()
		newPath := newLogs + string(os.PathSeparator) + entry.Name()
		if _, err := os.Stat(newPath); err == nil {
			fmt.Printf("  skip %s (already exists in new location)\n", entry.Name())
			continue
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			fmt.Printf("  skip %s: %v\n", entry.Name(), err)
			continue
		}
		fmt.Printf("  moved %s\n", entry.Name())
		moved++
	}

	// Clean up the now-empty old dir.
	if remaining, _ := os.ReadDir(oldLogs); len(remaining) == 0 {
		_ = os.Remove(oldLogs)
		fmt.Println("removed empty old logs dir")
	}

	fmt.Printf("migrate-logs: %d file(s) moved\n", moved)
}