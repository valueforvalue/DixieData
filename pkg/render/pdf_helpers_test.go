package render

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestRecordPDFFieldsUseDisplaySpecificEmptyStates moved from
// internal/archive/export_service_test.go as part of slice 0 of the
// Typst migration (extracting the PDF code to the render package).
// The test pins the display-empty-state behavior of the field
// constructors: blank fields render as "N/A" or "Unknown" depending on
// which type of field it is, and the rendered value drives the
// `omitPDFValue()` filter in the layout path.
func TestRecordPDFFieldsUseDisplaySpecificEmptyStates(t *testing.T) {
	soldier := models.Soldier{
		EntryType:             "soldier",
		ConfederateHomeStatus: "N/A",
		ConfederateHomeName:   "",
		PensionState:          "N/A",
	}

	identity := recordIdentityFields(soldier)
	service := recordServiceFields(soldier, false)

	assertPDFField := func(fields []pdfField, label, want string, wantVisible bool) {
		t.Helper()
		for _, field := range fields {
			if field.Label != label {
				continue
			}
			if field.visible() != wantVisible {
				t.Fatalf("%s visible = %v, want %v", label, field.visible(), wantVisible)
			}
			if field.renderedValue() != want {
				t.Fatalf("%s renderedValue = %q, want %q", label, field.renderedValue(), want)
			}
			return
		}
		t.Fatalf("missing field %s", label)
	}

	assertPDFField(identity, "Prefix", "", true)
	assertPDFField(identity, "First Name", "", true)
	assertPDFField(identity, "Birth Date", "Unknown", true)
	assertPDFField(identity, "Death Date", "Unknown", true)
	assertPDFField(service, "Rank In", "", true)
	assertPDFField(service, "Rank Out", "", true)
	assertPDFField(service, "Unit", "", true)
	assertPDFField(service, "Pension State", "N/A", true)
	assertPDFField(service, "Confederate Home Status", "N/A", true)
	assertPDFField(service, "Confederate Home Name", "N/A", true)
}
