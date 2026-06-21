package archive

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/peopleinfo"
	"github.com/valueforvalue/DixieData/pkg/render"
	"github.com/xuri/excelize/v2"
)

const exportBatchSize = 500

type ExportService struct {
	db         *db.DB
	soldier    *SoldierService
	rasterizer pdfToJPEGRasterizer
	// dataDir is the on-disk root the appshell (or any other
	// caller) passes via SetDataDir. Bulk export uses it to
	// resolve each soldier image's relative FilePath into an
	// absolute ResolvedPath before handing the record to the
	// typst renderer. The single-record export paths in the
	// appshell set ResolvedPath themselves; the bulk path
	// would otherwise miss it and the typst image-staging step
	// would silently skip the file.
	dataDir string
	// registry is the Typst-backed renderer. After slice 7, every
	// export goes through the registry; there is no fpdf fallback.
	// The appshell must wire a Registry at startup. If the Typst
	// binary or templates directory is missing, the export methods
	// return an error rather than silently falling back to a
	// different renderer.
	registry *render.Registry
}

type pdfToJPEGRasterizer interface {
	Rasterize(pdfPath, outputDir string) ([]string, error)
}

type ExportMetadata struct {
	AppVersion    string `json:"app_version"`
	SchemaVersion int    `json:"schema_version"`
	Format        string `json:"format"`
	Version       int    `json:"version"`
	GeneratedAt   string `json:"generated_at"`
}

type JSONExportDocument struct {
	Metadata ExportMetadata   `json:"metadata"`
	Soldiers []models.Soldier `json:"soldiers"`
}

func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return &ExportService{
		db:         database,
		soldier:    soldier,
		rasterizer: newPDFJPEGRasterizer(),
	}
}

// SetRegistry wires up the Typst-backed Registry. When set,
// ExportFullDatabasePDF dispatches each record to the Registry
// instead of the fpdf Service directly.
func (e *ExportService) SetRegistry(reg *render.Registry) {
	e.registry = reg
}

// SetDataDir records the on-disk root used to resolve image
// paths during bulk export. The single-record export paths
// (ExportSoldierPDF, ExportSoldierPDFWithoutImages) take a
// fully populated models.Soldier with ResolvedPath already
// filled in by the caller; the bulk export path (ExportFullDatabasePDF)
// fetches its own soldiers and would otherwise leave ResolvedPath
// empty when FilePath is stored as a dataDir-relative path. With
// SetDataDir wired, the bulk path resolves each image's FilePath
// against dataDir before handing the record to the typst
// renderer, so the image-staging step can find the source file.
func (e *ExportService) SetDataDir(dataDir string) {
	e.dataDir = dataDir
}

// errPDFRegistryMissing is returned by every PDF export method
// when the typst-backed Registry has not been wired. After slice
// 7 the appshell must wire a Registry at startup; a missing
// Registry is a configuration error, not a fallback to fpdf.
var errPDFRegistryMissing = errors.New("PDF export requires a render.Registry; the typst binary or templates directory is missing at startup")

// ExportSoldierPDF is a thin facade. Routes through the typst-backed
// Registry. The Registry MUST be wired (see SetRegistry); a missing
// Registry returns an error rather than falling back to the legacy
// fpdf Service, which has been removed.
func (e *ExportService) ExportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	if e.registry == nil {
		return errPDFRegistryMissing
	}
	return e.exportSingleRecordViaRegistry(outputPath, soldier, options, "soldier")
}

// ExportSoldierPDFWithoutImages is a thin facade. See ExportSoldierPDF.
func (e *ExportService) ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error {
	if e.registry == nil {
		return errPDFRegistryMissing
	}
	return e.exportSingleRecordViaRegistry(outputPath, soldier, PDFOptions{}, "soldier")
}

// ExportMonthlyAnniversaryPDF is a thin facade. The Registry
// path uses the 'anniversary' template.
func (e *ExportService) ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier, options PDFOptions) error {
	if e.registry == nil {
		return errPDFRegistryMissing
	}
	return e.exportAnniversaryViaRegistry(outputPath, month, calendar, options)
}

// ExportFullDatabasePDF is a thin facade. Routes through the
// typst-backed Registry for every record; the Registry MUST be
// wired (see SetRegistry). The Registry's Resolve method picks
// the default typst template for the (recordType, orientation)
// tuple when settings.Template is empty, so this method now
// always returns a typst-rendered PDF per record.
func (e *ExportService) ExportFullDatabasePDF(outputPath string, settings PrintSettings) error {
	settings = settings.Normalize()
	if e.registry == nil {
		return errPDFRegistryMissing
	}
	return e.exportFullDatabasePDFViaRegistry(outputPath, settings)
}

