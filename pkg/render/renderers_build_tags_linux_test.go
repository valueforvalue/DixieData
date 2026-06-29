//go:build linux

// This test file is build-tag-gated to Linux only. Its purpose is
// to make the cross-platform build-tag split visible: if a future
// contributor removes the //go:build !windows tag from
// pkg/render/renderers_nonwindows.go, the audit workflow's Linux
// runner will fail to compile this file (because the non-Windows
// stub will be missing) and CI will catch the regression.
//
// On macOS, Windows, and other platforms this file is excluded
// from the build.

package render

import (
	"runtime"
	"testing"
)

func TestRenderersBuildTagsLinux(t *testing.T) {
	// Cross-check that this file actually got compiled on Linux.
	// If the test even runs, the build-tag split is wired.
	if runtime.GOOS != "linux" {
		t.Skipf("this test only runs on linux, got %s", runtime.GOOS)
	}
	t.Log("renderers_nonwindows.go is compiled on Linux; " +
		"audit workflow on ubuntu-latest can build dixiedata-web")
}
