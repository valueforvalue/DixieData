// pdf_layout.go holds pure-data PDF/print types and standalone PDF rendering
// helpers. Extracted from export_service.go as PR1 of the God-class reduction
// (issue #42). No exported surface — everything is package-private. The
// *ExportService methods in export_service.go depend on these but do not
// re-export them.
package render

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/peopleinfo"
	"github.com/valueforvalue/DixieData/internal/persondisplay"
)

// pdfURLPattern is the package-level regex used to strip URLs from PDF text.
var pdfURLPattern = regexp.MustCompile(`https?://[^\s<]+`)


// --- PrintSettings struct ---
type PrintSettings struct {
	Scope                        string   `json:"scope"`
	Orientation                  string   `json:"orientation"`
	PrinterFriendly              bool     `json:"printerFriendly"`
	FullBiographyPage            bool     `json:"fullBiographyPage"`
	SortBy                       string   `json:"sortBy"`
	GroupByUnit                  bool     `json:"groupByUnit"`
	GroupByPensionState          bool     `json:"groupByPensionState"`
	GroupByConfederateHomeStatus bool     `json:"groupByConfederateHomeStatus"`
	GroupByBuriedIn              bool     `json:"groupByBuriedIn"`
	FilterBuriedIn               []string `json:"filterBuriedIn"`
	FilterEntryTypes             []string `json:"filterEntryTypes"`
	FilterUnits                  []string `json:"filterUnits"`
	FilterPensionStates          []string `json:"filterPensionStates"`
	FilterConfederateHomeStatus  []string `json:"filterConfederateHomeStatuses"`
	ExportAll                    bool     `json:"exportAll"`
	SelectedIDs                  []int64  `json:"selectedIds"`
	// Template overrides the registry's default template selection. When
	// set, the registry looks up <template>.typ in the templates
	// directory and renders it with Typst. When empty, the registry
	// picks a default based on the record type and orientation.
	Template string `json:"template"`
	// IncludeImages tells the renderer to embed the soldier's
	// primary image. PDFOptions also has this field; PrintSettings
	// carries it so the encode layer (which round-trips through
	// JSON) doesn't drop it before the template sees it.
	IncludeImages bool `json:"includeImages"`
	// PrintableArchive is set when the export is part of the
	// static archive flow rather than a one-off print. Affects
	// layout choice in some templates.
	PrintableArchive bool `json:"printableArchive"`
}


// --- PDFOptions struct ---
type PDFOptions struct {
	Orientation      string `json:"orientation"`
	PrinterFriendly  bool   `json:"printerFriendly"`
	IncludeImages    bool   `json:"includeImages"`
	PrintableArchive bool   `json:"printableArchive"`
	// Template overrides the registry's default template selection.
	// When empty, the registry picks a default by record type and
	// orientation.
	Template string `json:"template"`
}

const (
	PrintSortLastName  = "last_name"
	PrintSortBirthYear = "birth_year"
	PrintSortDeathYear = "death_year"

	PrintScopeAll      = "all"
	PrintScopeFiltered = "filtered"
	PrintScopeSelected = "selected"

	printFilterUnknownValue = "__unknown__"
)


// --- PrintSettings.Normalize ---
func (s PrintSettings) Normalize() PrintSettings {
	options := PDFOptions{
		Orientation:     s.Orientation,
		PrinterFriendly: s.PrinterFriendly,
		IncludeImages:   false,
	}.Normalize("L", false)
	s.Orientation = options.Orientation
	s.PrinterFriendly = options.PrinterFriendly
	s.SortBy = strings.TrimSpace(strings.ToLower(s.SortBy))
	switch s.SortBy {
	case PrintSortBirthYear, PrintSortDeathYear:
	default:
		s.SortBy = PrintSortLastName
	}
	s.Scope = normalizePrintScope(s.Scope, s.ExportAll, s.SelectedIDs)
	s.SelectedIDs = normalizeSelectedPrintIDs(s.SelectedIDs)
	s.FilterBuriedIn = normalizePrintFilterValues(s.FilterBuriedIn)
	s.FilterEntryTypes = normalizePrintFilterValues(s.FilterEntryTypes)
	s.FilterUnits = normalizePrintFilterValues(s.FilterUnits)
	s.FilterPensionStates = normalizePrintFilterValues(s.FilterPensionStates)
	s.FilterConfederateHomeStatus = normalizePrintFilterValues(s.FilterConfederateHomeStatus)
	if s.Scope == PrintScopeFiltered && !s.HasFilters() {
		s.Scope = PrintScopeAll
	}
	s.ExportAll = s.Scope == PrintScopeAll
	return s
}


// --- PrintSettings.HasFilters ---
func (s PrintSettings) HasFilters() bool {
	return len(s.FilterBuriedIn) > 0 ||
		len(s.FilterEntryTypes) > 0 ||
		len(s.FilterUnits) > 0 ||
		len(s.FilterPensionStates) > 0 ||
		len(s.FilterConfederateHomeStatus) > 0
}

func normalizePrintScope(scope string, exportAll bool, selectedIDs []int64) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case PrintScopeFiltered:
		return PrintScopeFiltered
	case PrintScopeSelected:
		return PrintScopeSelected
	case PrintScopeAll:
		return PrintScopeAll
	}
	if !exportAll || len(selectedIDs) > 0 {
		return PrintScopeSelected
	}
	return PrintScopeAll
}

func normalizePrintFilterValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i]) < strings.ToLower(normalized[j])
	})
	return normalized
}


// --- PDFOptions.Normalize ---
func (o PDFOptions) Normalize(defaultOrientation string, _ bool) PDFOptions {
	o.Orientation = strings.TrimSpace(strings.ToUpper(o.Orientation))
	switch o.Orientation {
	case "P", "L":
	default:
		o.Orientation = strings.TrimSpace(strings.ToUpper(defaultOrientation))
		if o.Orientation != "P" && o.Orientation != "L" {
			o.Orientation = "P"
		}
	}
	return o
}


// --- pdfField struct + methods ---
type pdfField struct {
	Label         string
	Value         string
	EmptyValue    string
	ShowWhenEmpty bool
}

func defaultPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "N/A"}
}

func blankPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, ShowWhenEmpty: true}
}

func unknownDatePDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "Unknown", ShowWhenEmpty: true}
}

func naPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "N/A", ShowWhenEmpty: true}
}

func (field pdfField) renderedValue() string {
	trimmed := strings.TrimSpace(field.Value)
	switch strings.ToUpper(trimmed) {
	case "", "NONE", "NA", "N/A", "NOT RECORDED":
		return sanitizePDFText(field.EmptyValue)
	default:
		return sanitizePDFText(trimmed)
	}
}

func (field pdfField) visible() bool {
	trimmed := strings.TrimSpace(field.Value)
	switch strings.ToUpper(trimmed) {
	case "", "NONE", "NA", "N/A", "NOT RECORDED":
		return field.ShowWhenEmpty
	default:
		return true
	}
}


// --- pdfBranding struct ---
type pdfBranding struct {
	ArchiveTitle string
	FooterText   string
}


// --- printGroupChange struct ---
type printGroupChange struct {
	Field string
	Key   string
	Label string
	Value string
	Title string
	Level int
}

// --- PDF rendering helpers ---
func writePDFSection(pdf *fpdf.Fpdf, title string) {
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(0, 8, title, "", 1, "", false, 0, "")
	pdf.SetTextColor(34, 48, 61)
	pdf.SetFont("Helvetica", "", 11)
}

func writePDFTitleBlock(pdf *fpdf.Fpdf, title, subtitle string) {
	pdf.SetFont("Times", "B", 20)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(0, 10, emptyPDFValue(title), "", 1, "", false, 0, "")
	if strings.TrimSpace(subtitle) != "" {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(68, 82, 96)
		pdf.MultiCell(0, 6, sanitizePDFText(strings.TrimSpace(subtitle)), "", "L", false)
	}
	pdf.Ln(1)
}

type pdfRecordCardLayout struct {
	LeftWidthRatio       float64
	SectionTitleFontSize float64
	SectionTitleLine     float64
	FieldLabelFontSize   float64
	FieldValueFontSize   float64
	FieldLineHeight      float64
	FieldRowGap          float64
	ColumnGap            float64
	SectionGap           float64
	ImagePanelHeight     float64
	ImageLabelFontSize   float64
	ImageLabelLine       float64
	BodyFontSize         float64
	BodyLineHeight       float64
	BulletIndent         float64
}

const minReadablePDFRecordCardScale = 0.76

func defaultPDFRecordCardLayout() pdfRecordCardLayout {
	return pdfRecordCardLayout{
		LeftWidthRatio:       0.52,
		SectionTitleFontSize: 9,
		SectionTitleLine:     6,
		FieldLabelFontSize:   8,
		FieldValueFontSize:   9,
		FieldLineHeight:      4.5,
		FieldRowGap:          1,
		ColumnGap:            8,
		SectionGap:           4,
		ImagePanelHeight:     64,
		ImageLabelFontSize:   8,
		ImageLabelLine:       4,
		BodyFontSize:         9,
		BodyLineHeight:       5,
		BulletIndent:         6,
	}
}

func (layout pdfRecordCardLayout) scaled(scale float64) pdfRecordCardLayout {
	return pdfRecordCardLayout{
		LeftWidthRatio:       layout.LeftWidthRatio,
		SectionTitleFontSize: layout.SectionTitleFontSize * scale,
		SectionTitleLine:     maxFloat(4.2, layout.SectionTitleLine*scale),
		FieldLabelFontSize:   layout.FieldLabelFontSize * scale,
		FieldValueFontSize:   layout.FieldValueFontSize * scale,
		FieldLineHeight:      maxFloat(3.4, layout.FieldLineHeight*scale),
		FieldRowGap:          maxFloat(0.4, layout.FieldRowGap*scale),
		ColumnGap:            maxFloat(5, layout.ColumnGap*scale),
		SectionGap:           maxFloat(2, layout.SectionGap*scale),
		ImagePanelHeight:     maxFloat(38, layout.ImagePanelHeight*scale),
		ImageLabelFontSize:   layout.ImageLabelFontSize * scale,
		ImageLabelLine:       maxFloat(3.2, layout.ImageLabelLine*scale),
		BodyFontSize:         layout.BodyFontSize * scale,
		BodyLineHeight:       maxFloat(3.8, layout.BodyLineHeight*scale),
		BulletIndent:         maxFloat(4.5, layout.BulletIndent*scale),
	}
}

func choosePDFRecordCardLayout(pdf *fpdf.Fpdf, soldier models.Soldier, startY float64, options PDFOptions) (pdfRecordCardLayout, bool) {
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	availableHeight := pageHeight - bottomMargin - startY - 18
	base := defaultPDFRecordCardLayout()
	if usesPortraitCompactRecordCardLayout(soldier, options) {
		base.LeftWidthRatio = 0.6
	} else if !options.IncludeImages {
		base.LeftWidthRatio = 0.43
	}
	if usesPortraitRecordPDFLayout(options) && !usesPortraitCompactRecordCardLayout(soldier, options) {
		return base.scaled(minReadablePDFRecordCardScale), false
	}
	for _, scale := range []float64{1, 0.94, 0.88, 0.82, minReadablePDFRecordCardScale} {
		layout := base.scaled(scale)
		if estimatePDFRecordCardHeight(pdf, soldier, options, layout) <= availableHeight {
			return layout, true
		}
	}
	return base.scaled(minReadablePDFRecordCardScale), false
}