// exportFullDatabasePDFViaRegistry writes a bulk export by
// rendering each record through the Registry. The output is one
// PDF per record in a directory under outputPath (rather than a
// single concatenated PDF). The user picked a Typst template so
// the records render as standalone PDFs that can be opened
// individually or zipped for sharing.
//
// The directory is named <outputPath-stem>-record-pdfs/.
func (e *ExportService) exportFullDatabasePDFViaRegistry(outputPath string, settings PrintSettings) error {
	settings = settings.Normalize()
	outDir := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + "-record-pdfs"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var selectedIDs []int64
	if settings.Scope == PrintScopeSelected {
		selectedIDs = settings.SelectedIDs
	}
	soldiers, err := exportDetailedSoldiers(e.soldier, selectedIDs)
	if err != nil {
		return err
	}
	if settings.Scope == PrintScopeFiltered {
		soldiers = render.FilterPrintableSoldiers(soldiers, settings)
	}
	if len(soldiers) == 0 {
		// Write a single empty PDF so the caller knows we ran.
		return writeNoRecordsPDF(outDir, settings)
	}

	render.SortPrintableSoldiers(soldiers, settings)

	if e.dataDir != "" {
		for i := range soldiers {
			for j := range soldiers[i].Images {
				if strings.TrimSpace(soldiers[i].Images[j].ResolvedPath) == "" &&
					strings.TrimSpace(soldiers[i].Images[j].FilePath) != "" {
					soldiers[i].Images[j].ResolvedPath = filepath.Join(
						e.dataDir,
						filepath.FromSlash(soldiers[i].Images[j].FilePath),
					)
				}
			}
		}
	}

	for _, soldier := range soldiers {
		recordType := recordTypeForSoldier(soldier)
		soldierCopy := soldier
		data := map[string]any{
			"soldier":  soldierCopy,
			"options":  render.PDFOptions{Orientation: settings.Orientation, PrinterFriendly: settings.PrinterFriendly, IncludeImages: true, PrintableArchive: true},
			"settings": settings,
			"branding": e.archiveBranding(settings.PrinterFriendly),
		}
		safe := printableArchiveFileName(soldier.DisplayID, settings)
		dst := filepath.Join(outDir, safe)
		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		ctx := context.Background()
		if err := e.registry.Render(ctx, settings, recordType, data, f); err != nil {
			f.Close()
			os.Remove(dst)
			return err
		}
		if err := f.Close(); err != nil {
			os.Remove(dst)
			return err
		}
	}
	return nil
}

// recordTypeForSoldier maps a soldier's entry_type to the
// Registry's recordType argument.
func recordTypeForSoldier(soldier models.Soldier) string {
	switch strings.ToLower(strings.TrimSpace(soldier.EntryType)) {
	case "soldier":
		return "soldier"
	case "widow":
		return "widow"
	case "wife":
		return "wife"
	case "linked_person":
		return "linked_person"
	default:
		return "soldier"
	}
}

// templateForRecordType picks the default Typst template for a
// given record type and orientation.
func templateForRecordType(recordType, orientation string) string {
	short := "landscape"
	if strings.EqualFold(orientation, "P") || strings.EqualFold(orientation, "portrait") {
		short = "portrait"
	}
	switch recordType {
	case "widow":
		return "widow_" + short
	case "wife", "linked_person":
		return "spouse_" + short
	default:
		return "soldier_" + short
	}
}

// exportSingleRecordViaRegistry renders a single soldier record
// via the Registry, using the default template for the
// (recordType, orientation) tuple. Used by ExportSoldierPDF and
// ExportSoldierPDFWithoutImages.
func (e *ExportService) exportSingleRecordViaRegistry(outputPath string, soldier models.Soldier, options PDFOptions, fallbackType string) error {
	recordType := recordTypeForSoldier(soldier)
	if recordType == "" {
		recordType = fallbackType
	}
	settings := PrintSettings{
		Orientation: options.Orientation,
		Template:    templateForRecordType(recordType, options.Orientation),
	}.Normalize()
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	soldierCopy := soldier
	// Normalize options for the typst data payload. The template
	// reads `opts.at("orientation", default: "L")` and a missing
	// default case (when the key exists with an empty value) is
	// treated as not-landscape, which would route a landscape
	// soldier to the portrait layout. Pass the normalized value.
	normalizedOptions := options.Normalize("L", true)
	data := map[string]any{
		"soldier":  soldierCopy,
		"options":  normalizedOptions,
		"settings": settings,
		"branding": e.archiveBranding(options.PrinterFriendly),
	}
	return e.registry.Render(context.Background(), settings, recordType, data, f)
}

// archiveBranding returns the header/footer strings used by the
// typst templates. Mirrors pkg/render/service.go::pdfBranding so
// the typst path produces the same archive title and footer
// text the fpdf path used to produce. If the user identity is
// not configured, returns a zero-value branding and the typst
// template falls back to its built-in defaults.
func (e *ExportService) archiveBranding(printerFriendly bool) map[string]string {
	identity, err := e.db.UserIdentity()
	if err != nil {
		return map[string]string{}
	}
	owner := strings.TrimSpace(identity.BrandingName())
	if owner == "" {
		return map[string]string{}
	}
	branding := map[string]string{
		"archive_title": owner + "'s Civil War Research Archive",
		"footer_text":   "Made with DixieData | Version: " + buildinfo.AppVersion + " | Build: " + buildinfo.BuildIdentity(),
	}
	_ = printerFriendly
	return branding
}

// exportAnniversaryViaRegistry renders the anniversary report
// via the Registry's 'anniversary' template.
func (e *ExportService) exportAnniversaryViaRegistry(outputPath string, month int, calendar map[int][]models.Soldier, options PDFOptions) error {
	settings := PrintSettings{
		Orientation: options.Orientation,
		Template:    "anniversary",
	}.Normalize()
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	normalizedOptions := options.Normalize("P", false)
	data := map[string]any{
		"options":  normalizedOptions,
		"settings": settings,
		"month":    month,
		"calendar": calendar,
		"branding": e.archiveBranding(options.PrinterFriendly),
	}
	return e.registry.Render(context.Background(), settings, "soldier", data, f)
}

