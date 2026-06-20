// metadata:
//   name: soldier_landscape
//   record_types: [soldier]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard Soldier record card (landscape)
//
// The visual target is the legacy fpdf record card produced by
// pkg/render/pdf_helpers.go. The two-column record card with
// identity+service in the LEFT column, and image+household+biography
// in the RIGHT column. See docs/audit/layout-theming-findings.md
// Section 1.1 for the color and font literals being reproduced.
//
// Iterations applied (see tools/tune/compare/54_typst.md for the
// feedback loop that drove them):
//  - Title-cased entry type normalization
//  - Long-form date formatting ("May 22, 1844" not "05/22/1844")
//  - Records section
//  - Tighter vertical density
//  - Empty fields hidden
//  - LAYOUT: both Identity and Service in the LEFT column (the
//    original fpdf has them stacked; the previous Typst version
//    put them side-by-side, which was wrong)
//  - Household & Context + Biography in the RIGHT column
//  - Right column also has the optional image panel

#import "common/theme.typ"

#let data = read("data.json", encoding: none)
#let data = json(data)

#let s = data.at("soldier", default: none)
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

#set page(
  paper: "us-letter",
  // Match the fpdf path's margins: 16mm left/right, 28mm top,
  // 16mm bottom. fpdf's 28mm top is 0.4in less than the audit's
  // 0.75in top margin; the audit's value was a generalization.
  margin: (top: 0.4in, bottom: 0.4in, left: 0.63in, right: 0.63in),
  header: align(center, text(
    size: 10pt,
    weight: "bold",
    fill: theme.palette.text_primary,
  )[#branding.at("archive_title", default: "DixieData Archive")]),
  footer: if not opts.at("printerFriendly", default: false) {
    align(center, text(
      size: 8pt,
      fill: theme.palette.text_secondary,
    )[#branding.at("footer_text", default: "")])
  },
)

#set text(
  font: "Arial",
  // The fpdf path uses a 9pt body font; the audit's theme.typ
  // was at 8pt which made the output too small and crowded.
  size: 9pt,
  fill: theme.palette.text_primary,
)
#set par(leading: 0.45em)

// --- helpers ---

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

// long-date renders a date string as the long form ("May 22, 1844")
// or "Unknown" if any component is "00". The data layer uses "00"
// as a sentinel for unknown date parts. The fpdf path also emits
// "Unknown" in this case.
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

// title-cased entry type label. fpdf shows "Soldier" / "Wife" /
// "Widow" / "Linked Person" / "Person Record" / "Soldier" fallback.
// The raw data is lowercase. Match the fpdf helper.
#let entry-type-label(raw) = {
  let r = raw
  if r == none or r == "" [Soldier]
  else if r == "soldier" [Soldier]
  else if r == "wife" [Wife]
  else if r == "widow" [Widow]
  else if r == "linked_person" [Person Record]
  else [#title-case(r)]
}

// Strip a leading zero from a day, e.g. "09" -> "9". Used by the
// rank-out helper which has values like "Pvt." that don't need
// padding.
#let strip-leading-zeros(s) = {
  if s == none or s == "" [s]
  else if s.starts-with("0") and s.len() > 1 [#s.slice(1)]
  else [#s]
}

// label-value renders a single field row. Returns none if the
// value is blank so the caller can decide to skip the row. Uses
// a `block` to force each row onto its own line, matching the
// fpdf path's per-row layout.
#let label-value(label, value) = {
  if value == none { none }
  else if type(value) == str and value.trim() == "" { none }
  else [
    // block() forces a line break after the content so multiple
    // field-value pairs don't flow onto a single line.
    #block[
      *#label* #h(0.6cm) #value
    ]
  ]
}

#let field-row(label, value) = {
  let v = label-value(label, value)
  if v != none [#v]
}

// --- title block ---

#let entry-type-raw = s.at("entry_type", default: "")
#let display-id = s.at("display_id", default: "")
#let prefix = s.at("prefix", default: "")
#let first = s.at("first_name", default: "")
#let middle = s.at("middle_name", default: "")
#let last = s.at("last_name", default: "")
#let suffix = s.at("suffix", default: "")
#let name = {
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

#align(center, text(size: 14pt, weight: "bold")[
  #name
  #if suffix != "" [, #suffix]
])
#v(0.3em)
#align(center, text(size: theme.type-scale.body.size, fill: theme.palette.text_secondary)[
  #display-id - #entry-type-label(entry-type-raw)
])
#v(0.5em)

// --- two-column record card ---
// Mirrors the fpdf layout: BOTH identity and service stacked in
// the left column; image + household + biography + records in
// the right column.

#grid(
  columns: (1fr, 0.6cm, 1fr),
  [
    // === Left column: Identity + Service ===

    #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Identity & Vital Details]
    #v(0.4em)
    #set text(size: 8pt, fill: theme.palette.text_secondary)

    #field-row("Prefix", s.at("prefix", default: ""))
    #field-row("First Name", first)
    #field-row("Middle Name", middle)
    #field-row("Last Name", last)
    #field-row("Suffix", suffix)
    #field-row("Birth Date", long-date(s.at("birth_date", default: "")))
    #field-row("Death Date", long-date(s.at("death_date", default: "")))
    #field-row("Birth Info", s.at("birth_info", default: ""))
    #field-row("Buried In", s.at("buried_in", default: ""))

    #v(0.6em)
    #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Service & Archive Details]
    #v(0.4em)

    #field-row("Record Type", entry-type-label(entry-type-raw))
    #field-row("Rank In", s.at("rank_in", default: ""))
    #field-row("Rank Out", s.at("rank_out", default: ""))
    #field-row("Unit", s.at("unit", default: ""))
    #field-row("Pension State", s.at("pension_state", default: "N/A"))
    #field-row("Pension ID", s.at("pension_id", default: ""))
    #field-row("Application ID", s.at("application_id", default: ""))
    #field-row("Confederate Home Status", s.at("confederate_home_status", default: "N/A"))
    #field-row("Confederate Home Name", s.at("confederate_home_name", default: "N/A"))
  ],
  [],
  [
    // === Right column: image, household, biography, records ===

    #set text(size: 8pt, fill: theme.palette.text_primary)

    // Household & Context
    #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Household & Context]
    #v(0.4em)

    #field-row("Spouse", s.at("spouse_name", default: ""))
    #field-row("Maiden Name", s.at("maiden_name", default: ""))

    #v(0.5em)

    // Biography
    #let biography = s.at("biography", default: "")
    #if biography != none and biography.trim() != "" [
      #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Biography]
      #v(0.4em)
      #set text(size: 9pt)
      #biography
    ]

    #v(0.6em)

    // Records
    #let records = s.at("records", default: ())
    #if records.len() > 0 [
      #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Records]
      #v(0.4em)
      #set text(size: 9pt)
      #for r in records [
        *#r.at("record_type", default: "")* (App: #r.at("app_id", default: ""))
        #if r.at("details", default: "") != "" [
          \ #r.at("details", default: "")
        ]
        #v(0.2em)
      ]
    ]
  ],
)
