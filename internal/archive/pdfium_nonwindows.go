//go:build !windows

package archive

import "fmt"

type unsupportedPDFJPEGRasterizer struct{}

func newPDFJPEGRasterizer() pdfToJPEGRasterizer {
	return unsupportedPDFJPEGRasterizer{}
}

// Rasterize is the archive-layer method matching its name.
func (unsupportedPDFJPEGRasterizer) Rasterize(string, string) ([]string, error) {
	return nil, fmt.Errorf("JPG export requires the Windows PDFium runtime")
}
