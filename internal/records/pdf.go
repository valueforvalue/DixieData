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
// Person-Record PDF in landscape orientation. Mirrors
// pdf-records-per-page-landscape (8) in templates/common/record_card.typ.
// If you change one, change both — the regression net in
// internal/records/pdf_test.go pins the Go const and renders a
// synthetic soldier payload through the export pipeline to
// assert the typst template doesn't blow past the cap.
const PDFRecordsPerPage = 8

// PDFRecordsPerPagePortrait is the per-page cap for portrait
// Person-Record PDFs. Portrait has ~70% of landscape's record
// column width (single 50% page column vs the 50% right column
// in landscape) and the same row pitch, so the cap is reduced
// to keep one-page predictability. 6 was picked over 8 after
// round 35 review: with 8 the inline bio (when set) pushed
// records into a second column, which still triggered a 3rd
// page from the dedicated bio page; 6 fits the same envelope
// cleanly with the inline bio. Mirrors
// pdf-records-per-page-portrait in templates/common/record_card.typ.
// Same dual-side invariant: change both, regen snapshots.
const PDFRecordsPerPagePortrait = 6