// exportAnalyticsViaRegistry renders the analytics summary via
// the Registry's 'analytics_summary' template.
func (e *ExportService) exportAnalyticsViaRegistry(outputPath string, snapshot AnalyticsSnapshot, options PDFOptions) error {
	settings := PrintSettings{
		Orientation: options.Orientation,
		Template:    "analytics_summary",
	}.Normalize()
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	normalizedOptions := options.Normalize("P", false)
	data := map[string]any{
		"options":  normalizedOptions,
		"settings": settings,
		"snapshot": snapshot,
		"branding": e.archiveBranding(options.PrinterFriendly),
	}
	return e.registry.Render(context.Background(), settings, "soldier", data, f)
}

// writeNoRecordsPDF writes a tiny placeholder PDF so callers can
// tell the export ran when no records matched.
func writeNoRecordsPDF(outDir string, settings PrintSettings) error {
	dst := filepath.Join(outDir, "no-records.pdf")
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("%PDF-1.4\n%placeholder\n")
	return err
}

// printableArchiveFileName returns a filesystem-safe filename for
// a record's PDF, given its display ID.
func printableArchiveFileName(displayID string, settings PrintSettings) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, displayID)
	return safe + ".pdf"
}

// ExportAnalyticsSummaryPDF is a thin facade. Routes through
// the Registry's 'analytics_summary' template. The Registry
// must be wired; a missing registry returns an error rather
// than falling back to fpdf (which has been removed).
func (e *ExportService) ExportAnalyticsSummaryPDF(outputPath string, snapshot AnalyticsSnapshot, options PDFOptions) error {
	if e.registry == nil {
		return errPDFRegistryMissing
	}
	return e.exportAnalyticsViaRegistry(outputPath, snapshot, options)
}

// ExportSoldierJPG still needs the temp PDF step, so it lives on
// ExportService. When a Registry is wired, the temp PDF is produced
// through it so the user's "Template engine" selection in the share
// modal is honoured for JPGs too (e.g. Typst soldier_landscape
// instead of the legacy fpdf path).
func (e *ExportService) ExportSoldierJPG(outputPath string, soldier models.Soldier, options PDFOptions) ([]string, error) {
	options = options.Normalize("L", true)
	outputPath = ensureJPGOutputPath(outputPath)

	tempDir, err := os.MkdirTemp(filepath.Dir(outputPath), ".dixiedata-soldier-jpg-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary JPG export directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "record.pdf")
	if e.registry == nil {
		return nil, errPDFRegistryMissing
	}
	if err := e.exportSingleRecordViaRegistry(pdfPath, soldier, options, "soldier"); err != nil {
		return nil, err
	}

	renderedDir := filepath.Join(tempDir, "pages")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporary JPG page directory: %w", err)
	}

	renderedPaths, err := e.rasterizer.Rasterize(pdfPath, renderedDir)
	if err != nil {
		return nil, err
	}
	if len(renderedPaths) == 0 {
		return nil, errors.New("PDF rasterizer did not produce any JPG pages")
	}

	finalPaths := make([]string, len(renderedPaths))
	for i := range renderedPaths {
		finalPaths[i] = jpgPagePath(outputPath, i+1)
	}

	if err := removeExistingJPGArtifacts(outputPath); err != nil {
		return nil, err
	}
	for i, renderedPath := range renderedPaths {
		if err := os.Rename(renderedPath, finalPaths[i]); err != nil {
			return nil, fmt.Errorf("save JPG page %d: %w", i+1, err)
		}
	}
	return finalPaths, nil
}

func ensureJPGOutputPath(outputPath string) string {
	if strings.EqualFold(filepath.Ext(outputPath), ".jpg") {
		return outputPath
	}
	return outputPath + ".jpg"
}

func jpgPagePath(outputPath string, pageNumber int) string {
	outputPath = ensureJPGOutputPath(outputPath)
	if pageNumber <= 1 {
		return outputPath
	}
	ext := filepath.Ext(outputPath)
	stem := strings.TrimSuffix(outputPath, ext)
	return fmt.Sprintf("%s-page-%03d%s", stem, pageNumber, ext)
}

func removeExistingJPGArtifacts(outputPath string) error {
	primaryPath := ensureJPGOutputPath(outputPath)
	if err := os.Remove(primaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing JPG export: %w", err)
	}

	ext := filepath.Ext(primaryPath)
	stem := strings.TrimSuffix(primaryPath, ext)
	matches, err := filepath.Glob(stem + "-page-*" + ext)
	if err != nil {
		return fmt.Errorf("list existing JPG page exports: %w", err)
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing JPG page export %s: %w", match, err)
		}
	}
	return nil
}

// ExportJSON writes a full hierarchical export document with metadata and
// soldiers/records/images, processing records in batches to avoid loading the
// entire dataset into memory at once.
func (e *ExportService) ExportJSON(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	payload := JSONExportDocument{
		Metadata: newExportMetadata("json", buildinfo.JSONExportVersion),
		Soldiers: []models.Soldier{},
	}
	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			enriched, err := e.soldier.GetByID(s.ID)
			if err != nil {
				return err
			}
			payload.Soldiers = append(payload.Soldiers, *enriched)
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	return enc.Encode(payload)
}

