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
