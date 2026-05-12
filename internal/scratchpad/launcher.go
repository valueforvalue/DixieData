package scratchpad

type Launcher struct {
	dataDir string
}

func NewLauncher(dataDir string) *Launcher {
	return &Launcher{dataDir: dataDir}
}
