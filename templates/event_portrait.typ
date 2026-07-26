// metadata:
//   name: event_portrait
//   record_types: [event]
//   orientation: portrait
//   export_types: [record_card]
//   description: Standard Event Record card (portrait). Issue #374
//                 side-issue to #320; mirrors event_landscape.typ
//                 so the Event detail page's per-export orientation
//                 picker can route portrait requests to a portrait
//                 template. Layout adapts the linked-Person-Records
//                 table to a narrower page (single-column stack
//                 when portrait, table when landscape).
//
// All real layout lives in this file. The template receives a
// per-Event payload via data.json (the per-record single-template
// path) and emits a single-page Event Record card. No images in
// v1; #320.6 (Event images gallery) will add the image staging
// hook later.
//
// Payload shape:
//   data["soldier"]     models.Soldier (EntryType=event); carries
//                       Kind, BeginDate, EndDate, Description,
//                       PDFExcerptOverride, Notes, DisplayID
//   data["linked"]      array of dicts {display_id, name, range}
//                       (the app-side pre-projection — the
//                        template does no DB lookups)
//   data["options"]     PDF options (orientation, etc.)
//   data["settings"]    PrintSettings from the export service
//   data["branding"]    map with archive_title, footer_text
//
// D3 precedence: PDFExcerptOverride takes precedence over
// Description when set. If both are empty the description block
// is omitted entirely.
//
// Portrait-vs-landscape divergence: the landscape template puts
// the header + description + linked-records table on a single
// wide page. The portrait template stacks the header above a
// narrower linked-records table that wraps the 3-cols when the
// page is too narrow (column widths adjusted). The body text
// uses 11pt to fit the 6.5" portrait page width.

#import "common/theme.typ"

#let data = read("data.json", encoding: none)
#let data = json(data)

#let s = data.at("soldier", default: none)
#let linked = data.at("linked", default: ())
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

// --- page setup (portrait: narrower margins, no wide header rule) ---

#let archive-title = if "archive_title" in branding {
  branding.archive_title
} else {
  "DixieData Event Record"
}

#let footer-text = if "footer_text" in branding {
  branding.footer_text
} else {
  "Made with DixieData"
}

// Issue #489: codename is now its own field on the branding
// map so the footer can italicize it. The plain footer-text
// stays unchanged for PDF text-extraction tests; the codename
// is appended with #emph() for the visual italics.
#let codename = if "codename" in branding {
  branding.codename
} else {
  ""
}

#set page(
  fill: rgb("#F4F4F4"),
  paper: "us-letter",
  margin: (
    top: 0.6in, bottom: 0.6in, left: 0.6in, right: 0.6in,
  ),
  header: align(right, text(
    size: 8pt, fill: theme.palette.text_muted,
    font: "Arial",
  )[ #archive-title ]),
  footer: align(center, text(
    size: 7pt, fill: theme.palette.text_muted,
    font: "Arial",
  )[
    #footer-text #h(0.4em) #emph[#codename]
  ]),
)

#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.5em, justify: true)

// --- helpers ---

#let month-names = (
  "01": "January", "02": "February", "03": "March",
  "04": "April",   "05": "May",      "06": "June",
  "07": "July",    "08": "August",   "09": "September",
  "10": "October", "11": "November", "12": "December",
)

#let fmt-date(s) = {
  if s == none or s == "" {
    return [Unknown]
  }
  if s == "Unknown" or s == "0000-00-00" {
    return [Unknown]
  }
  let parts = s.split("/")
  if parts.len() != 3 {
    return [Unknown]
  }
  let m = parts.at(0)
  let d = parts.at(1)
  let y = parts.at(2)
  if m == "00" and d == "00" and y == "00" {
    return [Unknown]
  }
  if m == "00" and d == "00" {
    return [#y]
  }
  if m == "00" {
    return [#y]
  }
  if d == "00" {
    return [#month-names.at(m, default: m) #y]
  }
  [#month-names.at(m, default: m) #d, #y]
}

#let date-range(begin, end) = {
  if begin == none or begin == "" {
    if end == none or end == "" {
      [Date unknown]
    } else {
      [until #fmt-date(end)]
    }
  } else {
    if end == none or end == "" {
      [#fmt-date(begin) (ongoing)]
    } else {
      [#fmt-date(begin) — #fmt-date(end)]
    }
  }
}

#let kind-pill(kind) = {
  if kind == none or str(kind).trim() == "" {
    none
  } else {
    box(
      inset: (x: 8pt, y: 3pt),
      outset: (y: 2pt),
      fill: theme.palette.panel_fill,
      stroke: 0.5pt + theme.palette.divider,
      radius: 4pt,
      text(
        size: 8pt,
        weight: "bold",
        fill: theme.palette.accent,
        font: "Arial",
      )[#{ upper(str(kind)) }],
    )
  }
}

// --- header ---

#align(left)[
  #text(size: 8pt, fill: theme.palette.text_muted, font: "Arial")[
    EVENT RECORD
  ]
]

#v(0.3em)

#text(size: 16pt, weight: "bold", fill: theme.palette.text_primary, font: "Arial")[
  #{
    if s == none {
      [Unknown Event]
    } else {
      let kind = str(s.at("kind", default: "")).trim()
      if kind != "" {
        [#kind]
      } else {
        [Unnamed Event]
      }
    }
  }
]

#v(0.15em)

#let display-id = if s == none [??] else { str(s.at("display_id", default: "??")).trim() }
#text(size: 9pt, fill: theme.palette.text_secondary, font: "Arial")[
  Display ID: #display-id
]

#v(0.25em)

#{
  if s != none {
    let kind = str(s.at("kind", default: "")).trim()
    if kind != "" {
      kind-pill(kind)
      h(0.5em)
    }
    let begin = s.at("begin_date", default: "")
    let end = s.at("end_date", default: "")
    text(size: 9pt, fill: theme.palette.text_secondary, font: "Arial")[
      #date-range(begin, end)
    ]
  }
}

