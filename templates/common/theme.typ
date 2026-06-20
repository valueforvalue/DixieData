// templates/common/theme.typ
//
// Centralized design tokens for all DixieData Typst templates. Every
// template imports this file and uses the named values for colors,
// fonts, margins, and the type scale. Changing a value here updates
// every template that imports it.
//
// The values mirror the audit's theme.json deliverable (see
// docs/audit/layout-theming-token-schema.md). The PDF triplet and the
// CSS hex resolve to the same color in the render package.

#let palette = (
  accent:         rgb("#8d7440"),
  accent_strong:  rgb("#a88a46"),
  text_primary:   rgb("#22303d"),
  text_secondary: rgb("#445260"),
  text_muted:     rgb("#71808e"),
  link:           rgb("#30577a"),
  danger:         rgb("#54211d"),
  divider:        rgb("#8d7440"),
  panel_fill:     rgb("#fff8e7"),
)

#let type-scale = (
  section_title: (size: 9pt, line: 6pt),
  field_label:   (size: 8pt, line: 4.5pt),
  field_value:   (size: 9pt, line: 4.5pt),
  body:          (size: 9pt, line: 5pt),
  image_label:   (size: 8pt, line: 4pt),
  header:        (size: 10pt),
  footer:        (size: 8pt),
  biography:     (size: 11pt, line: 6pt),
)

#let geometry = (
  page_margin:    (top: 0.75in, bottom: 0.75in, left: 0.75in, right: 0.75in),
  column_gap:     8mm,
  section_gap:    4mm,
  field_row_gap:  1mm,
  record_card_left_ratio: 52%,
  image_panel_height: 64mm,
)

#let branding = (
  header_suffix: "'s Civil War Research Archive",
  footer_template: "Made with DixieData | Version: {app_version} | Build: {build_identity}",
)
