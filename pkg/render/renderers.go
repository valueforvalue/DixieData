package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TypstRenderer implements Renderer by compiling .typ templates with
// the bundled Typst binary. It shells out directly to the binary
// (rather than via the github.com/Dadido3/go-typst wrapper) so we can
// pass Windows-specific SysProcAttr{HideWindow: true} and avoid the
// black console window that the wrapper would otherwise allocate
// when running on Windows.
type TypstRenderer struct {
	binPath  string
	rootDir  string
	fontDirs []string
}

// NewTypstRenderer constructs a TypstRenderer that shells out to the
// Typst binary at binPath. The rootDir is the working directory for
// `typst compile`; the template files are resolved relative to it.
func NewTypstRenderer(binPath, rootDir string) *TypstRenderer {
	return &TypstRenderer{
		binPath:  binPath,
		rootDir:  rootDir,
		fontDirs: nil,
	}
}

// Name returns the engine name.
func (t *TypstRenderer) Name() string { return "typst" }

// ListTemplates returns every .typ file in the renderer root.
func (t *TypstRenderer) ListTemplates() ([]Template, error) {
	templatesDir := filepath.Join(t.rootDir, "templates")
	return DiscoverTemplates(templatesDir)
}

// Render compiles a .typ template with the given data. The data is
// serialized as JSON and exposed to the template via #let data =
// json("data.json"). The output is written to w as a PDF.
//
// The renderer's job is:
//   1. Build a temporary working directory.
//   2. Copy the template's containing directory (so #import statements
//      resolve) plus the template itself as main.typ.
//   3. Stage any image files referenced by the soldier record at
//      workDir/images/, so the template can use `#image("images/...")`.
//   4. Write data.json into the workdir.
//   5. Shell out to `typst compile --root <workdir> <workdir>/main.typ
//      -o <outputPath>` with a hidden console window on Windows.
//   6. Stream the rendered PDF back to w.
//
// We write the PDF to a temp file first so the shell-out stays simple
// (no stdout-piping from a hidden child) and the caller gets a clean
// io.Writer stream.
func (t *TypstRenderer) Render(ctx context.Context, tpl Template, data map[string]any, w io.Writer) error {
	// Build a temporary working directory. Copy the template and any
	// sibling files it may import (e.g. common/theme.typ). Typst's
	// import paths are resolved relative to the root (which we set
	// to the work directory), so we need the full template tree
	// there.
	workDir, err := os.MkdirTemp("", "dixiedata-typst-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	// Copy the template's containing directory contents into workDir
	// so the template's #import statements resolve.
	srcDir := filepath.Dir(tpl.Path)
	if err := copyDir(srcDir, workDir); err != nil {
		return fmt.Errorf("copy template dir: %w", err)
	}
	// Also copy the template's name as main.typ so the import
	// statements find it under that name.
	mainPath := filepath.Join(workDir, "main.typ")
	if err := copyFile(tpl.Path, mainPath); err != nil {
		return fmt.Errorf("copy template: %w", err)
	}

	// Stage image files referenced by the data payload. The template
	// can reference them as `images/filename`. We look for soldier
	// images on data["soldier"].Images (a []models.Image or similar)
	// and copy each one whose ResolvedPath or FilePath exists.
	if err := stageSoldierImages(workDir, data); err != nil {
		return fmt.Errorf("stage images: %w", err)
	}

	dataPath := filepath.Join(workDir, "data.json")
	if err := writeJSONFile(dataPath, data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	// Run typst compile. Output goes to a temp PDF file which we
	// then stream to w.
	outputPath := filepath.Join(workDir, "out.pdf")
	if err := runTypstCompile(t.binPath, workDir, mainPath, outputPath); err != nil {
		return err
	}

	f, err := os.Open(outputPath)
	if err != nil {
		return fmt.Errorf("open typst output: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("copy typst output: %w", err)
	}
	return nil
}

// runTypstCompile invokes the bundled Typst binary with the
// arguments required for compilation. On Windows the child process
// is created with HideWindow so the user does not see a black
// console window flash during PDF export.
//
// The Typst CLI signature is `typst compile [OPTIONS] <INPUT>
// [OUTPUT]`. The output path is a positional argument, not `-o`.
func runTypstCompile(binPath, workDir, mainPath, outputPath string) error {
	args := []string{
		"compile",
		"--root", workDir,
		mainPath,
		outputPath,
	}

	cmd := exec.Command(binPath, args...)
	cmd.Dir = workDir

	hideWindow(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("typst compile failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// hideWindow sets the Windows-specific SysProcAttr fields so a child
// process spawned via exec.Command does not allocate a console
// window. This is a no-op on non-Windows platforms.
func hideWindow(cmd *exec.Cmd) {
	if runtime.GOOS != "windows" {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// stageSoldierImages copies any image files referenced by the
// soldier record into <workDir>/images/. The template can then
// reference them as images/filename. We deliberately accept either a
// []models.Image or a []map[string]any so this stays flexible across
// the encoder and any future refactors.
//
// Images whose source file does not exist (e.g. the soldier has no
// image, or the file was moved) are skipped silently — the template
// is expected to handle a missing file path gracefully.
func stageSoldierImages(workDir string, data map[string]any) error {
	soldier, ok := data["soldier"]
	if !ok {
		return nil
	}
	images := extractImages(soldier)
	if len(images) == 0 {
		return nil
	}

	imagesDir := filepath.Join(workDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return err
	}

	for _, img := range images {
		source := imageSourcePath(img)
		if source == "" {
			continue
		}
		info, err := os.Stat(source)
		if err != nil || info.IsDir() {
			continue
		}
		// Detect the actual image format from the magic bytes.
		// Some files in the DB have a `.jpg` extension but PNG
		// content (or vice versa). Typst's image decoder uses
		// the file extension to pick a decoder, so a mismatched
		// file fails with "illegal start bytes" errors. We rename
		// the staged copy to the detected extension and update
		// the data payload so the template can find the file
		// under its renamed name.
		ext, err := detectImageFormat(source)
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
		destName := base + ext
		dest := filepath.Join(imagesDir, destName)
		if err := copyFile(source, dest); err != nil {
			return fmt.Errorf("copy image %q: %w", source, err)
		}
		// Mutate the data so the template can find the renamed
		// file. Only the staged file_name matters; the original
		// `file_path` / `resolved_path` are not used by the
		// template at render time.
		switch v := img.(type) {
		case models.Image:
			v.FileName = destName
			// The image is a value in the caller's slice; the
			// caller reads `soldier.Images` after this returns,
			// so we need to persist the rename into the slice.
			if soldier, ok := data["soldier"].(models.Soldier); ok {
				for i := range soldier.Images {
					if soldier.Images[i].ResolvedPath == source ||
						soldier.Images[i].FilePath == source {
						soldier.Images[i].FileName = destName
					}
				}
				data["soldier"] = soldier
			}
		case map[string]any:
			v["file_name"] = destName
		}
	}
	return nil
}

// detectImageFormat reads the first 4 bytes of the file and
// returns the canonical file extension (".jpg" for JPEG, ".png"
// for PNG) based on the magic bytes. Returns an error if the
// file cannot be read or the format is not recognized. The
// extension is what the typst image decoder uses to pick its
// decoder, so this lets us route around files that are named
// with the wrong extension on disk (a real data issue in the
// DixieData image library).
func detectImageFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var head [4]byte
	n, err := f.Read(head[:])
	if err != nil || n < 2 {
		return "", fmt.Errorf("read %d bytes: %v", n, err)
	}
	switch {
	case head[0] == 0xFF && head[1] == 0xD8:
		// JPEG magic (FF D8). Either JFIF (FF D8 FF E0) or
		// Exif (FF D8 FF E1) are both JPEG.
		return ".jpg", nil
	case head[0] == 0x89 && head[1] == 0x50 && n >= 4 && head[2] == 0x4E && head[3] == 0x47:
		// PNG magic (89 50 4E 47).
		return ".png", nil
	}
	return "", fmt.Errorf("unrecognized image format in %q", path)
}

// extractImages pulls the Images field off the soldier record
// regardless of whether the encoder typed it as []models.Image or as
// []map[string]any (which happens when JSON has round-tripped through
// map[string]any). Returns nil if no images were found.
func extractImages(soldier any) []any {
	switch v := soldier.(type) {
	case models.Soldier:
		if len(v.Images) == 0 {
			return nil
		}
		out := make([]any, len(v.Images))
		for i := range v.Images {
			out[i] = v.Images[i]
		}
		return out
	case map[string]any:
		raw, ok := v["images"]
		if !ok {
			return nil
		}
		switch list := raw.(type) {
		case []any:
			return list
		case []map[string]any:
			out := make([]any, len(list))
			for i := range list {
				out[i] = list[i]
			}
			return out
		}
	}
	return nil
}

// imageSourcePath returns the absolute path of the image file,
// preferring ResolvedPath over FilePath. The two field names are
// tried in order to match the fpdf path's imagePathForPDF helper.
func imageSourcePath(img any) string {
	get := func(key string) string {
		switch v := img.(type) {
		case models.Image:
			switch key {
			case "resolved_path":
				return strings.TrimSpace(v.ResolvedPath)
			case "file_path":
				return strings.TrimSpace(v.FilePath)
			}
		case map[string]any:
			if s, ok := v[key].(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	if p := get("resolved_path"); p != "" {
		return p
	}
	return get("file_path")
}

// copyDir recursively copies a directory tree from src to dst. It
// skips the special "main.typ" filename since the renderer writes
// its own main.typ.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "main.typ" {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// writeJSONFile serializes v as indented JSON to path.
func writeJSONFile(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// openFile returns an io.Reader over the file at path. Retained for
// tests; production Render uses runTypstCompile instead.
func openFile(path string) io.Reader {
	f, err := os.Open(path)
	if err != nil {
		return bytes.NewReader(nil)
	}
	return f
}
