// Common helpers used by all record-card templates. Each per-
// variant template (soldier_*.typ, widow_*.typ, spouse_*.typ) is
// a thin file that imports these helpers and configures the
// fields/layout for its entry type.
//
// Reused by: soldier_landscape, soldier_portrait, widow_landscape,
// widow_portrait, spouse_landscape, spouse_portrait.

#import "theme.typ"

// --- formatting helpers ---

// pdf-records-per-page-landscape is the maximum Source Records
// rendered per Person-Record PDF in landscape orientation. 12
// fits the landscape 11x8.5 layout with margin at 9pt + v(0.2em)
// per row (~0.5in per card, 6.5in usable text height → ~13
// rows). 12 leaves ~8% headroom for tight `details` text.
//
// pdf-records-per-page-portrait is the cap for portrait
// orientation. Portrait has ~70% of landscape's record column
// width (a single 50% page column vs the 50% right column in
// landscape) and the same row pitch, so the cap is reduced to
// keep the one-page mental model honest. 6 was picked over 8
// after round 35 review: with 8 the inline bio (when set)
// pushed records into a second column, which still triggered
// a 3rd page from the dedicated bio page; 6 fits the same
// envelope cleanly with the inline bio. Round 36.
//
// Issue #513: mirrored as records.PDFRecordsPerPage (12) +
// records.PDFRecordsPerPagePortrait (6) in Go so the export-time
// warning toast (X-DixieData-Toast) can announce truncation
// using the same number. If you change one side, change the
// other — the audit net pins the pair.
#let pdf-records-per-page-landscape = 12
#let pdf-records-per-page-portrait = 6

// pdf-records-cap returns the per-orientation Source Records
// cap. Landscape uses the wider cap; portrait uses the
// narrower one. The render-records-section helper reads this
// when slicing its list so the typst side and the Go toast
// stay in sync.
#let pdf-records-cap(is-landscape) = {
  if is-landscape { pdf-records-per-page-landscape } else { pdf-records-per-page-portrait }
}

// title-case capitalizes the first character of a string and
// lowercases the rest. Used for entry-type fallbacks.
#let title-case(s) = {
  if s == none or s == "" { s }
  else {
    let first-char = s.at(0)
    let rest = s.clusters().slice(1).join("")
    upper(first-char) + lower(rest)
  }
}

#let month-names = (
  "01": "January",
  "02": "February",
  "03": "March",
  "04": "April",
  "05": "May",
  "06": "June",
  "07": "July",
  "08": "August",
  "09": "September",
  "10": "October",
  "11": "November",
  "12": "December",
)

