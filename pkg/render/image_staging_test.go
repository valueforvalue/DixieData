package render

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestStageSoldierImagesCopiesPrimaryAndFallback verifies that the
// renderer's image-staging step copies referenced image files into
// the typst workdir's images/ subdirectory. The template then uses
// `#image("images/filename")` to embed them.
//
// Two cases are covered:
//   - Map-shaped input (data round-tripped through JSON, where the
//     soldier is a map[string]any): copies via the dynamic path.
func TestStageSoldierImagesCopiesPrimaryAndFallback(t *testing.T) {
	// Set up an image file on disk that the staged copy will read.
	srcDir := t.TempDir()
	imgPath := filepath.Join(srcDir, "portrait.png")
	if err := os.WriteFile(imgPath, pngFixture(t), 0o644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}

	wd := t.TempDir()
	data := map[string]any{
		"soldier": map[string]any{
			"images": []any{
				map[string]any{
					"file_name":     "portrait.png",
					"file_path":     imgPath,
					"resolved_path": imgPath,
					"is_primary":    true,
				},
			},
		},
	}
	if err := stageSoldierImages(wd, data); err != nil {
		t.Fatalf("stageSoldierImages: %v", err)
	}
	staged := filepath.Join(wd, "images", "portrait.png")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("expected staged image at %q: %v", staged, err)
	}
}

// TestImagePathForPDFReturnsCorrectedPathForMisnamedFile exercises the
// fpdf path's image-rewriting helper. The fpdf library dispatches
// to its decoder based on the file's extension; a real PNG saved
// with a .jpg extension causes fpdf's parsejpg to fail with
// "invalid JPEG format: missing SOI marker", which sets fpdf's
// internal error and halts the rest of the render (producing a
// 0-byte / no-output PDF). The helper must detect the format
// mismatch and return a path with the corrected extension so
// fpdf dispatches to parsepng instead.
//
// (The two TestImagePathForPDF* tests that previously lived here
// tested the fpdf path's image-rewriting helper. After slice 7
// removed the fpdf path, those tests are gone; the rewrite
// helper itself was the only place the on-disk cache lived.)

// TestStageSoldierImagesTypedSoldierRenamesMisnamedFile exercises the
// typed-Soldier path: when the soldier is passed as a models.Soldier
// value (not a map[string]any from JSON round-trip), the staging step
// must still detect a misnamed image (PNG content in a .jpg file) and
// update the data payload so the template references the renamed
// file. The map-only code path left typed-Soldier data with the
// original extension, so typst failed with "file not found" at
// #image("images/<original>") even though the staged file was on
// disk under its detected name.
func TestStageSoldierImagesTypedSoldierRenamesMisnamedFile(t *testing.T) {
	srcDir := t.TempDir()
	// Real PNG bytes, but stored with a .jpg extension.
	misnamed := filepath.Join(srcDir, "DXD-00052-img-001.jpg")
	if err := os.WriteFile(misnamed, pngFixture(t), 0o644); err != nil {
		t.Fatalf("WriteFile misnamed: %v", err)
	}

	wd := t.TempDir()
	soldier := models.Soldier{
		Images: []models.Image{
			{
				FileName:      "DXD-00052-img-001.jpg",
				FilePath:      misnamed,
				ResolvedPath:  misnamed,
				IsPrimary:     true,
			},
		},
	}
	data := map[string]any{"soldier": soldier}
	if err := stageSoldierImages(wd, data); err != nil {
		t.Fatalf("stageSoldierImages: %v", err)
	}

	// File on disk must be staged under the detected extension.
	stagedJpg := filepath.Join(wd, "images", "DXD-00052-img-001.jpg")
	if _, err := os.Stat(stagedJpg); err == nil {
		t.Fatalf("did not expect misnamed-name file to be staged verbatim at %q", stagedJpg)
	}
	stagedPng := filepath.Join(wd, "images", "DXD-00052-img-001.png")
	if _, err := os.Stat(stagedPng); err != nil {
		t.Fatalf("expected staged image at %q: %v", stagedPng, err)
	}

	// And the data payload's FileName must have been updated so
	// the typst template resolves #image("images/<name>") to the
	// file that actually exists.
	out, ok := data["soldier"].(models.Soldier)
	if !ok {
		t.Fatalf("data[\"soldier\"] type = %T, want models.Soldier", data["soldier"])
	}
	if len(out.Images) != 1 {
		t.Fatalf("len(images) = %d, want 1", len(out.Images))
	}
	if out.Images[0].FileName != "DXD-00052-img-001.png" {
		t.Fatalf("FileName = %q, want %q", out.Images[0].FileName, "DXD-00052-img-001.png")
	}
}