#v(0.8em)

// --- body: description (D3 precedence) ---

#{
  let description = if s == none [none] else { s.at("description", default: "") }
  let override = if s == none [none] else { s.at("pdf_excerpt_override", default: "") }
  let ov-trim = str(override).trim()
  let desc-trim = str(description).trim()

  if ov-trim != "" or desc-trim != "" {
    line(length: 100%, stroke: 0.5pt + theme.palette.divider)
    v(0.3em)

    if ov-trim != "" {
      box(
        inset: (x: 8pt, y: 6pt),
        fill: theme.palette.panel_fill,
        stroke: 0.5pt + theme.palette.divider,
        radius: 4pt,
        width: 100%,
      )[
        #text(size: 7pt, weight: "bold", fill: theme.palette.accent, font: "Arial")[
          PDF EXCERPT OVERRIDE
        ]
        #v(0.2em)
        #text(size: 9pt, fill: theme.palette.text_primary, font: "Arial")[
          #ov-trim
        ]
      ]
      v(0.5em)
    }

    let effective = if ov-trim != "" [#ov-trim] else [#desc-trim]
    text(size: 10pt, fill: theme.palette.text_primary, font: "Arial")[
      #effective
    ]

    v(0.5em)
  }
}

// --- body: internal notes (separate from public description) ---

#{
  let notes = if s == none [none] else { s.at("notes", default: "") }
  let notes-trim = str(notes).trim()
  if s != none and notes-trim != "" {
    line(length: 100%, stroke: 0.5pt + theme.palette.divider)
    v(0.3em)
    text(size: 7pt, weight: "bold", fill: theme.palette.text_muted, font: "Arial")[Internal Notes]
    v(0.2em)
    text(size: 8pt, fill: theme.palette.text_secondary, font: "Arial")[
      #notes-trim
    ]
    v(0.5em)
  }
}

// --- body: linked Person Records (D5 = 3-col table, portrait-tweaked) ---

#let linked-count = linked.len()
#if linked-count > 0 {
  line(length: 100%, stroke: 0.5pt + theme.palette.divider)
  v(0.3em)
  text(size: 7pt, weight: "bold", fill: theme.palette.text_muted, font: "Arial")[
    LINKED PERSON RECORDS
  ]
  v(0.3em)

  // D5: 3-col flat table — Display ID | Name | Dates. Portrait
  // page is narrower than landscape, so the name column is the
  // flexible one and the ID/dates columns shrink to fit.
  let row-data = linked.map(p => (
    str(p.at("display_id", default: "??")),
    str(p.at("name", default: "")),
    str(p.at("range", default: "")),
  ))

  let header = ([
    #text(size: 7pt, weight: "bold", font: "Arial")[DISPLAY ID]
  ], [
    #text(size: 7pt, weight: "bold", font: "Arial")[NAME]
  ], [
    #text(size: 7pt, weight: "bold", font: "Arial")[DATES]
  ])

  let body-rows = row-data.map(row => (
    [#text(size: 7.5pt, font: "Arial")[#{row.at(0)}]],
    [#text(size: 7.5pt, font: "Arial")[#{row.at(1)}]],
    [#text(size: 7.5pt, font: "Arial")[#{row.at(2)}]],
  ))

  table(
    columns: (auto, 1fr, auto),
    stroke: 0.4pt + theme.palette.divider,
    inset: 5pt,
    fill: (col, row) => if calc.even(row) { white } else { theme.palette.panel_fill },
    align: (left, left, left),
    ..header,
    ..body-rows.flatten()
  )
} else {
  line(length: 100%, stroke: 0.5pt + theme.palette.divider)
  v(0.3em)
  text(size: 7pt, weight: "bold", fill: theme.palette.text_muted, font: "Arial")[Linked Person Records]
  v(0.2em)
  text(size: 8pt, fill: theme.palette.text_muted, font: "Arial")[
    No Person Records are linked to this Event yet. Attach this event to a Person Record from the Person Record detail page's Events tab.
  ]
}