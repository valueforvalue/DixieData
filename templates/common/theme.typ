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
  link:           rgb("#4A90E2"),
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
  // The image panel is sized to fit at the top of a right column on
  // a Letter page. 40mm keeps the panel compact enough that the
  // household + records sections below it can stay on the same page
  // for soldiers with up to ~6 records. fpdf uses 64mm here; the
  // typst number is smaller because typst's text is rendered with a
  // slightly larger effective line height and we want to keep the
  // right column from overflowing the page.
  image_panel_height: 50mm,
)

#let branding = (
  header_suffix: "'s Civil War Research Archive",
  footer_template: "Made with DixieData | Version: {app_version} | Build: {build_identity}",
)