// long-date renders a date string as the long form ("May 22, 1844"),
// the year alone if month/day are unknown ("1835"), or "Unknown"
// if all parts are unknown. Matches the fpdf path's
// internal/dates.DisplayUnknown behavior:
//   "00/00/0000" -> "Unknown"
//   "00/00/1835" -> "1835"   (year only, no "Unknown")
//   "05/00/1844" -> "May 1844"
//   "05/22/1844" -> "May 22, 1844"
#let long-date(s) = {
  if s == none or s == "" {
    return [Unknown]
  }
  if s == "Unknown" or s == "0000-00-00" {
    return [Unknown]
  }

  let parts = s.split("/")
  if parts.len() == 3 {
    let month-idx = parts.at(0)
    let day-raw = parts.at(1)
    let year-raw = parts.at(2)

    if month-idx == "00" and day-raw == "00" and year-raw == "00" {
      return [Unknown]
    }

    if month-idx == "00" and day-raw == "00" {
      return [#year-raw]
    }

    if day-raw == "00" and month-idx != "00" and year-raw != "00" {
      let mname = month-names.at(month-idx, default: month-idx)
      return [#mname #year-raw]
    }

    if month-idx == "00" or day-raw == "00" or year-raw == "00" {
      return [Unknown]
    }

    let day = if day-raw.starts-with("0") and day-raw.len() > 1 {
      day-raw.slice(1)
    } else {
      day-raw
    }
    let mname = month-names.at(month-idx, default: month-idx)
    return [#mname #day, #year-raw]
  }

  if s.contains("-") and s.len() >= 10 {
    return [s.split("-").at(0)]
  }

  return [#s]
}

// entry-type-label turns the raw lowercase entry_type into the
// title-cased label the fpdf path uses.
#let entry-type-label(raw) = {
  let r = raw
  if r == none or r == "" [Soldier]
  else if r == "soldier" [Soldier]
  else if r == "wife" [Wife]
  else if r == "widow" [Widow]
  else if r == "linked_person" [Person Record]
  else [#title-case(r)]
}

// render-link turns a URL into a clickable 'Click to view' anchor.
// In the PDF, the text is a clickable hyperlink annotation; the
// URL itself is hidden from the visible text. Returns nothing
// when the value is blank or missing, and returns plain text
// when the value doesn't look like a URL (so non-URL details
// like "Died of pneumonia, Rock Island Barracks, IL." still
// render as text rather than getting a misleading link).
//
// Usage:
//   #render-link("https://example.com/foo")
//   #render-link(r.at("details", default: ""))
//
// URL detection is a simple scheme check: the value must start
// with `http://` or `https://` after trimming. Anything else
// (free text, bare slugs, ancestry.com without scheme) is
// rendered as plain text.
//
// Typst's `#link(url, [text])` creates the clickable annotation.
// Typst has a hard limit on URL length (~4096 chars) so we
// guard against absurdly long values. Anything over 4000 chars
// falls through to plain text, which matches the old
// `text(size: 8pt)[#value]` behavior for that case.
#let render-link(url) = {
  if url == none or url == "" or (type(url) == str and url.trim() == "") {
    return
  }
  let s = url.trim()
  if not (s.starts-with("http://") or s.starts-with("https://")) {
    // not a URL, render as plain text
    return text(size: 8pt, s)
  }
  if s.len() > 4000 {
    // typst refuses to encode very long URL annotations; fall
    // back to plain text so the document still renders
    return text(size: 8pt, s)
  }
  link(
    s,
    text(
      fill: theme.palette.link,
    )[#underline[Click to view]]
  )
}

// --- field-row primitive ---

// visible: when false, hide the row entirely. fpdf always shows
// the label even when blank for service fields, but the widow
// and spouse variants override per-field. The default is to hide
// rows whose value is blank (matches the soldier_landscape
// behavior).
#let label-value(label, value, hide-if-blank: true) = {
  let is-blank = (
    value == none
      or (type(value) == str and value.trim() == "")
  )
  if hide-if-blank and is-blank {
    none
  } else {
    grid(
      columns: (32%, 1fr),
      column-gutter: 0.3cm,
      align: (left, left),
      [#text(size: 8pt, weight: "bold")[#label]],
      [#box(width: 100%)[#text(size: 9pt)[#value]]],
    )
  }
}

#let field-row(label, value, hide-if-blank: true) = {
  let v = label-value(label, value, hide-if-blank: hide-if-blank)
  if v != none [#v]
}

// --- name + title block ---

// compose-name returns the title-block name. Uses prefix if
// non-empty, otherwise the first/middle/last sequence.
#let compose-name(s) = {
  let prefix = s.at("prefix", default: "")
  let first = s.at("first_name", default: "")
  let middle = s.at("middle_name", default: "")
  let last = s.at("last_name", default: "")
  let display-id = s.at("display_id", default: "")

  if prefix != "" [
    #prefix #first #middle #last
  ] else if first != "" and last != "" [
    #first #middle #last
  ] else if last != "" [
    #last, #first
  ] else if first != "" [
    #first
  ] else [
    #display-id
  ]
}

// render-title-block renders the title and the display-id line
// below it. The original fpdf design was a 20pt Times serif
// name with a 0.6em gap to the first section heading; the
// rendering-iteration loop trimmed both — the name to 14pt and
// the gaps to 0.1em — so the title block reads as a single
// header unit rather than three separate chunks. Left-aligned
// for landscape; centered for portrait via the `align-title`
// flag.
#let render-title-block(s, align-title: left) = {
  let display-id = s.at("display_id", default: "")
  let entry-type-raw = s.at("entry_type", default: "")
  let suffix = s.at("suffix", default: "")

  align(align-title, text(
    size: 14pt,
    font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
    weight: "bold",
  )[
    #compose-name(s)
    #if suffix != "" [ #suffix]
  ])
  v(0em)
  align(align-title, text(
    size: 9pt,
    fill: theme.palette.text_secondary,
  )[
    #display-id - #entry-type-label(entry-type-raw)
  ])
  v(0.1em)
}

// --- field sections (the left column on landscape, single col on portrait) ---

#let render-identity-section(s) = {
  text(size: 9pt, weight: "bold", fill: theme.palette.accent)[
    Identity & Vital Details
  ]
  v(0.2em)
  field-row("Prefix", s.at("prefix", default: ""))
  field-row("First Name", s.at("first_name", default: ""))
  field-row("Middle Name", s.at("middle_name", default: ""))
  field-row("Last Name", s.at("last_name", default: ""))
  field-row("Suffix", s.at("suffix", default: ""))
  field-row("Birth Date", long-date(s.at("birth_date", default: "")))
  field-row("Death Date", long-date(s.at("death_date", default: "")))
  field-row("Birth Info", s.at("birth_info", default: ""))
  field-row("Buried In", s.at("buried_in", default: ""))
}

// render-service-section renders the "Service & Archive Details"
// section. The default behavior is to hide rows whose value is
// blank; the widow/spouse variants pass `show-all: true` to
// match the fpdf path which always shows the label.
#let render-service-section(s, show-all: false) = {
  v(0.4em)
  text(size: 9pt, weight: "bold", fill: theme.palette.accent)[
    Service & Archive Details
  ]
  v(0.2em)

  let entry-type-raw = s.at("entry_type", default: "")
  let pension-state = s.at("pension_state", default: "")
  let pension-id = s.at("pension_id", default: "")
  let app-id = s.at("application_id", default: "")
  let ch-status = s.at("confederate_home_status", default: "")
  let ch-name = s.at("confederate_home_name", default: "")

  field-row("Record Type", entry-type-label(entry-type-raw), hide-if-blank: false)
  // Rank In, Rank Out, and Unit are military service fields.
  // Widow and wife records inherit the linked soldier's
  // service info via SpouseSoldierID, but the user asked
  // for those rows to be dropped from widow/wife renders
  // (round 31) because the data is duplicated on the linked
  // soldier's record card and rarely useful in a widow/wife
  // context. The variant is detected via `show-all: true`,
  // which is the existing flag for widow/spouse rendering
  // (set by the per-variant templates at the call sites).
  if not show-all {
    field-row("Rank In", s.at("rank_in", default: ""), hide-if-blank: not show-all)
    field-row("Rank Out", s.at("rank_out", default: ""), hide-if-blank: not show-all)
    field-row("Unit", s.at("unit", default: ""), hide-if-blank: not show-all)
  }
  field-row("Pension State", if pension-state.trim() == "" [N/A] else [#pension-state])
  field-row("Pension ID", if pension-id.trim() == "" [N/A] else [#pension-id])
  field-row("Application ID", if app-id.trim() == "" [N/A] else [#app-id])
  field-row("Confederate Home Status", if ch-status.trim() == "" [N/A] else [#ch-status])
  field-row("Confederate Home Name", if ch-name.trim() == "" [N/A] else [#ch-name])
}

// household-has-visible-fields returns true when at least one
// household field has a non-blank value. The household section
// header is suppressed when no fields are visible so empty widow /
// spouse records don't waste vertical space.
#let household-has-visible-fields(s, show-all: false) = {
  let spouse-name = s.at("spouse_name", default: "")
  if spouse-name != none and spouse-name.trim() != "" { return true }
  let spouse-id = s.at("spouse_soldier_id", default: 0)
  if spouse-id != none and spouse-id > 0 { return true }
  if show-all {
    // The widow / spouse variants show the labels even when blank,
    // so the section is "visible" for layout purposes.
    return true
  }
  let maiden = s.at("maiden_name", default: "")
  if maiden != none and maiden.trim() != "" { return true }
  return false
}

// render-household-section renders the right-column "Household & Context".
// The default behavior is to hide blank fields; widow/spouse pass
// `show-all: true` to show Linked Spouse Record / Maiden Name even
// when blank. The section header is suppressed when no fields are
// visible (matches fpdf, which writes the section only when
// hasVisiblePDFField returns true).
#let render-household-section(s, show-all: false) = {
  if not household-has-visible-fields(s, show-all: show-all) {
    return none
  }
  text(size: 9pt, weight: "bold", fill: theme.palette.accent)[
    Household & Context
  ]
  v(0.2em)

  field-row("Spouse", s.at("spouse_name", default: ""))

  let spouse-soldier-id = s.at("spouse_soldier_id", default: 0)
  let spouse-name = s.at("spouse_name", default: "")
  if show-all or (spouse-soldier-id != none and spouse-soldier-id > 0) {
    // Build the linked-value as a single string so the field-row
    // doesn't close over variables from the if-block scope. The
    // linked spouse is referenced by the user-facing display_id
    // (DXD-XXXXX) populated by the export service in
    // internal/archive/export_service.go (the
    // `linked_spouse_display_id` field on models.Soldier). If
    // the field is missing — e.g. snapshot fixtures that don't
    // round-trip through ExportSoldierPDF — we fall back to the
    // legacy 'DB ID N' rendering so the field still shows
    // something rather than going blank.
    let linked-display-id = s.at("linked_spouse_display_id", default: "")
    let linked-value = if spouse-name.trim() != "" [
      #if linked-display-id != "" [
        #(spouse-name + " (" + linked-display-id + ")")
      ] else [
        #(spouse-name + " (DB ID " + str(spouse-soldier-id) + ")")
      ]
    ] else [
      #if linked-display-id != "" [
        #linked-display-id
      ] else [
        #("DB ID " + str(spouse-soldier-id))
      ]
    ]
    field-row("Linked Spouse Record", linked-value, hide-if-blank: not show-all)
  }

  field-row("Maiden Name", s.at("maiden_name", default: ""), hide-if-blank: not show-all)
}

// render-records-section renders the right-column "Records" section.
// Caps the rendered list at the per-orientation cap
// (pdf-records-per-page-landscape=12 / -portrait=8) and emits a
// muted footnote when records were truncated so the user can see
// they have more Source Records than the PDF shows. The export
// pipeline surfaces a parallel warning toast (X-DixieData-Toast)
// driven by records.PDFRecordsPerPage / PDFRecordsPerPagePortrait
// so the user gets the same truncation signal whether they look
// at the PDF or the UI.
#let render-records-section(s, is-landscape: true) = {
  v(0.5em)
  let records = s.at("records", default: ())
  if records.len() > 0 [
    #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Records]
    #v(0.2em)
    #set text(size: 9pt)
    #let limit = pdf-records-cap(is-landscape)
    #let shown = records.slice(0, calc.min(limit, records.len()))
    #let omitted = records.len() - shown.len()
    #for r in shown [
      #block(width: 100%)[
        *#r.at("record_type", default: "")* (App: #r.at("app_id", default: ""))
        #if r.at("details", default: "") != "" [
          #linebreak() #v(0.1em) #render-link(r.at("details", default: ""))
        ]
      ]
      #v(0.2em)
    ]
    #if omitted > 0 [
      #v(0.3em)
      #text(size: 7pt, fill: theme.palette.text_muted)[
        #omitted additional record(s) omitted — reorder Source Records to control which are included in this PDF.
      ]
    ]
  ]
}

