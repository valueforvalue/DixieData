package findagrave

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/valueforvalue/DixieData/internal/models"
)

type Result struct {
	SourceLabel string
	MemorialID  string
	MemorialURL string
	FirstName   string
	MiddleName  string
	LastName    string
	BirthDate   string
	BirthInfo   string
	DeathDate   string
	BuriedIn    string
	Warnings    []string
	Spouses     []models.ScrapedRelative
}

func ParseInput(ctx context.Context, input string) (Result, error) {
	_ = ctx
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Result{}, fmt.Errorf("paste Find a Grave memorial HTML first")
	}
	if looksLikeURL(trimmed) {
		return Result{}, fmt.Errorf("URL scraping is disabled. Paste the Find a Grave memorial HTML instead")
	}
	return ParseHTML(trimmed, "Parsed from pasted HTML", "")
}

func ParseHTML(html, sourceLabel, sourceURL string) (Result, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return Result{}, fmt.Errorf("parse memorial HTML: %w", err)
	}

	result := Result{
		SourceLabel: sourceLabel,
		MemorialURL: strings.TrimSpace(sourceURL),
	}

	jsFirstName, firstNameFromJS := jsStringValue(html, "firstName")
	if firstNameFromJS {
		result.FirstName, result.MiddleName = splitGivenAndMiddle(jsFirstName)
		if strings.Contains(strings.TrimSpace(jsFirstName), " ") {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Find a Grave firstName was %q, so the middle-name split is a best effort and should be reviewed.", strings.TrimSpace(jsFirstName)))
		}
	} else {
		result.FirstName, result.MiddleName = splitGivenAndMiddle(cleanText(doc.Find("#bio-name").First().Text()))
		result.Warnings = append(result.Warnings, "Name fields fell back to visible page text because the embedded memorial object did not provide a usable firstName value.")
	}

	if value, ok := jsStringValue(html, "lastName"); ok {
		result.LastName = value
	}
	if result.LastName == "" {
		result.LastName = lastToken(cleanText(doc.Find("#bio-name").First().Text()))
	}

	if value, ok := jsStringValue(html, "memorialId"); ok {
		result.MemorialID = value
	}
	if result.MemorialID == "" {
		result.MemorialID = cleanText(doc.Find("#memNumberLabel").First().Text())
	}

	if result.MemorialURL == "" {
		if value, ok := jsStringValue(html, "linkToShare"); ok {
			result.MemorialURL = value
		}
	}
	if result.MemorialURL == "" {
		if href, ok := doc.Find(`link[rel="canonical"]`).Attr("href"); ok {
			result.MemorialURL = strings.TrimSpace(href)
		}
	}

	birthYear, birthFromJS := jsStringValue(html, "birthYear")
	if birthYear == "" {
		birthYear = extractYear(cleanText(doc.Find(`#birthDateLabel,[itemprop="birthDate"]`).First().Text()))
		if birthYear != "" {
			result.Warnings = append(result.Warnings, "Birth year fell back to the visible memorial vitals instead of the embedded memorial object.")
		}
	} else if !birthFromJS {
		result.Warnings = append(result.Warnings, "Birth year fell back to the visible memorial vitals instead of the embedded memorial object.")
	}
	if birthYear != "" {
		result.BirthInfo = birthYear
		result.BirthDate = partialYearDate(birthYear)
	}

	deathYear, deathFromJS := jsStringValue(html, "deathYear")
	if deathYear == "" {
		deathYear = extractYear(cleanText(doc.Find(`#deathDateLabel,[itemprop="deathDate"]`).First().Text()))
		if deathYear != "" {
			result.Warnings = append(result.Warnings, "Death year fell back to the visible memorial vitals instead of the embedded memorial object.")
		}
	} else if !deathFromJS {
		result.Warnings = append(result.Warnings, "Death year fell back to the visible memorial vitals instead of the embedded memorial object.")
	}
	if deathYear != "" {
		result.DeathDate = partialYearDate(deathYear)
	}

	result.BuriedIn = buildBurialLocation(html, doc)
	if result.BuriedIn == "" {
		result.Warnings = append(result.Warnings, "Burial location could not be fully assembled from the memorial source and should be completed manually if needed.")
	}
	if value, ok := jsStringValue(html, "locationName"); ok && strings.TrimSpace(value) == "" {
		result.Warnings = append(result.Warnings, "Find a Grave locationName was blank in this memorial, so burial details were built from cemetery and address fields instead.")
	}

	result.Spouses = parseSpouses(doc)
	result.Warnings = append([]string{"Verify all scraped data manually before saving, especially names, year-only dates, cemetery text, and any spouse relationships."}, result.Warnings...)
	return result, nil
}

func looksLikeURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func jsStringValue(html, key string) (string, bool) {
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*:\s*(?:"([^"]*)"|'([^']*)')`)
	match := pattern.FindStringSubmatch(html)
	if len(match) == 0 {
		return "", false
	}
	for _, candidate := range match[1:] {
		if candidate != "" || len(match) == 3 {
			return strings.TrimSpace(candidate), true
		}
	}
	return "", true
}

func splitGivenAndMiddle(value string) (string, string) {
	parts := strings.Fields(cleanText(value))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func lastToken(value string) string {
	parts := strings.Fields(cleanText(value))
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func partialYearDate(year string) string {
	trimmed := strings.TrimSpace(year)
	if len(trimmed) != 4 {
		return ""
	}
	return "00/00/" + trimmed
}

func buildBurialLocation(html string, doc *goquery.Document) string {
	parts := []string{}
	for _, value := range []string{
		firstNonBlank(jsValueOrEmpty(html, "cemeteryName"), cleanText(doc.Find("#cemeteryNameLabel").First().Text())),
		firstNonBlank(jsValueOrEmpty(html, "cemeteryCityName"), cleanText(doc.Find("#cemeteryCityName").First().Text())),
		firstNonBlank(jsValueOrEmpty(html, "cemeteryCountyName"), cleanText(doc.Find("#cemeteryCountyName").First().Text())),
		firstNonBlank(jsValueOrEmpty(html, "cemeteryStateName"), cleanText(doc.Find("#cemeteryStateName").First().Text())),
		firstNonBlank(jsValueOrEmpty(html, "cemeteryCountryAbbrev"), cleanText(doc.Find("#cemeteryCountryName").First().Text())),
	} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ", ")
}

func parseSpouses(doc *goquery.Document) []models.ScrapedRelative {
	spouses := []models.ScrapedRelative{}
	doc.Find(`#family-grid ul[aria-labelledby="spouseLabel"] > li`).Each(func(_ int, item *goquery.Selection) {
		link := item.Find(`a[href*="/memorial/"]`).First()
		href, _ := link.Attr("href")
		name := cleanText(item.Find(`h3[itemprop="name"]`).First().Text())
		life := cleanText(item.Find(`p.life`).First().Text())
		spouses = append(spouses, models.ScrapedRelative{
			Name:       name,
			MemorialID: memorialIDFromHref(href),
			URL:        absoluteMemorialURL(href),
			BirthYear:  extractYear(life),
			DeathYear:  extractLastYear(life),
		})
	})
	return spouses
}

func jsValueOrEmpty(html, key string) string {
	value, _ := jsStringValue(html, key)
	return value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func memorialIDFromHref(href string) string {
	matches := regexp.MustCompile(`/memorial/(\d+)`).FindStringSubmatch(href)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func absoluteMemorialURL(href string) string {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "/") {
		return "https://www.findagrave.com" + trimmed
	}
	return trimmed
}

func extractYear(value string) string {
	matches := regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`).FindStringSubmatch(value)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func extractLastYear(value string) string {
	matches := regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`).FindAllString(value, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
