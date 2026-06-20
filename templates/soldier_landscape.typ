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

// Title block: soldier name, display ID, entry type
#let entry_type = s.at("entry_type", default: "")
#let display_id = s.at("display_id", default: "")
#let first = s.at("first_name", default: "")
#let middle = s.at("middle_name", default: "")
#let last = s.at("last_name", default: "")
#let name = {
  if first != "" and last != "" [#first #middle #last]
  else if last != "" [#last, #first]
  else if first != "" [#first]
  else [#display_id]
}

#align(center, text(size: 14pt, weight: "bold")[
  #name
])

#v(0.5em)
#align(center, text(size: theme.type-scale.body.size, fill: theme.palette.text_secondary)[
  #display_id - #entry_type
])
#v(0.5em)

// Two-column record card
#grid(
  columns: (1fr, 0.5cm, 1fr),
  [
    // Left column: Identity
    #text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent)[Identity & Vital Details]
    #v(0.3em)
    #set text(size: theme.type-scale.field_label.size, fill: theme.palette.text_secondary)
    #set par(leading: 0.4em)

    *Prefix* #h(1fr) #s.at("prefix", default: "")

    *First Name* #h(1fr) #first

    *Middle Name* #h(1fr) #middle

    *Last Name* #h(1fr) #last

    *Birth Date* #h(1fr) #s.at("birth_date", default: "Unknown")

    *Death Date* #h(1fr) #s.at("death_date", default: "Unknown")

    *Birth Info* #h(1fr) #s.at("birth_info", default: "")

    *Buried In* #h(1fr) #s.at("buried_in", default: "")

    #v(0.5em)
  ],
  [],
  [
    // Right column: Service & Archive Details
    #text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent)[Service & Archive Details]
    #v(0.3em)
    #set text(size: theme.type-scale.field_label.size, fill: theme.palette.text_secondary)
    #set par(leading: 0.4em)

    *Record Type* #h(1fr) #entry_type

    *Rank In* #h(1fr) #s.at("rank_in", default: "")

    *Rank Out* #h(1fr) #s.at("rank_out", default: "")

    *Unit* #h(1fr) #s.at("unit", default: "")

    *Pension State* #h(1fr) #s.at("pension_state", default: "N/A")

    *Pension ID* #h(1fr) #s.at("pension_id", default: "")

    *Application ID* #h(1fr) #s.at("application_id", default: "")

    *Confederate Home Status* #h(1fr) #s.at("confederate_home_status", default: "N/A")

    *Confederate Home Name* #h(1fr) #s.at("confederate_home_name", default: "N/A")

    #v(0.5em)
  ],
)
