// metadata:
//   name: soldier_landscape
//   record_types: [soldier]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard Soldier record card (landscape)
//
// The visual target is the legacy fpdf record card produced by
// internal/archive/pdf_layout.go. The two-column record card with
// identity fields on the left and service/archive details on the
// right. See docs/audit/layout-theming-findings.md Section 1.1 for
// the color and font literals being reproduced.
//
// Iterations applied (see tools/tune/compare/54_typst.md for the
// feedback loop that drove them):
//  - Title-cased entry type normalization
//  - Long-form date formatting ("May 22, 1844" not "05/22/1844")
//  - Source Records section in the right column
//  - Tighter vertical density (matches the fpdf's compact layout)

#import "common/theme.typ"

#let data = read("data.json", encoding: none)
#let data = json(data)

#let s = data.at("soldier", default: none)
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

#set page(
  paper: "us-letter",
  margin: theme.geometry.page_margin,
  header: align(center, text(
    size: theme.type-scale.header.size,
    weight: "bold",
    fill: theme.palette.text_primary,
  )[#branding.at("archive_title", default: "DixieData Archive")]),
  footer: if not opts.at("printerFriendly", default: false) {
    align(center, text(
      size: theme.type-scale.footer.size,
      fill: theme.palette.text_secondary,
    )[#branding.at("footer_text", default: "")])
  },
)

#set text(
  font: "Arial",
  size: theme.type-scale.body.size,
  fill: theme.palette.text_primary,
)
#set par(leading: 0.55em)

// --- helpers ---

// title-case normalizes the entry type. fpdf shows "Soldier" not
// "soldier"; the raw data is lowercase. This is the same logic
// the fpdf helper does.
#let title-case(s) = {
  if s == none or s == "" [#s]
  else [
    #let first-char = s.at(0)
    #let rest = s.clusters().slice(1).join("")
    #upper(first-char)#lower(rest)
  ]
}

// Long-form date formatter. fpdf renders dates like "May 22, 1844".
// The data layer stores ISO-like "1844-05-22" or "05/22/1844".
// We parse the year out of any input and look up the month name.
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

#let long-date(s) = {
  if s == none or s == "" [Unknown]
  else if s == "Unknown" [Unknown]
  else {
    let parts = s.split("/")
    if parts.len() == 3 [
      // MM/DD/YYYY format
      #let month-idx = parts.at(0)
      // Strip leading zeros from the day (e.g. "09" -> "9") to
      // match the fpdf output style ("February 9, 1926" not
      // "February 09, 1926").
      #let day = if parts.at(1).starts-with("0") and parts.at(1).len() > 1 {
        parts.at(1).slice(1)
      } else { parts.at(1) }
      #let year = parts.at(2)
      #let month-name = month-names.at(month-idx, default: month-idx)
      #month-name #day, #year
    ] else if s.contains("-") and s.len() >= 10 [
      // YYYY-MM-DD or partial
      #let year = s.split("-").at(0)
      #year
    ] else [#s]
  }
}

// label-value renders a single field row: bold label, value to
// the right. If the value is empty or N/A, returns empty content
// (matches the fpdf behavior of skipping empty fields). Returns
// content() so the caller can decide whether to render the
// result or not.
#let label-value(label, value) = {
  if value == none { none }
  else if type(value) == str and value.trim() == "" { none }
  else [
    *#label* #h(0.5cm) #value
  ]
}

// --- title block ---

#let entry-type-raw = s.at("entry_type", default: "")
#let display-id = s.at("display_id", default: "")
#let first = s.at("first_name", default: "")
#let middle = s.at("middle_name", default: "")
#let last = s.at("last_name", default: "")
#let name = {
  if first != "" and last != "" [#first #middle #last]
  else if last != "" [#last, #first]
  else if first != "" [#first]
  else [#display-id]
}

#align(center, text(size: 14pt, weight: "bold")[
  #name
])
#v(0.3em)
#align(center, text(size: theme.type-scale.body.size, fill: theme.palette.text_secondary)[
  #display-id - #title-case(entry-type-raw)
])
#v(0.5em)

// --- two-column record card ---

#grid(
  columns: (1fr, 0.6cm, 1fr),
  [
    // Left column: Identity
    #text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent)[Identity & Vital Details]
    #v(0.4em)
    #set text(size: theme.type-scale.field_label.size, fill: theme.palette.text_secondary)

    #let prefix = label-value("Prefix", s.at("prefix", default: ""))
    #if prefix != none [#prefix]
    #let first-name = label-value("First Name", first)
    #if first-name != none [#first-name]
    #let middle-name = label-value("Middle Name", middle)
    #if middle-name != none [#middle-name]
    #let last-name = label-value("Last Name", last)
    #if last-name != none [#last-name]
    #let birth-date = label-value("Birth Date", long-date(s.at("birth_date", default: "")))
    #if birth-date != none [#birth-date]
    #let death-date = label-value("Death Date", long-date(s.at("death_date", default: "")))
    #if death-date != none [#death-date]
    #let birth-info = label-value("Birth Info", s.at("birth_info", default: ""))
    #if birth-info != none [#birth-info]
    #let buried-in = label-value("Buried In", s.at("buried_in", default: ""))
    #if buried-in != none [#buried-in]
  ],
  [],
  [
    // Right column: Service & Archive Details
    #text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent)[Service & Archive Details]
    #v(0.4em)
    #set text(size: theme.type-scale.field_label.size, fill: theme.palette.text_secondary)

    #let record-type = label-value("Record Type", title-case(entry-type-raw))
    #if record-type != none [#record-type]
    #let rank-in = label-value("Rank In", s.at("rank_in", default: ""))
    #if rank-in != none [#rank-in]
    #let rank-out = label-value("Rank Out", s.at("rank_out", default: ""))
    #if rank-out != none [#rank-out]
    #let unit = label-value("Unit", s.at("unit", default: ""))
    #if unit != none [#unit]
    #let pension-state = label-value("Pension State", s.at("pension_state", default: ""))
    #if pension-state != none [#pension-state]
    #let pension-id = label-value("Pension ID", s.at("pension_id", default: ""))
    #if pension-id != none [#pension-id]
    #let application-id = label-value("Application ID", s.at("application_id", default: ""))
    #if application-id != none [#application-id]
    #let confederate-status = label-value("Confederate Home Status", s.at("confederate_home_status", default: ""))
    #if confederate-status != none [#confederate-status]
    #let confederate-name = label-value("Confederate Home Name", s.at("confederate_home_name", default: ""))
    #if confederate-name != none [#confederate-name]
  ],
)

// --- source records section ---

#let records = s.at("records", default: ())
#if records.len() > 0 [
  #v(0.5em)
  #text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent)[Records]
  #v(0.3em)
  #set text(size: theme.type-scale.field_label.size, fill: theme.palette.text_primary)
  #for r in records [
    *#r.at("record_type", default: "")* (App: #r.at("app_id", default: ""))
    #if r.at("details", default: "") != "" [
      \ #r.at("details", default: "")
    ]
    #v(0.2em)
  ]
]