// TestStageSoldierImagesSkipsMissingFile verifies the renderer does
// not error when an image's source file does not exist. This can
// happen when the database references a file the user has moved or
// deleted. The renderer leaves the images/ directory empty in that
// case so the template's `image("images/...")` reference degrades to
// a typst warning rather than crashing the compile.
func TestStageSoldierImagesSkipsMissingFile(t *testing.T) {
	wd := t.TempDir()
	data := map[string]any{
		"soldier": map[string]any{
			"images": []any{
				map[string]any{
					"file_name":     "missing.png",
					"resolved_path": filepath.Join(t.TempDir(), "does-not-exist.png"),
					"is_primary":    true,
				},
			},
		},
	}
	if err := stageSoldierImages(wd, data); err != nil {
		t.Fatalf("stageSoldierImages: %v", err)
	}
	imagesDir := filepath.Join(wd, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		// No directory was created because no images were found.
		// Acceptable.
		return
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty images directory for missing files, got %d entries", len(entries))
	}
}

// TestStageSoldierImagesEmptyDataNoError verifies the renderer does
// not error on empty data or on a soldier without images.
func TestStageSoldierImagesEmptyDataNoError(t *testing.T) {
	wd := t.TempDir()
	if err := stageSoldierImages(wd, map[string]any{}); err != nil {
		t.Fatalf("stageSoldierImages with empty data: %v", err)
	}
	if err := stageSoldierImages(wd, map[string]any{
		"soldier": map[string]any{},
	}); err != nil {
		t.Fatalf("stageSoldierImages with empty soldier: %v", err)
	}
}

// TestDetectImageFormat verifies the magic-byte sniffer used by
// stageSoldierImages to rename files whose on-disk extension
// does not match the file's actual format.
func TestDetectImageFormat(t *testing.T) {
	dir := t.TempDir()
	// PNG with a `.jpg` extension: should return ".png".
	pngPath := filepath.Join(dir, "misnamed.jpg")
	if err := os.WriteFile(pngPath, pngFixture(t), 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	ext, err := detectImageFormat(pngPath)
	if err != nil {
		t.Fatalf("detectImageFormat png: %v", err)
	}
	if ext != ".png" {
		t.Fatalf("ext = %q, want .png", ext)
	}
	// Real JPEG: should return ".jpg".
	jpgPath := filepath.Join(dir, "real.jpg")
	if err := os.WriteFile(jpgPath, jpegFixture(t), 0o644); err != nil {
		t.Fatalf("write jpg: %v", err)
	}
	ext, err = detectImageFormat(jpgPath)
	if err != nil {
		t.Fatalf("detectImageFormat jpg: %v", err)
	}
	if ext != ".jpg" {
		t.Fatalf("ext = %q, want .jpg", ext)
	}
	// Unknown format (text file): should return an error.
	txtPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if _, err := detectImageFormat(txtPath); err == nil {
		t.Fatalf("detectImageFormat txt: expected error, got nil")
	}
}

// jpegFixture returns the bytes of a tiny but valid JPEG (a
// 1x1 dark-grey image). Mirrors pngFixture.
func jpegFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 64, G: 64, B: 64, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg fixture: %v", err)
	}
	return buf.Bytes()
}

