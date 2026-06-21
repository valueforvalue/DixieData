package render

// Helpers extracted from internal/archive. The names match the original
// package-private symbols so the moved code is easy to read against the
// audit findings.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
)

// exportBatchSize is the pagination size for fetching all soldiers.
const exportBatchSize = 500

// exportSoldiers paginates the entire soldier table.
func exportSoldiers(soldierLister SoldierLister) ([]models.Soldier, error) {
	var all []models.Soldier
	page := 1
	for {
		batch, _, err := soldierLister.List(page, exportBatchSize)
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

// exportDetailedSoldiers returns the full enriched record for every soldier,
// optionally filtered to the given selection.
func exportDetailedSoldiers(soldierLister SoldierLister, selectedIDs []int64) ([]models.Soldier, error) {
	batch, err := exportSoldiers(soldierLister)
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
		soldier, err := soldierLister.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, *soldier)
	}
	return all, nil
}

// printablePDFMetadataDetails returns the export options as a string map for
// the printable archive metadata block.
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

// changedPrintGroups returns the list of group transitions that should
// trigger a divider page between the previous and current record.
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
			Field: field,
			Label: printGroupLabel(field),
			Value: value,
			Title: printGroupTitle(field, value),
			Level: index,
		})
	}
	return changes
}

// sortPrintableSoldiers orders the slice in place by group then by sort key.
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
	if leftOK && rightOK {
		if left != right {
			return left < right, true
		}
		return false, true
	}
	if leftOK {
		return true, true
	}
	if rightOK {
		return false, true
	}
	return false, false
}

func printBirthYear(soldier models.Soldier) (int, bool) {
	return printYearFromCanonicalValue(soldier.BirthDate)
}

func printDeathYear(soldier models.Soldier) (int, bool) {
	return printYearFromCanonicalValue(soldier.DeathDate)
}

func printYearFromCanonicalValue(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	year := firstFourDigitYear(trimmed)
	if year <= 0 {
		return 0, false
	}
	return year, true
}

func printYearFromCanonical(value string) int {
	year, _ := printYearFromCanonicalValue(value)
	return year
}

func firstFourDigitYear(value string) int {
	for index := 0; index+4 <= len(value); index++ {
		candidate := value[index : index+4]
		allDigits := true
		for _, r := range candidate {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			year := 0
			for _, r := range candidate {
				year = year*10 + int(r-'0')
			}
			return year
		}
	}
	return 0
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
		return strings.ReplaceAll(field, "_", " ")
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
		return emptyPDFValue(strings.TrimSpace(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)))
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

// FilterPrintableSoldiers is the public wrapper for
// filterPrintableSoldiers. Lets callers outside the package apply
// the same filtering rules used by ExportFullDatabasePDF.
func FilterPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) []models.Soldier {
	return filterPrintableSoldiers(soldiers, settings)
}

// SortPrintableSoldiers is the public wrapper for
// sortPrintableSoldiers. Sorts the slice in place using the same
// ordering ExportFullDatabasePDF applies.
func SortPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) {
	sortPrintableSoldiers(soldiers, settings)
}
