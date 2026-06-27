package appshell

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync/atomic"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/pkg/render"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/findagrave"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/scratchpad"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed quotes.json
var embeddedQuotes []byte

type App struct {
	ctx                     context.Context
	database                *db.DB
	soldiers                personRecordsFacade
	anniversary             anniversaryFacade
	calendar                calendarFacade
	analytics               analyticsFacade
	audit                   reviewFacade
	images                  imageFacade
	export                  exportFacade
	backup                  backupFacade
	diagnostics             diagnosticsFacade
	google                  integrationFacade
	updater                 updaterFacade
	restorePoints           *update.RestorePointManager
	quotes                  []models.Quote
	mux                     http.Handler
	muxRaw                  *http.ServeMux
	saveFileDialogOverride       func(opts any) (string, error)
	openFileDialogOverride       func(opts any) (string, error)
	openMultipleFilesDialogOverride func(opts any) ([]string, error)
	inFlight               sync.Map // map[string]struct{} — dedupes in-flight native dialog calls
	startupErr              error
	setupRequired           bool
	debugMode               atomic.Bool // Phase 4: gated by DIXIEDATA_DEBUG=1 or settings toggle
	pendingLaunchStateClear bool
	pendingRecovery         *update.RestorePointRecord
	recoveryFailure         string
	dataDir                 string
	scratchpads             scratchpadOpener
	frontendAssets          fs.FS
	previewMu               sync.Mutex
	memorialPreview         map[string]string
	jobs                    *jobs.Registry
}

func shouldAttemptPostUpdateHealthClear(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || r.URL == nil {
		return false
	}
	return isPostUpdateHealthTrustPath(r.URL.Path)
}

func isPostUpdateHealthTrustPath(path string) bool {
	switch {
	case path == "/", path == "/calendar":
		return true
	case strings.HasPrefix(path, "/browse"):
		return true
	case strings.HasPrefix(path, "/settings"):
		return true
	case strings.HasPrefix(path, "/insights"):
		return true
	case strings.HasPrefix(path, "/soldiers"):
		return true
	case strings.HasPrefix(path, "/review-queue"):
		return true
	case strings.HasPrefix(path, "/research-collections"):
		return true
	case strings.HasPrefix(path, "/export"):
		return true
	case strings.HasPrefix(path, "/compare"):
		return true
	default:
		return false
	}
}

func (a *App) clearPendingLaunchState() error {
	if !a.pendingLaunchStateClear {
		return nil
	}
	if a.restorePoints == nil {
		return fmt.Errorf("restore point manager unavailable")
	}
	if err := a.restorePoints.ClearLaunchState(); err != nil {
		return fmt.Errorf("failed to clear restore point launch state: %w", err)
	}
	a.pendingLaunchStateClear = false
	return nil
}