// --- biography page ---

// render-biography-inline renders the soldier's biography in
// compact form, suitable for fitting alongside a record card on
// the same page. Uses the user-supplied PDFExcerptOverride when
// set (typically a shortened version) and falls back to the full
// biography. When the full bio is used in portrait mode and
// exceeds the hard cap (max-bio-chars-portrait, ~2000 chars),
// the text is truncated with a visible note so the bio never
// overflows the single-page portrait card. Landscape has no
// cap because overflow can go to a dedicated bio page.
//
// The override is the intended mechanism; the cap is a safety
// net for records where the override was never set.
#let render-biography-inline(s, is-landscape: true) = {
  let excerpt = s.at("pdf_excerpt_override", default: "")
  let body = if excerpt != none and excerpt.trim() != "" {
    excerpt
  } else {
    s.at("biography", default: "")
  }
  if body == none or body.trim() == "" { return none }

  // Portrait cap: ~2000 chars fits ~25 lines at 7pt in the
  // 4.25in right column with standard leading. The cap is
  // a hard limit so the bio never pushes past the single-
  // page portrait card. The override (PDFExcerptOverride)
  // is the intended truncation mechanism; this cap is the
  // safety net for records that never set one.
  let max-bio-chars-portrait = 2250
  let cap-active = not is-landscape and body.len() > max-bio-chars-portrait
  let bio-text = if cap-active {
    body.clusters().slice(0, max-bio-chars-portrait).join("")
  } else {
    body
  }

  text(size: theme.type-scale.biography.size, weight: "bold", fill: theme.palette.accent)[
    Biography
  ]
  v(0.4em)
  set text(size: theme.type-scale.biography.size - 2pt, fill: theme.palette.text_primary)
  bio-text
  if cap-active [
    #v(0.3em)
    #text(size: 7pt, fill: theme.palette.text_muted)[...truncated — open this record in DixieData to read the full biography.]
  ]
}

