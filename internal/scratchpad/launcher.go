package scratchpad

import "time"

type Launcher struct {
	dataDir string
	store   store
}

type store interface {
	Scratchpad(displayID string) (string, time.Time, error)
	SaveScratchpad(displayID, content string) error
}

func NewLauncher(dataDir string, store store) *Launcher {
	return &Launcher{dataDir: dataDir, store: store}
}
