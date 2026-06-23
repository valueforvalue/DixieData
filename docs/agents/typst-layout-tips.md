# Typst layout tips for the DixieData record card

Lessons learned from the single-soldier-landscape iteration loop
(rounds 4-21) on the typst-backed record card. Most of these
are typst-specific gotchas; a few are general principles for
working in this template.

## Grid cell alignment is `center + horizon` by default

`grid()` cells vertically and horizontally center their content.
This is rarely what you want for a layout where the title text
is short and an adjacent cell holds a tall image. The tall cell
forces the row to its height, the short cell is then vertically
centered, and the title ends up pushed down by half the row
height.

To pin a cell to the top:

```typst
grid(
  columns: (1fr, 0.6cm, 1fr),
  [
    #align(top + left)[#render-title-block(...)]
  ],
  [],
  [
    #align(top + right)[#image-panel]
  ],
)
```

The `align(top + left)` inside the cell content pins the
content to the top-left of the cell, overriding the default
center alignment. This is the single most common typst layout
fix in this template.

## `place()` decouples the visual position from the layout flow

If you need an element at a specific Y position that doesn't
match where it would naturally fall in the layout, `place()` is
the right tool. It positions the element absolutely relative to
its parent block, and (with `float: false`, the default) does
NOT contribute to the layout's flow. The element's size is
ignored when computing where the next in-flow element goes.

This is the trick for "image at the title's Y while the body
data is not pushed down by the image":

```typst
render-title-block(s, align-title: align-title)
if image-panel != none {
  place(
    top + right,
    dx: 0pt,
    dy: 0pt,
    block(width: 50%)[
      #align(center)[#image-panel]
      #v(3mm)
      #set text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)
      #align(left)[#render-records-section(s)]
    ]
  )
}
block(width: 50%)[
  #render-identity-section(s)
  ...
]
```

The image and records are `place()`'d at the page's top-right
and don't affect the in-flow body's top Y. The body's left
block starts right after the title.

## `place()` is relative to the parent block, not the page

When you call `place(top + right, ...)` from inside a function
that's later inserted into a typst document, the position is
relative to the *parent block* of the place() call. If you put
the place() right after `render-title-block(...)`, the parent
block is whatever frame the title block produces. If you put
it at the top of a function body, the parent is the function's
returned content, which the caller then positions.

The practical consequence: if you want the image anchored to
the page's top-right, put the `place()` at the very top of the
function (before any in-flow content), so its parent is the
function's top frame.

## `align()` on a `place()`'d block has surprising side effects

`block(width: 50%, align(top + right)[...])` puts the content
in the top-right of the block. But `align(top + right)` is
applied to the block's children and may horizontally align
text to the right (matching the position), making "Records" and
"Find a Grave" right-aligned within the right column.

Fix: don't put `align()` on the outer block. Use explicit
`#align(left)[...]` around the children that should be
left-aligned, and `#align(center)[...]` around the image.

```typst
// Wrong: align on the outer block affects all children
block(width: 50%, align(top + right)[
  #align(center)[#image-panel]
  #render-records-section(s)  // ends up right-aligned!
])

// Right: explicit align per content type
block(width: 50%)[
  #align(center)[#image-panel]
  #v(3mm)
  #set text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)
  #align(left)[#render-records-section(s)]
]
```

## `set text()` does not accept `align` as a parameter

```typst
#set text(size: 9pt, align: left)  // error: unexpected argument: align
```

Use `#align(left)[...]` to wrap content, or set `align` on the
parent `block()` or `grid()` instead.

## `box(width: 100%)` is how you force text to wrap

Typst's text box is sized to its natural content width by
default. Long text in a `1fr` grid cell will overflow past the
cell's right edge rather than wrapping. To force wrapping,
wrap the text in `box(width: 100%)`:

```typst
grid(
  columns: (32%, 1fr),
  [#text(size: 8pt, weight: "bold")[#label]],
  [#box(width: 100%)[#text(size: 9pt)[#value]]],
)
```

The `box(width: 100%)` sets the text's containing box to the
grid cell's allocated width. The text then wraps at word
boundaries within that width.

Apply the same to the records section's details line:

```typst
#if r.at("details", default: "") != "" [
  #linebreak()
  #box(width: 100%)[#text(size: 8pt)[#r.at("details", default: "")]]
]
```

## `calc.percent()` does not exist

Typst accepts `50%` directly in `width:` (it's a length, not a
function call). `calc.percent(50%)` raises a compile error.

