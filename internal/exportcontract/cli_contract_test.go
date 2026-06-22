package exportcontract

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCLIContractSnapshots runs the dixiedata-tune CLI binary
// (built from tools/tune/) against the fixture DB and byte-compares
// the output against snapshots. Companion to TestArchiveContractSnapshots,
// which exercises the same path in-process. This test verifies the
// CLI surface (flag parsing, subcommand dispatch) as well as the
// rendering pipeline.
//
// Run from the repo root with:
//
//	go test ./internal/exportcontract/ -run TestCLIContractSnapshots -v
//
// Set UPDATE_SNAPSHOTS=1 to regenerate.
func TestCLIContractSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI contract snapshots in -short mode")
	}

	// Find the tools/tune source.
	tuneDir := findTuneSource(t)
	if tuneDir == "" {
		t.Skip("tools/tune source not found")
	}

	typstPath := mustFindTypstBinary(t)
	templatesDir := mustFindTemplatesDir(t)
	dataDir := FixturePath(t)

	type snap struct {
		name     string
		template string
		mode     string
		recordID int64
	}
	cases := []snap{
		{name: "soldier-landscape", template: "soldier_landscape", mode: "record", recordID: 1},
		{name: "soldier-portrait", template: "soldier_portrait", mode: "record", recordID: 1},
		{name: "widow-landscape", template: "widow_landscape", mode: "record", recordID: 2},
		{name: "widow-portrait", template: "widow_portrait", mode: "record", recordID: 2},
		{name: "wife-landscape", template: "spouse_landscape", mode: "record", recordID: 3},
		{name: "wife-portrait", template: "spouse_portrait", mode: "record", recordID: 3},
		{name: "linked-person-landscape", template: "spouse_landscape", mode: "record", recordID: 4},
		{name: "linked-person-portrait", template: "spouse_portrait", mode: "record", recordID: 4},
		{name: "bulk-landscape", template: "bulk_soldier", mode: "bulk"},
		{name: "bulk-portrait", template: "bulk_soldier", mode: "bulk"},
		{name: "grouped-by-pension-state", template: "bulk_soldier", mode: "bulk"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), c.name+".pdf")
			// Build the binary from tools/tune via go build so
			// the separate-module boundary is respected. Caching
			// the binary in a per-test-process temp dir avoids
			// rebuild on every subtest.
			binPath := buildTuneBinary(t)

			args := []string{
				"--db", dataDir,
				"--typst", typstPath,
				"--templates", templatesDir,
				"render",
				"--template", c.template,
				"--mode", c.mode,
				"--orientation", orientationFor(c.template),
				"--out", out,
			}
			if c.mode == "record" {
				args = append(args, "--record", "1")
			}
			if c.name == "grouped-by-pension-state" {
				args = append(args, "--group-by-pension-state")
			}
			if c.template == "bulk_soldier" {
				args = append(args, "--sort-by", "last_name")
			}

			cmd := exec.Command(binPath, args...)
			cmd.Dir = repoRootFromT(t)
			outBytes, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dixiedata-tune %s failed: %v\n%s", c.name, err, outBytes)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			snapPath := filepath.Join("testdata", "snapshots-cli", c.name+".pdf")
			if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
				if err := os.WriteFile(snapPath, got, 0o644); err != nil {
					t.Fatalf("write snapshot: %v", err)
				}
				t.Logf("snapshot updated: %s (%d bytes)", snapPath, len(got))
				return
			}
			want, err := os.ReadFile(snapPath)
			if err != nil {
				t.Skipf("snapshot %s missing (run with UPDATE_SNAPSHOTS=1 to create): %v",
					snapPath, err)
			}
			if !bytes.Equal(want, got) {
				t.Fatalf("CLI snapshot mismatch for %s: want %d got %d bytes",
					snapPath, len(want), len(got))
			}
		})
	}
}

// orientationFor returns "P" for portrait templates, "L" otherwise.
func orientationFor(template string) string {
	if bytes.Contains([]byte(template), []byte("portrait")) {
		return "P"
	}
	return "L"
}

// findTuneSource locates the tools/tune directory by walking up
// from the test's working directory looking for a main.go file.
func findTuneSource(t *testing.T) string {
	t.Helper()
	root := repoRootFromT(t)
	candidate := filepath.Join(root, "tools", "tune")
	if st, err := os.Stat(filepath.Join(candidate, "main.go")); err == nil && !st.IsDir() {
		return candidate
	}
	return ""
}

// buildTuneBinary compiles the dixiedata-tune binary once per test
// process and returns its absolute path.
var tuneBinPath string

func buildTuneBinary(t *testing.T) string {
	t.Helper()
	if tuneBinPath != "" {
		if _, err := os.Stat(tuneBinPath); err == nil {
			return tuneBinPath
		}
	}
	tuneDir := findTuneSource(t)
	if tuneDir == "" {
		t.Skip("tools/tune source not found")
	}
	bin := filepath.Join(t.TempDir(), "dixiedata-tune.exe")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = tuneDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tools/tune: %v\n%s", err, out)
	}
	tuneBinPath = bin
	return bin
}

// repoRootFromT walks up from the test's working directory to
// find the directory containing the main module's go.mod.
func repoRootFromT(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find repo root from %s", dir)
	return ""
}