func (e *ExportService) ExportExcel(outputPath string) error {
	const (
		archiveSheet  = "Archive Export"
		metadataSheet = "Metadata"
		spouseSheet   = "Linked Relationships"
	)

	soldiers, err := exportDetailedSoldiers(e.soldier, nil)
	if err != nil {
		return err
	}
	sort.Slice(soldiers, func(i, j int) bool {
		leftLast := strings.ToLower(strings.TrimSpace(soldiers[i].LastName))
		rightLast := strings.ToLower(strings.TrimSpace(soldiers[j].LastName))
		if leftLast != rightLast {
			return leftLast < rightLast
		}
		leftName := strings.ToLower(strings.TrimSpace(peopleinfo.SoldierFullName(soldiers[i])))
		rightName := strings.ToLower(strings.TrimSpace(peopleinfo.SoldierFullName(soldiers[j])))
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(strings.TrimSpace(soldiers[i].DisplayID)) < strings.ToLower(strings.TrimSpace(soldiers[j].DisplayID))
	})

	workbook := excelize.NewFile()
	workbook.SetSheetName(workbook.GetSheetName(0), archiveSheet)
	workbook.NewSheet(metadataSheet)
	workbook.NewSheet(spouseSheet)

	headerStyle, textStyle, dateStyle, wrapStyle, err := excelExportStyles(workbook)
	if err != nil {
		return err
	}

	metadata := newExportMetadata("xlsx", buildinfo.XLSXExportVersion)
	spouseIndex := map[int64]models.Soldier{}
	for _, soldier := range soldiers {
		spouseIndex[soldier.ID] = soldier
	}

	metadataHeaders := []string{"app_version", "schema_version", "export_version", "generated_at", "format"}
	metadataValues := []string{
		metadata.AppVersion,
		fmt.Sprintf("%d", metadata.SchemaVersion),
		fmt.Sprintf("%d", metadata.Version),
		metadata.GeneratedAt,
		metadata.Format,
	}
	metadataWidths, err := writeExcelHeaderRow(workbook, metadataSheet, metadataHeaders, headerStyle)
	if err != nil {
		return err
	}
	for index, value := range metadataValues {
		cell, _ := excelize.CoordinatesToCellName(index+1, 2)
		if err := workbook.SetCellValue(metadataSheet, cell, value); err != nil {
			return err
		}
		if err := workbook.SetCellStyle(metadataSheet, cell, cell, textStyle); err != nil {
			return err
		}
		updateExcelColumnWidth(metadataWidths, index, value)
	}
	if err := finalizeExcelSheet(workbook, metadataSheet, metadataWidths, 2); err != nil {
		return err
	}

	archiveHeaders := []string{
		"app_version", "schema_version", "export_version", "generated_at",
		"db_id", "display_id", "entry_type",
		"linked_spouse_db_id", "linked_spouse_display_id", "linked_spouse_name",
		"relationship_label", "maiden_name", "is_generated", "pension_id", "application_id",
		"prefix", "first_name", "middle_name", "last_name", "suffix",
		"rank", "rank_in", "rank_out", "unit", "pension_state",
		"confederate_home_status", "confederate_home_name",
		"birth_date", "death_date", "birth_info", "buried_in", "biography", "notes",
		"added_by", "last_edited_by", "last_edited_fields", "last_edited_at",
		"created_at", "updated_at", "records_count", "images_count",
	}
	archiveWidths, err := writeExcelHeaderRow(workbook, archiveSheet, archiveHeaders, headerStyle)
	if err != nil {
		return err
	}
	for rowIndex, soldier := range soldiers {
		rowNumber := rowIndex + 2
		spouse, spouseLinked := spouseIndex[soldier.SpouseSoldierID]
		linkedSpouseDisplayID := ""
		linkedSpouseName := strings.TrimSpace(soldier.SpouseName)
		if spouseLinked {
			linkedSpouseDisplayID = strings.TrimSpace(spouse.DisplayID)
			if linkedSpouseName == "" {
				linkedSpouseName = strings.TrimSpace(peopleinfo.SoldierFullName(spouse))
			}
		}
		values := []string{
			metadata.AppVersion,
			fmt.Sprintf("%d", metadata.SchemaVersion),
			fmt.Sprintf("%d", metadata.Version),
			metadata.GeneratedAt,
			fmt.Sprintf("%d", soldier.ID),
			soldier.DisplayID,
			soldier.EntryType,
			func() string {
				if soldier.SpouseSoldierID <= 0 {
					return ""
				}
				return fmt.Sprintf("%d", soldier.SpouseSoldierID)
			}(),
			linkedSpouseDisplayID,
			linkedSpouseName,
			soldier.RelationshipLabel,
			soldier.MaidenName,
			fmt.Sprintf("%t", soldier.IsGenerated),
			soldier.PensionID,
			soldier.ApplicationID,
			soldier.Prefix,
			soldier.FirstName,
			soldier.MiddleName,
			soldier.LastName,
			soldier.Suffix,
			soldier.Rank,
			soldier.RankIn,
			soldier.RankOut,
			soldier.Unit,
			soldier.PensionState,
			soldier.ConfederateHomeStatus,
			soldier.ConfederateHomeName,
			soldier.BirthDate,
			soldier.DeathDate,
			soldier.BirthInfo,
			soldier.BuriedIn,
			soldier.Biography,
			soldier.Notes,
			soldier.AddedBy,
			soldier.LastEditedBy,
			soldier.LastEditedFields,
			soldier.LastEditedAt,
			soldier.CreatedAt,
			soldier.UpdatedAt,
			fmt.Sprintf("%d", len(soldier.Records)),
			fmt.Sprintf("%d", len(soldier.Images)),
		}
		for columnIndex, value := range values {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowNumber)
			switch archiveHeaders[columnIndex] {
			case "db_id", "records_count", "images_count", "schema_version", "export_version":
				parsed, parseErr := strconv.Atoi(value)
				if parseErr == nil {
					if err := workbook.SetCellValue(archiveSheet, cell, parsed); err != nil {
						return err
					}
				} else if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
			case "birth_date", "death_date":
				displayValue, err := setExcelHistoricalDateCell(workbook, archiveSheet, cell, value, dateStyle, textStyle)
				if err != nil {
					return err
				}
				updateExcelColumnWidth(archiveWidths, columnIndex, displayValue)
				continue
			case "display_id", "linked_spouse_display_id", "app_version", "generated_at", "entry_type", "linked_spouse_name", "relationship_label", "maiden_name", "pension_id", "application_id", "prefix", "first_name", "middle_name", "last_name", "suffix", "rank", "rank_in", "rank_out", "unit", "pension_state", "confederate_home_status", "confederate_home_name", "birth_info", "buried_in", "biography", "notes", "added_by", "last_edited_by", "last_edited_fields", "last_edited_at", "created_at", "updated_at":
				if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
				if err := workbook.SetCellStyle(archiveSheet, cell, cell, textStyle); err != nil {
					return err
				}
				if archiveHeaders[columnIndex] == "birth_info" || archiveHeaders[columnIndex] == "biography" || archiveHeaders[columnIndex] == "notes" || archiveHeaders[columnIndex] == "last_edited_fields" {
					if err := workbook.SetCellStyle(archiveSheet, cell, cell, wrapStyle); err != nil {
						return err
					}
				}
			case "is_generated":
				if err := workbook.SetCellValue(archiveSheet, cell, soldier.IsGenerated); err != nil {
					return err
				}
			default:
				if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
			}
			updateExcelColumnWidth(archiveWidths, columnIndex, value)
		}
	}
	if err := finalizeExcelSheet(workbook, archiveSheet, archiveWidths, len(soldiers)+1); err != nil {
		return err
	}

	spouseHeaders := []string{
		"record_display_id", "record_name", "record_entry_type",
		"linked_spouse_db_id", "linked_spouse_display_id", "linked_spouse_name", "linked_spouse_entry_type",
		"relationship_label", "maiden_name",
	}
	spouseWidths, err := writeExcelHeaderRow(workbook, spouseSheet, spouseHeaders, headerStyle)
	if err != nil {
		return err
	}
	spouseRow := 2
	for _, soldier := range soldiers {
		if soldier.SpouseSoldierID <= 0 && strings.TrimSpace(soldier.SpouseName) == "" && strings.TrimSpace(soldier.RelationshipLabel) == "" && strings.TrimSpace(soldier.MaidenName) == "" {
			continue
		}
		spouse, spouseLinked := spouseIndex[soldier.SpouseSoldierID]
		rowValues := []string{
			soldier.DisplayID,
			peopleinfo.SoldierDisplayName(soldier),
			peopleinfo.DisplayEntryType(soldier),
			func() string {
				if soldier.SpouseSoldierID <= 0 {
					return ""
				}
				return fmt.Sprintf("%d", soldier.SpouseSoldierID)
			}(),
			func() string {
				if spouseLinked {
					return spouse.DisplayID
				}
				return ""
			}(),
			func() string {
				if strings.TrimSpace(soldier.SpouseName) != "" {
					return soldier.SpouseName
				}
				if spouseLinked {
					return peopleinfo.SoldierFullName(spouse)
				}
				return ""
			}(),
			func() string {
				if spouseLinked {
					return peopleinfo.DisplayEntryType(spouse)
				}
				return ""
			}(),
			soldier.RelationshipLabel,
			soldier.MaidenName,
		}
		for columnIndex, value := range rowValues {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, spouseRow)
			if err := workbook.SetCellValue(spouseSheet, cell, value); err != nil {
				return err
			}
			if err := workbook.SetCellStyle(spouseSheet, cell, cell, textStyle); err != nil {
				return err
			}
			updateExcelColumnWidth(spouseWidths, columnIndex, value)
		}
		spouseRow++
	}
	if err := finalizeExcelSheet(workbook, spouseSheet, spouseWidths, spouseRow-1); err != nil {
		return err
	}

	activeSheet, _ := workbook.GetSheetIndex(archiveSheet)
	workbook.SetActiveSheet(activeSheet)
	return workbook.SaveAs(outputPath)
}