```typst
// Wrong
block(width: calc.percent(50%) - 0.3cm)

// Right
block(width: 50% - 0.3cm)
```

The arithmetic works because `50%` is a length, and
`length - length` is a length.

## Half-column widths add up to 100%, with the gutter in the middle

A 2-column landscape layout on a Letter page with 0.75in
margins and a 0.6cm gutter has 684pt of usable width. Two
columns of 50% each = 342pt, plus the 0.6cm gutter = 359.4pt.
That's 359.4pt of layout in 684pt of space, leaving 324.6pt
unused.

To use the full width with a visible gutter, shrink each
column by half the gutter: `50% - 0.3cm` per column. Two
columns = 100% - 0.6cm, with a 0.6cm gap in the middle.

## `place()` + `block(width: %)` for image-and-text-on-floating-block

To put an image and a text section both in the same floating
right column (image on top, text below):

```typst
place(
  top + right,
  dx: 0pt,
  dy: 0pt,
  block(width: 50% - 0.3cm)[
    #align(center)[#image-panel]
    #v(3mm)
    #align(left)[#text-section]
  ],
)
```

The block's width constrains the image (which is in an
inner `align(center)`) and the text (which is in an inner
`align(left)`). The `v(3mm)` between them is the gap the user
asked for.

## Portrait-orientation source images look wrong in landscape boxes

The image source is 805x2000 (portrait). The 50mm landscape
panel clips it to a narrow strip, which is readable but not
photogenic. The 60mm height made it even narrower. The 50mm
height is a good compromise.

If a user has a landscape-orientation image source, the same
50mm height produces a wider, more readable image. The
panel height is a one-size-fits-all; consider per-image
aspect-ratio detection if the user wants different sizes per
record.

## Check the SVG, not just the rendered image, when the layout looks wrong

`mutool info` and the SVG `transform` matrix tell you exact
coordinates. If a column's content is at the wrong Y, the SVG
y-coordinates reveal which block's height is responsible. If
text is right-aligned when it shouldn't be, look for an
unexpected `align()` in the parent.

A 30-second SVG grep often saves an hour of typst trial-and-error.

## The page-top Y is page-margin-top + header-band-height

`place(top + right, dy: 0)` puts the element at the top of the
parent block. For a landscape page with 0.75in margins and a
header (with header rule), the "page top" for in-flow content
is roughly 50pt from the SVG canvas's top. The image will
start at that Y, not at Y=0. Use the rendered SVG to confirm
where things actually land; the math is more complex than it
looks.

## When the body block is `width: 100%`, the label-value grid uses the page width

```typst
block(width: 100%)[
  #render-identity-section(s)  // inside: grid(columns: (32%, 1fr), ...)
]
```

The 32% is 32% of the parent block's width. With a 100%-wide
parent, that's 32% of the page = ~250pt. The value column
ends up far to the right, leaving a huge empty space in the
middle.

Fix: constrain the body block to the column width you actually
want: `block(width: 50% - 0.3cm)[...]`.

## The `align(center)` on a child in a `place()`'d block doesn't override the block's own alignment

If the parent block has `align(top + right)`, the child's
`align(center)` is interpreted within the cell's own coordinate
space, not relative to the parent. Result: the child is
centered within the right cell, but the right cell's text is
right-aligned by the parent's `align(top + right)`. Drop
`align()` from the parent block and put explicit `#align()`s
on the children you care about.

## Don't trust the first attempt with `place()` and `align()`

Typst's `place()` and `align()` interact in non-obvious ways.
The first attempt usually puts the content at the right Y but
with the wrong alignment, or at the right alignment but the
wrong Y. Read the SVG after each attempt and adjust both the
position and the alignment. Two to three attempts is normal
for a new layout shape.

# Lessons from rounds 23-32 (post-rounds-4-to-21)

## `place()` accepts `align(left)`, `align(center)`, `align(right)`, `align(top)`, `align(bottom)`, `align(horizon)` as combined selectors

A second-argument `align()` on `place()` positions the
element within the parent. For an image+text block, you
typically want `place(top + right, ...)` to anchor the block
to the page's top-right. The inner content is then positioned
within that block via its own `align()` calls.

## `set text(...)` is a top-level directive, not an inline expression

You can write `#set text(size: 9pt)` at the start of a content
block and it cascades to everything inside. You cannot write
`#set text(size: 9pt); #render-records-section(s)` mid-flow
— typst parses that as a function-call expression. The right
pattern is:

```typst
#set text(size: 9pt)
#render-records-section(s)
```

Or for one-off sizing without polluting scope:

```typst
#text(size: 9pt)[...]
```

## `v(0.1em)` and `v(0.2em)` differences are sub-pixel at print

The user's PNG previews are 150 DPI; a typst `v(0.1em)` is
~1.3pt = ~3 pixels at 150 DPI. Going from 0.4em to 0.1em is
only ~8 pixels of difference — visible in the editor but
barely noticeable in print at Letter/8.5x11. If the user
wants a more impactful change, drop to 0em (zero leading)
or restructure the surrounding blocks. Always compare PNGs
side-by-side to see if your `v()` change actually changed
the visible output.

## `link()` URL length limit (~4000 chars) — guard with a length check

Typst refuses to encode very long URL annotations in the
PDF link metadata. A 5000-char URL passed to `#link()` will
fail with `error: URL is too long`. The fix is to check
`url.len() <= 4000` before wrapping in `link()`. Anything
longer falls through to plain text. This is a real issue for
test fixtures that put long strings in `details`; the
production data has shorter URLs.

## `link()` does not require `http://` / `https://`

Typst's `#link(url, [text])` accepts any string as the link
target. PDF viewers handle bare slugs and `file://` URIs on a
best-effort basis. For anniversary / record cards, the URLs
are always `https://` (findagrave, fold3, ancestry). A simple
`starts-with` check is enough to discriminate "real URL" from
"free-text details".

## `render-link` helper pattern: URL detection + fallback to plain text

When a `details` field can be either a URL or free text,
build a helper that detects and dispatches:

```typst
#let render-link(url) = {
  if url == none or url == "" { return }
  let s = url.trim()
  if not (s.starts-with("http://") or s.starts-with("https://")) {
    return text(size: 8pt, s)  // free text, not a URL
  }
  if s.len() > 4000 {
    return text(size: 8pt, s)  // typst's limit
  }
  link(s, text(fill: theme.palette.link)[#underline[Click to view]])
}
```

The fallback to plain text is the right behavior: a record
field that says "Filed in 1880. https://example.com/record."
isn't a URL even though it contains one, and a 5000-char
test-fixture string is not something the user can usefully
click. Both should render as text.

## Use the existing `palette.link` slot, even if it was unused

`templates/common/theme.typ` has a `palette.link` slot for
the link color. It was defined but unused (the old value was
a dark navy). Set the value to the color you want, then
reference it from typst via `theme.palette.link`. The slot
is already plumbed through to the typst runtime; you don't
need to extend the type.

## `v(0.15em)` (~1pt) between consecutive list items is the right gap

Bullet lists in typst at 7pt body are very tight by default.
A `v(0.15em)` between each item in a `for` loop adds a small
but visible gap. Don't overdo it — 0.3em starts to look like
the list is double-spaced.

## Adding a field to `models.Soldier` for the typst payload only

The `models.Soldier` struct has transient fields like
`SearchMatchField`, `SearchMatchSnippet`, `SpouseDisplayID`,
`BackLinkURL` with `json:"-"` so they don't appear in default
serializations. For fields that should be in the typst
payload but NOT in the appshell's other API responses, use
`json:"-"` (truly transient) or
`json:"linked_spouse_display_id,omitempty"` (visible to
typst but only when set, so appshell callers see nothing).

The risk with `json:"...,omitempty"` is that EVERY JSON
serialization of Soldier emits the field when non-empty,
including appshell views. Verify with grep that no existing
caller is doing string-matching on the JSON shape before
adding a new field.

## typst's text-box width defaults to natural content width

When you have a 1fr grid cell with `text(...)`, the text
overflows the cell if it's wider than the cell. To force
wrapping, wrap the text in `box(width: 100%)` so the
containing box matches the cell's allocated width. The
text then wraps at word boundaries. The same trick is
used in `record_card.typ`'s `label-value` and in the
records section's details line.

## `render-household-section` uses `show-all: false` for soldier, `true` for widow/spouse

The `show-all` parameter means "render even when blank" and
is the existing flag for distinguishing soldier from
widow/spouse variants. Reuse it for variant-conditional
rendering rather than adding a new `variant` parameter.
Example from round 31: the user wanted Rank In/Out/Unit
hidden in widow/wife. I wrapped those three `field-row` calls
in `if not show-all { ... }` rather than threading a new
`variant` param through.

