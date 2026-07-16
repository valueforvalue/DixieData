//go:build !windows

// cli_signal_test_posix.go -- POSIX-only helpers for the
// signal-handler subprocess tests. On POSIX, Setpgid in
// SysProcAttr lets the test signal the subprocess's process
// group cleanly; on Windows the field does not exist (and the
// tests themselves skip via t.Skip).
package appshell

import (
	"os/exec"
	"syscall"
)

func setStartProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}