func (e *ExportService) ExportICalendar(outputPath string, preferences models.CalendarEventPreferences) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	soldiers, err := exportSoldiers(e.soldier)
	if err != nil {
		return err
	}

	now := time.Now()
	dtstamp := now.UTC()
	preferences = models.NormalizeCalendarEventPreferences(preferences)
	iCalTimeZone := buildinfo.CalendarTimeZone
	location, err := time.LoadLocation(iCalTimeZone)
	if err != nil {
		location = time.UTC
		iCalTimeZone = "UTC"
	}
	for _, line := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		fmt.Sprintf("PRODID:-//%s v%s//Memorial Anniversaries v%d//EN", buildinfo.AppName, buildinfo.AppVersion, buildinfo.ICalendarExportVersion),
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:DixieData Memorial Anniversaries",
		fmt.Sprintf("X-WR-TIMEZONE:%s", iCalTimeZone),
		fmt.Sprintf("X-DIXIEDATA-APP-VERSION:%s", buildinfo.AppVersion),
		fmt.Sprintf("X-DIXIEDATA-SCHEMA-VERSION:%d", buildinfo.SchemaVersion),
		fmt.Sprintf("X-DIXIEDATA-EXPORT-VERSION:%d", buildinfo.ICalendarExportVersion),
	} {
		if err := writeICalendarLine(f, line); err != nil {
			return err
		}
	}

	for _, soldier := range soldiers {
		if soldier.DeathMonth < 1 || soldier.DeathDay < 1 {
			continue
		}
		start := nextGoogleAnniversaryDate(soldier, now.In(location))
		hour, minute, ok := models.CalendarTimeComponents(preferences.StartTime)
		if !ok {
			hour, minute = 9, 0
		}
		start = time.Date(start.Year(), start.Month(), start.Day(), hour, minute, 0, 0, location)
		end := start.Add(time.Hour)
		description := strings.Join(compactICalendarDescriptionLines(iCalendarManagedDescriptionLines(soldier, preferences)...), "\n")
		alarmLines := iCalendarAlarmLines(peopleinfo.SoldierDisplayName(soldier), preferences)

		lines := []string{
			"BEGIN:VEVENT",
			fmt.Sprintf("UID:%s", icalText("dixiedata-"+strings.ToLower(soldier.DisplayID)+"@dixiedata.local")),
			fmt.Sprintf("DTSTAMP:%s", dtstamp.Format("20060102T150405Z")),
			fmt.Sprintf("SUMMARY:%s", icalText(iCalendarManagedSummary(soldier, preferences))),
			fmt.Sprintf("DESCRIPTION:%s", icalText(description)),
			fmt.Sprintf("DTSTART;TZID=%s:%s", iCalTimeZone, start.Format("20060102T150405")),
			fmt.Sprintf("DTEND;TZID=%s:%s", iCalTimeZone, end.Format("20060102T150405")),
			"RRULE:FREQ=YEARLY",
			"STATUS:CONFIRMED",
			"TRANSP:TRANSPARENT",
		}
		lines = append(lines, alarmLines...)
		lines = append(lines, "END:VEVENT")
		for _, line := range lines {
			if err := writeICalendarLine(f, line); err != nil {
				return err
			}
		}
	}

	return writeICalendarLine(f, "END:VCALENDAR")
}