func estimatePDFRecordCardHeight(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions, layout pdfRecordCardLayout) float64 {
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	leftWidth := contentWidth * layout.LeftWidthRatio
	rightWidth := contentWidth - leftWidth - layout.ColumnGap
	usesPortraitCompact := usesPortraitCompactRecordCardLayout(soldier, options)

	leftHeight := estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordIdentityFields(soldier), layout)
	leftHeight += layout.SectionGap
	leftHeight += estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordServiceFields(soldier, false), layout)
	if usesPortraitCompact {
		householdHeight := estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordHouseholdFields(soldier, false), layout)
		if householdHeight > 0 {
			leftHeight += layout.SectionGap
			leftHeight += householdHeight
		}
		if len(soldier.Records) > 0 {
			leftHeight += layout.SectionGap
			leftHeight += estimatePDFRecordsSectionHeight(pdf, leftWidth, soldier.Records, layout)
		}
	}

	rightHeight := 0.0
	if options.IncludeImages {
		if imagePath, _ := firstRecordCardImage(soldier, false); imagePath != "" {
			rightHeight += estimatePDFImagePanelHeight(layout, recordPDFImageSectionTitle(options) != "")
		}
	}
	if !usesPortraitCompact {
		householdHeight := estimatePDFCompactFieldSectionHeight(pdf, rightWidth, recordHouseholdFields(soldier, false), layout)
		if householdHeight > 0 {
			if rightHeight > 0 {
				rightHeight += layout.SectionGap
			}
			rightHeight += householdHeight
		}
	}
	_, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		if rightHeight > 0 {
			rightHeight += layout.SectionGap
		}
		rightHeight += estimatePDFRichTextSectionHeight(pdf, rightWidth, narrativeText, layout)
	}
	if !usesPortraitCompact && len(soldier.Records) > 0 {
		if rightHeight > 0 {
			rightHeight += layout.SectionGap
		}
		rightHeight += estimatePDFRecordsSectionHeight(pdf, rightWidth, soldier.Records, layout)
	}

	return maxFloat(leftHeight, rightHeight)
}

func estimatePDFCompactFieldSectionHeight(pdf *fpdf.Fpdf, width float64, fields []pdfField, layout pdfRecordCardLayout) float64 {
	if !hasVisiblePDFField(fields) {
		return 0
	}
	labelWidth := width * 0.32
	if labelWidth < 28 {
		labelWidth = 28
	}
	if labelWidth > 44 {
		labelWidth = 44
	}
	valueWidth := width - labelWidth - 3
	height := layout.SectionTitleLine
	for _, field := range fields {
		if !field.visible() {
			continue
		}
		labelLines := wrappedPDFLineCount(pdf, sanitizePDFText(field.Label), labelWidth, "Helvetica", "B", layout.FieldLabelFontSize)
		valueLines := wrappedPDFLineCount(pdf, field.renderedValue(), valueWidth, "Helvetica", "", layout.FieldValueFontSize)
		height += float64(maxInt(labelLines, valueLines))*layout.FieldLineHeight + layout.FieldRowGap
	}
	return height
}

func estimatePDFRichTextSectionHeight(pdf *fpdf.Fpdf, width float64, text string, layout pdfRecordCardLayout) float64 {
	return layout.SectionTitleLine + float64(wrappedPDFMultilineCount(pdf, emptyPDFValue(text), width, "Helvetica", "", layout.BodyFontSize))*layout.BodyLineHeight*1.18
}

func estimatePDFRecordsSectionHeight(pdf *fpdf.Fpdf, width float64, records []models.Record, layout pdfRecordCardLayout) float64 {
	height := layout.SectionTitleLine
	for _, record := range records {
		line := record.RecordType
		if strings.TrimSpace(record.AppID) != "" {
			line += fmt.Sprintf(" (App: %s)", record.AppID)
		}
		height += float64(wrappedPDFLineCount(pdf, emptyPDFValue(line), width-layout.BulletIndent, "Helvetica", "", layout.BodyFontSize)) * layout.BodyLineHeight
		if strings.TrimSpace(record.Details) != "" {
			height += float64(wrappedPDFMultilineCount(pdf, emptyPDFValue(record.Details), width, "Helvetica", "", layout.BodyFontSize)) * layout.BodyLineHeight * 1.18
		}
	}
	return height
}

func wrappedPDFLineCount(pdf *fpdf.Fpdf, text string, width float64, family, style string, size float64) int {
	pdf.SetFont(family, style, size)
	lines := pdf.SplitText(sanitizePDFText(strings.TrimSpace(text)), width)
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

func wrappedPDFMultilineCount(pdf *fpdf.Fpdf, text string, width float64, family, style string, size float64) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		count += wrappedPDFLineCount(pdf, line, width, family, style, size)
	}
	if count == 0 {
		return 1
	}
	return count
}

func writePDFGroupDividerPage(pdf *fpdf.Fpdf, label, value string, level int) {
	pdf.SetY(maxFloat(pdf.GetY(), 46))
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(0, 7, sanitizePDFText("Grouped by "+label), "", 1, "L", false, 0, "")
	pdf.Ln(float64(level) * 2)
	pdf.SetFont("Times", "B", maxFloat(20, 28-float64(level)*2))
	pdf.SetTextColor(34, 48, 61)
	pdf.MultiCell(0, 11, sanitizePDFText(emptyPDFValue(value)), "", "L", false)
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.MultiCell(0, 6, sanitizePDFText("The following record pages belong to this section."), "", "L", false)
}

func writePDFRecordCard(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions) {
	options = options.Normalize("L", true)
	startY := pdf.GetY()
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	_, _, _, bottomMargin := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	layout, fitsSinglePage := choosePDFRecordCardLayout(pdf, soldier, startY, options)
	if !fitsSinglePage {
		writePDFRecordCardMultiPage(pdf, soldier, options, layout)
		return
	}
	leftWidth := contentWidth * layout.LeftWidthRatio
	gapWidth := layout.ColumnGap
	rightWidth := contentWidth - leftWidth - gapWidth
	rightX := leftMargin + leftWidth + gapWidth
	usesPortraitCompact := usesPortraitCompactRecordCardLayout(soldier, options)
	defer pdf.SetAutoPageBreak(true, bottomMargin)
	pdf.SetAutoPageBreak(false, bottomMargin)

	leftY := writePDFCompactFieldSection(pdf, leftMargin, startY, leftWidth, "Identity & Vital Details", recordIdentityFields(soldier), layout)
	leftY = writePDFCompactFieldSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, "Service & Archive Details", recordServiceFields(soldier, options.PrinterFriendly), layout)
	if usesPortraitCompact {
		householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
		if hasVisiblePDFField(householdFields) {
			leftY = writePDFCompactFieldSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, "Household & Context", householdFields, layout)
		}
		if len(soldier.Records) > 0 {
			leftY = writePDFRecordsColumnSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, soldier.Records, layout, options.PrinterFriendly)
		}
	}

	rightY := startY
	if options.IncludeImages {
		if imagePath, imageLabel := firstRecordCardImage(soldier, options.PrinterFriendly); imagePath != "" {
			rightY = writePDFImagePanel(pdf, rightX, rightY, rightWidth, recordPDFImageSectionTitle(options), imagePath, imageLabel, layout)
		}
	}
	if !usesPortraitCompact {
		householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
		if hasVisiblePDFField(householdFields) {
			if rightY > startY {
				rightY += layout.SectionGap
			}
			rightY = writePDFCompactFieldSection(pdf, rightX, rightY, rightWidth, "Household & Context", householdFields, layout)
		}
	}
	narrativeTitle, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		if rightY > startY {
			rightY += layout.SectionGap
		}
		rightY = writePDFRichTextColumnSection(pdf, rightX, rightY, rightWidth, narrativeTitle, narrativeText, layout)
	}
	if !usesPortraitCompact && len(soldier.Records) > 0 {
		if rightY > startY {
			rightY += layout.SectionGap
		}
		rightY = writePDFRecordsColumnSection(pdf, rightX, rightY, rightWidth, soldier.Records, layout, options.PrinterFriendly)
	}

	pdf.SetY(maxPDFY(leftY, rightY) + 2)
}

