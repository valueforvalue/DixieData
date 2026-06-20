// Package templatespec defines the public types that describe a
// discovered .typ template: its name, the record types it serves,
// the orientation, and the export types. This is the canonical
// shape for both the render package's Registry and the tools/tune
// CLI's "list-templates" output.
package templatespec

// Spec is a discovered .typ template's metadata block. The
// template's filename (minus the .typ extension) is the Name. The
// RecordTypes field lists the Person Record subtypes the template
// supports (soldier, spouse, widow). Orientation is the page
// orientation the template expects. ExportTypes is a free-form
// tag list used by the export UI to group templates.
type Spec struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	RecordTypes []string `json:"record_types"`
	Orientation string   `json:"orientation"`
	ExportTypes []string `json:"export_types"`
	Description string   `json:"description"`
	Engine      string   `json:"engine"`
}

// Supports returns true if the spec is for the given record type and
// orientation. An empty RecordTypes list means "any".
func (s Spec) Supports(recordType, orientation string) bool {
	if len(s.RecordTypes) > 0 {
		found := false
		for _, rt := range s.RecordTypes {
			if rt == recordType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if s.Orientation != "" && s.Orientation != "any" && s.Orientation != orientation {
		return false
	}
	return true
}
