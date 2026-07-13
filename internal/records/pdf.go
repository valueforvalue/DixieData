// pdf.go — PDF export constants shared between the Go export
// pipeline (appshell handlers, archive export_service) and the
// Typst template (templates/common/record_card.typ). The const
// here is the single source of truth for the "how many Source
// Records fit on one Person Record PDF" number; the matching
// Typst constant pdf-records-per-page lives in record_card.typ
// at the top of the formatting helpers section.
//
// Issue #513: when a soldier has more Source Records than this
// cap, the Typst render truncates the list to PDFRecordsPerPage
// + emits a muted footnote, AND the export handler emits an
// X-DixieData-Toast warning so the user sees the truncation in
// the UI even when they only look at the PDF or only at the
// toast. Both sides must agree on the number; the audit net
// pins the pair (audit/smoke_pdf_records_cap.test.mjs or
// similar — see issue apply sites).
package records

// PDFRecordsPerPage is the maximum Source Records rendered per
// Person-Record PDF. Mirrors pdf-records-per-page in
// templates/common/record_card.typ. If you change one, change
// both — the regression net in internal/records/pdf_test.go
// pins the Go const and renders a synthetic soldier payload
// through the export pipeline to assert the typst template
// doesn't blow past the cap.
const PDFRecordsPerPage = 12
