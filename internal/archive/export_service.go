package archive

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/xuri/excelize/v2"
)

const exportBatchSize = 500

type ExportService struct {
	db         *db.DB
	soldier    *SoldierService
	rasterizer pdfToJPEGRasterizer
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
		leftName := strings.ToLower(strings.TrimSpace(soldierFullName(soldiers[i])))
		rightName := strings.ToLower(strings.TrimSpace(soldierFullName(soldiers[j])))
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
				linkedSpouseName = strings.TrimSpace(soldierFullName(spouse))
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
			soldierDisplayName(soldier),
			displayEntryType(soldier),
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
					return soldierFullName(spouse)
				}
				return ""
			}(),
			func() string {
				if spouseLinked {
					return displayEntryType(spouse)
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
		alarmLines := iCalendarAlarmLines(soldierDisplayName(soldier), preferences)

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
	name := soldierDisplayName(soldier)
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
		lines = append(lines, "Record ID: "+emptyPDFValue(strings.TrimSpace(soldier.DisplayID)))
	}
	if preferences.IncludeUnit {
		lines = append(lines, "Unit: "+emptyPDFValue(strings.TrimSpace(soldier.Unit)))
	}
	if preferences.IncludeBuriedIn {
		lines = append(lines, "Buried In: "+emptyPDFValue(strings.TrimSpace(soldier.BuriedIn)))
	}
	if preferences.IncludeOriginalDate {
		lines = append(lines, "Original Death Date: "+emptyPDFValue(soldierDeathLine(soldier)))
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

func (e *ExportService) ExportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	return e.exportSoldierPDF(outputPath, soldier, options.Normalize("L", true))
}

func (e *ExportService) ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error {
	return e.exportSoldierPDF(outputPath, soldier, PDFOptions{Orientation: "L", IncludeImages: false})
}

func (e *ExportService) ExportSoldierJPG(outputPath string, soldier models.Soldier, options PDFOptions) ([]string, error) {
	options = options.Normalize("L", true)
	outputPath = ensureJPGOutputPath(outputPath)

	tempDir, err := os.MkdirTemp(filepath.Dir(outputPath), ".dixiedata-soldier-jpg-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary JPG export directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "record.pdf")
	if err := e.exportSoldierPDF(pdfPath, soldier, options); err != nil {
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

func (e *ExportService) exportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	options = options.Normalize("L", true)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Record Card", "soldier-pdf", buildinfo.SoldierPDFExportVersion, "", options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()

	writePDFTitleBlock(pdf, recordPDFTitle(soldier), fmt.Sprintf("%s - %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)))
	writePDFRecordCard(pdf, soldier, options)
	if shouldAppendSingleRecordBiographyPage(soldier, options) {
		writeSingleRecordBiographyPage(pdf, soldier, options.PrinterFriendly)
	}

	return pdf.OutputFileAndClose(outputPath)
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

func (e *ExportService) ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Monthly Anniversary Report", "monthly-pdf", buildinfo.MonthlyPDFExportVersion, pdfFooterMetadata("monthly-pdf", buildinfo.MonthlyPDFExportVersion), options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()

	title := fmt.Sprintf("%s Anniversary Report", monthLabel(month))
	writePDFTitleBlock(pdf, title, "Includes soldier names and database numbers for the selected month.")

	days := make([]int, 0, len(calendar))
	for day := range calendar {
		if day == 0 {
			continue
		}
		days = append(days, day)
	}
	sort.Ints(days)

	if len(days) == 0 {
		writePDFBody(pdf, "No soldiers are recorded for this month.")
		return pdf.OutputFileAndClose(outputPath)
	}

	for _, day := range days {
		soldiers := append([]models.Soldier(nil), calendar[day]...)
		sort.Slice(soldiers, func(i, j int) bool {
			left := strings.ToLower(soldierDisplayName(soldiers[i]))
			right := strings.ToLower(soldierDisplayName(soldiers[j]))
			return left < right
		})

		writePDFSection(pdf, fmt.Sprintf("%s %d", monthLabel(month), day))
		for _, soldier := range soldiers {
			writePDFBullet(pdf, fmt.Sprintf("%s - %s", soldierDisplayName(soldier), soldier.DisplayID))
		}
	}

	return pdf.OutputFileAndClose(outputPath)
}

func (e *ExportService) ExportFullDatabasePDF(outputPath string, settings PrintSettings) error {
	settings = settings.Normalize()
	pdf, err := e.brandedPDFDocument(settings.Orientation, "Printable Archive Registry", "database-pdf", buildinfo.DatabasePDFExportVersion, pdfFooterMetadata("database-pdf", buildinfo.DatabasePDFExportVersion), settings.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()
	writePDFTitleBlock(pdf, "Printable Archive Registry", "Full database export with concise record pages, captioned primary images, and bounded biography excerpts that continue onto additional pages when needed.")

	var selectedIDs []int64
	if settings.Scope == PrintScopeSelected {
		selectedIDs = settings.SelectedIDs
	}
	soldiers, err := exportDetailedSoldiers(e.soldier, selectedIDs)
	if err != nil {
		return err
	}
	if settings.Scope == PrintScopeFiltered {
		soldiers = filterPrintableSoldiers(soldiers, settings)
	}
	if len(soldiers) == 0 {
		writePDFBody(pdf, "No records are currently stored in this archive.")
		return pdf.OutputFileAndClose(outputPath)
	}

	sortPrintableSoldiers(soldiers, settings)
	groupOrder := selectedPrintGroups(settings)
	lastGroupValues := map[string]string{}
	firstRecord := true

	for _, soldier := range soldiers {
		for _, groupChange := range changedPrintGroups(lastGroupValues, soldier, groupOrder, firstRecord) {
			pdf.AddPage()
			writePDFGroupDividerPage(pdf, groupChange.Label, groupChange.Title, groupChange.Level)
		}
		firstRecord = false
		pdf.AddPage()
		writePDFTitleBlock(
			pdf,
			recordPDFTitle(soldier),
			fmt.Sprintf("%s | %s | Captioned primary image + concise biography excerpt", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)),
		)
		writePDFRecordCard(pdf, soldier, PDFOptions{Orientation: settings.Orientation, PrinterFriendly: settings.PrinterFriendly, IncludeImages: true, PrintableArchive: true})
		if settings.FullBiographyPage {
			writePrintableBiographyAppendixPage(pdf, soldier, settings.PrinterFriendly)
		}
	}

	return pdf.OutputFileAndClose(outputPath)
}

func (e *ExportService) ExportAnalyticsSummaryPDF(outputPath string, snapshot AnalyticsSnapshot, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Archive Summary Report", "analytics-pdf", buildinfo.AnalyticsPDFExportVersion, pdfFooterMetadata("analytics-pdf", buildinfo.AnalyticsPDFExportVersion), options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()
	writePDFTitleBlock(pdf, "Archive Summary Report", "High-level archive analytics covering burial density, Confederate Home participation, record types, pension geography, unit representation, and decade trends.")

	writePDFSection(pdf, "Record Types")
	writePDFBullet(pdf, fmt.Sprintf("Soldiers: %d", snapshot.RecordTypes.TotalSoldiers))
	writePDFBullet(pdf, fmt.Sprintf("Spouses (Wives & Widows): %d", snapshot.RecordTypes.TotalWivesWidows))
	writePDFBullet(pdf, fmt.Sprintf("Linked People: %d", snapshot.RecordTypes.TotalLinkedPeople))

	writePDFSection(pdf, "Top Cemeteries")
	writePDFAnalyticsRows(pdf, snapshot.CemeteryDensity, "No burial locations are recorded yet.")

	writePDFSection(pdf, "Confederate Home Participation")
	writePDFBody(pdf, "Status breakdown")
	writePDFAnalyticsRows(pdf, snapshot.ConfederateHomeStatus, "No Confederate Home statuses are recorded yet.")
	pdf.Ln(2)
	writePDFBody(pdf, "Most frequent home names")
	writePDFAnalyticsRows(pdf, snapshot.ConfederateHomeNames, "No Confederate Home names are recorded yet.")

	writePDFSection(pdf, "Pension Distribution")
	writePDFAnalyticsRows(pdf, snapshot.PensionDistribution, "No pension states are recorded yet.")

	writePDFSection(pdf, "Unit Representation")
	writePDFAnalyticsRows(pdf, snapshot.UnitRepresentation, "No units are recorded yet.")

	writePDFSection(pdf, "Chronological Overview")
	writePDFBody(pdf, "Birth decades")
	writePDFAnalyticsRows(pdf, snapshot.BirthDecadeDistribution, "No birth decades are recorded yet.")
	pdf.Ln(2)
	writePDFBody(pdf, "Death decades")
	writePDFAnalyticsRows(pdf, snapshot.DeathDecadeDistribution, "No death decades are recorded yet.")

	return pdf.OutputFileAndClose(outputPath)
}

func newPDFDocument(orientation, title, format string, version int) *fpdf.Fpdf {
	pdf := fpdf.New(orientation, "mm", "Letter", "")
	pdf.SetTitle(title, false)
	pdf.SetAuthor(buildinfo.AppLabel(), false)
	pdf.SetCreator(fmt.Sprintf("%s %s export v%d", buildinfo.AppName, format, version), false)
	pdf.SetSubject(fmt.Sprintf("%s schema v%d", buildinfo.AppName, buildinfo.SchemaVersion), false)
	pdf.SetMargins(16, 28, 16)
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetCompression(false)
	return pdf
}

func (e *ExportService) brandedPDFDocument(orientation, title, format string, version int, footerDetail string, printerFriendly bool) (*fpdf.Fpdf, error) {
	branding, err := e.pdfBranding()
	if err != nil {
		return nil, err
	}
	pdf := newPDFDocument(orientation, title, format, version)
	pdf.SetHeaderFuncMode(func() {
		pageWidth, _ := pdf.GetPageSize()
		leftMargin, _, rightMargin, _ := pdf.GetMargins()
		pdf.SetY(10)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(34, 48, 61)
		pdf.CellFormat(0, 5, sanitizePDFText(branding.ArchiveTitle), "", 1, "L", false, 0, "")
		pdf.SetDrawColor(141, 116, 64)
		pdf.Line(leftMargin, 17, pageWidth-rightMargin, 17)
		pdf.Ln(3)
	}, true)
	if !printerFriendly {
		pdf.SetFooterFunc(func() {
			pageWidth, _ := pdf.GetPageSize()
			leftMargin, _, rightMargin, _ := pdf.GetMargins()
			pdf.SetY(-11)
			pdf.SetDrawColor(141, 116, 64)
			pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())
			pdf.Ln(1)
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(68, 82, 96)
			footerText := sanitizePDFText(branding.FooterText)
			if strings.TrimSpace(footerDetail) != "" {
				footerText = footerText + " | " + sanitizePDFText(footerDetail)
			}
			pdf.CellFormat(0, 4, footerText, "", 0, "C", false, 0, "")
		})
	}
	return pdf, nil
}

func (e *ExportService) pdfBranding() (pdfBranding, error) {
	identity, err := e.db.UserIdentity()
	if err != nil {
		return pdfBranding{}, err
	}
	owner := strings.TrimSpace(identity.BrandingName())
	if owner == "" {
		return pdfBranding{}, fmt.Errorf("user identity is incomplete")
	}
	return pdfBranding{
		ArchiveTitle: owner + "'s Civil War Research Archive",
		FooterText:   "Made with DixieData | Version: " + buildinfo.AppVersion + " | Build: " + buildinfo.BuildIdentity(),
	}, nil
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

func printablePDFMetadataDetails(settings PrintSettings) map[string]string {
	settings = settings.Normalize()
	metadata := map[string]string{
		"Includes Images":     "true",
		"Full Biography Page": fmt.Sprintf("%t", settings.FullBiographyPage),
		"Sort By":             printableSortLabel(settings.SortBy),
		"Group By":            printableGroupSummary(settings),
	}
	switch settings.Scope {
	case PrintScopeSelected:
		metadata["Export Scope"] = fmt.Sprintf("Selected records (%d)", len(settings.SelectedIDs))
	case PrintScopeFiltered:
		metadata["Export Scope"] = printableFilterScopeSummary(settings)
	default:
		metadata["Export Scope"] = "All records"
	}
	metadata["Printer Friendly"] = fmt.Sprintf("%t", settings.PrinterFriendly)
	metadata["Orientation"] = pdfOrientationLabel(settings.Orientation)
	return metadata
}

func printableFilterScopeSummary(settings PrintSettings) string {
	settings = settings.Normalize()
	if !settings.HasFilters() {
		return "All records"
	}
	return fmt.Sprintf("Filtered records (%d active filter family)", activePrintableFilterFamilyCount(settings))
}

func activePrintableFilterFamilyCount(settings PrintSettings) int {
	settings = settings.Normalize()
	count := 0
	for _, values := range [][]string{
		settings.FilterBuriedIn,
		settings.FilterEntryTypes,
		settings.FilterUnits,
		settings.FilterPensionStates,
		settings.FilterConfederateHomeStatus,
	} {
		if len(values) > 0 {
			count++
		}
	}
	return count
}

func printableSortLabel(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case PrintSortBirthYear:
		return "Chronological by Birth Year"
	case PrintSortDeathYear:
		return "Chronological by Death Year"
	default:
		return "Alphabetical by Last Name"
	}
}

func printableGroupSummary(settings PrintSettings) string {
	fields := selectedPrintGroups(settings.Normalize())
	if len(fields) == 0 {
		return "None"
	}
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, printGroupLabel(field))
	}
	return strings.Join(labels, ", ")
}

func selectedPrintGroups(settings PrintSettings) []string {
	fields := []string{}
	if settings.GroupByUnit {
		fields = append(fields, "unit")
	}
	if settings.GroupByPensionState {
		fields = append(fields, "pension_state")
	}
	if settings.GroupByConfederateHomeStatus {
		fields = append(fields, "confederate_home_status")
	}
	if settings.GroupByBuriedIn {
		fields = append(fields, "buried_in")
	}
	return fields
}

func normalizeSelectedPrintIDs(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	normalized := make([]int64, 0, len(values))
	for _, value := range values {
		if value < 1 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized
}

func filterPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) []models.Soldier {
	settings = settings.Normalize()
	if settings.Scope != PrintScopeFiltered || !settings.HasFilters() {
		return soldiers
	}
	filtered := make([]models.Soldier, 0, len(soldiers))
	for _, soldier := range soldiers {
		if matchesPrintableFilters(soldier, settings) {
			filtered = append(filtered, soldier)
		}
	}
	return filtered
}

func matchesPrintableFilters(soldier models.Soldier, settings PrintSettings) bool {
	settings = settings.Normalize()
	return matchesPrintableFilterFamily(settings.FilterBuriedIn, printableBuriedInFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterEntryTypes, printableEntryTypeFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterUnits, printableUnitFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterPensionStates, printablePensionStateFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterConfederateHomeStatus, printableConfederateHomeStatusFilterValue(soldier))
}

func matchesPrintableFilterFamily(selected []string, actual string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, candidate := range selected {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(actual)) {
			return true
		}
	}
	return false
}

func printableBuriedInFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(soldier.BuriedIn)
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printableEntryTypeFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(strings.ToLower(soldier.EntryType))
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printableUnitFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(soldier.Unit)
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printablePensionStateFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(pensionstate.Normalize(soldier.PensionState))
	if omitPDFValue(value) {
		return printFilterUnknownValue
	}
	return value
}

func printableConfederateHomeStatusFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	if omitPDFValue(value) {
		return printFilterUnknownValue
	}
	return value
}

func changedPrintGroups(previous map[string]string, soldier models.Soldier, groupOrder []string, firstRecord bool) []printGroupChange {
	changes := []printGroupChange{}
	startLevel := len(groupOrder)
	if firstRecord {
		startLevel = 0
	} else {
		for index, field := range groupOrder {
			value := printGroupValue(soldier, field)
			if previous[field] != value {
				startLevel = index
				break
			}
		}
	}
	if startLevel >= len(groupOrder) {
		return changes
	}
	for index := startLevel; index < len(groupOrder); index++ {
		field := groupOrder[index]
		value := printGroupValue(soldier, field)
		previous[field] = value
		changes = append(changes, printGroupChange{
			Key:   field,
			Label: printGroupLabel(field),
			Value: value,
			Title: printGroupTitle(field, value),
			Level: index,
		})
	}
	return changes
}

func sortPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) {
	settings = settings.Normalize()
	groupOrder := selectedPrintGroups(settings)
	sort.Slice(soldiers, func(i, j int) bool {
		left := soldiers[i]
		right := soldiers[j]

		for _, field := range groupOrder {
			leftValue := printGroupSortKey(left, field)
			rightValue := printGroupSortKey(right, field)
			if leftValue != rightValue {
				return leftValue < rightValue
			}
		}

		switch settings.SortBy {
		case PrintSortBirthYear:
			leftYear, leftHasYear := printBirthYear(left)
			rightYear, rightHasYear := printBirthYear(right)
			if result, decided := compareOptionalYears(leftYear, leftHasYear, rightYear, rightHasYear); decided {
				return result
			}
			leftDate := strings.TrimSpace(left.BirthDate)
			rightDate := strings.TrimSpace(right.BirthDate)
			if leftDate != rightDate {
				return leftDate < rightDate
			}
		case PrintSortDeathYear:
			leftYear, leftHasYear := printDeathYear(left)
			rightYear, rightHasYear := printDeathYear(right)
			if result, decided := compareOptionalYears(leftYear, leftHasYear, rightYear, rightHasYear); decided {
				return result
			}
			leftDate := strings.TrimSpace(left.DeathDate)
			rightDate := strings.TrimSpace(right.DeathDate)
			if leftDate != rightDate {
				return leftDate < rightDate
			}
		default:
			leftLast := strings.ToLower(strings.TrimSpace(left.LastName))
			rightLast := strings.ToLower(strings.TrimSpace(right.LastName))
			if leftLast != rightLast {
				return leftLast < rightLast
			}
		}

		leftName := strings.ToLower(strings.TrimSpace(soldierFullName(left)))
		rightName := strings.ToLower(strings.TrimSpace(soldierFullName(right)))
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(strings.TrimSpace(left.DisplayID)) < strings.ToLower(strings.TrimSpace(right.DisplayID))
	})
}

