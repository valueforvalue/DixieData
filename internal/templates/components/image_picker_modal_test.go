// image_picker_modal_test.go — issue #612 slice 3
// Pinning tests for the article editor's image picker modal.
// Covers the render shape for both active (articleID > 0)
// and new-article (articleID == 0) modes.
package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestImagePickerModal_RendersWithTabs pins the active
// (edit-form) shape: the modal renders both Upload + Pick
// existing tabs, the file input, and the upload button.
func TestImagePickerModal_RendersWithTabs(t *testing.T) {
	var buf bytes.Buffer
	if err := ImagePickerModal(42).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `data-image-picker-modal`) {
		t.Errorf("modal missing data-image-picker-modal marker\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-tab="upload"`) {
		t.Errorf("modal missing Upload tab\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-tab="existing"`) {
		t.Errorf("modal missing Pick existing tab\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-file-input`) {
		t.Errorf("modal missing file input\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-upload-btn`) {
		t.Errorf("modal missing upload button\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-insert-url`) {
		t.Errorf("modal missing manual URL insert button\nfull render:\n%s", got)
	}
}

// TestImagePickerModal_NewArticleShowsSaveFirst pins the
// new-article (articleID == 0) shape: no tabs, a "save first"
// message, but the manual URL input is still available.
func TestImagePickerModal_NewArticleShowsSaveFirst(t *testing.T) {
	var buf bytes.Buffer
	if err := ImagePickerModal(0).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, `data-image-picker-tab="upload"`) {
		t.Errorf("new-article modal must NOT render Upload tab\nfull render:\n%s", got)
	}
	if strings.Contains(got, `data-image-picker-tab="existing"`) {
		t.Errorf("new-article modal must NOT render Pick existing tab\nfull render:\n%s", got)
	}
	if !strings.Contains(got, "Save the article first") {
		t.Errorf("new-article modal missing save-first message\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-insert-url`) {
		t.Errorf("new-article modal must still have manual URL insert button\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `data-image-picker-url`) {
		t.Errorf("new-article modal missing URL input\nfull render:\n%s", got)
	}
}

// TestImagePickerModal_HiddenByDefault pins the overlay
// vocabulary: the modal starts hidden (class contains
// "hidden") so it doesn't paint on page load.
func TestImagePickerModal_HiddenByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := ImagePickerModal(1).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "hidden") {
		t.Errorf("modal must start hidden\nfull render:\n%s", got)
	}
}

// TestImagePickerModal_HasA11yAttrs pins the accessibility
// contract: role="dialog", aria-modal="true",
// aria-labelledby pointing to the heading.
func TestImagePickerModal_HasA11yAttrs(t *testing.T) {
	var buf bytes.Buffer
	if err := ImagePickerModal(1).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `role="dialog"`) {
		t.Errorf("modal missing role=dialog\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `aria-modal="true"`) {
		t.Errorf("modal missing aria-modal=true\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `aria-labelledby="image-picker-modal-heading"`) {
		t.Errorf("modal missing aria-labelledby\nfull render:\n%s", got)
	}
}
