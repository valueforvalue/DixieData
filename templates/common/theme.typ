// templates/common/theme.typ
//
// Centralized design tokens for all DixieData Typst templates.
// Reads from theme.json (injected by the Go renderer at #637)
// so palette, type-scale, fonts, and branding have ONE source
// of truth shared with the CSS build.
//
// When theme.json is absent (e.g. running typst directly for
// template development), the file read will error — use the
// Go render pipeline or copy config.default.json's theme block
// into the workdir as theme.json.

#let _cfg = json("../theme.json")

// Exported as module-level variables so templates can write
//   theme.palette.accent
//   theme.type-scale.field_label.size
//   theme.fonts.body_sans
// etc. (the module is imported as `theme` because the file
// is named theme.typ).

// Convenience: convert palette hex strings to typst colors.
// json() returns strings; rgb() expects a hex string, so this
// is a thin wrapper that makes theme.palette.accent a color.
#let _hex(c) = rgb(c)

#let palette = (
  accent:        _hex(_cfg.palette.accent),
  accent_strong: _hex(_cfg.palette.accent_strong),
  text_primary:  _hex(_cfg.palette.text_primary),
  text_secondary: _hex(_cfg.palette.text_secondary),
  text_muted:    _hex(_cfg.palette.text_muted),
  link:          _hex(_cfg.palette.link),
  danger:        _hex(_cfg.palette.danger),
  divider:       _hex(_cfg.palette.divider),
  panel_fill:    _hex(_cfg.palette.panel_fill),
)
// Convert JSON numbers to typst lengths for type-scale entries.
#let _pt(n) = n * 1pt

#let type-scale = (
  section_title: (size: _pt(_cfg.type_scale.section_title.size_pt), line: _pt(_cfg.type_scale.section_title.line_pt)),
  field_label:   (size: _pt(_cfg.type_scale.field_label.size_pt),   line: _pt(_cfg.type_scale.field_label.line_pt)),
  field_value:   (size: _pt(_cfg.type_scale.field_value.size_pt),   line: _pt(_cfg.type_scale.field_value.line_pt)),
  body:          (size: _pt(_cfg.type_scale.body.size_pt),          line: _pt(_cfg.type_scale.body.line_pt)),
  biography:     (size: _pt(_cfg.type_scale.biography.size_pt),     line: _pt(_cfg.type_scale.biography.line_pt)),
  header:        (size: _pt(_cfg.type_scale.header.size_pt)),
  footer:        (size: _pt(_cfg.type_scale.footer.size_pt)),
  image_label:   (size: 8pt, line: 4pt),
)
#let fonts     = _cfg.fonts
#let branding  = _cfg.branding

// Geometry is NOT in theme.json — these are template-specific
// layout constants, not user-tunable design tokens. They stay
// hard-coded here so templates that need them can reference
//   theme.geometry.page_margin
//   theme.geometry.column_gap
// etc. by importing this file (see #637 scope).

#let geometry = (
  page_margin:    (top: 0.75in, bottom: 0.75in, left: 0.75in, right: 0.75in),
  column_gap:     8mm,
  section_gap:    4mm,
  field_row_gap:  1mm,
  record_card_left_ratio: 52%,
  image_panel_height: 50mm,
)
