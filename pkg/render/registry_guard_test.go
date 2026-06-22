package render

import (
	"strings"
	"testing"
)

// TestRegistryResolveRejectsPerRecordTemplateAsBulk pins the
// issue #68 bulk guard. A BulkTemplate that names a per-record
// template (one whose metadata block has record_types containing
// a per-record type but not "bulk") must be rejected with a clear
// error before typst is invoked. The bulk payload's
// data["soldiers"] array is incompatible with per-record
// templates that read data["soldier"].
func TestRegistryResolveRejectsPerRecordTemplateAsBulk(t *testing.T) {
	templatesDir := findTemplatesDir(t)
	typst := NewTypstRenderer(findTypstBinary(t), "")
	reg := NewRegistry(typst, templatesDir)

	// soldier_landscape is a per-record template. Its metadata
	// block declares record_types: [soldier]. The bulk guard
	// must reject it as a BulkTemplate.
	_, err := reg.Resolve(PrintSettings{BulkTemplate: "soldier_landscape"}, "bulk")
	if err == nil {
		t.Fatalf("expected Registry.Resolve to reject per-record template as BulkTemplate, got nil")
	}
	if !strings.Contains(err.Error(), "BulkTemplate") {
		t.Fatalf("expected error to mention BulkTemplate, got: %v", err)
	}
	if !strings.Contains(err.Error(), "soldier_landscape") {
		t.Fatalf("expected error to name the offending template, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bulk") {
		t.Fatalf("expected error to mention the bulk payload, got: %v", err)
	}
}

// TestRegistryResolveAcceptsBulkTemplateForBulk verifies the
// positive case: bulk_soldier is the canonical bulk template
// and Resolve returns it without error.
func TestRegistryResolveAcceptsBulkTemplateForBulk(t *testing.T) {
	templatesDir := findTemplatesDir(t)
	typst := NewTypstRenderer(findTypstBinary(t), "")
	reg := NewRegistry(typst, templatesDir)

	tpl, err := reg.Resolve(PrintSettings{BulkTemplate: "bulk_soldier"}, "bulk")
	if err != nil {
		t.Fatalf("Resolve(bulk_soldier, bulk) failed: %v", err)
	}
	if tpl.Name != "bulk_soldier" {
		t.Fatalf("expected bulk_soldier, got %q", tpl.Name)
	}
}

// TestRegistryResolveDispatchesOnRecordType pins the issue #68
// dispatch. With both SingleRecordTemplate and BulkTemplate
// unset, Resolve falls through to defaultTemplateName based on
// recordType. With BulkTemplate set and recordType="bulk", the
// bulk override wins; with SingleRecordTemplate set and
// recordType="soldier", the per-record override wins.
func TestRegistryResolveDispatchesOnRecordType(t *testing.T) {
	templatesDir := findTemplatesDir(t)
	typst := NewTypstRenderer(findTypstBinary(t), "")
	reg := NewRegistry(typst, templatesDir)

	// Default for recordType="soldier", orientation="L" is
	// soldier_landscape.
	tpl, err := reg.Resolve(PrintSettings{Orientation: "L"}, "soldier")
	if err != nil {
		t.Fatalf("Resolve(soldier, default) failed: %v", err)
	}
	if tpl.Name != "soldier_landscape" {
		t.Fatalf("expected soldier_landscape, got %q", tpl.Name)
	}

	// SingleRecordTemplate wins for non-bulk recordTypes.
	tpl, err = reg.Resolve(PrintSettings{
		Orientation:          "L",
		SingleRecordTemplate: "widow_landscape",
	}, "widow")
	if err != nil {
		t.Fatalf("Resolve(single override) failed: %v", err)
	}
	if tpl.Name != "widow_landscape" {
		t.Fatalf("expected widow_landscape, got %q", tpl.Name)
	}

	// BulkTemplate wins for recordType="bulk".
	tpl, err = reg.Resolve(PrintSettings{
		Orientation: "L",
		BulkTemplate: "bulk_soldier",
	}, "bulk")
	if err != nil {
		t.Fatalf("Resolve(bulk override) failed: %v", err)
	}
	if tpl.Name != "bulk_soldier" {
		t.Fatalf("expected bulk_soldier, got %q", tpl.Name)
	}

	// Default for recordType="bulk" is bulk_soldier.
	tpl, err = reg.Resolve(PrintSettings{Orientation: "L"}, "bulk")
	if err != nil {
		t.Fatalf("Resolve(bulk, default) failed: %v", err)
	}
	if tpl.Name != "bulk_soldier" {
		t.Fatalf("expected default bulk_soldier, got %q", tpl.Name)
	}
}
