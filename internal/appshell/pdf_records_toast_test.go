// Issue #513 regression net: setPDFRecordsTruncationToast emits
// an X-DixieData-Toast warning header when the soldier carries
// more Source Records than records.PDFRecordsPerPage, and stays
// silent otherwise. The toast mirrors the Typst-side cap so the
// user sees the same truncation signal in the UI that they see
// in the PDF body.
//
// The helper is the only new behavior in this issue; the handler
// integration is exercised by the live audit harness via the
// existing soldier PDF smoke probes (audit/smoke_soldier_images.mjs
// and friends). This file owns the unit-level pin so the helper's
// trim/omit math stays correct across future refactors.
package appshell

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

func TestSetPDFRecordsTruncationToast_NoOpUnderCap(t *testing.T) {
	rec := httptest.NewRecorder()
	soldier := &models.Soldier{
		Records: make([]models.Record, records.PDFRecordsPerPage),
	}
	setPDFRecordsTruncationToast(rec, soldier)
	if got := rec.Header().Get("X-DixieData-Toast"); got != "" {
		t.Errorf("under-cap toast = %q, want empty", got)
	}
}

func TestSetPDFRecordsTruncationToast_NoOpOnNilSoldier(t *testing.T) {
	rec := httptest.NewRecorder()
	setPDFRecordsTruncationToast(rec, nil)
	if got := rec.Header().Get("X-DixieData-Toast"); got != "" {
		t.Errorf("nil-soldier toast = %q, want empty", got)
	}
}

func TestSetPDFRecordsTruncationToast_AtCapPlusOne(t *testing.T) {
	rec := httptest.NewRecorder()
	// PDFRecordsPerPage + 1 records — minimal overflow that the
	// Typst cap will truncate to PDFRecordsPerPage.
	count := records.PDFRecordsPerPage + 1
	soldier := &models.Soldier{
		Records: make([]models.Record, count),
	}
	setPDFRecordsTruncationToast(rec, soldier)

	toast := rec.Header().Get("X-DixieData-Toast")
	if toast == "" {
		t.Fatalf("overflow toast not set; headers = %v", rec.Header())
	}
	if !strings.Contains(toast, "first 8") {
		t.Errorf("toast = %q, want substring %q", toast, "first 8")
	}
	if !strings.Contains(toast, "1 additional omitted") {
		t.Errorf("toast = %q, want substring %q", toast, "1 additional omitted")
	}
	if got := rec.Header().Get("X-DixieData-Toast-Type"); got != "warning" {
		t.Errorf("toast type = %q, want %q", got, "warning")
	}
}

func TestSetPDFRecordsTruncationToast_WellOverCap(t *testing.T) {
	rec := httptest.NewRecorder()
	// Far over the cap — verifies the omission count is reported
	// correctly (PDFRecordsPerPage + 5 = 17, omitted should be 5).
	count := records.PDFRecordsPerPage + 5
	soldier := &models.Soldier{
		Records: make([]models.Record, count),
	}
	setPDFRecordsTruncationToast(rec, soldier)

	toast := rec.Header().Get("X-DixieData-Toast")
	if !strings.Contains(toast, "5 additional omitted") {
		t.Errorf("toast = %q, want substring %q", toast, "5 additional omitted")
	}
}
