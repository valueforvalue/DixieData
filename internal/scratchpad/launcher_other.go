//go:build !windows

package scratchpad

import "errors"

func (l *Launcher) Open(displayID, seed string) error {
	return errors.New("scratch pad window is only supported on Windows builds")
}
