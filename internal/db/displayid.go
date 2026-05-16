package db

import (
	"fmt"
	"strconv"
	"strings"
)

const LegacyDisplayIDNamespace = "DXD"

func NormalizeNodePrefix(prefix string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(prefix))
	if trimmed == "" {
		return LegacyDisplayIDNamespace
	}
	return trimmed
}

func SanitizeID(id string, namespace string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, "-")
	if len(parts) >= 2 {
		if canonical, ok := canonicalNamespaceSequence(parts); ok {
			return canonical
		}
		if strings.EqualFold(parts[0], NormalizeNodePrefix(namespace)) {
			return trimmed
		}
		return trimmed
	}

	return trimmed
}

func CanonicalDisplayID(id string) (string, int, bool) {
	canonical, ok := canonicalNamespaceSequence(strings.Split(strings.TrimSpace(id), "-"))
	if !ok {
		return "", 0, false
	}
	parts := strings.Split(canonical, "-")
	sequence, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], sequence, true
}

func NextGeneratedDisplayID(namespace string, nextSequence int) string {
	return fmt.Sprintf("%s-%05d", NormalizeNodePrefix(namespace), nextSequence)
}

func canonicalNamespaceSequence(parts []string) (string, bool) {
	if len(parts) < 2 {
		return "", false
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if !isFiveDigitSequence(last) {
		return "", false
	}
	for i := len(parts) - 2; i >= 0; i-- {
		namespace := normalizeNamespaceSegment(parts[i])
		if namespace == "" {
			continue
		}
		return namespace + "-" + last, true
	}
	return "", false
}

func normalizeNamespaceSegment(value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	if trimmed == "" {
		return ""
	}
	for _, r := range trimmed {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return trimmed
}

func isFiveDigitSequence(value string) bool {
	if len(value) != 5 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
