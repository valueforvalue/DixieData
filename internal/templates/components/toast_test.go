package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestToast_RendersCardClass verifies the primitive emits the
// `toast-card` class the existing JS renderer looks for. Mirrors
// the contract documented in the toast primitive file header.
func TestToast_RendersCardClass(t *testing.T) {
	var buf bytes.Buffer
	if err := Toast("success", "Saved").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `class="toast-card"`) {
		t.Fatalf("toast missing toast-card class:\n%s", got)
	}
	if !strings.Contains(got, `data-toast-kind="success"`) {
		t.Fatalf("toast missing data-toast-kind:\n%s", got)
	}
	if !strings.Contains(got, `<p>Saved</p>`) {
		t.Fatalf("toast missing message body:\n%s", got)
	}
}