// render-biography-page appends a dedicated page with the
// biography if the record has one. Matches the fpdf path's
// shouldAppendSingleRecordBiographyPage behavior. The page
// layout is just the soldier's name (large, serif) and the
// biography text below a 'Biography' section header; the
// `display_id • entry_type • Full Biography` subtitle that
// was rendered between the name and the section header is
// dropped per user request (round 30) so the appendix reads
// as a clean "Full Biography" page without the metadata
// preamble.
#let render-biography-page(s) = {
  let biography = s.at("biography", default: "")
  if biography != none and biography.trim() != "" [
    #pagebreak()
    #text(
      size: 20pt,
      font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
      weight: "bold",
    )[
      #compose-name(s)
      #if s.at("suffix", default: "") != "" [ #s.at("suffix")]
    ]
    #v(0.8em)
    #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Biography]
    #v(0.4em)
    #set text(size: 9pt, fill: theme.palette.text_primary)
    #biography
  ]
}

// --- page setup ---

// page-params returns the page parameters dictionary the
// per-variant template applies via #set page(...). Returns a
// dict, not content, so the caller can do
// '#set page(..page-params(...))' at document scope.
//
// Header is top-left, 7pt secondary colour, with a horizontal
// rule below it. Footer is centered, 6pt muted colour, with a
// horizontal rule above it. The two rules match the accent
// colour so a per-document colour cue ties the chrome to the
// body content. The original fpdf design used a centered bold
// 10pt header without rules; the user feedback over the
// rendering-iteration loop moved the header off-centre and
// wanted the rules as visual frames.
#let page-params(is-landscape, branding, opts) = {
  let page-width = if is-landscape { 11in } else { 8.5in }
  let page-height = if is-landscape { 8.5in } else { 11in }
  let margins = (top: 0.4in, bottom: 0.4in, left: 0.63in, right: 0.63in)
  let header-content = {
    align(left, text(
      size: 7pt,
      fill: theme.palette.text_secondary,
    )[#branding.at("archive_title", default: "DixieData Archive")])
    // The v(0.3em) pushes the line away from the text by
    // reserving vertical space in the header. The line is
    // anchored at the bottom of the header area, so the v()
    // creates visible breathing room between the branding
    // text and the rule. The user wanted the line "slightly
    // below the text" rather than flush against it.
    v(0.3em)
    place(bottom, line(length: 100%, stroke: 0.6pt + theme.palette.accent))
  }
  // Body of the footer: a horizontal rule above the text. The
  // rule uses place(top, ...) to anchor the line at the top of
  // the footer area; the v(0.4em) reserves the gap between the
  // rule and the centered footer text below it. Issue #489:
  // the codename is now passed in as its own field on the
  // branding map so the footer can italicize it. The plain
  // "Made with DixieData | Version: ..." string lives in
  // branding.footer_text and is unchanged for PDF text-
  // extraction tests; the codename is appended with #emph()
  // for the visual italics.
  let footer-content = if not opts.at("printerFriendly", default: false) {
    {
      place(top, line(length: 100%, stroke: 0.6pt + theme.palette.accent))
      v(0.4em)
      align(center, text(
        size: 6pt,
        fill: theme.palette.text_muted,
      )[
        #branding.at("footer_text", default: "")
        #h(0.4em)
        #emph[#branding.at("codename", default: "")]
      ])
    }
  } else {
    none
  }
  (
    width: page-width,
    height: page-height,
    margin: margins,
    header: header-content,
    footer: footer-content,
  )
}

// --- orientation detection ---

#let detect-landscape(opts) = {
  let orientation-raw = str(opts.at("orientation", default: "L")).trim()
  return (
    orientation-raw == "L"
      or orientation-raw == "LANDSCAPE"
      or orientation-raw == "l"
      or orientation-raw == "landscape"
  )
}

