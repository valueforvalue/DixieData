//go:build !windows

package archive

import "os"

// renameOS is the rename function used by renameWithRetry and
// replaceDataDir. On non-Windows platforms, os.Rename supports
// the over-target semantics we need (replaces an existing
// destination directory atomically on POSIX systems that
// implement rename(2)). On Windows we need a separate
// implementation that calls MoveFileExW with
// MOVEFILE_REPLACE_EXISTING — see rename_windows.go.
//
// Exposed as a package-level var so tests can inject a failing
// rename to exercise the retry logic without needing real
// Windows handle conflicts.
var renameOS = os.Rename
