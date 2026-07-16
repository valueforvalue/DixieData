//go:build windows

// cli_signal_test_windows.go -- Windows stub for the POSIX-only
// helper. The signal-handler tests t.Skip on Windows so this
// shim exists only to keep the cross-compile clean.
package appshell

import "os/exec"

func setStartProcAttr(_ *exec.Cmd) {}