// --- image panel ---

// render-image-panel returns the soldier's primary image (or the
// first image if there is no primary) sized to fit the panel area
// defined in theme.geometry. Returns none when the user has not
// asked for images, when the soldier has no images, or when the
// image file is missing on disk.
//
// The TypstRenderer stages image files at <workdir>/images/ before
// compiling, so this template can reference them as relative paths
// like `images/<file_name>`. The renderer only stages files that
// exist on disk; a missing file means the template just renders
// nothing here.
#let render-image-panel(opts, s) = {
  if not opts.at("includeImages", default: false) { return none }
  let images = s.at("images", default: ())
  if images.len() == 0 { return none }

  let chosen = none
  for img in images {
    if img.at("is_primary", default: false) {
      chosen = img
      break
    }
  }
  if chosen == none { chosen = images.first() }

  let file-name = chosen.at("file_name", default: none)
  if file-name == none or file-name == "" { return none }

  // Image renders alone. Captions are intentionally not rendered
  // under the image in printable exports; they were never part of
  // the documented layout and in practice they often contained
  // source-document filenames from imported archives, which leaked
  // into the PDF as ugly text under otherwise clean record cards.
  // The caption field is still stored on the model for in-app
  // display (image viewer) but is suppressed in the printable
  // archive.
  block(
    width: 100%,
    inset: (bottom: 0.4em),
  )[
    #box(
      width: 100%,
      height: theme.geometry.image_panel_height,
      clip: true,
      align(top + center)[
        // The image lookup is rooted at the typst workdir, which is
        // the temp dir we pass via `--root`. The renderer's image
        // staging step copies the image to <workdir>/images/, so
        // the absolute path "/images/..." resolves regardless of
        // which file the template is being evaluated from. (A
        // relative path like "images/..." would be resolved
        // relative to common/record_card.typ, which is wrong.)
        #image("/images/" + file-name, fit: "contain")
      ],
    )
  ]
}

