// Debug grid overlay for the PDF tuning iteration loop. Renders
// a 5mm square grid across the page body + a 1-inch ruler down
// the left edge + 5mm tick marks on the rulers, so a developer
// reading the PDF can give precise Y / X coordinates back to
// the agent ("move the image up to the 1.5 inch ruler mark").
//
// Activated by the export option `debugGrid: true`. The
// dixiedata-tune CLI accepts `--debug-grid` (issue #640 follow-
// up) which passes the flag through to the typst data payload.
//
// Implementation notes:
//   - The grid is rendered as a `place()` overlay anchored to
//     the page top-left so it covers the full body region
//     (margin to margin). The horizontal lines are drawn at
//     every 5mm using a `for` loop, the verticals the same.
//   - The ruler down the left edge is a column of numbered
//     inch marks (1in, 2in, 3in, ...), drawn in red so they
//     stand out against the 5mm grey grid.
//   - The 5mm ticks (small black marks) sit just inside the
//     left margin so the developer can read sub-inch coords
//     without having to count grid lines.
//   - The grid is intentionally UGLY (raw rgb, no theme) so
//     the developer can tell at a glance whether the flag is
//     on. The flag is off by default.
//
// Per-variant templates import this file and call
// `render-debug-grid(opts)` after the `#set page(...)` and
// `#set text(...)` lines. The call is a no-op when the flag
// is off, so production exports pay zero cost.

#let render-debug-grid(opts) = {
  if not opts.at("debugGrid", default: false) { return none }

  // Page dimensions mirror page-params. Kept inline (not
  // imported from page-params) to avoid coupling this debug
  // helper to the live page setup. If the page geometry
  // changes, update both.
  let is-landscape = {
    let o = str(opts.at("orientation", default: "L")).trim()
    o == "L" or o == "LANDSCAPE" or o == "l" or o == "landscape"
  }
  let page-width = if is-landscape { 11in } else { 8.5in }
  let page-height = if is-landscape { 8.5in } else { 11in }
  let margin-top = 0.4in
  let margin-bottom = 0.4in
  let margin-left = 0.63in
  let margin-right = 0.63in

  let body-width = page-width - margin-left - margin-right
  let body-height = page-height - margin-top - margin-bottom

  // 5mm grid lines.
  let grid-stroke = 0.2pt + gray
  let h-lines = ()
  let y = 0mm
  while y < body-height {
    h-lines.push(line(start: (0pt, y), end: (body-width, y), stroke: grid-stroke))
    y = y + 5mm
  }
  let v-lines = ()
  let x = 0mm
  while x < body-width {
    v-lines.push(line(start: (x, 0pt), end: (x, body-height), stroke: grid-stroke))
    x = x + 5mm
  }

  // Inch ruler down the left edge (in the body region, not the
  // margin — the body region is where the grid lives, and the
  // developer reads inch marks relative to the body's top).
  let inch-marks = ()
  let yi = 1in
  let n = 1
  while yi < body-height {
    inch-marks.push(
      place(
        top + left,
        dx: -2mm,
        dy: yi - 2.5mm,
        text(size: 7pt, fill: red, weight: "bold")[#str(n)in],
      )
    )
    n = n + 1
    yi = yi + 1in
  }

  // 5mm tick marks down the left edge (small black marks, no
  // label — the developer counts them for sub-inch coords).
  let ticks = ()
  let yt = 5mm
  while yt < body-height {
    ticks.push(
      place(
        top + left,
        dx: -1mm,
        dy: yt,
        line(start: (0pt, 0pt), end: (1.5mm, 0pt), stroke: 0.4pt + black),
      )
    )
    yt = yt + 5mm
  }

  place(
    top + left,
    dx: margin-left,
    dy: 0pt,
    block(width: body-width, height: body-height)[
      #for l in h-lines { l }
      #for l in v-lines { l }
    ],
  )
  for m in inch-marks { m }
  for t in ticks { t }
}
