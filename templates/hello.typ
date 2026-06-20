// metadata:
//   name: hello
//   record_types: [any]
//   orientation: any
//   export_types: [debug]
//   description: A trivial smoke-test template that prints "Hello <name>".
//
// Slice 1 of the Typst migration uses this template to prove the
// renderer pipeline end-to-end. Real record-card templates ship in
// slice 2.

#let data = read("data.json", encoding: none)
#let data = json(data)

#set page(
  paper: "us-letter",
  margin: (top: 0.75in, bottom: 0.75in, left: 0.75in, right: 0.75in),
)

#set text(
  font: "Libertinus Serif",
  size: 12pt,
  fill: rgb("#22303d"),
)

Hello, DixieData!

#let s = data.at("soldier", default: none)
#if s != none [
  Soldier: #s.display_id
]
