package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestEmptyState_Default verifies the simplest call renders the
// empty-state container with title + body and the data-empty-state
// audit hook.
func TestEmptyState_Default(t *testing.T) {
	var buf bytes.Buffer
	if err := EmptyState("No results", "Try a different search.", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`class="empty-state"`,
		`data-empty-state="true"`,
		`data-empty-state-kind="info"`,
		`No results`,
		`Try a different search.`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("empty-state missing %q\nfull: %s", needle, got)
		}
	}
}

// TestEmptyState_ExtraClass verifies the extra-class append.
func TestEmptyState_ExtraClass(t *testing.T) {
	var buf bytes.Buffer
	if err := EmptyState("Empty", "Nothing here.", "mt-4 p-4").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `class="empty-state mt-4 p-4"`) {
		t.Fatalf("empty-state extra-class drift:\n%s", got)
	}
}

// TestEmptyStateError_Default verifies the error variant renders the
// red border + warning icon + role="alert" surface used by
// appshell.respondErrorFragment. Issue #384 / Slice 1 regression net.
func TestEmptyStateError_Default(t *testing.T) {
	var buf bytes.Buffer
	if err := EmptyStateError("Could not load articles.", "Try refreshing the page.", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`class="empty-state empty-state-error"`,
		`data-empty-state="true"`,
		`data-empty-state-kind="error"`,
		`role="alert"`,
		`Could not load articles.`,
		`Try refreshing the page.`,
		`⚠`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("empty-state-error missing %q\nfull: %s", needle, got)
		}
	}
}

// TestEmptyStateError_ExtraClass verifies the error variant honours
// extra-class appends (used when an htmx swap wants tighter padding
// than the default 1rem 1.25rem).
func TestEmptyStateError_ExtraClass(t *testing.T) {
	var buf bytes.Buffer
	if err := EmptyStateError("Render failed.", "See the debug console for details.", "mt-4").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `class="empty-state empty-state-error mt-4"`) {
		t.Fatalf("empty-state-error extra-class drift:\n%s", got)
	}
}