// --- main card layouts ---

// --- main card layouts ---

// --- public entry point ---

// render-record-card renders the title block, card layout, and
// biography page. The per-variant template must apply page
// setup at document scope before calling this.
//
// Landscape layout: a 2-column row at the top (title on the
// left, image on the right) so the image's top edge aligns
// with the title text. Below that, the existing 3-column
// body grid (left = identity+service+household, middle =
// gutter, right = records). Portrait keeps the previous
// layout where the image sits at the top of the right
// column in a body-level grid (the image is taller than
// the title there, so the alignment works out differently).
#let render-record-card(opts, branding, s, variant) = {
  let is-landscape = detect-landscape(opts)
  // Title is always left-aligned (round 34): the user asked
  // for portrait to match landscape's left-aligned name +
  // display-id. Centred portraits read as a different
  // document family from the landscape cards; left alignment
  // ties both modes to a single brand voice.
  let align-title = left
  let image-panel = render-image-panel(opts, s)

  if is-landscape {
    // Landscape layout (round 18): the title block spans the
    // full page width and drives the title-row height. The
    // body's left column (identity + service + household) is
    // a 50%-page-width block that starts right after the title
    // text so the label-value grids inside it use the column
    // width, not the page width. The right column (image +
    // records) is `place()`'d as a single floating block on
    // the right side, with the image at the top and records
    // just below it. Floating the right column keeps the
    // left column's top Y pinned to the title's bottom Y,
    // while the right column's top Y is pinned to the image's
    // top Y (the page's top). This satisfies all three
    // constraints: image at the title's Y, records just below
    // the image, and the body's left-column data not pushed
    // down by the image.
    render-title-block(s, align-title: align-title)
    let service-show-all = variant == "widow" or variant == "spouse"
    let household-show-all = variant == "widow" or variant == "spouse"
    // Left column: 50% page width minus half the gutter so the
    // block ends at 50% - 0.3cm, leaving a 0.6cm visual gap
    // before the right column's content begins.
    block(width: 50% - 0.3cm)[
      #render-identity-section(s)
      #v(theme.geometry.section_gap)
      #render-service-section(s, show-all: service-show-all)
      #v(theme.geometry.section_gap)
      #render-household-section(s, show-all: household-show-all)
    ]
    if image-panel != none {
      // Round 57: dynamic image height (same as portrait
      // branch). Image scales to column width, capped at
      // 60mm for very tall images. Landscape column is
      // wider (~4.75in) so the height cap fires more
      // often; a 3:2 landscape image at 4.75in wide
      // would be 3.17in (80mm) tall, so the 60mm cap
      // constrains it to ~60mm × 3.6in.
      let img-file = ""
      let images = s.at("images", default: ())
      let chosen = none
      for img in images {
        if img.at("is_primary", default: false) { chosen = img; break }
      }
      if chosen == none and images.len() > 0 { chosen = images.first() }
      if chosen != none { img-file = chosen.at("file_name", default: "") }
      place(
        top + right,
        dx: 0pt,
        dy: 6.4mm,
        block(width: 50% - 0.3cm)[
          #if img-file != "" [
            #align(center)[#image("/images/" + img-file, width: 100%, height: 60mm, fit: "contain")]
          ]
          #v(3mm)
          #set text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)
          #align(left)[#render-records-section(s, is-landscape: true)]
        ]
      )
    } else {
      // No image: render records in the right column, anchored
      // to the title's Y via `place(top + right)`. This matches
      // the with-image layout's right column position (image +
      // records just below) but without the image, so the
      // records section starts at the title's Y instead of
      // being pushed down by the household section's height.
      // The 50% - 0.3cm block width keeps the 0.6cm gutter
      // between left and right columns.
      place(
        top + right,
        dx: 0pt,
        dy: 6.4mm,
        block(width: 50% - 0.3cm)[
          #set text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)
          #align(left)[#render-records-section(s, is-landscape: true)]
        ]
      )
    }
    render-biography-page(s)
  } else {
    // Portrait layout: single-page by design (round 36). The
    // right column carries the image at the top with the
    // inline biography (using PDFExcerptOverride when set,
    // otherwise the full biography — Typst's block model
    // fits it) directly underneath. The dedicated full-bio
    // page from render-biography-page is intentionally NOT
    // called in portrait because a second page breaks the
    // single-page contract the user wants for portrait cards
    // (a portrait page is half a landscape page in vertical
    // space, so anything beyond a short bio would push the
    // layout over). Records render in the left col under
    // household with the portrait cap of 6; if the soldier
    // has more than 6 records the truncation footnote names
    // the count so the user knows to reorder Source Records.
    render-title-block(s, align-title: align-title)
    let service-show-all = variant == "widow" or variant == "spouse"
    let household-show-all = variant == "widow" or variant == "spouse"
    block(width: 50% - 0.3cm)[
      #render-identity-section(s)
      #v(theme.geometry.section_gap)
      #render-service-section(s, show-all: service-show-all)
      #v(theme.geometry.section_gap)
      #render-household-section(s, show-all: household-show-all)
      #v(theme.geometry.section_gap)
      #render-records-section(s, is-landscape: false)
    ]
    if image-panel != none {
      // Round 57: render the image with dynamic height
      // (width: 100%, height: 60mm, fit: contain) so
      // landscape-aspect images render shorter and the
      // bio starts closer to the image bottom. Tall
      // portrait images cap at 60mm. The old fixed
      // 50mm box left empty space under short images
      // that the user flagged as too much gap.
      let img-file = ""
      let images = s.at("images", default: ())
      let chosen = none
      for img in images {
        if img.at("is_primary", default: false) { chosen = img; break }
      }
      if chosen == none and images.len() > 0 { chosen = images.first() }
      if chosen != none { img-file = chosen.at("file_name", default: "") }
      place(
        top + right,
        dx: 0pt,
        dy: 6.4mm,
        block(width: 50% - 0.3cm)[
          #if img-file != "" [
            #align(center)[#image("/images/" + img-file, width: 100%, height: 60mm, fit: "contain")]
          ]
          #v(3mm)
          #align(left)[#render-biography-inline(s, is-landscape: false)]
        ]
      )
    } else {
      // No image: biography flows from the top of the right
      // column at the title's Y via `place(top + right)`.
      place(
        top + right,
        dx: 0pt,
        dy: 6.4mm,
        block(width: 50% - 0.3cm)[
          #align(left)[#render-biography-inline(s, is-landscape: false)]
        ]
      )
    }
  }
}