// TestRenderSoldierLandscapeWithImage is the closest end-to-end
// test we can run inside the test suite without depending on the
// tune tool. It renders the soldier_landscape template with a
// soldier that has a real PNG image and asserts the rendered PDF
// is non-empty. Visual fidelity is verified separately via the
// tune tool's pixel-diff harness.
func TestRenderSoldierLandscapeWithImage(t *testing.T) {
	binPath := findTypstBinary(t)
	templatesDir := findTemplatesDir(t)

	typst := NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	tpls, err := typst.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	var tpl *Template
	for i, c := range tpls {
		if c.Name == "soldier_landscape" {
			tpl = &tpls[i]
			break
		}
	}
	if tpl == nil {
		t.Skip("soldier_landscape template not present")
	}

	imgPath := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(imgPath, pngFixture(t), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data := map[string]any{
		"soldier": map[string]any{
			"display_id": "DD 100",
			"entry_type": "soldier",
			"first_name": "John",
			"last_name":  "Doe",
			"images": []any{
				map[string]any{
					"file_name":     "portrait.png",
					"resolved_path": imgPath,
					"is_primary":    true,
				},
			},
		},
		"options": map[string]any{
			"includeImages": true,
			"orientation":   "L",
		},
		"settings": map[string]any{
			"orientation": "L",
			"template":    "soldier_landscape",
		},
	}

	var buf []byte
	w := newBytesWriter(&buf)
	if err := typst.Render(context.Background(), *tpl, data, w); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(buf) < 100 {
		t.Fatalf("rendered PDF suspiciously small (%d bytes)", len(buf))
	}
}

// TestRenderWithImagesEndToEnd renders hello.typ with a real PNG on
// disk and asserts the rendered PDF is non-empty. hello.typ does
// not actually embed images, so this only checks that the image
// staging step integrates cleanly with the typst compile pipeline.
func TestRenderWithImagesEndToEnd(t *testing.T) {
	binPath := findTypstBinary(t)
	templatesDir := findTemplatesDir(t)

	typst := NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	tpls, err := typst.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	var hello *Template
	for i, c := range tpls {
		if c.Name == "hello" {
			hello = &tpls[i]
			break
		}
	}
	if hello == nil {
		t.Skip("hello template not present")
	}

	imgPath := filepath.Join(t.TempDir(), "test.png")
	if err := os.WriteFile(imgPath, pngFixture(t), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data := map[string]any{
		"soldier": map[string]any{
			"display_id": "IMG-001",
			"images": []any{
				map[string]any{
					"file_name":     "test.png",
					"resolved_path": imgPath,
					"is_primary":    true,
				},
			},
		},
	}

	var buf []byte
	w := newBytesWriter(&buf)
	if err := typst.Render(context.Background(), *hello, data, w); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(buf) < 100 {
		t.Fatalf("rendered PDF suspiciously small (%d bytes)", len(buf))
	}
}

// pngFixture returns the bytes of a tiny but valid PNG (a 1x1 red
// square). Typst validates PNG CRCs, so we have to generate a
// proper image rather than embedding a hand-rolled byte sequence.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf []byte
	enc := &byteWriter{dst: &buf}
	if err := png.Encode(enc, img); err != nil {
		t.Fatalf("Encode png fixture: %v", err)
	}
	return buf
}

// byteWriter is an io.Writer that appends to a byte slice. Avoids
// the bytes.Buffer import for the small fixture.
type byteWriter struct{ dst *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.dst = append(*w.dst, p...)
	return len(p), nil
}

// newBytesWriter returns an io.Writer that appends to dst.
func newBytesWriter(dst *[]byte) *bytesWriter {
	return &bytesWriter{dst: dst}
}

type bytesWriter struct {
	dst *[]byte
}

func (w *bytesWriter) Write(p []byte) (int, error) {
	*w.dst = append(*w.dst, p...)
	return len(p), nil
}
