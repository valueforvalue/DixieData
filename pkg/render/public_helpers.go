package render

import "github.com/valueforvalue/DixieData/internal/models"

// Public wrappers for the package-private helpers that the legacy test
// file in internal/archive/export_service_test.go depends on. The
// wrappers let the existing tests continue to verify internal layout
// behavior without re-importing the test into this package. After the
// Typst migration is complete (slice 7), the tests that depend on
// these wrappers can be deleted or moved to the render package's own
// test file.

// ImagePathForPDF returns a usable file path for the image, or empty if
// the image is not renderable.
func ImagePathForPDF(image models.Image) string {
	return imagePathForPDF(image)
}

// FirstRecordCardImage returns the path and label of the soldier's
// primary image, or ("", "") if no image should be rendered.
func FirstRecordCardImage(soldier models.Soldier, printerFriendly bool) (string, string) {
	return firstRecordCardImage(soldier, printerFriendly)
}

// FitPDFImageToBounds returns the fitted image dimensions within the
// (maxWidth, maxHeight) box, preserving aspect ratio.
func FitPDFImageToBounds(imagePath string, x, y, maxWidth, maxHeight float64) (float64, float64, float64, float64, bool) {
	return fitPDFImageToBounds(imagePath, x, y, maxWidth, maxHeight)
}

// UsesPortraitCompactRecordCardLayout returns true when the soldier
// record should use the compact portrait layout (60% left column ratio).
func UsesPortraitCompactRecordCardLayout(soldier models.Soldier, options PDFOptions) bool {
	return usesPortraitCompactRecordCardLayout(soldier, options)
}

// PDFTextSegments splits the text into segments of (text, isLink) for
// the rich-text writer.
func PDFTextSegments(text string) []PDFTextSegment {
	return pdfTextSegments(text)
}

// PDFTextSegment is the public form of pdfTextSegment.
type PDFTextSegment = pdfTextSegment

// RegistryEntryLines returns the printable fields for the registry index.
func RegistryEntryLines(soldier models.Soldier) []PDFField {
	return registryEntryLines(soldier)
}

// PDFField is the public form of pdfField.
type PDFField = pdfField

// RecordIdentityFields returns the identity section fields for a soldier.
func RecordIdentityFields(soldier models.Soldier) []PDFField {
	return recordIdentityFields(soldier)
}

// RecordServiceFields returns the service section fields for a soldier.
func RecordServiceFields(soldier models.Soldier, printerFriendly bool) []PDFField {
	return recordServiceFields(soldier, printerFriendly)
}

// PrintFilterUnknownValue is the public form of printFilterUnknownValue.
const PrintFilterUnknownValue = "__unknown__"
