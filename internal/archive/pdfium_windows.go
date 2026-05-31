//go:build windows

package archive

import (
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const pdfiumRenderDPI = 144

type pdfiumJPEGRasterizer struct{}

type pdfiumAPI struct {
	dll                 *syscall.LazyDLL
	initLibrary         *syscall.LazyProc
	destroyLibrary      *syscall.LazyProc
	loadMemDocument64   *syscall.LazyProc
	closeDocument       *syscall.LazyProc
	getPageCount        *syscall.LazyProc
	getPageSizeByIndexF *syscall.LazyProc
	loadPage            *syscall.LazyProc
	closePage           *syscall.LazyProc
	bitmapCreate        *syscall.LazyProc
	bitmapDestroy       *syscall.LazyProc
	bitmapFillRect      *syscall.LazyProc
	renderPageBitmap    *syscall.LazyProc
	bitmapGetBuffer     *syscall.LazyProc
	bitmapGetStride     *syscall.LazyProc
	getLastError        *syscall.LazyProc
}

type pdfiumPageSize struct {
	Width  float32
	Height float32
}

func newPDFJPEGRasterizer() pdfToJPEGRasterizer {
	return pdfiumJPEGRasterizer{}
}

func (pdfiumJPEGRasterizer) Rasterize(pdfPath, outputDir string) ([]string, error) {
	dllPath, err := resolvePDFiumDLLPath()
	if err != nil {
		return nil, err
	}
	api := newPDFiumAPI(dllPath)
	if err := api.load(); err != nil {
		return nil, err
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("read temporary PDF: %w", err)
	}
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("temporary PDF was empty")
	}

	api.initLibrary.Call()
	defer api.destroyLibrary.Call()

	document, err := api.loadDocument(pdfBytes)
	if err != nil {
		return nil, err
	}
	defer api.closeDocument.Call(document)

	pageCount := int(callUintptr(api.getPageCount, document))
	if pageCount < 1 {
		return nil, fmt.Errorf("PDF contained no pages")
	}

	outputPaths := make([]string, 0, pageCount)
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		pagePath := filepath.Join(outputDir, fmt.Sprintf("page-%03d.jpg", pageIndex+1))
		if err := api.renderPageToJPG(document, pageIndex, pagePath); err != nil {
			return nil, err
		}
		outputPaths = append(outputPaths, pagePath)
	}
	return outputPaths, nil
}

func newPDFiumAPI(dllPath string) *pdfiumAPI {
	dll := syscall.NewLazyDLL(dllPath)
	return &pdfiumAPI{
		dll:                 dll,
		initLibrary:         dll.NewProc("FPDF_InitLibrary"),
		destroyLibrary:      dll.NewProc("FPDF_DestroyLibrary"),
		loadMemDocument64:   dll.NewProc("FPDF_LoadMemDocument64"),
		closeDocument:       dll.NewProc("FPDF_CloseDocument"),
		getPageCount:        dll.NewProc("FPDF_GetPageCount"),
		getPageSizeByIndexF: dll.NewProc("FPDF_GetPageSizeByIndexF"),
		loadPage:            dll.NewProc("FPDF_LoadPage"),
		closePage:           dll.NewProc("FPDF_ClosePage"),
		bitmapCreate:        dll.NewProc("FPDFBitmap_Create"),
		bitmapDestroy:       dll.NewProc("FPDFBitmap_Destroy"),
		bitmapFillRect:      dll.NewProc("FPDFBitmap_FillRect"),
		renderPageBitmap:    dll.NewProc("FPDF_RenderPageBitmap"),
		bitmapGetBuffer:     dll.NewProc("FPDFBitmap_GetBuffer"),
		bitmapGetStride:     dll.NewProc("FPDFBitmap_GetStride"),
		getLastError:        dll.NewProc("FPDF_GetLastError"),
	}
}

func (a *pdfiumAPI) load() error {
	if err := a.dll.Load(); err != nil {
		return fmt.Errorf("load pdfium.dll from %s: %w", a.dll.Name, err)
	}
	for _, proc := range []*syscall.LazyProc{
		a.initLibrary,
		a.destroyLibrary,
		a.loadMemDocument64,
		a.closeDocument,
		a.getPageCount,
		a.getPageSizeByIndexF,
		a.loadPage,
		a.closePage,
		a.bitmapCreate,
		a.bitmapDestroy,
		a.bitmapFillRect,
		a.renderPageBitmap,
		a.bitmapGetBuffer,
		a.bitmapGetStride,
		a.getLastError,
	} {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("load PDFium procedure %s: %w", proc.Name, err)
		}
	}
	return nil
}