func compareOptionalYears(left int, leftOK bool, right int, rightOK bool) (bool, bool) {
	switch {
	case leftOK && rightOK && left != right:
		return left < right, true
	case leftOK != rightOK:
		return leftOK, true
	default:
		return false, false
	}
}

func printBirthYear(soldier models.Soldier) (int, bool) {
	if year := printYearFromCanonical(strings.TrimSpace(soldier.BirthDate)); year > 0 {
		return year, true
	}
	if year := firstFourDigitYear(strings.TrimSpace(soldier.BirthInfo)); year > 0 {
		return year, true
	}
	return 0, false
}

func printDeathYear(soldier models.Soldier) (int, bool) {
	if soldier.DeathYear > 0 {
		return soldier.DeathYear, true
	}
	if year := printYearFromCanonical(strings.TrimSpace(soldier.DeathDate)); year > 0 {
		return year, true
	}
	return 0, false
}

func printYearFromCanonical(value string) int {
	if len(value) < 4 {
		return 0
	}
	year := strings.TrimSpace(value[len(value)-4:])
	if len(year) != 4 {
		return 0
	}
	if year == "0000" {
		return 0
	}
	parsed, err := strconv.Atoi(year)
	if err != nil {
		return 0
	}
	return parsed
}

func firstFourDigitYear(value string) int {
	match := regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`).FindString(value)
	if match == "" {
		return 0
	}
	parsed, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return parsed
}

func printGroupLabel(field string) string {
	switch field {
	case "unit":
		return "Unit"
	case "pension_state":
		return "Pension State"
	case "confederate_home_status":
		return "Confederate Home Status"
	case "buried_in":
		return "Burial Location"
	default:
		return "Group"
	}
}

func printGroupSortKey(soldier models.Soldier, field string) string {
	if field == "buried_in" && strings.TrimSpace(soldier.BuriedIn) == "" {
		return "\uffff"
	}
	return strings.ToLower(printGroupValue(soldier, field))
}

func printGroupValue(soldier models.Soldier, field string) string {
	switch field {
	case "unit":
		return emptyPDFValue(strings.TrimSpace(soldier.Unit))
	case "pension_state":
		return emptyPDFValue(strings.TrimSpace(soldier.PensionState))
	case "confederate_home_status":
		return emptyPDFValue(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	case "buried_in":
		value := strings.TrimSpace(soldier.BuriedIn)
		if value == "" {
			return "Location Unknown"
		}
		return value
	default:
		return "N/A"
	}
}

func printGroupTitle(field, value string) string {
	if field == "buried_in" {
		return "Cemetery: " + value
	}
	return value
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