// ExportCSV streams a flat CSV export of all soldiers, processing records in
// batches to avoid loading the entire dataset into memory at once.
func (e *ExportService) ExportCSV(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	metadata := newExportMetadata("csv", buildinfo.CSVExportVersion)
	header := []string{"app_version", "schema_version", "export_version", "generated_at", "id", "display_id", "entry_type", "spouse_soldier_id", "relationship_label", "maiden_name", "is_generated", "pension_id", "application_id", "prefix", "first_name", "middle_name", "last_name", "suffix", "rank", "rank_in", "rank_out", "unit", "pension_state", "confederate_home_status", "confederate_home_name", "birth_date", "death_date", "birth_info", "buried_in", "biography", "notes", "added_by", "last_edited_by", "last_edited_fields", "last_edited_at", "created_at", "updated_at"}
	if err := w.Write(header); err != nil {
		return err
	}

	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			row := []string{
				metadata.AppVersion,
				fmt.Sprintf("%d", metadata.SchemaVersion),
				fmt.Sprintf("%d", metadata.Version),
				metadata.GeneratedAt,
				fmt.Sprintf("%d", s.ID),
				s.DisplayID,
				s.EntryType,
				fmt.Sprintf("%d", s.SpouseSoldierID),
				s.RelationshipLabel,
				s.MaidenName,
				fmt.Sprintf("%v", s.IsGenerated),
				s.PensionID,
				s.ApplicationID,
				s.Prefix,
				s.FirstName,
				s.MiddleName,
				s.LastName,
				s.Suffix,
				s.Rank,
				s.RankIn,
				s.RankOut,
				s.Unit,
				s.PensionState,
				s.ConfederateHomeStatus,
				s.ConfederateHomeName,
				s.BirthDate,
				s.DeathDate,
				s.BirthInfo,
				s.BuriedIn,
				s.Biography,
				s.Notes,
				s.AddedBy,
				s.LastEditedBy,
				s.LastEditedFields,
				s.LastEditedAt,
				s.CreatedAt,
				s.UpdatedAt,
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return nil
}

func excelExportStyles(workbook *excelize.File) (int, int, int, int, error) {
	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "22303D"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F2EDE1"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "bottom", Color: "8D7440", Style: 1},
		},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{
		NumFmt:    49,
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	dateStyle, err := workbook.NewStyle(&excelize.Style{
		CustomNumFmt: excelStringPtr("mm/dd/yyyy"),
		Alignment:    &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	wrapStyle, err := workbook.NewStyle(&excelize.Style{
		NumFmt: 49,
		Alignment: &excelize.Alignment{
			Vertical: "top",
			WrapText: true,
		},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return headerStyle, textStyle, dateStyle, wrapStyle, nil
}

func writeExcelHeaderRow(workbook *excelize.File, sheet string, headers []string, headerStyle int) ([]float64, error) {
	widths := make([]float64, len(headers))
	for index, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := workbook.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		if err := workbook.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return nil, err
		}
		updateExcelColumnWidth(widths, index, header)
	}
	return widths, nil
}

func finalizeExcelSheet(workbook *excelize.File, sheet string, widths []float64, rowCount int) error {
	if rowCount > 0 && len(widths) > 0 {
		lastColumn, _ := excelize.ColumnNumberToName(len(widths))
		if err := workbook.AutoFilter(sheet, fmt.Sprintf("A1:%s%d", lastColumn, rowCount), []excelize.AutoFilterOptions{}); err != nil {
			return err
		}
	}
	for index, width := range widths {
		column, _ := excelize.ColumnNumberToName(index + 1)
		if err := workbook.SetColWidth(sheet, column, column, width); err != nil {
			return err
		}
	}
	return workbook.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})
}

func updateExcelColumnWidth(widths []float64, index int, value string) {
	if index < 0 || index >= len(widths) {
		return
	}
	estimated := float64(len(strings.TrimSpace(value)))*1.15 + 2
	if estimated < 10 {
		estimated = 10
	}
	if estimated > 64 {
		estimated = 64
	}
	if estimated > widths[index] {
		widths[index] = estimated
	}
}

func setExcelHistoricalDateCell(workbook *excelize.File, sheet, cell, canonical string, dateStyle, textStyle int) (string, error) {
	if dateValue, ok := excelDateValue(canonical); ok {
		if err := workbook.SetCellValue(sheet, cell, dateValue); err != nil {
			return "", err
		}
		if err := workbook.SetCellStyle(sheet, cell, cell, dateStyle); err != nil {
			return "", err
		}
		return dateValue.Format("01/02/2006"), nil
	}
	if err := workbook.SetCellValue(sheet, cell, canonical); err != nil {
		return "", err
	}
	if err := workbook.SetCellStyle(sheet, cell, cell, textStyle); err != nil {
		return "", err
	}
	return canonical, nil
}

func excelDateValue(canonical string) (time.Time, bool) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(canonical))
	if err != nil {
		return time.Time{}, false
	}
	if partial.Year <= 0 || partial.Month <= 0 || partial.Day <= 0 {
		return time.Time{}, false
	}
	return time.Date(partial.Year, time.Month(partial.Month), partial.Day, 0, 0, 0, 0, time.UTC), true
}

