//go:build windows

package archive

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// MOVEFILE_REPLACE_EXISTING = 0x00000001 — overwrite the destination
// if it already exists. os.Rename on Windows refuses to overwrite
// an existing directory; MoveFileExW with this flag is the
// Microsoft-documented workaround (see
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw).
//
// MOVEFILE_COPY_ALLOWED = 0x00000002 — allow moving to a different
// volume. Useful when staging and target live on different drives
// (e.g. test tempdir on C: vs production data on D:).
//
// MOVEFILE_WRITE_THROUGH = 0x00000008 — the function does not
// return until the file is actually moved on the disk. Forces a
// flush of any pending cached writes.
const (
	moveFileReplaceExisting = 0x00000001
	moveFileCopyAllowed     = 0x00000002
	moveFileWriteThrough    = 0x00000008
)

// renameOS is the rename function used by renameWithRetry and
// replaceDataDir. On Windows, os.Rename refuses to overwrite an
// existing directory ("If newpath already exists and is a
// directory, Rename returns an error" — see os.Rename docs).
// The cross-volume + over-target rename path is exercised by
// the import / restore flow when the target data dir already
// contains files (e.g. a second restore over a previously
// restored data dir).
//
// The Go 1.7+ os.Rename uses MoveFileExW WITHOUT
// MOVEFILE_REPLACE_EXISTING for directories. This shim adds
// the flag. Falls back to os.Rename on any non-Windows build
// (see rename_unix.go).
//
// Exposed as a package-level var so tests can inject a failing
// rename to exercise the retry logic without needing real
// Windows handle conflicts.
var renameOS = renameOSWindows

func renameOSWindows(src, dst string) error {
	srcPtr, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fmt.Errorf("UTF16PtrFromString(src): %w", err)}
	}
	dstPtr, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fmt.Errorf("UTF16PtrFromString(dst): %w", err)}
	}
	if err := windows.MoveFileEx(srcPtr, dstPtr, moveFileReplaceExisting|moveFileCopyAllowed|moveFileWriteThrough); err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}
