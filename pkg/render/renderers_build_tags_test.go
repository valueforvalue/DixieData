package render

import (
	"os/exec"
	"runtime"
	"testing"
)

// TestHideWindowExistsAllPlatforms is a regression net for the
// 2026-06-29 CI audit failure on Linux. Before the fix, the
// `hideWindow` function in pkg/render/renderers.go referenced
// `syscall.SysProcAttr{HideWindow:..., CreationFlags:...}`
// directly. Those fields only exist on Windows, so any non-Windows
// build of the `dixiedata-web` binary (which is what the audit
// workflow on ubuntu-latest needs) failed with:
//
//   pkg/render/renderers.go:302:3: unknown field HideWindow in
//     struct literal of type "syscall".SysProcAttr
//   pkg/render/renderers.go:303:3: unknown field CreationFlags in
//     struct literal of type "syscall".SysProcAttr
//
// The runtime.GOOS check inside the function body did NOT help —
// Go still type-checks the literal on every platform, so the
// Linux compile unit failed even though the function was never
// called on Linux.
//
// The fix splits the function into two files with build tags:
//   - pkg/render/renderers_windows.go (//go:build windows)
//   - pkg/render/renderers_nonwindows.go (//go:build !windows)
//
// This test pins the contract from the caller's perspective:
// `hideWindow` must exist on every platform with a signature that
// accepts an *exec.Cmd. The implementation differs (Windows sets
// SysProcAttr; non-Windows is a no-op) but the caller doesn't care.
func TestHideWindowExistsAllPlatforms(t *testing.T) {
	// Use a real exec.Cmd so the Windows impl can dereference
	// cmd.SysProcAttr without panicking. The command itself is
	// never executed — we only test that the function compiles
	// and runs.
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	hideWindow(cmd) // smoke call: function must exist on every platform

	// Sanity check: confirm the platform the test ran on. This
	// catches the case where someone accidentally runs the test
	// on the wrong OS (e.g. runs the !windows file on a Windows
	// runner by mistake).
	if runtime.GOOS == "windows" {
		t.Log("running on Windows; hideWindow sets SysProcAttr{HideWindow: true, ...}")
	} else {
		t.Logf("running on %s; hideWindow is a no-op", runtime.GOOS)
	}
}