func compactICalendarDescriptionLines(lines ...string) []string {
	compacted := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			compacted = append(compacted, trimmed)
		}
	}
	return compacted
}

func iCalendarManagedSummary(soldier models.Soldier, preferences models.CalendarEventPreferences) string {
	name := peopleinfo.SoldierDisplayName(soldier)
	switch preferences.TitlePreset {
	case models.CalendarEventTitlePresetNameLead:
		return name + " Memorial Anniversary"
	case models.CalendarEventTitlePresetDisplay:
		return strings.TrimSpace(soldier.DisplayID) + " • " + name
	default:
		return "Memorial Anniversary: " + name
	}
}

func iCalendarManagedDescriptionLines(soldier models.Soldier, preferences models.CalendarEventPreferences) []string {
	lines := []string{}
	if preferences.IncludeRecordID {
		lines = append(lines, "Record ID: "+render.EmptyPDFValue(strings.TrimSpace(soldier.DisplayID)))
	}
	if preferences.IncludeUnit {
		lines = append(lines, "Unit: "+render.EmptyPDFValue(strings.TrimSpace(soldier.Unit)))
	}
	if preferences.IncludeBuriedIn {
		lines = append(lines, "Buried In: "+render.EmptyPDFValue(strings.TrimSpace(soldier.BuriedIn)))
	}
	if preferences.IncludeOriginalDate {
		lines = append(lines, "Original Death Date: "+render.EmptyPDFValue(dates.DisplayUnknown(soldier.DeathDate)))
	}
	lines = append(lines, "Generated by DixieData.")
	return lines
}

func iCalendarAlarmLines(displayName string, preferences models.CalendarEventPreferences) []string {
	lines := []string{}
	for _, reminder := range []string{preferences.ReminderPrimary, preferences.ReminderSecondary} {
		minutes, ok := models.CalendarReminderMinutes(reminder)
		if !ok || minutes <= 0 {
			continue
		}
		description := "Upcoming memorial anniversary for " + displayName
		if minutes <= 60 {
			description = "Memorial anniversary in one hour for " + displayName
		}
		lines = append(lines,
			"BEGIN:VALARM",
			fmt.Sprintf("TRIGGER:-%s", iCalendarDurationFromMinutes(minutes)),
			"ACTION:DISPLAY",
			fmt.Sprintf("DESCRIPTION:%s", icalText(description)),
			"END:VALARM",
		)
	}
	return lines
}

func iCalendarDurationFromMinutes(minutes int64) string {
	if minutes%(24*60) == 0 {
		days := minutes / (24 * 60)
		return fmt.Sprintf("P%dD", days)
	}
	if minutes%60 == 0 {
		hours := minutes / 60
		return fmt.Sprintf("PT%dH", hours)
	}
	return fmt.Sprintf("PT%dM", minutes)
}

func excelStringPtr(value string) *string {
	return &value
}

