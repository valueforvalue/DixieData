//go:build windows

package render

import (
	"os/exec"
	"syscall"
)

// hideWindow sets the Windows-specific SysProcAttr fields so a child
// process spawned via exec.Command does not allocate a console
// window. This file is gated to Windows only via the build tag so
// the syscall.SysProcAttr{HideWindow, CreationFlags} literals do
// not break the Linux/macOS build (those fields exist only on
// Windows). On non-Windows platforms pkg/render/renderers_nonwindows.go
// provides a no-op stub with the same signature.
//
// See pkg/render/renderers_nonwindows.go for the no-op counterpart.
// See internal/archive/pdfium_{windows,nonwindows}.go for the
// established convention this slice follows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
