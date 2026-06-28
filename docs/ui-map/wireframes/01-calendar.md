# 01 — Calendar

- **Route**: `/calendar` and `/calendar/{m}` (GET, full page render)
- **Builder**: none (handler-direct URL)
- **Template**: `internal/templates/calendar.templ`
- **Layout**: both (relaxed default, splits to floating dock <1000px)
- **Owner**: package `templates`
- **Audit**: (link when round-N flags)

## Regions (relaxed mode)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ [Layout header — brand + pill-link nav, primary CTA]                     │
├──────────────────────────────────────────────────────────────────────────┤
│ Calendar                                                               h2│
│ Track soldier anniversaries alongside custom holidays and events.        │
│ ┌─Soldiers─┐ ┌─Spouse─┐ ┌─Person─┐                                       │
│ │   N      │ │   N    │ │   N    │  (counts chips, inline w/ heading)    │
│ └──────────┘ └────────┘ └────────┘                                       │
│ ┌──────── Month ─────────┐  ┌── Export Month ──[popout]──┐               │
│ │ <select> month-select  │  │ PDF / Print                │               │
│ └────────────────────────┘  │ Orientation [P|L]          │               │
│                             │ ☑ Printer-friendly         │               │
│                             │ <btn: Export Month PDF>    │               │
│                             └────────────────────────────┘               │
├──────────────────────────────────────────────────────────────────────────┤
│ [panel.calendar.quote]                                                  │
│ ROTATING LOCAL ARCHIVE QUOTE                                            │
│ "..."                                                       blockquote   │
│ — Author                                                                │
│ Advances every 3 soldiers added to the local archive                     │
├──────────────────────────────────────────────────────────────────────────┤
│ ┌────[panel.calendar.grid]───────┐ ┌──[#details-pane]──┐                 │
│ │ Month 20XX                     │ │ Select a day      │                 │
│ │ • Anniversaries • Events • ... │ │                  │                 │
│ │ ─────────────────────────────  │ │ Click any day...│                 │
│ │ Sun Mon Tue Wed Thu Fri Sat    │ │                  │                 │
│ │  ┌──┐┌──┐┌──┐┌──┐┌──┐┌──┐┌──┐  │ │                  │                 │
│ │  │ 1││ 2││ 3││ 4││ 5││ 6││ 7│  │ │                  │                 │
│ │  └──┘└──┘└──┘└──┘└──┘└──┘└──┘  │ │                  │                 │
│ │  ... day buttons, hx-get ...   │ │                  │                 │
│ │  Today ribbon on current day   │ │                  │                 │
│ │  Count markers (Holidays/Events/Anniv) per cell                         │
│ └────────────────────────────────┘ └────────────────────┘                │
├──────────────────────────────────────────────────────────────────────────┤
│ [Layout footer — build identity, optional debug button]                  │
└──────────────────────────────────────────────────────────────────────────┘
[Layout floating dock — Scratch Pad · Feedback · Menu]                     │
[Floating nav panel — same links as header, plus layout mode toggle]      │
[overlay.feedback.modal — opened via Feedback button]                      │
[overlay.jobs.progress — polled every 3s]                                  │
```

### Empty state (zero records)

When `counts.TotalRecords() == 0`, the grid is preceded by
`@partials.EmptyStateCard("calendar", counts)` (see [gaps.md](../gaps.md)
on the `partials` vs `components` redundancy).

```
┌──[panel.calendar.quote]──┐
│ ROTATING LOCAL ARCHIVE QUOTE                                                │
│ "..."                                                                       │
└─────────────────────────┘
┌──[EmptyStateCard(calendar)]──┐
│ No records yet — start by adding a Person Record.                          │
│ <btn: Add Person Record> → /soldiers/new                                   │
└──────────────────────────────┘
┌──[panel.calendar.grid]──┐
│  grid renders but with all cells empty                                     │
└─────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.calendar.quote` | Header band, dark slate | Quote text, author, rotation note |
| `panel.calendar.grid` | Card with header + grid | Month name, legend, weekday header, day buttons |
| `panel.calendar.details` | Right-side card (`#details-pane`) | Default: "Select a day"; HTMX swap target |

No tabs on this screen.

## Atomic components

- `Button` (`components/button.templ`) — "Export Month PDF" primary button.
- `Card` — wraps grid + details panels.
- `EmptyState` — see Empty state variant.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Day button | GET | `routebuilder.Anniversary(month, day)` | `#details-pane` | `innerHTML` | Loads `calendar_day.templ` |
| Export popout form | POST | `routebuilder.CalendarReportPDF(month)` | `this` | `none` | Triggers PDF download via `data-pdf-pref-scope="calendar"` |
| Layout jobs overlay | GET | `routebuilder.ActiveJobs()` | `data-jobs-progress-region` | `innerHTML` | `load, every 3s` |

Inline `onchange="window.location.href='/calendar/' + this.value"` on
the month select. Bare string URL — candidate for `routebuilder`
coverage. See [gaps.md](../gaps.md).

## Modals / overlays

- `overlay.floating.menu` — global, opened via Menu button.
- `overlay.feedback.modal` — global, opened via Feedback button.
- `overlay.jobs.progress` — global, polled.
- `overlay.print-config.modal` — global; this page's popout uses
  `data-pdf-pref-scope="calendar"` instead of triggering the global
  modal. Verify behavior matches.

## State variants

### Loading

Initial load is full HTML. No skeleton. Day-button click → HTMX swap
flashes stale `#details-pane` content briefly.

### Empty

`counts.TotalRecords() == 0` → `EmptyStateCard("calendar", counts)`
above grid. Quote band still renders.

### Error

HTMX swap target receives server error response inline. Toast region
captures top-level errors.

## Footguns

- **Month select URL is a string literal** —
  `window.location.href='/calendar/' + this.value` in `calendar.templ`.
  Renaming the route silently breaks month navigation. Should use
  `routebuilder.CalendarMonth(month)`. See
  [`docs/COMMON_BUGS.md`](../../COMMON_BUGS.md) § HTMX wiring.
- **Popout form uses `hx-target="this"` with `hx-swap="none"`** — relies
  on client-side PDF download trigger. Verify `data-pdf-pref-scope` is
  wired in `frontend/app.js`. Recent change in `docs/COMMON_BUGS.md`.
- **Today ribbon uses `aria-current="date"`** — verify goquery invariant
  test asserts the attribute presence on the current-day button only.
- **Day button `aria-label`** includes marker counts ("2 anniversaries,
  3 events, 1 holiday, press enter to load details"). Screen-reader
  friendly, but long. Test in [wireframes/_template.md](_template.md).
- **`#details-pane` is a string ID**, not a `uiids.*` constant. Should
  probably be `panel.calendar.details` (already exists in the
  Registry). Currently both are in play — verify the template uses
  the constant.
- **`isCurrentCalendarDay` reads `time.Now` via `calendarNow` var** —
  swapped in tests via `calendarNow` package var. If tests don't
  swap it, "today" assertions are flaky.

## Audit forward links

(Will be populated as `audit/reports-rN/` flags land. Format:
`<round>-<finding-id>: <one-liner> → audit/reports-rN/file.md`)

## See also

- [02-calendar-day.md](02-calendar-day.md) — fragment rendered into
  `#details-pane`. Will be combined or separate after pilot review.