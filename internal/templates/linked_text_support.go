package templates

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

var linkedTextPattern = regexp.MustCompile(`https?://[^\s<]+|\[\[[^\[\]\r\n]+\]\]`)

type textSegment struct {
	Text       string
	Href       string
	IsExternal bool
}

func linkifiedLines(text string) [][]textSegment {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	result := make([][]textSegment, 0, len(lines))
	for _, line := range lines {
		result = append(result, linkifiedSegments(line))
	}
	return result
}

func linkifiedSegments(text string) []textSegment {
	matches := linkedTextPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []textSegment{{Text: text}}
	}

	segments := make([]textSegment, 0, len(matches)*2+1)
	cursor := 0
	for _, match := range matches {
		start := match[0]
		end := match[1]
		if start > cursor {
			segments = append(segments, textSegment{Text: text[cursor:start]})
		}

		token := text[start:end]
		switch {
		case strings.HasPrefix(token, "[[") && strings.HasSuffix(token, "]]"):
			target := strings.TrimSpace(token[2 : len(token)-2])
			if target == "" {
				segments = append(segments, textSegment{Text: token})
				break
			}
			segments = append(segments, textSegment{
				Text: target,
				Href: "/soldiers/display/" + url.PathEscape(target),
			})
		default:
			urlText, suffix := splitURLSuffix(token)
			if urlText != "" {
				segments = append(segments, textSegment{Text: urlText, Href: urlText, IsExternal: true})
			}
			if suffix != "" {
				segments = append(segments, textSegment{Text: suffix})
			}
		}
		cursor = end
	}
	if cursor < len(text) {
		segments = append(segments, textSegment{Text: text[cursor:]})
	}
	return segments
}

func splitURLSuffix(value string) (string, string) {
	trimmed := strings.TrimRight(value, ".,;:!?)]}")
	return trimmed, value[len(trimmed):]
}

func detailSubheadingIsMaidenName(s viewmodel.PersonRecord) bool {
	return !isSoldierEntry(s) &&
		strings.TrimSpace(s.RelationshipLabel) == "" &&
		strings.TrimSpace(s.SpouseName) == "" &&
		strings.TrimSpace(s.MaidenName) != ""
}
