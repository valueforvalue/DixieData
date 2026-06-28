// cli_import_zip.go — small zip helper for Phase 5 dry-run paths.
//
// Kept separate from cli_import.go so the main file stays focused
// on dispatch logic. The helper exists because archive/zip needs
// its *zip.ReadCloser closed after every call; we want a single
// openZip() seam to test (and a future `import backup inspect`
// command can reuse it).
package appshell

import "archive/zip"

// openZip wraps archive/zip.OpenReader. The *zip.ReadCloser is
// returned directly because that's what archive/zip gives us and
// the caller (readBackupManifestFromZip / readSharedArchiveManifest)
// already needs the concrete type to iterate File headers.
func openZip(path string) (*zip.ReadCloser, error) {
	return zip.OpenReader(path)
}