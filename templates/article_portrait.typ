// metadata:
//   name: article_portrait
//   record_types: [article]
//   orientation: portrait
//   export_types: [record_card]
//   description: Standard Article Record card (portrait). Issue #321 v1.
//
// Renders one Article Record as a portrait PDF card. The body
// is the sanitized HTML from data["article"].body_html (already
// passed through goldmark + bluemonday on the Go side). The
// "Cited Person Records" block renders the resolve-refs output
// from data["resolved_refs"] (the App's pre-projection; the
// template does no DB lookups).
//
// Payload shape:
//   data["article"]       models.Article (Title, Subtitle, DisplayID,
//                          BodyHTML, CreatedAt, UpdatedAt)
//   data["resolved_refs"] array of dicts {display_id, name,
//                          resolved} -- one per in-body #person
//                          token, with resolved=true when the
//                          lookup succeeded (the App emits
//                          resolved=false entries for unknown
//                          tokens so the template can render the
//                          "Unknown" marker)
//   data["options"]       PDF options (orientation, etc.)
//   data["settings"]      PrintSettings from the export service
//   data["branding"]      map with archive_title, footer_text
//
// D6 precedence: every in-body token gets a row in the
// "Cited Person Records" block; a row with resolved=false
// renders as "⚠ Unknown: <display_id>" (fail-loud per locked
// decision #6).

#import "common/theme.typ"

#let data = read("data.json", encoding: none)
#let data = json(data)

#let a = data.at("article", default: none)
#let refs = data.at("resolved_refs", default: ())
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

// --- page setup ---

#let archive-title = if "archive_title" in branding {
  branding.archive_title
} else {
  "DixieData Article Record"
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
  paper: "us-letter",
  margin: (x: 1.5cm, y: 2cm),
  header: [#text(size: 9pt, fill: luma(120))[#archive-title]],
  footer: [#text(size: 8pt, fill: luma(120))[#footer-text #h(0.4em) #emph[#codename] #h(1fr) Page #counter(page).]],
)

// --- metadata ---

#let display-id = a.at("display_id", default: "ART-00000")

#block(
  fill: theme.palette.panel_fill,
  inset: 12pt,
  radius: 4pt,
  width: 100%,
)[
  #text(size: 11pt, weight: "bold", fill: theme.palette.text_primary)[
    ARTICLE RECORD
  ]
  #h(1fr)
  #text(size: 9pt, font: "DejaVu Sans Mono", fill: luma(80))[
    #display-id
  ]
]

#v(0.6em)

// --- title ---

#text(size: 22pt, weight: "bold")[#a.at("title", default: "Untitled")]

#if a.at("subtitle", default: "") != "" {
  v(0.3em)
  text(size: 12pt, fill: luma(80))[#a.at("subtitle", default: "")]
}

#v(0.8em)

// --- body ---

#let body-typst = a.at("body_typst", default: "")
#if body-typst != "" {
  set par(leading: 0.65em); set text(size: 11pt)
  // body_typst is a typst-markup string produced by
  // MarkdownRenderer.RenderTypst in the Go side. typst
  // treats a string as literal text in content mode;
  // eval(..., mode: "markup") parses it as typst markup
  // and emits the rendered result. See commit 6cb6e40
  // + the article body PDF bug for context.
  eval(body-typst, mode: "markup")
}

#v(1.2em)

// --- cited person records ---

#if refs.len() > 0 {
  block(
    fill: luma(245),
    inset: 10pt,
    radius: 4pt,
    width: 100%,
  )[
    #text(size: 10pt, weight: "bold")[Cited Person Records]
    #v(0.4em)
    #set text(size: 10pt)
    #for r in refs {
      let did = r.at("display_id", default: "")
      let name = r.at("name", default: "")
      let resolved = r.at("resolved", default: false)
      if resolved {
        [#text(fill: theme.palette.text_primary)[*#did*] #h(0.5em) #name \n]
      } else {
        [#text(fill: red)[⚠ Unknown: #did] \n]
      }
    }
  ]
}

#v(1fr)

// --- footer ---

#line(length: 100%, stroke: 0.5pt + luma(200))
#v(0.3em)
#text(size: 8pt, fill: luma(120))[
  Created: #a.at("created_at", default: "") #h(1em)
  Updated: #a.at("updated_at", default: "")
]