func (a *pdfiumAPI) loadDocument(pdfBytes []byte) (uintptr, error) {
	document := callUintptr(a.loadMemDocument64, uintptr(unsafe.Pointer(&pdfBytes[0])), uintptr(len(pdfBytes)), 0)
	if document == 0 {
		return 0, fmt.Errorf("load generated PDF into PDFium: %s", a.lastError())
	}
	return document, nil
}

func (a *pdfiumAPI) renderPageToJPG(document uintptr, pageIndex int, outputPath string) error {
	var size pdfiumPageSize
	if callUintptr(a.getPageSizeByIndexF, document, uintptr(pageIndex), uintptr(unsafe.Pointer(&size))) == 0 {
		return fmt.Errorf("read PDF page %d size: %s", pageIndex+1, a.lastError())
	}

	width := int(math.Ceil(float64(size.Width) * pdfiumRenderDPI / 72.0))
	height := int(math.Ceil(float64(size.Height) * pdfiumRenderDPI / 72.0))
	if width < 1 || height < 1 {
		return fmt.Errorf("render PDF page %d: invalid page size %.2fx%.2f", pageIndex+1, size.Width, size.Height)
	}

	page := callUintptr(a.loadPage, document, uintptr(pageIndex))
	if page == 0 {
		return fmt.Errorf("load PDF page %d: %s", pageIndex+1, a.lastError())
	}
	defer a.closePage.Call(page)

	bitmap := callUintptr(a.bitmapCreate, uintptr(width), uintptr(height), 0)
	if bitmap == 0 {
		return fmt.Errorf("allocate PDF bitmap for page %d", pageIndex+1)
	}
	defer a.bitmapDestroy.Call(bitmap)

	callUintptr(a.bitmapFillRect, bitmap, 0, 0, uintptr(width), uintptr(height), 0xFFFFFFFF)
	callUintptr(a.renderPageBitmap, bitmap, page, 0, 0, uintptr(width), uintptr(height), 0, 0)

	buffer := callUintptr(a.bitmapGetBuffer, bitmap)
	stride := int(callUintptr(a.bitmapGetStride, bitmap))
	if buffer == 0 || stride < width*4 {
		return fmt.Errorf("read rendered bitmap for page %d", pageIndex+1)
	}

	imageData := unsafe.Slice((*byte)(unsafe.Pointer(buffer)), stride*height)
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sourceRow := imageData[y*stride:]
		destRow := rgba.Pix[y*rgba.Stride:]
		for x := 0; x < width; x++ {
			sourceOffset := x * 4
			destOffset := x * 4
			destRow[destOffset] = sourceRow[sourceOffset+2]
			destRow[destOffset+1] = sourceRow[sourceOffset+1]
			destRow[destOffset+2] = sourceRow[sourceOffset]
			destRow[destOffset+3] = 0xFF
		}
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create JPG page %d: %w", pageIndex+1, err)
	}
	defer file.Close()

	if err := jpeg.Encode(file, rgba, &jpeg.Options{Quality: 92}); err != nil {
		return fmt.Errorf("encode JPG page %d: %w", pageIndex+1, err)
	}
	return nil
}

func (a *pdfiumAPI) lastError() string {
	switch int(callUintptr(a.getLastError)) {
	case 0:
		return "unknown error"
	case 1:
		return "unknown PDF error"
	case 2:
		return "file not found"
	case 3:
		return "invalid format"
	case 4:
		return "password required or incorrect"
	case 5:
		return "unsupported security scheme"
	case 6:
		return "page not found"
	default:
		return fmt.Sprintf("PDFium error code %d", callUintptr(a.getLastError))
	}
}

func callUintptr(proc *syscall.LazyProc, args ...uintptr) uintptr {
	result, _, _ := proc.Call(args...)
	return result
}

func resolvePDFiumDLLPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("DIXIEDATA_PDFIUM_DLL")); configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured, nil
		}
		return "", fmt.Errorf("DIXIEDATA_PDFIUM_DLL points to %s, but the file was not found", configured)
	}

	candidates := make([]string, 0, 4)
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "pdfium.dll"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pdfium.dll"),
			filepath.Join(cwd, "build", "bin", "pdfium.dll"),
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("pdfium.dll was not found; expected it beside the app executable, in the working directory, or at DIXIEDATA_PDFIUM_DLL")
}
