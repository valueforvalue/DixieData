// metadata:
//   name: article_landscape
//   record_types: [article]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard Article Record card (landscape). Issue #321 v1.
//
// Mirror of article_portrait.typ for landscape orientation --
// two-column body layout for wider research bundles. The Cite-in
// panel sits on the right rail so the body has full width on
// the left.
//
// Payload shape: identical to article_portrait.typ.

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

#set page(
  paper: "a4",
  margin: (x: 1.5cm, y: 1.5cm),
  header: [#text(size: 9pt, fill: luma(120))[#archive-title]],
  footer: [#text(size: 8pt, fill: luma(120))[#footer-text #h(1fr) Page #counter(page).]],
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

// --- title (full width) ---

#text(size: 22pt, weight: "bold")[#a.at("title", default: "Untitled")]

#if a.at("subtitle", default: "") != "" {
  v(0.3em)
  text(size: 12pt, fill: luma(80))[#a.at("subtitle", default: "")]
}

#v(0.6em)

// --- two-column body + refs ---

#grid(
  columns: (1fr, 12cm),
  column-gutter: 1cm,
  [
    #let body-html = a.at("body_html", default: "")
    #if body-html != "" {
      set par(leading: 0.6em); set text(size: 10.5pt)
      raw(block: true, body-html)
    }
  ],
  [
    #block(
      fill: luma(245),
      inset: 10pt,
      radius: 4pt,
      width: 100%,
    )[
      #text(size: 10pt, weight: "bold")[Cited Person Records]
      #v(0.4em)
      #set text(size: 10pt)
      #if refs.len() == 0 {
        text(fill: luma(120))[No Person Record tokens in this article.]
      } else {
        for r in refs {
          let did = r.at("display_id", default: "")
          let name = r.at("name", default: "")
          let resolved = r.at("resolved", default: false)
          if resolved {
            [#text(fill: theme.palette.text_primary)[*#did*] #h(0.5em) #name \n]
          } else {
            [#text(fill: red)[⚠ Unknown: #did] \n]
          }
        }
      }
    ]
  ],
)

#v(1fr)

// --- footer ---

#line(length: 100%, stroke: 0.5pt + luma(200))
#v(0.3em)
#text(size: 8pt, fill: luma(120))[
  Created: #a.at("created_at", default: "") #h(1em)
  Updated: #a.at("updated_at", default: "")
]