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
	"time"

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
	// Issue #67: capture per-phase timing when TYPST_TIMING is set.
	// Disabled by default to avoid log noise; the bulk bench enables
	// it to characterize where time goes.
	timing := strings.TrimSpace(os.Getenv("TYPST_TIMING")) != ""
	phaseStart := func(name string) {
		if timing {
			fmt.Fprintf(os.Stderr, "[typst-timing] %s start\n", name)
		}
	}
	phaseEnd := func(name string, start time.Time) {
		if timing {
			fmt.Fprintf(os.Stderr, "[typst-timing] %s %dms\n", name, time.Since(start).Milliseconds())
		}
	}

	// Build a temporary working directory. Copy the template and any
	// sibling files it may import (e.g. common/theme.typ). Typst's
	// import paths are resolved relative to the root (which we set
	// to the work directory), so we need the full template tree
	// there.
	mkdirStart := time.Now()
	workDir, err := os.MkdirTemp("", "dixiedata-typst-")
	if err != nil {
		return err
	}
	if keepDir := strings.TrimSpace(os.Getenv("TYPST_KEEP_WORKDIR")); keepDir != "" {
		// Diagnostic mode: move the freshly-created tempdir into
		// keepDir so the caller can inspect data.json, images/,
		// out.pdf, etc. Skip the deferred cleanup.
		if err := os.MkdirAll(keepDir, 0o755); err != nil {
			return fmt.Errorf("TYPST_KEEP_WORKDIR mkdir: %w", err)
		}
		target := filepath.Join(keepDir, filepath.Base(workDir))
		if err := os.Rename(workDir, target); err != nil {
			return fmt.Errorf("TYPST_KEEP_WORKDIR rename: %w", err)
		}
		workDir = target
	} else {
		defer os.RemoveAll(workDir)
	}
	phaseEnd("mkdir", mkdirStart)

	// Copy the template's containing directory contents into workDir
	// so the template's #import statements resolve.
	copyDirStart := time.Now()
	srcDir := filepath.Dir(tpl.Path)
	if err := copyDir(srcDir, workDir); err != nil {
		return fmt.Errorf("copy template dir: %w", err)
	}
	phaseEnd("copy_template_dir", copyDirStart)

	// Also copy the template's name as main.typ so the import
	// statements find it under that name.
	copyMainStart := time.Now()
	mainPath := filepath.Join(workDir, "main.typ")
	if err := copyFile(tpl.Path, mainPath); err != nil {
		return fmt.Errorf("copy template: %w", err)
	}
	phaseEnd("copy_main", copyMainStart)

	// Stage image files referenced by the data payload. The template
	// can reference them as `images/filename`. We look for soldier
	// images on data["soldier"].Images (a []models.Image or similar)
	// and copy each one whose ResolvedPath or FilePath exists.
	stageStart := time.Now()
	if err := stageSoldierImages(workDir, data); err != nil {
		return fmt.Errorf("stage images: %w", err)
	}
	phaseEnd("stage_images", stageStart)

	writeDataStart := time.Now()
	dataPath := filepath.Join(workDir, "data.json")
	if err := writeJSONFile(dataPath, data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	phaseEnd("write_data", writeDataStart)

	// Run typst compile. Output goes to a temp PDF file which we
	// then stream to w.
	compileStart := time.Now()
	outputPath := filepath.Join(workDir, "out.pdf")
	if err := runTypstCompile(t.binPath, workDir, mainPath, outputPath); err != nil {
		return err
	}
	phaseEnd("typst_compile", compileStart)

	streamStart := time.Now()
	f, err := os.Open(outputPath)
	if err != nil {
		return fmt.Errorf("open typst output: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("copy typst output: %w", err)
	}
	phaseEnd("stream_pdf", streamStart)
	if timing {
		_ = phaseStart
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

	// Pin the PDF's CreationDate / ModDate when the caller hasn't
	// already set SOURCE_DATE_EPOCH. Typst honours SOURCE_DATE_EPOCH
	// per the reproducible-builds spec; the result is byte-stable
	// across runs when the env var is fixed. Issue #69 contract
	// tests rely on this.
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if os.Getenv("SOURCE_DATE_EPOCH") == "" {
		cmd.Env = append(cmd.Env, "SOURCE_DATE_EPOCH=1577836800")
	}

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
// payload into <workDir>/images/. The template can then reference
// them as images/filename. We deliberately accept either a
// []models.Image or a []map[string]any so this stays flexible across
// the encoder and any future refactors.
//
// Single-record payloads expose the soldier on data["soldier"];
// bulk payloads expose the sorted array on data["soldiers"]. Both
// shapes stage images into the same <workDir>/images/ directory
// because templates/bulk_soldier.typ references images by the
// renamed file_name which is unique across the archive.
//
// Images whose source file does not exist (e.g. the soldier has no
// image, or the file was moved) are skipped silently — the template
// is expected to handle a missing file path gracefully.
func stageSoldierImages(workDir string, data map[string]any) error {
	if soldier, ok := data["soldier"]; ok {
		return stageOneSoldierImages(workDir, data, soldier)
	}
	if soldiers, ok := data["soldiers"]; ok {
		return stageBulkSoldiersImages(workDir, data, soldiers)
	}
	if groups, ok := data["groups"]; ok {
		return stageBulkSoldiersImages(workDir, data, groups)
	}
	return nil
}

// stageOneSoldierImages stages images for the single-record path.
// The original code re-iterated the soldier's Images slice after
// staging each file to look up the source path and persist the
// rename. We preserve that behaviour here: stageOneImage copies
// the file under the detected-format filename and returns the
// (source, destName) pair so the caller can write the rename back
// into the right slice.
func stageOneSoldierImages(workDir string, data map[string]any, soldier any) error {
	images := extractImages(soldier)
	if len(images) == 0 {
		return nil
	}
	imagesDir := filepath.Join(workDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return err
	}
	for _, img := range images {
		source, destName, err := stageOneImage(imagesDir, img)
		if err != nil {
			return err
		}
		if source == "" {
			continue
		}
		// Persist the rename into the caller-visible soldier so
		// the template lookup matches the staged file name.
		switch v := img.(type) {
		case models.Image:
			if s, ok := data["soldier"].(models.Soldier); ok {
				for i := range s.Images {
					if s.Images[i].ResolvedPath == source ||
						s.Images[i].FilePath == source {
						s.Images[i].FileName = destName
					}
				}
				data["soldier"] = s
			}
			_ = v
		case map[string]any:
			v["file_name"] = destName
		}
	}
	return nil
}

// stageBulkSoldiersImages stages images for the bulk path. The
// payload is a []models.Soldier or []map[string]any. We walk each
// soldier's Images and stage each unique source file. The
// `soldier.Images[i].FileName` mutation happens in place on the
// typed-Soldier case so the JSON serialization carries the renamed
// name to the typst template.
//
// Issue #67: file copies run on a bounded worker pool so a 3000-image
// archive doesn't take 4-5 seconds of serial disk I/O before typst
// compile. Worker count is min(runtime.NumCPU, 8); more than that
// is wasteful for disk-bound work on this hardware.
func stageBulkSoldiersImages(workDir string, data map[string]any, soldiers any) error {
	list := extractSoldiersList(soldiers)
	if len(list) == 0 {
		return nil
	}
	imagesDir := filepath.Join(workDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return err
	}
	// Build a flat work queue: one job per (soldierIndex, image).
	type job struct {
		soldierIdx int
		img        any
	}
	var jobs []job
	for soldierIdx, s := range list {
		images := extractImages(s)
		for _, img := range images {
			jobs = append(jobs, job{soldierIdx: soldierIdx, img: img})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	// Run jobs in parallel; collect (source, destName) results per
	// soldier so the rename-mutation phase can stay sequential and
	// race-free.
	type result struct {
		soldierIdx int
		img        any
		source     string
		destName   string
		err        error
	}
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	if len(jobs) < workers {
		workers = len(jobs)
	}
	jobCh := make(chan job, len(jobs))
	resultCh := make(chan result, len(jobs))
	for w := 0; w < workers; w++ {
		go func() {
			for j := range jobCh {
				source, destName, err := stageOneImage(imagesDir, j.img)
				resultCh <- result{
					soldierIdx: j.soldierIdx,
					img:        j.img,
					source:     source,
					destName:   destName,
					err:        err,
				}
			}
		}()
	}
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)
	var results []result
	for i := 0; i < len(jobs); i++ {
		results = append(results, <-resultCh)
	}
	// Sequential mutation phase: each image only updates its own
	// soldier's slice, so there's no cross-goroutine write hazard
	// but the workers may have finished in any order. Group
	// results by soldier for the rename pass.
	var typedSoldiers []models.Soldier
	if ts, ok := soldiers.([]models.Soldier); ok {
		typedSoldiers = ts
	} else if gs, ok := soldiers.([]GroupKey); ok {
		// Flatten so the mutation loop can index by r.soldierIdx
		// the same way it does for the flat []models.Soldier
		// case.
		for _, g := range gs {
			typedSoldiers = append(typedSoldiers, g.Soldiers...)
		}
	}
	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		if r.source == "" {
			continue
		}
		if r.soldierIdx < len(typedSoldiers) {
			switch v := r.img.(type) {
			case models.Image:
				for i := range typedSoldiers[r.soldierIdx].Images {
					if typedSoldiers[r.soldierIdx].Images[i].ResolvedPath == r.source ||
						typedSoldiers[r.soldierIdx].Images[i].FilePath == r.source {
						typedSoldiers[r.soldierIdx].Images[i].FileName = r.destName
					}
				}
				_ = v
			case map[string]any:
				v["file_name"] = r.destName
			}
		}
	}
	if typedSoldiers != nil {
		if _, ok := soldiers.([]GroupKey); ok {
			// Re-pack the mutated soldiers into the original
			// grouped shape so the rename survives the JSON
			// serialisation.
			gs := soldiers.([]GroupKey)
			idx := 0
			for gi := range gs {
				count := len(gs[gi].Soldiers)
				gs[gi].Soldiers = typedSoldiers[idx : idx+count]
				idx += count
			}
			data["groups"] = gs
		} else {
			data["soldiers"] = typedSoldiers
		}
	}
	return nil
}

// stageOneImage copies a single image file into <imagesDir> under
// its detected-format filename. Returns the (source, destName) pair
// so the caller can write the rename back into the right slice.
// Returning the pair instead of mutating the in-memory image keeps
// the staging step independent of the caller's payload type.
func stageOneImage(imagesDir string, img any) (source string, destName string, err error) {
	source = imageSourcePath(img)
	if source == "" {
		return source, "", nil
	}
	info, statErr := os.Stat(source)
	if statErr != nil || info.IsDir() {
		return source, "", nil
	}
	ext, formatErr := detectImageFormat(source)
	if formatErr != nil {
		return source, "", nil
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	destName = base + ext
	dest := filepath.Join(imagesDir, destName)
	if copyErr := copyFile(source, dest); copyErr != nil {
		return source, "", fmt.Errorf("copy image %q: %w", source, copyErr)
	}
	return source, destName, nil
}

// extractSoldiersList normalises the soldiers payload to a []any so
// the bulk staging loop can iterate uniformly over both
// []models.Soldier and []map[string]any.
func extractSoldiersList(soldiers any) []any {
	switch v := soldiers.(type) {
	case []models.Soldier:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out
	case []any:
		return v
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out
	case []GroupKey:
		// Flatten the grouped bulk payload to all soldiers across
		// groups so the staging loop can walk them uniformly.
		var out []any
		for _, g := range v {
			for i := range g.Soldiers {
				out = append(out, g.Soldiers[i])
			}
		}
		return out
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

// StageImages runs the renderer's image-staging step against
// <workDir>. Used by diagnostic tooling (pkg/exportbridge) to
// validate that the renderer would find every referenced image on
// disk without producing a PDF.
func (t *TypstRenderer) StageImages(workDir string, data map[string]any) error {
	return stageSoldierImages(workDir, data)
}
