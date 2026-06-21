// Package encode produces the JSON payload that Typst templates read
// via #sys.inputs. The encoding is the bridge between Go domain
// types and Typst's data model.
//
// Today the encoding is straight JSON marshalling. Future
// refinements (e.g. typst-specific date formatting, base64 image
// embedding) live here so that the render package and the tuning
// tool share the same shape.
package encode

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/render"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

// TemplateData is the JSON shape passed to every Typst template. The
// fields are accessed in templates as sys.inputs.soldier,
// sys.inputs.options, sys.inputs.branding, sys.inputs.app.
type TemplateData struct {
	Soldier  models.Soldier             `json:"soldier,omitempty"`
	Records  []models.Record             `json:"records,omitempty"`
	Images   []models.Image              `json:"images,omitempty"`
	Options  render.PrintSettings        `json:"options"`
	Settings render.PrintSettings        `json:"settings,omitempty"`
	Snapshot render.AnalyticsSnapshot    `json:"snapshot,omitempty"`
	Calendar map[int][]models.Soldier    `json:"calendar,omitempty"`
	Month    int                         `json:"month,omitempty"`
	Branding Branding                     `json:"branding"`
	App      AppMeta                     `json:"app"`
}

// Branding is the user-identity-derived strings rendered in the PDF
// header and footer.
type Branding struct {
	ArchiveTitle string `json:"archive_title"`
	FooterText   string `json:"footer_text"`
}

// AppMeta carries build-identity and version info to the template.
type AppMeta struct {
	Version       string `json:"version"`
	BuildIdentity string `json:"build_identity"`
}

// Marshal serializes the TemplateData to indented JSON.
func (d *TemplateData) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return nil, fmt.Errorf("encode template data: %w", err)
	}
	return buf.Bytes(), nil
}

// NewTemplateDataForSoldier builds the canonical payload for a single
// soldier export. The branding is read from the provided identity
// (a real LocalArchive's user identity, or a placeholder for tests).
func NewTemplateDataForSoldier(soldier models.Soldier, options render.PDFOptions, branding Branding) TemplateData {
	return TemplateData{
		Soldier:  soldier,
		Records:  soldier.Records,
		Images:   soldier.Images,
		Options:  mergeOptionsWithDefaults(options),
		Branding: branding,
		App:      appMeta(),
	}
}

// NewTemplateDataForBulk builds the payload for a bulk export.
// The settings carry scope, filter, sort, and group options.
func NewTemplateDataForBulk(settings render.PrintSettings, branding Branding) TemplateData {
	return TemplateData{
		Settings: settings,
		Options: render.PrintSettings{
			Orientation:     settings.Orientation,
			PrinterFriendly: settings.PrinterFriendly,
			Template:        settings.Template,
		},
		Branding: branding,
		App:      appMeta(),
	}
}

// NewTemplateDataForAnniversary builds the payload for the monthly
// anniversary report.
func NewTemplateDataForAnniversary(month int, calendar map[int][]models.Soldier, options render.PDFOptions, branding Branding) TemplateData {
	return TemplateData{
		Month:    month,
		Calendar: calendar,
		Options:  mergeOptionsWithDefaults(options),
		Branding: branding,
		App:      appMeta(),
	}
}

// NewTemplateDataForAnalytics builds the payload for the analytics
// summary report.
func NewTemplateDataForAnalytics(snapshot render.AnalyticsSnapshot, options render.PDFOptions, branding Branding) TemplateData {
	return TemplateData{
		Snapshot: snapshot,
		Options:  mergeOptionsWithDefaults(options),
		Branding: branding,
		App:      appMeta(),
	}
}

// BrandingFromIdentity maps a models.UserIdentity to the package's
// flat Branding struct.
func BrandingFromIdentity(identity models.UserIdentity) Branding {
	return Branding{
		ArchiveTitle: identity.BrandingName() + "'s Civil War Research Archive",
		FooterText:   "Made with DixieData | Version: " + buildinfo.AppVersion + " | Build: " + buildinfo.BuildIdentity(),
	}
}

func appMeta() AppMeta {
	return AppMeta{
		Version:       buildinfo.AppVersion,
		BuildIdentity: buildinfo.BuildIdentity(),
	}
}

// mergeOptionsWithDefaults fills in the PrintSettings fields that
// the template needs to read. PrintSettings and PDFOptions have
// been kept in sync: both include IncludeImages and PrintableArchive
// so the encode layer (which JSON-round-trips the payload) doesn't
// silently drop the flags before the template sees them.
func mergeOptionsWithDefaults(options render.PDFOptions) render.PrintSettings {
	return render.PrintSettings{
		Orientation:     options.Orientation,
		PrinterFriendly: options.PrinterFriendly,
		Template:        options.Template,
		IncludeImages:   options.IncludeImages,
		PrintableArchive: options.PrintableArchive,
	}
}