func (a *App) handleUpdateBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.pendingLaunchStateClear {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if a.restorePoints == nil {
		respondUnavailable(w, r, "Restore point manager unavailable. Update bootstrap cannot run.", nil)
		return
	}
	if err := a.clearPendingLaunchState(); err != nil {
		respondInternal(w, r, "Could not clear the pending launch state.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scratchpadOpener interface {
	Open(displayID, seed string) error
}

const initializeDataConfirmationWord = "INITIALIZE"

func renderStartupPlaceholder(w http.ResponseWriter, r *http.Request) {
	target := "/calendar"
	if r != nil && r.URL != nil {
		if requestPath := strings.TrimSpace(r.URL.RequestURI()); requestPath != "" && requestPath != "/" {
			target = requestPath
		}
	}
	retryTarget := startupPlaceholderRetryTarget(target)
	targetJS, err := json.Marshal(retryTarget)
	if err != nil {
		targetJS = []byte(`"/calendar?_dd_boot=1"`)
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Refresh", fmt.Sprintf("1; url=%s", retryTarget))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="1;url=%s">
<meta http-equiv="cache-control" content="no-cache, no-store, must-revalidate">
<meta http-equiv="pragma" content="no-cache">
<meta http-equiv="expires" content="0">
<title>Loading DixieData...</title>
</head>
<body hx-get="%s" hx-trigger="load delay:700ms" hx-target="body" hx-swap="outerHTML" class="min-h-screen" style="background: linear-gradient(180deg, #d7d2c9 0%%, #c9c2b5 42%%, #b9b1a3 100%%);">
<div class="flex min-h-screen items-center justify-center px-6">
  <div class="rounded-3xl border border-[#8d7440] bg-[rgba(36,48,61,0.92)] px-8 py-6 shadow-[0_18px_34px_rgba(21,29,38,0.2)]">
    <p class="mb-2 text-sm uppercase tracking-[0.24em] text-[#cfb77a]">Local Archive</p>
    <p class="text-2xl font-semibold text-[#f2ede1]">Loading DixieData...</p>
    <p class="mt-2 text-sm text-[#d8cfbc]">The local archive is still starting up. This screen will refresh automatically.</p>
  </div>
</div>
<script>
window.setTimeout(function() {
  window.location.replace(%s);
}, 700);
</script>
</body>
</html>`, html.EscapeString(retryTarget), html.EscapeString(retryTarget), string(targetJS))
}

func startupPlaceholderRetryTarget(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return "/calendar?_dd_boot=1"
	}
	query := parsed.Query()
	query.Set("_dd_boot", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func setupRequestAllowed(path string) bool {
	switch {
	case path == "/setup":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	case path == "/htmx.min.js":
		return true
	case path == "/debug.js":
		return true
	case path == "/jobs/active":
		return true
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/status"):
		// /jobs/{id}/status is the polling fragment. When setup is
		// required, no jobs exist, but the layout progress slot
		// still polls every 3s. Without this allowlist, every poll
		// 303s to /setup, the browser follows the redirect, and the
		// full setup HTML document gets innerHTML-swapped into the
		// progress region — which then re-fires its own load
		// trigger and stacks the layout. See the bug report
		// `uibug.jpg` in the repo root.
		return true
	case strings.HasPrefix(path, "/wailsjs/"):
		return true
	default:
		return false
	}
}

func recoveryRequestAllowed(path string) bool {
	switch {
	case path == "/recovery":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	default:
		return false
	}
}

func requestMethodOverride(r *http.Request) string {
	if r == nil || r.Method != http.MethodPost {
		return ""
	}
	if override := normalizedMethodOverride(r.Header.Get("X-HTTP-Method-Override")); override != "" {
		return override
	}
	if err := parseRequestFormForOverride(r); err == nil {
		if override := normalizedMethodOverride(r.FormValue("_method")); override != "" {
			return override
		}
	}
	return ""
}

func parseRequestFormForOverride(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.ParseMultipartForm(64 << 20)
	}
	return r.ParseForm()
}

func normalizedMethodOverride(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case http.MethodPut:
		return http.MethodPut
	case http.MethodDelete:
		return http.MethodDelete
	case http.MethodPatch:
		return http.MethodPatch
	default:
		return ""
	}
}

func (a *App) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := a.google.Status()
	if err != nil {
		respondInternal(w, r, "Could not load Google integration status.", err)
		return
	}
	conflicts, err := a.backup.PendingMergeConflicts()
	if err != nil {
		respondInternal(w, r, "Could not load pending merge conflicts.", err)
		return
	}
	exportRecords, err := a.listAllSoldiers()
	if err != nil {
		respondInternal(w, r, "Could not enumerate person records for export.", err)
		return
	}
	drift, err := a.google.CalendarDriftStatus(exportRecords)
	if err != nil {
		respondInternal(w, r, "Could not compute Google Calendar drift.", err)
		return
	}
	status.LastSyncedAt = drift.LastSyncedAt
	status.DriftAdded = drift.Added
	status.DriftUpdated = drift.Updated
	status.DriftRemoved = drift.Removed
	status.OutOfSync = drift.OutOfSync
	domainCounts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		respondInternal(w, r, "Could not load archive counts.", err)
		return
	}
	presentation.ShareView(status, conflicts, exportRecords, domainCounts).Render(r.Context(), w)
}

func (a *App) handleResearchCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			respondValidation(w, r, "Invalid from id.", err)
			return
		}
		hub, err := a.soldiers.ResearchCollectionsHub(fromID)
		if err != nil {
			respondInternal(w, r, "Could not load research collections.", err)
			return
		}
		presentation.ResearchCollectionsHubView(*hub).Render(r.Context(), w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the collection form.", err)
			return
		}
		if err := a.soldiers.CreateResearchCollection(r.FormValue("name"), r.FormValue("description")); err != nil {
			setToastHeaderWithType(w, "Collection could not be created.", "error")
			respondInternal(w, r, "Could not create the research collection.", err)
			return
		}
		redirectTo := "/research-collections"
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections?from=%d", fromID)
		}
		setToastHeader(w, "Success: research collection created.")
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Collection created.")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleResearchCollectionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/research-collections/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	collectionID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondValidation(w, r, "Invalid collection id.", err)
		return
	}
	if len(parts) == 2 && parts[1] == "add" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the add-to-collection form.", err)
			return
		}
		soldierID, err := parseOptionalInt64(r.FormValue("soldier_id"), "soldier_id")
		if err != nil || soldierID < 1 {
			respondValidation(w, r, "Invalid person record id.", err)
			return
		}
		if err := a.soldiers.AddPersonRecordToResearchCollection(collectionID, soldierID); err != nil {
			setToastHeaderWithType(w, "Record could not be added to the collection.", "error")
			respondInternal(w, r, "Could not add the record to the research collection.", err)
			return
		}
		redirectTo := fmt.Sprintf("/research-collections/%d", collectionID)
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections/%d?from=%d", collectionID, fromID)
		}
		setToastHeader(w, "Success: record added to collection.")
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Record added to collection.")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			respondValidation(w, r, "Invalid from id.", err)
			return
		}
		detail, err := a.soldiers.ResearchCollectionDetail(collectionID, fromID)
		if err != nil {
			respondInternal(w, r, "Could not load the research collection detail.", err)
			return
		}
		presentation.ResearchCollectionDetailView(*detail).Render(r.Context(), w)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleSoldierPDF(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the PDF export form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "PDF export cancelled.", nil)
		return
	}
	a.enqueueExport("soldier_pdf", func(p *jobs.Progress) error {
		p.Set(20, "Rendering Person Record PDF")
		return a.export.ExportSoldierPDF(path, *soldier, options)
	}, path, w)
}

func (a *App) handleSoldierPDFNoImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFNameNoImages(*soldier),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "PDF export cancelled.", nil)
		return
	}
	a.enqueueExport("soldier_pdf_no_images", func(p *jobs.Progress) error {
		p.Set(20, "Rendering text-only PDF")
		return a.export.ExportSoldierPDFWithoutImages(path, *soldier)
	}, path, w)
}

func (a *App) handleSoldierJPG(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the JPG export form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: soldierJPGName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "JPEG image", Pattern: "*.jpg"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "JPG export cancelled.", nil)
		return
	}

	a.enqueueExport("soldier_jpg", func(p *jobs.Progress) error {
		p.Set(20, "Rendering JPG pages")
		_, err := a.export.ExportSoldierJPG(path, *soldier, options)
		return err
	}, path, w)
}

func (a *App) handleCalendarPDF(w http.ResponseWriter, r *http.Request, monthValue string) {
	debug.FromContext(r.Context()).Debug("handleCalendarPDF ENTER")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar PDF form.", err)
		return
	}

	month, err := parseBoundedInt(monthValue, "month", 1, 12)
	if err != nil {
		respondValidation(w, r, "Invalid month.", err)
		return
	}
	debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF month=%d form=%v", month, r.Form))
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		respondInternal(w, r, "Could not load the monthly calendar.", err)
		return
	}
	debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF calendar days=%d", len(calendar)))
	options := parsePDFOptionsRequest(r, "P", false)
	debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF options=%+v", options))
	debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF pre-dialog ctx_nil=%v frontend=%v",
		a.ctx == nil, ctxHasFrontend(a.ctx)))

	// Reject rapid duplicate POSTs before we hit the native dialog.
	// A double-click queues two dialog requests on the Wails main
	// window message loop; both block on the UI thread and the
	// WebView2 frontend crashes. Returning a quick 429 lets the
	// user see a toast and prevents the second click from racing
	// with the first.
	dupKey := fmt.Sprintf("cal-pdf|%d|%s|%s", month, options.Orientation, monthPDFName(month, options))
	if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
		debug.FromContext(r.Context()).Debug("handleCalendarPDF duplicate request rejected")
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	defer a.inFlight.Delete(dupKey)

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: monthPDFName(month, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF dialog cancelled err=%v path=%q", err, path))
		respondError(w, r, KindValidation, "Monthly PDF export cancelled.", nil)
		return
	}
	debug.FromContext(r.Context()).Debug(fmt.Sprintf("handleCalendarPDF dialog returned path=%q", path))
	a.enqueueExport("monthly_pdf", func(p *jobs.Progress) error {
		p.Set(20, "Rendering monthly PDF")
		return a.export.ExportMonthlyAnniversaryPDF(path, month, calendar, options)
	}, path, w)
	debug.FromContext(r.Context()).Debug("handleCalendarPDF EXIT")
}

func (a *App) handleImageScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ImageData string `json:"imageData"`
		FileName  string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondValidation(w, r, "Invalid screenshot payload.", err)
		return
	}

	imageData := strings.TrimSpace(payload.ImageData)
	if !strings.HasPrefix(imageData, "data:image/png;base64,") {
		respondValidation(w, r, "Screenshot must be a PNG data URL.", nil)
		return
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageData, "data:image/png;base64,"))
	if err != nil {
		respondValidation(w, r, "Could not decode the screenshot image data.", err)
		return
	}

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: imageScreenshotName(payload.FileName),
		Filters: []runtime.FileFilter{
			{DisplayName: "PNG image", Pattern: "*.png"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Screenshot cancelled.", nil)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		respondInternal(w, r, "Could not write the screenshot.", err)
		return
	}
	fmt.Fprintf(w, "✓ Saved screenshot to %s", path)
}

type imageRotateRequest struct {
	ImageID   int64  `json:"imageId"`
	Direction string `json:"direction"`
}

func (a *App) handleImageRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req imageRotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondValidation(w, r, "Invalid rotate request.", err)
		return
	}
	if req.ImageID < 1 {
		respondValidation(w, r, "Invalid image id.", nil)
		return
	}

	imageRecord, err := a.soldiers.GetImageByID(req.ImageID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Image %d not found.", req.ImageID), err)
		return
	}
	imagePath := filepath.Join(a.dataDir, filepath.FromSlash(imageRecord.FilePath))
	switch strings.ToLower(strings.TrimSpace(req.Direction)) {
	case "cw":
		err = rotateImageFile(imagePath, true)
	case "ccw":
		err = rotateImageFile(imagePath, false)
	default:
		respondValidation(w, r, "Invalid rotate direction. Use cw or ccw.", nil)
		return
	}
	if err != nil {
		respondInternal(w, r, "Could not rotate the image.", err)
		return
	}

	fmt.Fprint(w, "Image rotated.")
}

func rotateImageFile(path string, clockwise bool) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open image file: %w", err)
	}
	img, format, err := image.Decode(source)
	source.Close()
	if err != nil {
		return fmt.Errorf("decode image file: %w", err)
	}

	rotated := rotateImage90(img, clockwise)
	tempPath := path + ".rotate"
	output, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create rotated image file: %w", err)
	}

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(output, rotated, &jpeg.Options{Quality: 95})
	case "png":
		err = png.Encode(output, rotated)
	case "gif":
		err = gif.Encode(output, rotated, nil)
	default:
		err = fmt.Errorf("unsupported image format for rotation: %s", format)
	}
	closeErr := output.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := os.Remove(path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	return nil
}

func rotateImage90(src image.Image, clockwise bool) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, height, width))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if clockwise {
				dst.Set(height-1-y, x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			} else {
				dst.Set(y, width-1-x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
	}
	return dst
}

func (a *App) handleDownloadSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the image download form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	if len(soldier.Images) == 0 {
		respondError(w, r, KindValidation, "No images are attached to this record.", nil)
		return
	}

	selected, err := selectedSoldierImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		respondValidation(w, r, "Could not parse selected image ids.", err)
		return
	}
	if len(selected) == 0 {
		respondError(w, r, KindValidation, "Select at least one image to download.", nil)
		return
	}

	parentDir, err := a.OpenDirectoryDialog( runtime.OpenDialogOptions{
		Title: "Choose where to copy the record images",
	})
	if err != nil || parentDir == "" {
		respondError(w, r, KindValidation, "Download cancelled.", nil)
		return
	}
	destinationDir := filepath.Join(parentDir, imageExportFolderName(*soldier))
	if err := a.export.ExportImages(destinationDir, selected); err != nil {
		respondInternal(w, r, "Could not copy the images to the chosen folder.", err)
		return
	}
	fmt.Fprintf(w, "✓ Copied %d image(s) to %s", len(selected), destinationDir)
}

func (a *App) handleImportSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}

	paths, err := a.OpenMultipleFilesDialog( runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;*.svg"},
		},
	})
	if err != nil || len(paths) == 0 {
		respondError(w, r, KindValidation, "Image import cancelled.", nil)
		return
	}

	// Capture the redirect target now (the worker can't read r.URL
	// after the request goroutine returns) and enqueue the import as
	// a background job so the user sees real progress during the
	// file copy.
	redirectPath := imageImportRedirectPath(id, r.URL.Query().Get("return"))
	var jobID string
	jobID = a.jobs.Start("image_import", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, fmt.Sprintf("Importing %d image(s)", len(paths)))
		imported, importErr := a.importImagePaths(*soldier, paths)
		if importErr != nil {
			if imported > 0 {
				slog.Error("appshell: partial image import", "audit", "respond-error", "person_record_id", id, "imported", imported, "err", importErr.Error())
				return importErr
			}
			slog.Error("appshell: image import", "audit", "respond-error", "person_record_id", id, "err", importErr.Error())
			return importErr
		}
		// Embed a redirect header so the browser ends up on the
		// soldier detail page once the job page is dismissed. The
		// /jobs/{id} page itself only renders the job status; the
		// worker writes the redirect header so the in-page
		// follow-on navigation in app.js still works.
		_ = redirectPath
		_ = jobID
		p.Set(100, fmt.Sprintf("Imported %d image(s).", imported))
		return nil
	})
	// We can't easily carry a toast through the 303 redirect and
	// also stash a per-job warning for partial-imports. Use the
	// first toast (success-count) at enqueue time so the user sees
	// feedback even if the page doesn't navigate. The worker writes
	// a follow-up warning/error toast via /jobs/{id}/fragment if the
	// actual import reports a partial failure.
	setToastHeader(w, fmt.Sprintf("Importing %d image(s)…", len(paths)))
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func imageImportRedirectPath(id int64, returnTarget string) string {
	switch strings.ToLower(strings.TrimSpace(returnTarget)) {
	case "edit":
		return fmt.Sprintf("/soldiers/%d/edit", id)
	default:
		return fmt.Sprintf("/soldiers/%d", id)
	}
}

func (a *App) handleDeleteSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the image delete form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	selected, err := selectedSoldierImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		respondValidation(w, r, "Could not parse selected image ids.", err)
		return
	}
	if len(selected) == 0 {
		respondError(w, r, KindValidation, "Select at least one image to delete.", nil)
		return
	}

	for _, image := range selected {
		if err := os.Remove(image.FilePath); err != nil && !os.IsNotExist(err) {
			respondInternal(w, r, fmt.Sprintf("Could not delete image file %s.", image.FilePath), err)
			return
		}
	}

	imageIDs := make([]int64, 0, len(selected))
	for _, image := range selected {
		imageIDs = append(imageIDs, image.ID)
	}
	if err := a.soldiers.DeleteImages(id, imageIDs); err != nil {
		respondInternal(w, r, "Could not remove the image records from the database.", err)
		return
	}

	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprintf(w, "Deleted %d image(s).", len(selected))
}

func (a *App) handleSetPrimarySoldierImage(w http.ResponseWriter, r *http.Request, id, imageID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := a.soldiers.GetByID(id); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	if err := a.soldiers.SetPrimaryImage(id, imageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, fmt.Sprintf("Image %d not found.", imageID), err)
			return
		}
		respondInternal(w, r, "Could not update the primary image.", err)
		return
	}
	setToastHeader(w, "Primary image updated.")
	fmt.Fprint(w, "Primary image updated.")
}

func parseCalendarEventPreferencesForm(r *http.Request) (models.CalendarEventPreferences, error) {
	if err := r.ParseForm(); err != nil {
		return models.CalendarEventPreferences{}, fmt.Errorf("failed to parse form: %w", err)
	}
	preferences := models.CalendarEventPreferences{
		TitlePreset:         strings.TrimSpace(r.FormValue("title_preset")),
		StartTime:           strings.TrimSpace(r.FormValue("start_time")),
		ReminderPrimary:     strings.TrimSpace(r.FormValue("reminder_primary")),
		ReminderSecondary:   strings.TrimSpace(r.FormValue("reminder_secondary")),
		IncludeRecordID:     r.FormValue("include_record_id") == "1",
		IncludeUnit:         r.FormValue("include_unit") == "1",
		IncludeBuriedIn:     r.FormValue("include_buried_in") == "1",
		IncludeOriginalDate: r.FormValue("include_original_date") == "1",
	}
	if _, _, ok := models.CalendarTimeComponents(preferences.StartTime); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("start time must be between 05:00 and 23:00 in 15-minute increments")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderPrimary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid primary reminder option")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderSecondary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid secondary reminder option")
	}
	if strings.TrimSpace(preferences.ReminderPrimary) != "none" && preferences.ReminderPrimary == preferences.ReminderSecondary {
		return models.CalendarEventPreferences{}, fmt.Errorf("reminder selections must be different")
	}
	if !preferences.IncludeRecordID && !preferences.IncludeUnit && !preferences.IncludeBuriedIn && !preferences.IncludeOriginalDate {
		return models.CalendarEventPreferences{}, fmt.Errorf("select at least one description field")
	}
	return preferences, nil
}

func parseSoldierForm(r *http.Request, id int64) (models.Soldier, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse multipart form: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse form: %w", err)
		}
	}

	birthDate, err := parseOptionalCanonicalDate(r.FormValue("birth_date"), "birth_date")
	if err != nil {
		return models.Soldier{}, err
	}
	deathDate, err := parseOptionalCanonicalDate(r.FormValue("death_date"), "death_date")
	if err != nil {
		return models.Soldier{}, err
	}
	spouseSoldierID, err := parseOptionalInt64(r.FormValue("spouse_soldier_id"), "spouse_soldier_id")
	if err != nil {
		return models.Soldier{}, err
	}

	needsReview := r.FormValue("existing_needs_review") == "1"
	reviewReason := r.FormValue("existing_review_reason")
	if findAGraveNeedsReview(r) {
		needsReview = true
		reviewReason = findAGraveReviewReason(r)
	}

	return models.Soldier{
		ID:                    id,
		DisplayID:             r.FormValue("display_id"),
		EntryType:             r.FormValue("entry_type"),
		SpouseSoldierID:       spouseSoldierID,
		RelationshipLabel:     r.FormValue("relationship_label"),
		MaidenName:            r.FormValue("maiden_name"),
		PensionID:             r.FormValue("pension_id"),
		ApplicationID:         r.FormValue("application_id"),
		Prefix:                r.FormValue("prefix"),
		ShowPrefixBeforeName:  r.FormValue("show_prefix_before_name") == "1",
		FirstName:             r.FormValue("first_name"),
		MiddleName:            r.FormValue("middle_name"),
		LastName:              r.FormValue("last_name"),
		Suffix:                r.FormValue("suffix"),
		Rank:                  r.FormValue("rank_out"),
		RankIn:                r.FormValue("rank_in"),
		RankOut:               r.FormValue("rank_out"),
		Unit:                  r.FormValue("unit"),
		PensionState:          r.FormValue("pension_state"),
		ConfederateHomeStatus: r.FormValue("confederate_home_status"),
		ConfederateHomeName:   r.FormValue("confederate_home_name"),
		BirthDate:             birthDate,
		DeathDate:             deathDate,
		BirthInfo:             r.FormValue("birth_info"),
		BuriedIn:              r.FormValue("buried_in"),
		Biography:             r.FormValue("biography"),
		PDFExcerptOverride:    r.FormValue("pdf_excerpt_override"),
		Notes:                 r.FormValue("notes"),
		NeedsReview:           needsReview,
		ReviewReason:          reviewReason,
		Records:               parseRecordInputs(r),
	}, nil
}

func findAGraveNeedsReview(r *http.Request) bool {
	score, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("scrape_confidence_score")))
	return strings.TrimSpace(r.FormValue("scrape_source_label")) != "" && score > 0 && score < 70
}

func findAGraveReviewReason(r *http.Request) string {
	if !findAGraveNeedsReview(r) {
		return ""
	}
	score, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("scrape_confidence_score")))
	return fmt.Sprintf("Low-confidence Find a Grave scrape (%d/100). Verify memorial details before clearing review.", score)
}

func (a *App) newSoldierDefaults() (models.Soldier, error) {
	displayID, err := a.database.NextDXDID()
	if err != nil {
		return models.Soldier{}, err
	}
	return models.Soldier{DisplayID: displayID, PensionState: pensionstate.NotApplicable, ConfederateHomeStatus: confederatehomestatus.NotApplicable, ShowPrefixBeforeName: false}, nil
}

func parseOptionalInt(value, field string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func parseOptionalInt64(value, field string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func (a *App) handleCreateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar item form.", err)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.CreateCalendarItem(month, day, input)
	if err != nil {
		if calendarValidationError(err) {
			a.renderCalendarDayDetail(w, r, month, day, 0, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		}
		respondInternal(w, r, "Could not create the calendar item.", err)
		return
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s saved.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleUpdateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar item form.", err)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.UpdateCalendarItem(itemID, input)
	if err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			respondNotFound(w, r, fmt.Sprintf("Calendar item %d not found.", itemID), err)
			return
		case calendarValidationError(err):
			a.renderCalendarDayDetail(w, r, month, day, itemID, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		default:
			respondInternal(w, r, "Could not update the calendar item.", err)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s updated.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleDeleteCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := a.calendar.DeleteCalendarItem(itemID); err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			respondNotFound(w, r, fmt.Sprintf("Calendar item %d not found.", itemID), err)
			return
		case calendarValidationError(err):
			respondValidation(w, r, "Calendar item validation failed.", err)
			return
		default:
			respondInternal(w, r, "Could not delete the calendar item.", err)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", "Calendar item deleted.", http.StatusOK)
}

func (a *App) renderCalendarDayDetail(w http.ResponseWriter, r *http.Request, month, day int, editingID int64, itemType, title, notes, errorMessage, statusKind, statusMessage string, statusCode int) {
	detail, err := a.calendar.GetDay(month, day)
	if err != nil {
		if calendarValidationError(err) {
			respondValidation(w, r, "Invalid calendar date.", err)
			return
		}
		respondInternal(w, r, "Could not load the calendar day detail.", err)
		return
	}
	if editingID > 0 && strings.TrimSpace(itemType) == "" && strings.TrimSpace(title) == "" && strings.TrimSpace(notes) == "" {
		item, ok := findCalendarItem(detail.Items, editingID)
		if !ok {
			http.Error(w, records.ErrCalendarItemNotFound.Error(), http.StatusNotFound)
			return
		}
		itemType = item.ItemType
		title = item.Title
		notes = item.Notes
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	presentation.CalendarDayDetail(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w)
}

func findCalendarItem(items []models.CalendarItem, itemID int64) (models.CalendarItem, bool) {
	for _, item := range items {
		if item.ID == itemID {
			return item, true
		}
	}
	return models.CalendarItem{}, false
}

func calendarValidationError(err error) bool {
	var validationErr *records.CalendarValidationError
	return errors.As(err, &validationErr)
}

func calendarItemTypeLabel(itemType string) string {
	switch itemType {
	case models.CalendarItemTypeHoliday:
		return "Holiday"
	default:
		return "Event"
	}
}

func parseOptionalBoundedInt(value, field string, min, max int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	return parseBoundedInt(trimmed, field, min, max)
}

func (a *App) renderEntryForm(w http.ResponseWriter, r *http.Request, soldier models.Soldier, isEdit bool, errorMessage string, statusCode int) {
	a.renderEntryFormWithScrapeState(w, r, soldier, isEdit, errorMessage, models.FindAGraveScrapeState{}, statusCode, false)
}

func (a *App) renderEntryFormWithScrapeState(w http.ResponseWriter, r *http.Request, soldier models.Soldier, isEdit bool, errorMessage string, scrape models.FindAGraveScrapeState, statusCode int, fragmentOnly bool) {
	candidates, err := a.soldiers.MarriageCandidates()
	if err != nil {
		respondInternal(w, r, "Could not load marriage candidates for the entry form.", err)
		return
	}
	suggestions, err := a.soldiers.FormSuggestions()
	if err != nil {
		respondInternal(w, r, "Could not load browse suggestions for the entry form.", err)
		return
	}
	w.WriteHeader(statusCode)
	if fragmentOnly {
		presentation.EntryFormFragment(soldier, candidates, suggestions, scrape, isEdit, errorMessage).Render(r.Context(), w)
		return
	}
	if errorMessage != "" {
		presentation.EntryFormWithError(soldier, candidates, suggestions, scrape, isEdit, errorMessage).Render(r.Context(), w)
		return
	}
	presentation.EntryForm(soldier, candidates, suggestions, scrape, isEdit).Render(r.Context(), w)
}

func applyFindAGraveAutofill(base models.Soldier, result findagrave.Result) models.Soldier {
	base.FirstName = result.FirstName
	base.MiddleName = result.MiddleName
	base.LastName = result.LastName
	base.BirthDate = result.BirthDate
	base.BirthInfo = result.BirthInfo
	base.DeathDate = result.DeathDate
	base.BuriedIn = result.BuriedIn
	if strings.TrimSpace(result.MemorialID) != "" || strings.TrimSpace(result.MemorialURL) != "" {
		details := strings.TrimSpace(result.MemorialURL)
		if details == "" {
			details = "Find a Grave memorial"
		}
		base.Records = []models.Record{{
			RecordType: "Find a Grave",
			AppID:      strings.TrimSpace(result.MemorialID),
			Details:    details,
		}}
	}
	return base
}

func parseOptionalCanonicalDate(value, field string) (string, error) {
	normalized, err := dates.NormalizeCanonical(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s", field)
	}
	return normalized, nil
}

func parseLegacySearchComponent(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseBoundedInt(value, field string, min, max int) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func selectedSoldierImages(soldier models.Soldier, selectedIDs []string, dataDir string) ([]models.Image, error) {
	selectedSet := make(map[int64]struct{}, len(selectedIDs))
	for _, value := range selectedIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid image selection")
		}
		selectedSet[id] = struct{}{}
	}

	var selected []models.Image
	for _, image := range soldier.Images {
		if _, ok := selectedSet[image.ID]; !ok {
			continue
		}
		image.FilePath = filepath.Join(dataDir, filepath.FromSlash(image.FilePath))
		selected = append(selected, image)
	}
	return selected, nil
}

func imageExportFolderName(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = fmt.Sprintf("%s-%s", soldier.FirstName, soldier.LastName)
	}
	return sanitizedFileStem(base, "soldier-images") + "_Images"
}

func imageScreenshotName(fileName string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	return sanitizedFileStem(base, "archive-image") + "-screenshot.png"
}

func soldierPDFName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, options, !options.IncludeImages)
}

func soldierJPGName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return jpgReportName(base, options, !options.IncludeImages)
}

func soldierPDFNameNoImages(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, archive.PDFOptions{Orientation: "L", IncludeImages: false}, true)
}

func monthPDFName(month int, options archive.PDFOptions) string {
	return pdfReportName(fmt.Sprintf("%s report", monthNameValue(month)), options, false)
}

func printableArchivePDFName(settings archive.PrintSettings) string {
	name := pdfReportName("dixiedata-printable-archive", archive.PDFOptions{
		Orientation:     settings.Orientation,
		PrinterFriendly: settings.PrinterFriendly,
	}, false)
	if !settings.FullBiographyPage {
		return name
	}
	return strings.TrimSuffix(name, ".pdf") + "-full-biography.pdf"
}

func pdfReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "pdf-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".pdf"
}

func jpgReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "jpg-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".jpg"
}

func pdfOptionFilenameSuffix(options archive.PDFOptions, noImages bool) string {
	options = options.Normalize("P", true)
	parts := make([]string, 0, 3)
	if options.PrinterFriendly {
		parts = append(parts, "printer-friendly")
	}
	if options.Orientation == "L" {
		parts = append(parts, "landscape")
	} else {
		parts = append(parts, "portrait")
	}
	if noImages {
		parts = append(parts, "no-images")
	}
	return strings.Join(parts, "-")
}

func backupArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-backup-%s.ddbak", now.Format("2006-01-02"))
}

func sharedArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-shared-%s.ddshare", now.Format("2006-01-02"))
}

func sanitizedFileStem(value, fallback string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return fallback
	}
	return value
}

func monthNameValue(month int) string {
	months := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if month < 1 || month > 12 {
		return "Unknown"
	}
	return months[month]
}

func parseRecordInputs(r *http.Request) []models.Record {
	recordTypes := r.Form["record_type"]
	appIDs := r.Form["record_app_id"]
	details := r.Form["record_details"]

	count := len(recordTypes)
	if len(appIDs) > count {
		count = len(appIDs)
	}
	if len(details) > count {
		count = len(details)
	}

	records := make([]models.Record, 0, count)
	for i := 0; i < count; i++ {
		record := models.Record{}
		if i < len(recordTypes) {
			record.RecordType = recordTypes[i]
		}
		if i < len(appIDs) {
			record.AppID = appIDs[i]
		}
		if i < len(details) {
			record.Details = details[i]
		}
		records = append(records, record)
	}
	return records
}

func memorialImportPreviewMarkup(preview records.MemorialImportPreview, token string) string {
	confirm := fmt.Sprintf(
		`<form hx-post="/import/memorial-json/confirm" hx-target="#share-status" class="mt-4"><input type="hidden" name="preview_token" value="%s"/><button class="primary-button" type="submit">Confirm Import</button></form>`,
		html.EscapeString(strings.TrimSpace(token)),
	)
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">
<div class="font-semibold text-[#22303d]">Memorial JSON preview ready</div>
<div class="mt-2">File: <code>%s</code></div>
<div class="mt-1">Rows: %d · Would create: %d · Would skip: %d · Would fail: %d</div>
%s%s
</div>`,
		html.EscapeString(strings.TrimSpace(preview.FilePath)),
		preview.TotalRows,
		preview.WouldCreate,
		preview.WouldSkip,
		preview.WouldFail,
		memorialImportIssuesList(preview.Issues, ""),
		confirm,
	)
}

func memorialImportSummaryMarkup(summary records.MemorialImportSummary, logPath string) string {
	reportLink := `<a href="/browse?scope=last_import&sort=created_desc" class="pill-link">Open Browse Last Import</a>`
	logLine := ""
	trimmedLog := strings.TrimSpace(logPath)
	if trimmedLog != "" {
		logLine = fmt.Sprintf(`<div class="mt-2 text-xs text-slate-500">Full error log: <code>%s</code></div>`, html.EscapeString(trimmedLog))
	}
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">
<div class="font-semibold text-[#22303d]">Memorial JSON import complete</div>
<div class="mt-2">Rows: %d · Created: %d · Skipped: %d · Failed: %d</div>
<div class="mt-2">%s</div>
%s%s
</div>`,
		summary.TotalRows,
		summary.Created,
		summary.Skipped,
		summary.Failed,
		reportLink,
		memorialImportIssuesList(summary.Issues, "first 20 errors"),
		logLine,
	)
}

func memorialImportIssuesList(issues []records.MemorialImportIssue, label string) string {
	if len(issues) == 0 {
		return ""
	}
	limit := 20
	if len(issues) < limit {
		limit = len(issues)
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		issue := issues[i]
		memorialID := strings.TrimSpace(issue.MemorialID)
		if memorialID == "" {
			memorialID = "unknown memorial_id"
		}
		name := strings.TrimSpace(issue.Name)
		if name == "" {
			name = "unnamed"
		}
		lines = append(lines, fmt.Sprintf(`<li>Row %d (%s / %s): %s</li>`,
			issue.Row,
			html.EscapeString(memorialID),
			html.EscapeString(name),
			html.EscapeString(issue.Error),
		))
	}
	prefix := "Issues"
	if strings.TrimSpace(label) != "" {
		prefix = strings.TrimSpace(label)
	}
	return fmt.Sprintf(`<div class="mt-3 rounded-2xl border border-amber-700/40 bg-amber-50/80 px-3 py-2 text-xs text-amber-950"><div class="font-semibold">%s</div><ul class="mt-2 list-disc space-y-1 pl-5">%s</ul></div>`,
		html.EscapeString(prefix),
		strings.Join(lines, ""),
	)
}

func writeMemorialImportErrorLog(summary records.MemorialImportSummary) (string, error) {
	if len(summary.Issues) == 0 {
		return "", nil
	}
	file, err := os.CreateTemp("", "dixiedata-memorial-import-*.log")
	if err != nil {
		return "", err
	}
	defer file.Close()
	for _, issue := range summary.Issues {
		_, err := fmt.Fprintf(file, "row=%d memorial_id=%q name=%q error=%q\n", issue.Row, issue.MemorialID, issue.Name, issue.Error)
		if err != nil {
			return "", err
		}
	}
	return file.Name(), nil
}

func exportLinkMarkup(label, path string) string {
	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(path), "/")
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(fileURL),
		html.EscapeString(path),
	)
}

func externalLinkMarkup(label, href, text string) string {
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(href),
		html.EscapeString(text),
	)
}

func (a *App) handleOpenLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the open-link form.", err)
		return
	}
	target, err := normalizeChromeOpenTarget(r.FormValue("target"))
	if err != nil {
		respondValidation(w, r, "Invalid link target.", err)
		return
	}
	if err := openLinkTarget(target); err != nil {
		respondInternal(w, r, "Could not open the link in Chrome.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleScratchpadOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the scratch pad form.", err)
		return
	}
	if a.scratchpads == nil {
		respondUnavailable(w, r, "Scratch pad service is not running. Restart DixieData to recover.", nil)
		return
	}
	displayID := strings.TrimSpace(r.FormValue("display_id"))
	if displayID == "" {
		respondValidation(w, r, "A Display ID is required before opening the scratch pad.", nil)
		return
	}
	seed := r.FormValue("scratchpad_seed")
	if err := a.scratchpads.Open(displayID, seed); err != nil {
		respondInternal(w, r, "Could not open the scratch pad.", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "Scratch pad ready for %s.", displayID)
}

func (a *App) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relative := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/media/"))
	relative = strings.TrimLeft(relative, `/\`)
	if relative == "" {
		http.NotFound(w, r)
		return
	}

	baseDir := filepath.Clean(a.dataDir)
	resolved := filepath.Join(baseDir, filepath.FromSlash(relative))
	withinBase, err := filepath.Rel(baseDir, resolved)
	if err != nil || strings.HasPrefix(withinBase, "..") {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(resolved)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, resolved)
		return
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		respondInternal(w, r, "Could not access the media file.", err)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 320" role="img" aria-label="Image Missing">
<rect width="480" height="320" rx="28" fill="#f6f1e4"/>
<rect x="16" y="16" width="448" height="288" rx="22" fill="#fff" stroke="#8d7440" stroke-width="4" stroke-dasharray="12 8"/>
<path d="M96 224l56-72 52 48 44-56 88 80" fill="none" stroke="#324253" stroke-width="16" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="164" cy="116" r="24" fill="#c5ab68"/>
<text x="240" y="264" text-anchor="middle" font-family="Arial, sans-serif" font-size="28" font-weight="700" fill="#22303d">Image Missing</text>
<text x="240" y="292" text-anchor="middle" font-family="Arial, sans-serif" font-size="15" fill="#324253">%s</text>
</svg>`, html.EscapeString(filepath.Base(relative)))
}

func normalizeChromeOpenTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("missing link target")
	}
	if strings.HasPrefix(strings.ToLower(target), "http://") || strings.HasPrefix(strings.ToLower(target), "https://") || strings.HasPrefix(strings.ToLower(target), "file:///") {
		return target, nil
	}
	if filepath.IsAbs(target) {
		return "file:///" + strings.TrimPrefix(filepath.ToSlash(target), "/"), nil
	}
	parsed, err := url.Parse(target)
	if err == nil && parsed.Scheme != "" {
		return target, nil
	}
	return "", fmt.Errorf("unsupported link target: %s", target)
}