func writePDFRecordCardMultiPage(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions, layout pdfRecordCardLayout) {
	options = options.Normalize("L", true)
	startY := pdf.GetY()
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, bottomMargin := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	defer pdf.SetAutoPageBreak(true, bottomMargin)
	pdf.SetAutoPageBreak(true, bottomMargin)

	currentY := startY
	wroteSection := false

	identityFields := recordIdentityFields(soldier)
	if hasVisiblePDFField(identityFields) {
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Identity & Vital Details", identityFields, layout)
		wroteSection = true
	}

	serviceFields := recordServiceFields(soldier, options.PrinterFriendly)
	if hasVisiblePDFField(serviceFields) {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.FieldLineHeight)
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Service & Archive Details", serviceFields, layout)
		wroteSection = true
	}

	householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
	if hasVisiblePDFField(householdFields) {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.FieldLineHeight)
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Household & Context", householdFields, layout)
		wroteSection = true
	}

	if options.IncludeImages {
		if imagePath, imageLabel := firstRecordCardImage(soldier, options.PrinterFriendly); imagePath != "" {
			currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, estimatePDFImagePanelHeight(layout, recordPDFImageSectionTitle(options) != ""))
			currentY = writePDFImagePanel(pdf, leftMargin, currentY, contentWidth, recordPDFImageSectionTitle(options), imagePath, imageLabel, layout)
			wroteSection = true
		}
	}

	narrativeTitle, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.BodyLineHeight)
		currentY = writePDFRichTextColumnSection(pdf, leftMargin, currentY, contentWidth, narrativeTitle, narrativeText, layout)
		wroteSection = true
	}

	if len(soldier.Records) > 0 {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.BodyLineHeight)
		currentY = writePDFRecordsColumnSection(pdf, leftMargin, currentY, contentWidth, soldier.Records, layout, options.PrinterFriendly)
		wroteSection = true
	}

	if wroteSection {
		pdf.SetY(currentY + 2)
	}
}

func preparePDFRecordCardSection(pdf *fpdf.Fpdf, currentY float64, wroteSection bool, gap, minHeight float64) float64 {
	if wroteSection {
		currentY += gap
	}
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	if currentY+minHeight > pageHeight-bottomMargin {
		pdf.AddPage()
		return pdf.GetY()
	}
	return currentY
}

func writePDFCompactFieldSection(pdf *fpdf.Fpdf, x, y, width float64, title string, fields []pdfField, layout pdfRecordCardLayout) float64 {
	if !hasVisiblePDFField(fields) {
		return y
	}
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
	currentY := pdf.GetY()
	labelWidth := width * 0.32
	if labelWidth < 28 {
		labelWidth = 28
	}
	if labelWidth > 44 {
		labelWidth = 44
	}
	valueWidth := width - labelWidth - 3
	for _, field := range fields {
		if !field.visible() {
			continue
		}
		rowTop := currentY
		pdf.SetXY(x, rowTop)
		pdf.SetFont("Helvetica", "B", layout.FieldLabelFontSize)
		pdf.SetTextColor(68, 82, 96)
		pdf.MultiCell(labelWidth, layout.FieldLineHeight, sanitizePDFText(field.Label), "", "L", false)
		labelBottom := pdf.GetY()
		pdf.SetXY(x+labelWidth+3, rowTop)
		if strings.TrimSpace(field.Label) == "Maiden Name" {
			pdf.SetFont("Helvetica", "I", layout.FieldValueFontSize)
		} else {
			pdf.SetFont("Helvetica", "", layout.FieldValueFontSize)
		}
		pdf.SetTextColor(34, 48, 61)
		pdf.MultiCell(valueWidth, layout.FieldLineHeight, field.renderedValue(), "", "L", false)
		valueBottom := pdf.GetY()
		currentY = maxPDFY(labelBottom, valueBottom) + layout.FieldRowGap
	}
	return currentY
}

func writePDFImagePanel(pdf *fpdf.Fpdf, x, y, width float64, title, imagePath, label string, layout pdfRecordCardLayout) float64 {
	panelY := y
	if strings.TrimSpace(title) != "" {
		pdf.SetXY(x, y)
		pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
		pdf.SetTextColor(141, 116, 64)
		pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
		panelY = pdf.GetY()
	}
	panelHeight := layout.ImagePanelHeight
	imageX, imageY, imageWidth, imageHeight, ok := fitPDFImageToBounds(imagePath, x+2, panelY+2, width-4, panelHeight-14)
	if ok {
		pdf.ImageOptions(imagePath, imageX, imageY, imageWidth, imageHeight, false, fpdf.ImageOptions{
			ImageType: strings.TrimPrefix(strings.ToLower(filepath.Ext(imagePath)), "."),
		}, 0, "")
	}
	pdf.SetXY(x+2, panelY+panelHeight-10)
	pdf.SetFont("Helvetica", "", layout.ImageLabelFontSize)
	pdf.SetTextColor(68, 82, 96)
	if strings.TrimSpace(label) != "" {
		pdf.MultiCell(width-4, layout.ImageLabelLine, emptyPDFValue(label), "", "L", false)
	}
	return panelY + panelHeight + 2
}

