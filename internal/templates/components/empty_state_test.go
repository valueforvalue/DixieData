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