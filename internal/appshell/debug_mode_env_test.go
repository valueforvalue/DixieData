package appshell

import (
	"os"
	"testing"
)

// TestDebugMode_EnvSeedsOnWhenSettingOff is a regression test
// for the issue surfaced while smoke-testing issue #309: a
// fresh user runs `make debug` (which sets DIXIEDATA_DEBUG=1 in
// the launcher env), opens any page, and sees the new dev
// badge (bottom-right amber pill) but it never appears.
//
// Root cause: a.debugMode was seeded solely from
// records.LoadLocalSettings.DebugMode (which defaults to false
// in a fresh install). The env var DIXIEDATA_DEBUG=1 was
// honored by internal/debug.log.debugMode but never reached
// appshell.App.debugMode, so debug.IsDebugMode(ctx) returned
// false and the @components.DevPageBadge(...) branch in the
// layout never rendered.
//
// Fix: the seeding block in a.startup() now OR's in the env
// var so the env wins. The contract is pinned by
// decideDebugModeAtStartupSettings in lifecycle.go, exercised
// by these tests.
func TestDebugMode_EnvSeedsOnWhenSettingOff(t *testing.T) {
	t.Setenv("DIXIEDATA_DEBUG", "1")
	t.Cleanup(func() { os.Unsetenv("DIXIEDATA_DEBUG") })
	got := decideDebugModeAtStartupSettings(false)
	if !got {
		t.Fatalf("decideDebugModeAtStartupSettings with DIXIEDATA_DEBUG=1 + settings.DebugMode=false must return true")
	}
}

// TestDebugMode_EnvOffRespectsSettingOff mirrors the previous
// regression: without the env, appshell respects the settings
// file (default OFF on a fresh install).
func TestDebugMode_EnvOffRespectsSettingOff(t *testing.T) {
	t.Setenv("DIXIEDATA_DEBUG", "")
	os.Unsetenv("DIXIEDATA_DEBUG")
	got := decideDebugModeAtStartupSettings(false)
	if got {
		t.Fatalf("decideDebugModeAtStartupSettings with no env + settings.DebugMode=false must return false")
	}
}

// TestDebugMode_SettingsOnIgnoresEnv covers the case where
// the user has Debug Mode on in settings -- the helper should
// return true regardless of the env (the OR is inclusive).
func TestDebugMode_SettingsOnIgnoresEnv(t *testing.T) {
	t.Setenv("DIXIEDATA_DEBUG", "")
	os.Unsetenv("DIXIEDATA_DEBUG")
	got := decideDebugModeAtStartupSettings(true)
	if !got {
		t.Fatalf("decideDebugModeAtStartupSettings with settings.DebugMode=true must return true regardless of env")
	}
}
