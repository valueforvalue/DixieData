# 22 — Initial Setup

- **Route**: `/setup` (GET, POST)
- **Builder**: none (form action `/setup` literal)
- **Template**: `internal/templates/initial_setup.templ:InitialSetupView`
- **Owner**: package `templates`

## Regions

```
┌── First Launch Setup ────────────────────────────────────────────┐
│ h2 "First Launch Setup"                                           │
│ [Card]                                                            │
│  intro copy (explains DisplayID prefix derives from name + year) │
│  if errorMessage: red callout                                     │
│  <form method=post action=/setup>                                │
│    First Name | Middle Name | Last Name | Birth Year             │
│    if PrefixPreview: [preview card] "New ID prefix: XXX"          │
│    <btn: Save Identity>                                           │
└───────────────────────────────────────────────────────────────────┘
```

Shown only when `setupRequired` is true (redirect from
`internal/appshell/lifecycle.go`).

## Panels / tabs

`page.setup` registered. No inner panels.

## Atomic components

- `Button` — Save Identity.
- `Card` — wrapper.
- `Field` — every input.

## HTMX wiring

None. Native `<form method="post" action="/setup">` — full page
submit.

## State variants

- **Validation error**: red callout at top.
- **Prefix preview**: shown when name + year produce a valid prefix.

## Footguns

- **No routebuilder** for `/setup` (literal action). Acceptable since
  this is a single-use page.
- **`inputmode="numeric" maxlength="4"`** on birth year — verify
  server-side validation matches.
- **`autocomplete` attributes** are present — verify behavior in
  Wails webview (autofill may not work).

## See also

- [21-settings.md](21-settings.md) (Initialize Data is the destructive
  sibling)