func estimatePDFImagePanelHeight(layout pdfRecordCardLayout, includeTitle bool) float64 {
	height := layout.ImagePanelHeight + 2
	if includeTitle {
		height += layout.SectionTitleLine
	}
	return height
}

func writePDFRichTextColumnSection(pdf *fpdf.Fpdf, x, y, width float64, title, text string, layout pdfRecordCardLayout) float64 {
	if strings.TrimSpace(text) == "" {
		return y
	}
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, topMargin, rightMargin, _ := pdf.GetMargins()
	defer pdf.SetMargins(leftMargin, topMargin, rightMargin)
	pdf.SetMargins(x, topMargin, pageWidth-(x+width))
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
	pdf.SetX(x)
	pdf.SetFont("Helvetica", "", layout.BodyFontSize)
	pdf.SetTextColor(34, 48, 61)
	writePDFRichTextSized(pdf, emptyPDFValue(text), layout.BodyLineHeight, layout.BodyFontSize)
	return pdf.GetY()
}

func writePDFRecordsColumnSection(pdf *fpdf.Fpdf, x, y, width float64, records []models.Record, layout pdfRecordCardLayout, printerFriendly bool) float64 {
	if len(records) == 0 {
		return y
	}
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, topMargin, rightMargin, _ := pdf.GetMargins()
	defer pdf.SetMargins(leftMargin, topMargin, rightMargin)
	pdf.SetMargins(x, topMargin, pageWidth-(x+width))
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, "Records", "", 1, "L", false, 0, "")
	for _, record := range records {
		line := record.RecordType
		if strings.TrimSpace(record.AppID) != "" {
			line += fmt.Sprintf(" (App: %s)", record.AppID)
		}
		pdf.SetX(x)
		writePDFBulletSized(pdf, line, layout)
		details := pdfFreeTextValue(record.Details, printerFriendly)
		if strings.TrimSpace(details) != "" {
			pdf.SetX(x)
			writePDFRichTextSized(pdf, emptyPDFValue(details), layout.BodyLineHeight, layout.BodyFontSize)
		}
	}
	return pdf.GetY()
}

func recordIdentityFields(soldier models.Soldier) []pdfField {
	fields := []pdfField{
		blankPDFField("Prefix", soldier.Prefix),
		blankPDFField("First Name", soldier.FirstName),
		blankPDFField("Middle Name", soldier.MiddleName),
		blankPDFField("Last Name", soldier.LastName),
		defaultPDFField("Suffix", soldier.Suffix),
		unknownDatePDFField("Birth Date", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death Date", soldierDeathLine(soldier)),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Buried In", soldier.BuriedIn),
	}
	return fields
}

func recordServiceFields(soldier models.Soldier, printerFriendly bool) []pdfField {
	pensionStateField := naPDFField("Pension State", pensionstate.Normalize(soldier.PensionState))
	homeStatusField := naPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	homeNameField := confederateHomeNamePDFField(soldier)
	if printerFriendly {
		pensionStateField = defaultPDFField("Pension State", pensionstate.Normalize(soldier.PensionState))
		homeStatusField = defaultPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
		homeNameField = defaultPDFField("Confederate Home Name", pdfConfederateHomeName(soldier))
	}
	fields := []pdfField{
		defaultPDFField("Record Type", displayEntryType(soldier)),
		blankPDFField("Rank In", soldier.RankIn),
		blankPDFField("Rank Out", displaySoldierRank(soldier)),
		blankPDFField("Unit", soldier.Unit),
		pensionStateField,
		defaultPDFField("Pension ID", soldier.PensionID),
		defaultPDFField("Application ID", soldier.ApplicationID),
		homeStatusField,
		homeNameField,
	}
	return fields
}

func recordHouseholdFields(soldier models.Soldier, printerFriendly bool) []pdfField {
	fields := []pdfField{
		defaultPDFField("Spouse", soldier.SpouseName),
		defaultPDFField("Linked Spouse Record", func() string {
			if printerFriendly {
				return ""
			}
			if soldier.SpouseSoldierID > 0 {
				if strings.TrimSpace(soldier.SpouseName) != "" {
					return fmt.Sprintf("%s (DB ID %d)", strings.TrimSpace(soldier.SpouseName), soldier.SpouseSoldierID)
				}
				return fmt.Sprintf("DB ID %d", soldier.SpouseSoldierID)
			}
			return ""
		}()),
		defaultPDFField("Maiden Name", soldier.MaidenName),
	}
	return fields
}

func firstRecordCardImage(soldier models.Soldier, printerFriendly bool) (string, string) {
	for _, image := range soldier.Images {
		if !image.IsPrimary {
			continue
		}
		if imagePath := imagePathForPDF(image); imagePath != "" {
			return imagePath, pdfImageCaption(image)
		}
	}
	for _, image := range soldier.Images {
		if imagePath := imagePathForPDF(image); imagePath != "" {
			return imagePath, pdfImageCaption(image)
		}
	}
	return "", ""
}

func recordPDFImageSectionTitle(options PDFOptions) string {
	return ""
}

func recordPDFNarrativeSection(soldier models.Soldier, options PDFOptions) (string, string) {
	if options.PrintableArchive {
		return "Biography", printableArchiveBiographyText(soldier, options.PrinterFriendly)
	}
	if shouldAppendSingleRecordBiographyPage(soldier, options) {
		return "", ""
	}
	text := strings.TrimSpace(soldier.PDFExcerptOverride)
	if text == "" {
		text = soldier.Biography
	}
	return "Biography", pdfFreeTextValue(text, options.PrinterFriendly)
}

func usesPortraitCompactRecordCardLayout(soldier models.Soldier, options PDFOptions) bool {
	options = options.Normalize("L", true)
	if options.PrintableArchive || !usesPortraitRecordPDFLayout(options) || !options.IncludeImages {
		return false
	}
	imagePath, _ := firstRecordCardImage(soldier, options.PrinterFriendly)
	return imagePath != ""
}

func usesPortraitRecordPDFLayout(options PDFOptions) bool {
	return strings.TrimSpace(strings.ToUpper(options.Normalize("L", true).Orientation)) != "L"
}