func (e *ExportService) StaticArchiveFileName(now time.Time) (string, error) {
	owner, err := e.staticArchiveOwner()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DixieData_Archive_%s_%s.zip", owner.FileStem, now.Format("2006-01-02")), nil
}

func (e *ExportService) ExportStaticArchive(outputPath, dataDir string) error {
	owner, err := e.staticArchiveOwner()
	if err != nil {
		return err
	}
	records, err := e.staticArchiveRecords()
	if err != nil {
		return err
	}

	exportRoot, err := os.MkdirTemp("", "dixiedata-static-archive-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(exportRoot)

	if err := copyDirectoryContents(filepath.Join(dataDir, "images"), filepath.Join(exportRoot, "images")); err != nil {
		return err
	}

	dataPayload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	dataJS := "window.DIXIE_DATA = " + string(dataPayload) + ";\n"
	if err := os.WriteFile(filepath.Join(exportRoot, "archive_data.js"), []byte(dataJS), 0o644); err != nil {
		return err
	}

	indexHTML, err := renderStaticArchiveIndex(staticArchiveIndexData{
		ArchiveTitle: owner.DisplayName + "'s Civil War Research Archive",
		OwnerShort:   owner.DisplayName,
		Version:      buildinfo.AppVersion,
		Build:        buildinfo.BuildIdentity(),
		GeneratedAt:  time.Now().Format("January 2, 2006"),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(exportRoot, "viewer.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(exportRoot, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}

	return zipDirectory(outputPath, exportRoot)
}

func (e *ExportService) ExportImages(outputPath string, images []models.Image) error {
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return err
	}

	usedNames := map[string]bool{}
	for _, image := range images {
		source, err := os.Open(image.FilePath)
		if err != nil {
			return err
		}

		entryName := image.FileName
		if entryName == "" {
			entryName = filepath.Base(image.FilePath)
		}
		destPath := uniqueCopiedImagePath(outputPath, entryName, usedNames)
		target, err := os.Create(destPath)
		if err != nil {
			source.Close()
			return err
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			source.Close()
			return err
		}
		if err := target.Close(); err != nil {
			source.Close()
			return err
		}
		source.Close()
	}

	return nil
}

func uniqueCopiedImagePath(rootDir, fileName string, usedNames map[string]bool) string {
	base := strings.TrimSpace(fileName)
	if base == "" {
		base = "image"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "image"
	}
	candidate := base
	index := 2
	for {
		fullPath := filepath.Join(rootDir, candidate)
		key := strings.ToLower(candidate)
		if !usedNames[key] {
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				usedNames[key] = true
				return fullPath
			}
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, index, ext)
		index++
	}
}


func newExportMetadata(format string, version int) ExportMetadata {
	return ExportMetadata{
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		Format:        format,
		Version:       version,
		GeneratedAt:   time.Now().Format(time.RFC3339),
	}
}


func icalText(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func nextAnniversaryDate(soldier models.Soldier, now time.Time) time.Time {
	year := now.Year()
	for i := 0; i < 8; i++ {
		candidateYear := year + i
		candidate := time.Date(candidateYear, time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.UTC)
		if candidate.Month() != time.Month(soldier.DeathMonth) || candidate.Day() != soldier.DeathDay {
			continue
		}
		if !candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)) {
			return candidate
		}
	}
	return time.Date(now.Year(), time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.UTC)
}

func writeICalendarLine(w io.Writer, line string) error {
	const maxLineLength = 75
	for len(line) > maxLineLength {
		if _, err := fmt.Fprintf(w, "%s\r\n ", line[:maxLineLength]); err != nil {
			return err
		}
		line = line[maxLineLength:]
	}
	_, err := fmt.Fprintf(w, "%s\r\n", line)
	return err
}

// --- bulk-fetch helpers (kept for non-PDF exports) ---

// exportSoldiers paginates the entire soldier table, returning a slice of
// minimally-populated Soldier rows. Used by ExportJSON, ExportICalendar,
// and the static archive. The PDF bulk export's render package has its
// own copy of this helper, but the non-PDF exports keep using this one
// to avoid a cross-package call for every export.
func exportSoldiers(soldierSvc *SoldierService) ([]models.Soldier, error) {
	var all []models.Soldier
	page := 1
	for {
		batch, _, err := soldierSvc.List(page, exportBatchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].DisplayID) < strings.ToLower(all[j].DisplayID)
	})
	return all, nil
}

// exportDetailedSoldiers returns the fully enriched record for every
// soldier, optionally filtered to a set of selected IDs.
func exportDetailedSoldiers(soldierSvc *SoldierService, selectedIDs []int64) ([]models.Soldier, error) {
	batch, err := exportSoldiers(soldierSvc)
	if err != nil {
		return nil, err
	}
	if len(selectedIDs) > 0 {
		selectedSet := make(map[int64]struct{}, len(selectedIDs))
		for _, id := range selectedIDs {
			selectedSet[id] = struct{}{}
		}
		filtered := make([]models.Soldier, 0, len(selectedIDs))
		for _, item := range batch {
			if _, ok := selectedSet[item.ID]; ok {
				filtered = append(filtered, item)
			}
		}
		batch = filtered
	}
	all := make([]models.Soldier, 0, len(batch))
	for _, item := range batch {
		soldier, err := soldierSvc.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, *soldier)
	}
	return all, nil
}

