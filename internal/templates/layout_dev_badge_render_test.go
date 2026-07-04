package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/debug"
)

// TestLayout_DevBadgeRendersOnlyWhenDebugMode asserts the
// layout's `if debug.IsDebugMode(ctx) { @components.DevPageBadge(...) }`
// branch from internal/templates/layout.templ:259-260 emits the
// floating dev badge element only when the request context was
// tagged debug-on by the appshell.
//
// This is the Phase 1 feedback loop for the issue #309 dev-badge-
// invisible bug: the rendering is deterministic once `ctx` is
// tagged, so the test runs without a browser. The hypothesis
// chain below covers why this matters:
//   hypothesis 1 (env seeding): appshell.App.debugMode stays false
//     on a `make debug` run because settings.DebugMode=false and
//     the env var isn't read.
//   hypothesis 2 (per-request ctx tagging): appshell.App.debugMode
//     IS true but debug.WithDebugMode isn't being applied to the
//     request context, so debug.IsDebugMode(ctx) returns false
//     inside Layout.
//   hypothesis 3 (badge branch bypass): the templ `if` somehow
//     executes but the rendered HTML is malformed.
//   hypothesis 4 (CSS hides it): the templ branch renders the
//     element but JS or CSS strips visibility.
//
// This test attacks hypothesis 2 directly: regardless of how
// appshell loaded the flag, if a debug-tagged ctx is passed to
// Layout, the badge MUST appear. If appshell didn't tag the ctx
// properly, the badge is missing, and that's the bug.
func TestLayout_DevBadgeRendersOnlyWhenDebugMode(t *testing.T) {
	SetCurrentPagePath("/browse")
	t.Cleanup(ClearCurrentPagePath)

	t.Run("debug-on-context-renders-badge", func(t *testing.T) {
		ctx := debug.WithDebugMode(context.Background(), true)
		out := new(strings.Builder)
		if err := Layout("Browse").Render(ctx, out); err != nil {
			t.Fatalf("Layout.Render: %v", err)
		}
		content := out.String()
		if !strings.Contains(content, `data-dixie-page-badge`) {
			t.Fatalf("expected dev badge to render when debug.IsDebugMode(ctx)=true; missing in HTML:\n%s", strings.Split(content, "\n")[0])
		}
		// Print the rendered DIXIEDATA_DEVTOOLS=... snippet so the
		// matching shape is visible in the test output when the
		// substring check fails.
		if i := strings.Index(content, "DIXIEDATA_DEVTOOLS"); i >= 0 {
			end := i + 80
			if end > len(content) {
				end = len(content)
			}
			t.Logf("rendered DIXIEDATA_DEVTOOLS region: %q", content[i:end])
		}
		if !strings.Contains(content, `DIXIEDATA_DEVTOOLS = true`) {
			t.Fatalf("expected head script to inject DIXIEDATA_DEVTOOLS=true; missing in HTML")
		}
	})

	t.Run("debug-off-context-omits-badge", func(t *testing.T) {
		ctx := debug.WithDebugMode(context.Background(), false)
		out := new(strings.Builder)
		if err := Layout("Browse").Render(ctx, out); err != nil {
			t.Fatalf("Layout.Render: %v", err)
		}
		content := out.String()
		if strings.Contains(content, `data-dixie-page-badge`) {
			t.Fatalf("did not expect dev badge to render when debug.IsDebugMode(ctx)=false")
		}
		if !strings.Contains(content, `DIXIEDATA_DEVTOOLS = false`) {
			t.Fatalf("expected head script to inject DIXIEDATA_DEVTOOLS=false; missing in HTML")
		}
	})

	t.Run("untagged-context-omits-badge-and-injects-false", func(t *testing.T) {
		// No debug.WithDebugMode applied. Per debug.IsDebugMode
		// the helper returns false when the ctx did not flow
		// through WithDebugMode. This is the case that would
		// surface if appshell forgot to tag the ctx.
		out := new(strings.Builder)
		if err := Layout("Browse").Render(context.Background(), out); err != nil {
			t.Fatalf("Layout.Render: %v", err)
		}
		content := out.String()
		if strings.Contains(content, `data-dixie-page-badge`) {
			t.Fatalf("did not expect dev badge to render when ctx lacks debug tag")
		}
		if !strings.Contains(content, `DIXIEDATA_DEVTOOLS = false`) {
			t.Fatalf("expected head script to inject DIXIEDATA_DEVTOOLS=false when ctx lacks debug tag")
		}
	})
}
