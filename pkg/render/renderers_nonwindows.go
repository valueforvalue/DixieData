//go:build !windows

package render

import "os/exec"

// hideWindow is a no-op on non-Windows platforms. The Windows
// implementation lives in pkg/render/renderers_windows.go and
// sets SysProcAttr{HideWindow: true, CreationFlags:
// CREATE_NO_WINDOW} to suppress the black console window that
// would otherwise pop up when shelling out to the Typst binary.
//
// This file is gated to non-Windows builds so the Windows-specific
// SysProcAttr fields don't have to be defined for Linux/macOS
// (they don't exist there, which would fail the build with
// "unknown field HideWindow in struct literal of type
// syscall.SysProcAttr").
//
// See pkg/render/renderers_windows.go for the Windows counterpart.
// See internal/archive/pdfium_{windows,nonwindows}.go for the
// established convention this slice follows.
func hideWindow(_ *exec.Cmd) {
	// no-op
}