## `render-biography-page` has its own duplicate of the title-row pattern

`render-biography-page` re-renders the soldier's name (large
serif) followed by the section header. It used to also
render a `display_id • entry_type • Full Biography` subtitle
between the name and the section. That subtitle was removed
in round 30 because the user thought it was metadata noise.
If you ever need to add or remove fields in the title row,
check both `render-title-block` and `render-biography-page` —
they have parallel structure and need parallel updates.

## Anniversary entry rendering: helper that wraps a single bullet

The anniversary template iterates `for s in soldiers` and
emits one bullet per entry. Each bullet needs to look up
a link from a Go-injected map and conditionally render as
`link(url, [text])` or plain text. Pattern:

```typst
#let render-anniversary-entry(s, soldier-links) = {
  let sid = str(s.at("id", default: 0))
  let url = soldier-links.at(sid, default: "")
  let entry-content = { /* name + display_id + year */ }
  if url != "" and (url.starts-with("http://") or url.starts-with("https://")) {
    if url.len() <= 4000 [
      - #link(url, text(fill: theme.palette.link, underline[#entry-content]))
    ] else [
      - #entry-content
    ]
  } else [
    - #entry-content
  ]
}
```

Pass `soldier-links` (the data payload map) into the
function. The lookup is per-entry and cheap.

## Injecting a per-record map into the typst data payload (Go side)

When typst needs a per-record lookup table that's not in the
soldier struct itself, inject it as a separate top-level
field in the data map. Pattern in
`internal/archive/export_service.go`:

```go
links, err := e.firstFindAGraveLinks(calendar)
data := map[string]any{
  "options":       normalizedOptions,
  "settings":      settings,
  "month":         month,
  "calendar":      calendar,
  "soldier_links": links,  // map[string]string
  "branding":      e.archiveBranding(...),
}
```

In the typst template, read it once at top level:

```typst
#let soldier-links = data.at("soldier_links", default: (:))
```

Then pass it to the per-entry helper. The `default: (:)`
matters: if the field is missing (e.g. a test fixture
that doesn't go through this code path), the template
falls back to an empty map and every entry renders as
plain text. No crash.

## One bulk query for N record lookups beats N individual queries

For the anniversary's first-Find-a-Grave-link-per-soldier
lookup, the natural code path is `for s in soldiers { getRecords(s.ID) }` — N+1 queries. A single bulk
`SELECT ... FROM records WHERE soldier_id IN (?, ?, ...)`
runs once and is O(1) database roundtrips. For 100+ records
in a calendar this is 2-3 orders of magnitude faster.

## A test fixture that omits a field is a silent regression trap

The exportcontract snapshot test fixture has soldier records
with empty `details` strings. My `render-link` change
preserved byte-for-byte output for those records (empty
details → no link rendered). Snapshots stayed stable. This
is good for the snapshot contract but bad for regression
coverage: a future bug that breaks link rendering for
records WITH details won't be caught by these tests. Add
fixtures that have details-with-URLs if you want that
coverage.

## `v(0.05em)` between groups inside a `for` loop

When iterating a list with sub-grouping (e.g. decade
buckets inside a day), typst's default trailing whitespace
after the last item in a group can compress things
uncomfortably. A `v(0.05em)` at the end of each group
iteration adds consistent breathing room. Pair with
`v(0.05em)` between iterations of the outer list if the
group spacing is also tight.

## The `compose-name` function: prefix + first + middle + last, with edge cases

The name composition has four branches based on which
fields are non-empty:

1. prefix + first + middle + last
2. first + middle + last
3. last, first (when only last + first exist, with a comma)
4. just first
5. just display_id

The suffix is appended separately by the caller
(`render-title-block` and `render-biography-page`),
prefixed with whatever separator the caller chooses
(comma for the standard American style, single space
for the round-32 simpler style).

The fallback `last, first` is for records that have only
those two fields (e.g. imported records that lost the
middle name). The comma here is data-driven, not a style
choice — it disambiguates "John Smith" (first=John, last=Smith)
from "John, Smith" (a first name that's a comma-separated list
of "John, Smith" as one name).

## `align(align-title, ...)` in `render-title-block` lets the variant pick left or center

`render-title-block` takes an `align-title` parameter:
- landscape → `left` (the title sits at the page's top-left,
  in line with the body's left column)
- portrait → `center` (the title is centered above the
  portrait card, which is narrower than the page)

Don't hardcode the alignment in `render-title-block`;
let the caller decide based on orientation.