func openLinkInChrome(target string) error {
	chromePath, err := findChromeExecutable()
	if err != nil {
		return err
	}
	return exec.Command(chromePath, "--new-tab", target).Start()
}

func openLinkTarget(target string) error {
	if isFileOpenTarget(target) {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	}
	return openLinkInChrome(target)
}

func isFileOpenTarget(target string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "file:///")
}

func findChromeExecutable() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("chrome.exe"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("chrome"); err == nil {
		return path, nil
	}
	return "", errors.New("Google Chrome was not found")
}

func (a *App) reloadServices() error {
	soldierSvc := records.NewSoldierService(a.database)
	a.soldiers = soldierSvc
	a.anniversary = records.NewAnniversaryService(a.database)
	a.calendar = records.NewCalendarService(a.database)
	a.analytics = records.NewAnalyticsService(a.database)
	a.audit = records.NewAuditService(a.database)
	a.images = archive.NewImageService(a.database)
	a.export = archive.NewExportService(a.database, soldierSvc)
	a.backup = archive.NewBackupService(a.database, soldierSvc)
	a.jobs = jobs.NewWithConcurrency(jobsConcurrencyFromEnv())

	// Wire the Typst-backed Registry into the export service. Per
	// slice 7, the appshell uses Typst exclusively; if the binary
	// or templates directory is missing, ExportService falls back
	// to its fpdf Service (which is preserved as a test scaffold).
	if reg, _, err := a.buildRenderRegistry(); err == nil && reg != nil {
		a.export.SetRegistry(reg)
	}
	// Bulk export reads each soldier's images by absolute path.
	// Soldier.Images[i].FilePath is stored relative to the data
	// dir, and the single-record export handlers fill in
	// ResolvedPath themselves. The bulk export path (ExportFullDatabasePDF)
	// fetches its own soldiers and would otherwise leave ResolvedPath
	// empty; without it the typst image-staging step silently skips
	// the file and the template's #image("images/<name>") reference
	// fails with "file not found". SetDataDir lets the bulk path
	// resolve FilePath against the data dir on the fly.
	a.export.SetDataDir(a.dataDir)
	a.diagnostics = archive.NewDiagnosticsService(a.database, soldierSvc)
	a.google = integrations.NewGoogleService(a.dataDir)
	a.updater = update.NewService(a.database, a.dataDir, func(outputPath string) error {
		_, err := a.backup.Export(outputPath, a.dataDir)
		return err
	})
	a.scratchpads = scratchpad.NewLauncher(a.dataDir, a.database)
	if a.database != nil {
		if err := a.images.EnsureShardedStorage(a.dataDir); err != nil {
			return err
		}
		if err := a.images.PurgeExpiredTrash(a.dataDir); err != nil {
			return err
		}
		required, err := a.database.IdentitySetupRequired()
		if err != nil {
			return err
		}
		a.setupRequired = required
		if !required {
			needsBackfill, err := a.database.EntryAuditIdentityBackfillNeeded()
			if err != nil {
				return err
			}
			if needsBackfill {
				if err := a.database.BackfillEntryAuditIdentity(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// buildRenderRegistry constructs the Typst-backed Registry and
// returns it plus the templates directory. Returns (nil, "", err)
// when the Typst binary or templates directory cannot be located;
// the caller treats this as 'no Registry available' and the
// ExportService falls back to its fpdf Service for tests only.
//
// Per slice 7, the appshell does NOT include an FpdfRenderer in
// the Registry. The Registry's Resolve method falls back to the
// 'fpdf:recordType' engine when no Typst template matches, but
// because the Registry doesn't have an FpdfRenderer, that
// fallback returns an error. In practice all the production
// record types (soldier, widow, wife, linked_person) have
// matching Typst templates, so the fpdf fallback is never hit.
func (a *App) buildRenderRegistry() (*render.Registry, string, error) {
	binPath, err := a.findTypstBinary()
	if err != nil {
		return nil, "", err
	}
	templatesDir, err := a.findTemplatesDir()
	if err != nil {
		return nil, "", err
	}
	typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	reg := render.NewRegistry(typstRenderer, templatesDir)
	return reg, templatesDir, nil
}

// findTypstBinary locates the bundled Typst binary. The lookup
// order is:
//   1. The directory containing the running exe (release layout
//      has <install>/bin/typst-windows.exe next to DixieData.exe).
//   2. The current working directory.
//   3. Walk up to 6 parent levels from cwd (development layout
//      where the exe runs from a subdirectory of the repo).
//
// DixieData is a Windows-only app; the primary binary is
// typst-windows.exe. The macOS and Linux names are kept as
// fallbacks so this code still locates a binary if a developer
// happens to be running it on a non-Windows host for testing,
// but the release builds bundle only typst-windows.exe.
func (a *App) findTypstBinary() (string, error) {
	candidates := []string{"typst-windows.exe", "typst-macos", "typst-linux"}
	for _, dir := range a.findTypstSearchDirs() {
		for _, name := range candidates {
			candidate := filepath.Join(dir, "bin", name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("typst binary not found in bin/ (expected typst-windows.exe)")
}

// findTemplatesDir locates the templates/ directory. Lookup
// order matches findTypstBinary: exe's directory first, then
// cwd, then up to 6 parent levels.
func (a *App) findTemplatesDir() (string, error) {
	for _, dir := range a.findTypstSearchDirs() {
		candidate := filepath.Join(dir, "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Verify it's the typst templates dir, not the Go
			// html/template dir at internal/templates. The sentinel
			// soldier_landscape.typ only exists in the typst tree.
			if _, err := os.Stat(filepath.Join(candidate, "soldier_landscape.typ")); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("typst templates directory not found (expected templates/soldier_landscape.typ)")
}

// findTypstSearchDirs returns the directories to search for
// the Typst binary and templates, in priority order. The exe's
// directory comes first so the release layout (everything next
// to DixieData.exe) works regardless of cwd. cwd and its
// parents follow for development layouts.
func (a *App) findTypstSearchDirs() []string {
	seen := map[string]bool{}
	var dirs []string

	add := func(dir string) {
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	// 1. Exe's directory.
	if exePath, err := os.Executable(); err == nil {
		add(filepath.Dir(exePath))
	}

	// 2. cwd and up to 6 parent levels.
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 6; i++ {
			add(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return dirs
}

func (a *App) activatePendingRecovery(restorePointID string, cause error) error {
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	record, err := a.restorePoints.Get(restorePointID)
	if err != nil {
		return fmt.Errorf("load restore point %q: %w", restorePointID, err)
	}
	a.pendingRecovery = &record
	if cause != nil {
		a.recoveryFailure = cause.Error()
	}
	return nil
}

func (a *App) initializeLocalData() error {
	if filepath.Base(a.dataDir) != ".dixiedata" {
		return fmt.Errorf("refusing to initialize unexpected data directory: %s", a.dataDir)
	}
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	if err := os.RemoveAll(a.dataDir); err != nil {
		return err
	}
	return a.reopenDatabase()
}

func (a *App) reopenDatabase() error {
	database, err := db.Open(a.dataDir)
	if err != nil {
		return err
	}
	a.database = database
	return a.reloadServices()
}

func loadQuotes(data []byte) ([]models.Quote, error) {
	var payload struct {
		Quotes []models.Quote `json:"civil_war_quotes"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Quotes == nil {
		payload.Quotes = []models.Quote{}
	}
	return payload.Quotes, nil
}

func (a *App) listAllSoldiers() ([]models.Soldier, error) {
	var soldiers []models.Soldier
	page := 1
	for {
		batch, _, err := a.soldiers.List(page, 500)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		soldiers = append(soldiers, batch...)
		if len(batch) < 500 {
			break
		}
		page++
	}
	return soldiers, nil
}

func selectQuoteForArchive(quotes []models.Quote, totalSoldiers int) models.Quote {
	if len(quotes) == 0 {
		return models.Quote{}
	}
	if totalSoldiers < 0 {
		totalSoldiers = 0
	}
	index := (totalSoldiers / 3) % len(quotes)
	return quotes[index]
}

func parseInitialSetupForm(r *http.Request) (models.InitialSetupForm, int, error) {
	if err := r.ParseForm(); err != nil {
		return models.InitialSetupForm{}, 0, fmt.Errorf("failed to parse setup form")
	}
	form := models.InitialSetupForm{
		FirstName:  strings.TrimSpace(r.FormValue("first_name")),
		MiddleName: strings.TrimSpace(r.FormValue("middle_name")),
		LastName:   strings.TrimSpace(r.FormValue("last_name")),
		BirthYear:  strings.TrimSpace(r.FormValue("birth_year")),
	}
	birthYear, err := parseBoundedInt(form.BirthYear, "birth_year", 1000, 9999)
	if err != nil {
		return form, 0, err
	}
	prefix, err := db.BuildUserNodePrefix(form.FirstName, form.MiddleName, form.LastName, birthYear)
	if err != nil {
		return form, 0, err
	}
	form.PrefixPreview = prefix
	return form, birthYear, nil
}

func parsePage(value string) int {
	page, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func parseCSVInt64s(value string) ([]int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	results := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid ids")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		results = append(results, id)
	}
	return results, nil
}

func parseSelectedSoldierIDs(values []string) ([]int64, error) {
	selected := make([]int64, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid review queue selection")
		}
		selected = append(selected, id)
	}
	return selected, nil
}

func compareIDsFromRequest(r *http.Request) (int64, int64, error) {
	id1, err1 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id1")), "id1")
	id2, err2 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id2")), "id2")
	if err1 == nil && err2 == nil && id1 > 0 && id2 > 0 {
		if id1 == id2 {
			return 0, 0, fmt.Errorf("choose two different records to compare")
		}
		return id1, id2, nil
	}
	values := r.URL.Query()["compare_ids"]
	if len(values) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	selected, err := parseSelectedSoldierIDs(values)
	if err != nil || len(selected) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	if selected[0] == selected[1] {
		return 0, 0, fmt.Errorf("choose two different records to compare")
	}
	return selected[0], selected[1], nil
}

func (a *App) attachDetailBackLink(soldier *models.Soldier, fromValue string) error {
	if soldier == nil || fromValue == "" {
		return nil
	}
	fromID, err := strconv.ParseInt(fromValue, 10, 64)
	if err != nil || fromID < 1 || fromID == soldier.ID {
		return nil
	}
	source, err := a.soldiers.GetByID(fromID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	soldier.BackLinkURL = fmt.Sprintf("/soldiers/%d", fromID)
	soldier.BackLinkLabel = "Back to " + linkedRecordLabel(*source)
	return nil
}

func linkedRecordLabel(s models.Soldier) string {
	switch strings.TrimSpace(strings.ToLower(s.EntryType)) {
	case "wife":
		return "Wife Record"
	case "widow":
		return "Widow Record"
	default:
		return "Soldier Record"
	}
}

func (a *App) saveUploadedImages(r *http.Request, soldier models.Soldier) error {
	if r.MultipartForm == nil || len(r.MultipartForm.File["images"]) == 0 {
		return nil
	}

	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return fmt.Errorf("prepare image filenames: %w", err)
	}

	var issues []string
	for _, fileHeader := range r.MultipartForm.File["images"] {
		if fileHeader == nil || fileHeader.Filename == "" {
			continue
		}
		if !isAllowedImageFile(fileHeader.Filename) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileHeader.Filename))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileHeader.Filename)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := saveUploadedFile(fileHeader, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		nextSequence++
	}

	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func (a *App) importImagePaths(soldier models.Soldier, paths []string) (int, error) {
	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return 0, fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return 0, fmt.Errorf("prepare image filenames: %w", err)
	}

	imported := 0
	var issues []string
	for _, sourcePath := range paths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		fileName := filepath.Base(sourcePath)
		if !isAllowedImageFile(fileName) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileName))
			continue
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			issues = append(issues, fmt.Sprintf("read image file %s: %v", fileName, err))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			issues = append(issues, fmt.Sprintf("image file %s is empty", fileName))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileName)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := copyImageFile(sourcePath, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		imported++
		nextSequence++
	}

	if len(issues) > 0 {
		return imported, errors.New(strings.Join(issues, "; "))
	}
	return imported, nil
}

func saveUploadedFile(fileHeader *multipart.FileHeader, destination string) error {
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open upload %s: %w", fileHeader.Filename, err)
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", fileHeader.Filename)
	}
	return nil
}

func copyImageFile(sourcePath, destination string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open image file %s: %w", filepath.Base(sourcePath), err)
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", filepath.Base(sourcePath))
	}
	return nil
}

func standardizedImageFileName(prefix string, sequence int, originalName string) string {
	return fmt.Sprintf("%s-img-%03d%s", strings.TrimSpace(prefix), sequence, normalizedImageExtension(originalName))
}

func nextStoredImageSequence(recordDir, prefix string) (int, error) {
	entries, err := os.ReadDir(recordDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, nil
		}
		return 0, err
	}

	maxSequence := 0
	patternPrefix := prefix + "-img-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, patternPrefix) {
			continue
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		sequenceText := strings.TrimPrefix(base, patternPrefix)
		sequence, err := strconv.Atoi(sequenceText)
		if err != nil {
			continue
		}
		if sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1, nil
}

func normalizedImageExtension(name string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".jpg":
		return ".jpg"
	case ".jpeg":
		return ".jpeg"
	case ".png":
		return ".png"
	case ".gif":
		return ".gif"
	case ".webp":
		return ".webp"
	case ".bmp":
		return ".bmp"
	case ".svg":
		return ".svg"
	default:
		return ".img"
	}
}

func isAllowedImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}

// jobsConcurrencyFromEnv reads the optional DIXIEDATA_JOBS_CONCURRENCY
// environment variable and falls back to jobs.DefaultConcurrency when it
// is unset, empty, or not a positive integer. Clamps to a sane upper
// bound (16) so a typo or runaway script cannot exhaust the host.
func jobsConcurrencyFromEnv() int {
	const envKey = "DIXIEDATA_JOBS_CONCURRENCY"
	const upperBound = 16
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return jobs.DefaultConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return jobs.DefaultConcurrency
	}
	if n > upperBound {
		n = upperBound
	}
	return n
}