func shouldAppendSingleRecordBiographyPage(soldier models.Soldier, options PDFOptions) bool {
	options = options.Normalize("L", true)
	if options.PrintableArchive || usesPortraitRecordPDFLayout(options) {
		return false
	}
	return strings.TrimSpace(pdfFreeTextValue(soldier.Biography, options.PrinterFriendly)) != ""
}

func printableArchiveBiographyText(soldier models.Soldier, printerFriendly bool) string {
	text := strings.TrimSpace(soldier.PDFExcerptOverride)
	if text == "" {
		text = soldier.Biography
	}
	return truncatePDFText(pdfFreeTextValue(text, printerFriendly), 480)
}

func truncatePDFText(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if maxRunes <= 0 || trimmed == "" {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func writePrintableBiographyAppendixPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool) {
	writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography Appendix")
}

func writeSingleRecordBiographyPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool) {
	writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography")
}

func writeFullBiographyPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool, label string) {
	biography := pdfFreeTextValue(soldier.Biography, printerFriendly)
	if strings.TrimSpace(biography) == "" {
		return
	}
	pdf.AddPage()
	writePDFTitleBlock(
		pdf,
		recordPDFTitle(soldier),
		fmt.Sprintf("%s | %s | %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier), label),
	)
	writePDFSection(pdf, "Biography")
	writePDFRichTextSized(pdf, emptyPDFValue(biography), 6, 11)
}

func pdfImageCaption(image models.Image) string {
	caption := strings.TrimSpace(image.Caption)
	if caption == "" {
		return ""
	}
	if strings.EqualFold(caption, strings.TrimSpace(image.FileName)) {
		return ""
	}
	if filePath := strings.TrimSpace(image.FilePath); filePath != "" && strings.EqualFold(caption, filepath.Base(filePath)) {
		return ""
	}
	if looksLikePDFImageFileName(caption) {
		return ""
	}
	return caption
}

func looksLikePDFImageFileName(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	base := filepath.Base(trimmed)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return strings.TrimSuffix(base, ext) != ""
	default:
		return false
	}
}

func recordPDFTitle(soldier models.Soldier) string {
	if name := strings.TrimSpace(soldier.GetFullName()); name != "" {
		return name
	}
	return soldierDisplayName(soldier)
}

func maxPDFY(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func hasVisiblePDFField(fields []pdfField) bool {
	for _, field := range fields {
		if field.visible() {
			return true
		}
	}
	return false
}

func omitPDFValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	switch strings.ToUpper(trimmed) {
	case "NONE", "NA", "N/A", "NOT RECORDED":
		return true
	default:
		return false
	}
}

func pdfFreeTextValue(value string, printerFriendly bool) string {
	trimmed := strings.TrimSpace(value)
	if !printerFriendly {
		return trimmed
	}
	cleaned := pdfURLPattern.ReplaceAllString(trimmed, "")
	cleaned = regexp.MustCompile(`\[\[([^\]]+)\]\]`).ReplaceAllString(cleaned, "$1")
	cleaned = strings.TrimSpace(strings.Join(strings.Fields(cleaned), " "))
	return cleaned
}

func pdfOrientationLabel(value string) string {
	if strings.TrimSpace(strings.ToUpper(value)) == "L" {
		return "Landscape"
	}
	return "Portrait"
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func writePDFInlineField(pdf *fpdf.Fpdf, field pdfField, width float64) {
	x := pdf.GetX()
	y := pdf.GetY()
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(34, 6, field.Label, "", 0, "", false, 0, "")
	if strings.TrimSpace(field.Label) == "Maiden Name" {
		pdf.SetFont("Helvetica", "I", 10)
	} else {
		pdf.SetFont("Helvetica", "", 10)
	}
	pdf.SetTextColor(68, 82, 96)
	pdf.SetXY(x+34, y)
	pdf.MultiCell(width-34, 6, field.renderedValue(), "", "L", false)
	if pdf.GetY() < y+6 {
		pdf.SetY(y + 6)
	}
	pdf.SetX(x)
}

func pdfFooterMetadata(format string, version int) string {
	return fmt.Sprintf("Generated %s | %s v%d | App %s | Schema %d", time.Now().Format("2006-01-02 15:04"), format, version, buildinfo.AppVersion, buildinfo.SchemaVersion)
}

func writePDFField(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(34, 8, label+":", "", 0, "", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.MultiCell(0, 8, emptyPDFValue(value), "", "", false)
}

func writePDFBody(pdf *fpdf.Fpdf, text string) {
	writePDFRichText(pdf, emptyPDFValue(text), 7)
}

func writePDFBullet(pdf *fpdf.Fpdf, text string) {
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(6, 7, "-", "", 0, "", false, 0, "")
	pdf.MultiCell(0, 7, emptyPDFValue(text), "", "", false)
}

func writePDFAnalyticsRows(pdf *fpdf.Fpdf, items []AnalyticsCount, emptyMessage string) {
	if len(items) == 0 {
		writePDFBody(pdf, emptyMessage)
		return
	}
	for _, item := range items {
		writePDFBullet(pdf, fmt.Sprintf("%s - %d", item.Label, item.Count))
	}
}

func writePDFBulletSized(pdf *fpdf.Fpdf, text string, layout pdfRecordCardLayout) {
	pdf.SetFont("Helvetica", "", layout.BodyFontSize)
	pdf.CellFormat(layout.BulletIndent, layout.BodyLineHeight, "-", "", 0, "", false, 0, "")
	pdf.MultiCell(0, layout.BodyLineHeight, emptyPDFValue(text), "", "", false)
}

func writePDFRichText(pdf *fpdf.Fpdf, text string, lineHeight float64) {
	writePDFRichTextSized(pdf, text, lineHeight, 10)
}

func analyticsPDFMetadataDetails(snapshot AnalyticsSnapshot) map[string]string {
	return map[string]string{
		"Top cemeteries": fmt.Sprintf("%d", len(snapshot.CemeteryDensity)),
		"Home statuses":  fmt.Sprintf("%d", len(snapshot.ConfederateHomeStatus)),
		"Pension states": fmt.Sprintf("%d", len(snapshot.PensionDistribution)),
		"Top units":      fmt.Sprintf("%d", len(snapshot.UnitRepresentation)),
		"Birth decades":  fmt.Sprintf("%d", len(snapshot.BirthDecadeDistribution)),
		"Death decades":  fmt.Sprintf("%d", len(snapshot.DeathDecadeDistribution)),
		"Record types":   fmt.Sprintf("%d soldiers / %d spouses / %d linked people", snapshot.RecordTypes.TotalSoldiers, snapshot.RecordTypes.TotalWivesWidows, snapshot.RecordTypes.TotalLinkedPeople),
	}
}

func writePDFRichTextSized(pdf *fpdf.Fpdf, text string, lineHeight, fontSize float64) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for lineIndex, line := range lines {
		segments := pdfTextSegments(line)
		if len(segments) == 0 {
			segments = []pdfTextSegment{{Text: ""}}
		}
		for _, segment := range segments {
			if segment.Link != "" {
				pdf.SetFont("Helvetica", "I", fontSize)
				pdf.SetTextColor(48, 87, 122)
				pdf.WriteLinkString(lineHeight, segment.Text, segment.Link)
				pdf.SetTextColor(0, 0, 0)
				pdf.SetFont("Helvetica", "", fontSize)
				continue
			}
			pdf.SetFont("Helvetica", "", fontSize)
			pdf.Write(lineHeight, segment.Text)
		}
		if lineIndex < len(lines)-1 {
			pdf.Ln(lineHeight)
		}
	}
	pdf.Ln(lineHeight)
}

type pdfTextSegment struct {
	Text string
	Link string
}

func pdfTextSegments(text string) []pdfTextSegment {
	matches := pdfURLPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []pdfTextSegment{{Text: text}}
	}

	segments := make([]pdfTextSegment, 0, len(matches)*2+1)
	cursor := 0
	for _, match := range matches {
		start := match[0]
		end := match[1]
		if start > cursor {
			segments = append(segments, pdfTextSegment{Text: text[cursor:start]})
		}

		linkText, suffix := splitPDFURLSuffix(text[start:end])
		if linkText != "" {
			segments = append(segments, pdfTextSegment{Text: linkText, Link: linkText})
		}
		if suffix != "" {
			segments = append(segments, pdfTextSegment{Text: suffix})
		}
		cursor = end
	}
	if cursor < len(text) {
		segments = append(segments, pdfTextSegment{Text: text[cursor:]})
	}
	return segments
}

