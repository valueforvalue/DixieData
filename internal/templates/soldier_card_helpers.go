// Helper functions for soldier_card.templ and the upcoming
// soldiers_list.templ + soldier_detail.templ split. Pure Go, no
// templ syntax; lives here so the .templ files stay focused on
// markup. Functions here are referenced from soldier_card.templ;
// keep them in this file rather than duplicating across the
// split files when PR #F2 lands.
//
// When extracting helpers from soldier_card.templ, follow this
// rule: if the function body contains only Go (no templ syntax,
// no references to other templ symbols), move it here. Functions
// that reference other templ symbols (e.g. templ Components or
// templ helper functions inlined in the file) must stay in the
// .templ file.
package templates

import (
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/routebuilder"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func soldierCardClass(highlighted bool) string {
	className := "group relative card rounded-2xl p-4 mb-3"
	if highlighted {
		className += " border-[rgba(197,171,104,0.92)] bg-[rgba(245,241,230,0.97)] shadow-[0_0_30px_rgba(197,171,104,0.16)] ring-1 ring-[rgba(197,171,104,0.7)]"
	}
	return className
}

func hasActiveSearch(search viewmodel.PersonRecordSearch) bool {
	if search.Browse {
		return true
	}
	if search.Mode == "advanced" {
		return len(searchParams(search)) > 0
	}
	return strings.TrimSpace(search.Query) != ""
}

func deathDate(s viewmodel.PersonRecord) string {
	return dates.DisplayUnknown(s.DeathDate)
}

func emptyDetail(value string) string {
	if strings.TrimSpace(value) == "" {
		return "N/A"
	}
	return value
}

func blankDetail(value string) string {
	return strings.TrimSpace(value)
}

func formatAuditTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.Local().Format("Jan 2, 2006 3:04 PM")
		}
	}
	return trimmed
}

func auditHistoryLines(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := []string{}
	if strings.Contains(trimmed, "\n") {
		parts = strings.Split(trimmed, "\n")
	} else {
		parts = strings.Split(trimmed, ",")
	}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if strings.Contains(item, " changed from ") || strings.HasSuffix(item, ".") {
			lines = append(lines, item)
			continue
		}
		label := strings.TrimSpace(strings.ReplaceAll(item, "_", " "))
		if label == "" {
			continue
		}
		lines = append(lines, strings.Title(label)+" updated.")
	}
	return lines
}

func pageHref(search viewmodel.PersonRecordSearch, page int) templ.SafeURL {
	return templ.SafeURL(pageRequestURL(search, page))
}

func pageRequestURL(search viewmodel.PersonRecordSearch, page int) string {
	params := searchParams(search)
	params.Set("page", intToString(page))
	if search.Mode == "advanced" {
		return routebuilder.SoldierSearchAdvanced() + "?" + params.Encode()
	}
	return routebuilder.SoldierSearch(false) + "?" + params.Encode()
}

// intToString converts an int to a base-10 string. Pulled out so
// fmt.Sprintf isn't needed for simple integer formatting in URL
// builders and similar contexts.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func searchParams(search viewmodel.PersonRecordSearch) url.Values {
	values := url.Values{}
	if search.Mode == "advanced" {
		if strings.TrimSpace(search.DisplayID) != "" {
			values.Set("display_id", search.DisplayID)
		}
		if strings.TrimSpace(search.EntryType) != "" {
			values.Set("entry_type", search.EntryType)
		}
		if strings.TrimSpace(search.FirstName) != "" {
			values.Set("first_name", search.FirstName)
		}
		if strings.TrimSpace(search.MiddleName) != "" {
			values.Set("middle_name", search.MiddleName)
		}
		if strings.TrimSpace(search.LastName) != "" {
			values.Set("last_name", search.LastName)
		}
		if strings.TrimSpace(search.MaidenName) != "" {
			values.Set("maiden_name", search.MaidenName)
		}
		if strings.TrimSpace(search.Rank) != "" {
			values.Set("rank", search.Rank)
		}
		if strings.TrimSpace(search.RankIn) != "" {
			values.Set("rank_in", search.RankIn)
		}
		if strings.TrimSpace(search.RankOut) != "" {
			values.Set("rank_out", search.RankOut)
		}
		if strings.TrimSpace(search.Unit) != "" {
			values.Set("unit", search.Unit)
		}
		if strings.TrimSpace(search.SourceRecordType) != "" {
			values.Set("record_type", search.SourceRecordType)
		}
		if strings.TrimSpace(search.PensionState) != "" {
			values.Set("pension_state", search.PensionState)
		}
		if strings.TrimSpace(search.ConfederateHomeStatus) != "" {
			values.Set("confederate_home_status", search.ConfederateHomeStatus)
		}
		if strings.TrimSpace(search.ConfederateHomeName) != "" {
			values.Set("confederate_home_name", search.ConfederateHomeName)
		}
		if strings.TrimSpace(search.BuriedIn) != "" {
			values.Set("buried_in", search.BuriedIn)
		}
		if strings.TrimSpace(search.ReviewStatus) != "" {
			values.Set("review_status", search.ReviewStatus)
		}
		if strings.TrimSpace(search.BirthDate) != "" {
			values.Set("birth_date", search.BirthDate)
		}
		if strings.TrimSpace(search.BirthYear) != "" {
			values.Set("birth_year", search.BirthYear)
		}
		if strings.TrimSpace(search.BirthYearTo) != "" {
			values.Set("birth_year_to", search.BirthYearTo)
		}
		if strings.TrimSpace(search.DeathDate) != "" {
			values.Set("death_date", search.DeathDate)
		}
		if strings.TrimSpace(search.DeathYear) != "" {
			values.Set("death_year", search.DeathYear)
		}
		if strings.TrimSpace(search.DeathYearTo) != "" {
			values.Set("death_year_to", search.DeathYearTo)
		}
		if strings.TrimSpace(search.DeathMonth) != "" {
			values.Set("death_month", search.DeathMonth)
		}
		if strings.TrimSpace(search.DeathDay) != "" {
			values.Set("death_day", search.DeathDay)
		}
		return values
	}
	if search.Browse {
		values.Set("browse", "1")
	}
	if strings.TrimSpace(search.Query) != "" {
		values.Set("q", search.Query)
	}
	return values
}

