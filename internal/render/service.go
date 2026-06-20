package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
)

// newPDFDocument constructs the fpdf document. The hard-coded margins and
// page size are tracked by the layout audit (see docs/audit/...).
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

// brandedPDFDocument adds the header and (optional) footer callbacks to a
// fresh document. The header reads branding.ArchiveTitle; the footer reads
// branding.FooterText and is suppressed when printerFriendly is true.
func (s *Service) brandedPDFDocument(orientation, title, format string, version int, footerDetail string, printerFriendly bool) (*fpdf.Fpdf, error) {
	branding, err := s.pdfBranding()
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

// pdfBranding returns the user identity-derived strings used in the header
// and footer.
func (s *Service) pdfBranding() (pdfBranding, error) {
	identity, err := s.users.UserIdentity()
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

// sortByLastName orders the slice by lowercased last name. Used by ExportJSON.
func sortByLastName(soldiers []models.Soldier) {
	sort.Slice(soldiers, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(soldiers[i].LastName))
		right := strings.ToLower(strings.TrimSpace(soldiers[j].LastName))
		if left != right {
			return left < right
		}
		leftFirst := strings.ToLower(strings.TrimSpace(soldiers[i].FirstName))
		rightFirst := strings.ToLower(strings.TrimSpace(soldiers[j].FirstName))
		if leftFirst != rightFirst {
			return leftFirst < rightFirst
		}
		return strings.ToLower(strings.TrimSpace(soldiers[i].DisplayID)) < strings.ToLower(strings.TrimSpace(soldiers[j].DisplayID))
	})
}

// --- public methods on Service ---

// ExportSoldierPDF writes a single-soldier record card PDF.
func (s *Service) ExportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	return s.exportSoldierPDF(outputPath, soldier, options.Normalize("L", true))
}

// ExportSoldierPDFWithoutImages writes a single-soldier record card PDF
// with image rendering disabled.
func (s *Service) ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error {
	return s.exportSoldierPDF(outputPath, soldier, PDFOptions{Orientation: "L", IncludeImages: false})
}

// ExportMonthlyAnniversaryPDF writes a monthly anniversary report PDF.
func (s *Service) ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := s.brandedPDFDocument(options.Orientation, "Monthly Anniversary Report", "monthly-pdf", buildinfo.MonthlyPDFExportVersion, pdfFooterMetadata("monthly-pdf", buildinfo.MonthlyPDFExportVersion), options.PrinterFriendly)
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

// ExportFullDatabasePDF writes the bulk archive PDF. Honors PrintSettings
// scope, filter, sort, and group options. Optional FullBiographyPage
// appends a biography page per record.
func (s *Service) ExportFullDatabasePDF(outputPath string, settings PrintSettings) error {
	settings = settings.Normalize()
	pdf, err := s.brandedPDFDocument(settings.Orientation, "Printable Archive Registry", "database-pdf", buildinfo.DatabasePDFExportVersion, pdfFooterMetadata("database-pdf", buildinfo.DatabasePDFExportVersion), settings.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()
	writePDFTitleBlock(pdf, "Printable Archive Registry", "Full database export with concise record pages, captioned primary images, and bounded biography excerpts that continue onto additional pages when needed.")

	var selectedIDs []int64
	if settings.Scope == PrintScopeSelected {
		selectedIDs = settings.SelectedIDs
	}
	soldiers, err := exportDetailedSoldiers(s.soldier, selectedIDs)
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

// ExportAnalyticsSummaryPDF writes the archive analytics summary PDF.
func (s *Service) ExportAnalyticsSummaryPDF(outputPath string, snapshot AnalyticsSnapshot, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := s.brandedPDFDocument(options.Orientation, "Archive Summary Report", "analytics-pdf", buildinfo.AnalyticsPDFExportVersion, pdfFooterMetadata("analytics-pdf", buildinfo.AnalyticsPDFExportVersion), options.PrinterFriendly)
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

// exportSoldierPDF is the shared body for the two public soldier PDF methods.
func (s *Service) exportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	options = options.Normalize("L", true)
	pdf, err := s.brandedPDFDocument(options.Orientation, "Record Card", "soldier-pdf", buildinfo.SoldierPDFExportVersion, "", options.PrinterFriendly)
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

// FetchAllSoldiers is exposed for callers (e.g. the static archive) that
// need the same paginated, sorted, full-table walk that the bulk PDF uses.
func (s *Service) FetchAllSoldiers() ([]models.Soldier, error) {
	return exportSoldiers(s.soldier)
}