func splitPDFURLSuffix(value string) (string, string) {
	trimmed := strings.TrimRight(value, ".,;:!?)]}")
	return trimmed, value[len(trimmed):]
}

func writePDFImageRow(pdf *fpdf.Fpdf, image models.Image) {
	const thumbnailWidth = 34.0
	const thumbnailHeight = 22.0
	const rowHeight = 36.0

	_, pageHeight := pdf.GetPageSize()
	if pdf.GetY()+rowHeight > pageHeight-16 {
		pdf.AddPage()
	}

	x := pdf.GetX()
	y := pdf.GetY()
	imagePath := imagePathForPDF(image)
	pdf.Rect(x, y, thumbnailWidth, thumbnailHeight, "D")

	if imagePath != "" {
		imageX, imageY, imageWidth, imageHeight, ok := fitPDFImageToBounds(imagePath, x, y, thumbnailWidth, thumbnailHeight)
		if ok {
			pdf.ImageOptions(imagePath, imageX, imageY, imageWidth, imageHeight, false, fpdf.ImageOptions{
				ImageType: strings.TrimPrefix(strings.ToLower(filepath.Ext(imagePath)), "."),
			}, 0, "")
		}
	}

	pdf.SetXY(x+thumbnailWidth+4, y)
	title := pdfImageCaption(image)
	if title == "" {
		title = "Image"
	}
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 6, emptyPDFValue(title), "", "", false)

	if pdf.GetY() < y+rowHeight {
		pdf.SetY(y + rowHeight)
	}
}

func writePDFRegistryEntry(pdf *fpdf.Fpdf, soldier models.Soldier) {
	if pdf.GetY() > 230 {
		pdf.AddPage()
	}
	pdf.SetDrawColor(141, 116, 64)
	pdf.Line(16, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(3)
	pdf.SetFont("Times", "B", 13)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(0, 7, soldierDisplayName(soldier), "", 1, "", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.CellFormat(0, 5, fmt.Sprintf("%s - %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)), "", 1, "", false, 0, "")
	for _, line := range registryEntryLines(soldier) {
		if !line.visible() {
			continue
		}
		writePDFInlineField(pdf, line, 178)
	}
	pdf.Ln(2)
}

func recordCardFields(soldier models.Soldier) []pdfField {
	fields := []pdfField{
		defaultPDFField("Record Type", displayEntryType(soldier)),
		defaultPDFField("Full Name", soldierFullName(soldier)),
		defaultPDFField("Display ID", soldier.DisplayID),
		unknownDatePDFField("Birth", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death", soldierDeathLine(soldier)),
	}
	if strings.TrimSpace(soldier.Prefix) != "" {
		fields = append(fields, blankPDFField("Prefix", soldier.Prefix))
	}
	if strings.TrimSpace(soldier.Suffix) != "" {
		fields = append(fields, defaultPDFField("Suffix", soldier.Suffix))
	}
	if isSoldierEntry(soldier) {
		fields = append(fields,
			blankPDFField("Rank In", soldier.RankIn),
			blankPDFField("Rank Out", displaySoldierRank(soldier)),
			blankPDFField("Unit", soldier.Unit),
			naPDFField("Pension State", pensionstate.Normalize(soldier.PensionState)),
			naPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)),
			confederateHomeNamePDFField(soldier),
		)
	} else {
		fields = append(fields,
			defaultPDFField("Married To", soldier.SpouseName),
			defaultPDFField("Maiden Name", soldier.MaidenName),
		)
	}
	fields = append(fields,
		defaultPDFField("Pension ID", soldier.PensionID),
		defaultPDFField("Application ID", soldier.ApplicationID),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Buried In", soldier.BuriedIn),
	)
	return fields
}

func registryEntryLines(soldier models.Soldier) []pdfField {
	lines := []pdfField{
		unknownDatePDFField("Birth Date", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death Date", soldierDeathLine(soldier)),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Service Summary", soldierServiceLine(soldier)),
		defaultPDFField("Pension / Application", strings.TrimSpace(strings.Join(compactPDFValues(pensionstate.Normalize(soldier.PensionState), soldier.PensionID, soldier.ApplicationID), " | "))),
		defaultPDFField("Confederate Home", strings.TrimSpace(strings.Join(compactPDFValuesExcludingNA(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus), soldier.ConfederateHomeName), " | "))),
		defaultPDFField("Buried In", soldier.BuriedIn),
	}
	if strings.TrimSpace(soldier.SpouseName) != "" || strings.TrimSpace(soldier.MaidenName) != "" {
		if spouseName := strings.TrimSpace(soldier.SpouseName); spouseName != "" {
			lines = append(lines, defaultPDFField("Spouse", spouseName))
		}
		if maidenName := strings.TrimSpace(soldier.MaidenName); maidenName != "" {
			lines = append(lines, defaultPDFField("Maiden Name", maidenName))
		}
	}
	if strings.TrimSpace(soldier.Notes) != "" {
		lines = append(lines, defaultPDFField("Notes", soldier.Notes))
	}
	if len(soldier.Records) > 0 {
		recordLines := make([]string, 0, len(soldier.Records))
		for _, record := range soldier.Records {
			recordLines = append(recordLines, strings.TrimSpace(strings.Join(compactPDFValues(record.RecordType, record.AppID, record.Details), " | ")))
		}
		lines = append(lines, defaultPDFField("Records", strings.Join(recordLines, "\n")))
	}
	return lines
}

func compactPDFValues(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func compactPDFValuesExcludingNA(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && trimmed != confederatehomestatus.NotApplicable {
			result = append(result, trimmed)
		}
	}
	return result
}

func emptyPDFValue(value string) string {
	sanitized := sanitizePDFText(strings.TrimSpace(value))
	if sanitized == "" {
		return "N/A"
	}
	return sanitized
}

// EmptyPDFValue is the public form of emptyPDFValue, exposed for callers
// outside this package (e.g. the iCal export in internal/archive) that
// want the same "N/A" placeholder convention.
func EmptyPDFValue(value string) string {
	return emptyPDFValue(value)
}

func sanitizePDFText(value string) string {
	replaced := strings.NewReplacer(
		"\u2018", "'",
		"\u2019", "'",
		"\u201c", `"`,
		"\u201d", `"`,
		"\u2013", "-",
		"\u2014", "-",
		"\u2026", "...",
		"\u2022", "-",
		"\u00a0", " ",
	).Replace(value)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return r
		case r < 32 || r > 126:
			return ' '
		default:
			return r
		}
	}, replaced)
}

func soldierDisplayName(soldier models.Soldier) string {
	return peopleinfo.SoldierDisplayName(soldier)
}

func soldierFullName(soldier models.Soldier) string {
	return peopleinfo.SoldierFullName(soldier)
}

func soldierServiceLine(soldier models.Soldier) string {
	return persondisplay.SoldierServiceLine(soldier.RankOut, soldier.Rank, soldier.RankIn, soldier.Unit)
}

func displaySoldierRank(soldier models.Soldier) string {
	if strings.TrimSpace(soldier.RankOut) != "" {
		return strings.TrimSpace(soldier.RankOut)
	}
	if strings.TrimSpace(soldier.Rank) != "" {
		return strings.TrimSpace(soldier.Rank)
	}
	return strings.TrimSpace(soldier.RankIn)
}

func isSoldierEntry(soldier models.Soldier) bool {
	return strings.TrimSpace(soldier.EntryType) == "" || soldier.EntryType == "soldier"
}

func displayEntryType(soldier models.Soldier) string {
	return peopleinfo.DisplayEntryType(soldier)
}

func soldierDeathLine(soldier models.Soldier) string {
	return dates.DisplayUnknown(soldier.DeathDate)
}

func pdfConfederateHomeName(soldier models.Soldier) string {
	if confederatehomestatus.Normalize(soldier.ConfederateHomeStatus) == confederatehomestatus.NotApplicable {
		return confederatehomestatus.NotApplicable
	}
	return strings.TrimSpace(soldier.ConfederateHomeName)
}

func confederateHomeNamePDFField(soldier models.Soldier) pdfField {
	if confederatehomestatus.Normalize(soldier.ConfederateHomeStatus) == confederatehomestatus.NotApplicable {
		return naPDFField("Confederate Home Name", confederatehomestatus.NotApplicable)
	}
	return defaultPDFField("Confederate Home Name", strings.TrimSpace(soldier.ConfederateHomeName))
}

func monthLabel(month int) string {
	if month < 1 || month > 12 {
		return "Unknown"
	}
	return time.Month(month).String()
}

func imagePathForPDF(image models.Image) string {
	candidate := strings.TrimSpace(image.ResolvedPath)
	if candidate == "" {
		candidate = strings.TrimSpace(image.FilePath)
	}
	if candidate == "" {
		return ""
	}
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return ""
	}
	switch strings.ToLower(filepath.Ext(candidate)) {
	case ".jpg", ".jpeg", ".png", ".gif":
	default:
		return ""
	}
	if !validPDFImage(candidate) {
		return ""
	}
	return candidate
}

func fitPDFImageToBounds(imagePath string, x, y, maxWidth, maxHeight float64) (float64, float64, float64, float64, bool) {
	if maxWidth <= 0 || maxHeight <= 0 {
		return x, y, 0, 0, false
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return x, y, 0, 0, false
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return x, y, 0, 0, false
	}

	width := float64(config.Width)
	height := float64(config.Height)
	scale := maxWidth / width
	if height*scale > maxHeight {
		scale = maxHeight / height
	}
	if scale <= 0 {
		return x, y, 0, 0, false
	}

	fittedWidth := width * scale
	fittedHeight := height * scale
	fittedX := x + (maxWidth-fittedWidth)/2
	fittedY := y + (maxHeight-fittedHeight)/2
	return fittedX, fittedY, fittedWidth, fittedHeight, true
}

func validPDFImage(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		return false
	}
	switch strings.ToLower(format) {
	case "jpeg", "png", "gif":
		return true
	default:
		return false
	}
}
