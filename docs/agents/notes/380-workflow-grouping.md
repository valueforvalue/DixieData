# #380 — Top-Nav Workflow Grouping Analysis

**Scope.** Recon of current 11 top-nav destinations (10 pills + 1 CTA + 2 foldouts), mapped to user workflows via `internal/templates/layout.templ:115-176` (nav) and `internal/templates/layout.templ:224-238` (floating-dock Quick Nav). Route inventory cross-checked against `internal/appshell/routes.go:34-387`. Calendar stays a flat pill (home page per user direction).

## 1. Current nav, confirmed

Order on disk (`layout.templ:115-176`):

| # | Pill / trigger | href | Type | Verb (what user does here) |
|---|---|---|---|---|
| 1 | Calendar | `/calendar` | pill | Browse anniversaries by month |
| 2 | Search/Quick View | `/soldiers` | pill | Type-and-find a soldier record |
| 3 | Browse | `/browse` | pill | Scroll/tag-filter the full roster |
| 4 | Events | `/events` | pill | Browse event records (battles, units) |
| 5 | Articles | `/articles` | pill | Read long-form research write-ups |
| 6 | Insights | `/insights` | pill | View archive analytics dashboards |
| 7 | Research & Review | `FoldoutWithBadge` → `/review-queue`, `/research-picker?next=timeline`, `?next=research-log`, `/research-collections`, `/research` | foldout (5 items, badge wired) | Resolve review findings; jump to per-person research tools |
| 8 | Share | `Foldout` → `/share/exports`, `/share/imports`, `/share/queue`, `/share/sync` | foldout (4 items) | Export, import, queue, sync the archive |
| 9 | Tags | `/tags` | pill | Manage taxonomy |
| 10 | Settings | `/settings` | pill | Configure app + run scans/updates |
| 11 | Add Person Record | `/soldiers/new` | CTA | Create a new person record |

**Floating-dock Quick Nav** (`layout.templ:224-238`) currently renders 9 flat pills (Calendar, Search, Browse, Review Queue, Insights, Share, Tags, Settings, Add). Note asymmetry: Events, Articles, and all R&R-foldout sub-items are absent; Share is a flat pill here even though it's a foldout up top.

## 2. Workflow clusters

### Cluster A — "Find records" (Search / Browse / Events / Articles)

- **Search** (`/soldiers`, `entry_form.templ:835 SearchResults`) — type-to-find single soldier.
- **Browse** (`/browse`, `browse.templ:16 BrowseView`) — paginated roster + tag filters + bulk-tag (`routes.go:88 /browse/bulk-tag`).
- **Events** (`/events`, `event_list.templ:15 EventList`) — list of event records, link-back to soldiers.
- **Articles** (`/articles`, `articles.templ:17 ArticlesListShell`) — long-form write-ups; cross-link soldiers via `/articles/{id}/refs`.

**Verb**: *find / read*. **Verdict:** 4 entry points to overlapping "records family" surfaces. They are NOT chained (browse→record→review is the chain, not browse→events). Each surfaces a different record *type*: person vs. event vs. article. They ARE used together in a research sitting (Search for the man, Events for the battle, Articles for the narrative). Two options:

- **One flat "Find" pill with a dropdown** of Search / Browse / Events / Articles
- **Keep all four flat**

Visual cost of folding: Browse is the second-most-used destination (default landing for tag-filter workflows per `routes.go:88`); folding it hides it behind two clicks for a high-frequency surface.

### Cluster B — "Review & enrich" (Research & Review foldout + Insights)

- R&R foldout post-#455 already folds Review Queue, Timeline, Research Log, Research Collections, Change Person (`layout.templ:131-167`). The badge wire at `/layout/review-count` (`routes.go:323`) proves the foldout is mission-critical for triage.
- Insights (`/insights`, `insights.templ:15 InsightsView`) — analytics dashboard, exports as PDF (`routes.go:334 /insights/report/pdf`); run-duplicates-audit action at `/insights/audit/duplicates` (`routes.go:286`). **Analytics, not review**: read-only summary, drilldown at `/insights/drilldown`.

**Verdict:** R&R foldout stays as is. Insights is a different verb (analyze) — but **adjacent in data model** (review queue surfaces findings that insights aggregates). Candidate to fold into the R&R foldout.

### Cluster C — "Taxonomy" (Tags)

- `/tags` (`tags.templ:23 TagsManagementPage`) — CRUD tags, view members. Used from Browse (bulk-tag), from soldier detail (attach/detach at `routes.go:65-67`), and from Event panels (`event_panels.templ:133`).
- **Verdict: standalone.** Folding Tags into a Records group buries a low-frequency admin surface behind two clicks for a function that is itself a one-click action. Keep flat.

### Cluster D — "Export/Share" (Share foldout + `/jobs/{id}`)

- Share foldout already exists. `/jobs/{id}` (`routes.go:43 r.Get("/jobs/*", a.handleJobStatus)`) is a **redirect target**, not a primary entry — every export action returns `X-DixieData-Redirect: /jobs/{id}` (see `app.go:319`, `app.go:1215`, `exports_handlers.go:277/318`, `google_handlers.go:81`). Users land on `/jobs/{id}` after an action; they don't navigate to it from the nav.
- `/share` landing (`share.templ:25 ShareView`, post-#284 slimmed to a sub-overview with 4 Quick Action tiles linking to the 4 subpages).
- **Verdict: Share foldout stays as is.** `/jobs/{id}` belongs nowhere in the nav — it's a job result surface served by the persistent `/jobs/active` overlay (`layout.templ:99-107`, every-3s polling).

### Cluster E — "Configure" (Settings + `/debug`)

- `/debug/*` (`routes.go:379-387`) is gated by debug mode toggle (`routes.go:381 /settings/debug-mode` POST); only visible when user opted in. Not in current nav.
- **Verdict: Settings stays flat.** `/debug` is debug-only — don't promote it; the dev page badge (`components/dev_page_badge.templ:28`) is already the witness surface.

## 3. Fold vs. keep-flat decision matrix

| Destination | Frequency | Chained with? | Fold? | Reasoning |
|---|---|---|---|---|
| Calendar | very high (home) | n/a | **flat** | Required standalone |
| Search | very high | Browse, Review | **flat** | Daily entry point |
| Browse | very high | Search, Review | **flat** | Default tag-filter landing |
| Events | medium | Articles, Research | **flat** | Distinct record type, not chained |
| Articles | medium | Events | **flat** | Distinct record type, not chained |
| Insights | medium | (standalone) | **fold candidate** | Only analytics surface; adjacent to R&R data model |
| Research & Review | high (badge-driven) | (folds 5 items) | **foldout** | Already folded |
| Share | low–medium | (folds 4 items) | **foldout** | Already folded |
| Tags | low | Browse, soldier-edit | **flat** | Single-function admin, no cluster candidate |
| Settings | low | (admin only) | **flat** | No siblings to fold with |
| Add Person Record | medium | (standalone CTA) | **flat** | Primary CTA, stays prominent |

## 4. Three candidate shapes (target: 6–8 visible items)

### Shape 1 — "Minimal fold" (10 visible)

```
[ Calendar ] [ Search ] [ Browse ] [ Events ] [ Articles ] [ Insights ] [ Tags ] [ Settings ]
[ v Research & Review (5) ] [ v Share ]                  [ + Add Person Record ]
```

Saves 1 pill. **Recommend against** — high churn for tiny win, doesn't fix the real bloat.

### Shape 2 — "Find group" (8 visible)

```
[ Calendar ] [ v Find ] [ Insights ] [ Tags ] [ Settings ]
[ v Research & Review (5) ] [ v Share ]                  [ + Add Person Record ]
```

**Find foldout**: Search, Browse, Events, Articles. Saves 3 pills. Risk: Browse is the most-used destination behind Calendar; one extra click hurts the tag-filter workflow. **Recommend only if a /browse shortcut is added to the floating-dock Menu** (currently missing — see §5).

### Shape 2.5 — "Absorb Insights into R&R foldout" (9 visible — recommended)

```
[ Calendar ] [ Search ] [ Browse ] [ Events ] [ Articles ] [ Tags ] [ Settings ]
[ v Research & Review (6) ] [ v Share ]                  [ + Add Person Record ]
```

7 pills + 2 foldouts + 1 CTA = **9 visible**. Trade Insights (low-frequency analysis) for one pill saved; Insights becomes a sub-item inside the R&R foldout (it complements review — same data, different lens).

R&R foldout contents after change:

```
v Research & Review (6)
   |-- Open Review Queue          (/review-queue)              <- badge-driven
   |-- Open Timeline              (/research-picker?next=timeline)
   |-- Open Research Log          (/research-picker?next=research-log)
   |-- Research Collections       (/research-collections)
   |-- Insights                   (/insights)                  <- NEW: absorbed
   |-- Change Person...           (/research)
```

Rationale: Insights IS adjacent to review in the data model (review queue surfaces findings that insights aggregates); one foldout menu beat cost is acceptable for a medium-frequency surface.

### Shape 3 — "Aggressive" (7 visible)

```
[ Calendar ] [ Search ] [ Browse ] [ Tags ] [ Settings ]
[ v Research & Review (6+3) ] [ v Share ]                  [ + Add Person Record ]
```

Events + Articles both fold into R&R foldout. **Recommend against** — hides 2 distinct record-type surfaces behind foldouts; users lose the "left-to-right by record type" rhythm.

## 5. Honorable mentions (things #380 might miss)

1. **`/jobs/{id}` is NOT a destination** — it's a redirect sink (`app.go:319`, `exports_handlers.go:277/318`, `google_handlers.go:81`). The persistent `jobs-progress-overlay` (`layout.templ:99-107`) is the real UI. **Don't add it to the nav; don't fold it.**

2. **`/share` landing vs. foldout asymmetry** — the Share foldout's first item is `/share/exports`, NOT `/share`. The landing page (`share.templ:25`, post-#284 slimmed) is reachable only from `/jobs/{id}` back-button or by direct URL. The floating-dock Quick Nav (`layout.templ:235`) has a flat `/share` pill that goes to the landing. **Recommendation:** add the landing as a 5th item in the Share foldout, OR remove the floating-dock `/share` flat pill.

3. **Floating-dock Quick Nav parity** — currently `layout.templ:224-238` lists 9 flat pills missing Events, Articles, and all R&R sub-items. Post-Shape 2.5 it should mirror the **7 flat pills + Add CTA** only (foldouts stay top-nav exclusive).

4. **Add Person Record as CTA** — `layout.templ:175` uses `primary-button` class (`top-nav-primary`), visually distinct from pills. It's the highest-intent action and stays a CTA in every shape. Don't fold it into a "New" foldout.

5. **Top-nav order encodes a workflow** — the in-source comments at `layout.templ:120-127` deliberately put Browse next to Events ("researchers can reach the Event list from the same surface they use for Person Records") and Articles next to the R&R foldout ("long-form content surfaces stay grouped on the left"). Any grouping decision should preserve the left-anchored record-type rhythm (Calendar | Person | Person-bulk | Event | Article | ...).

## 6. Decisions the user needs to lock

1. **Shape** — pick 1, 2, 2.5, or 3 (recommend **2.5**: absorb Insights into R&R foldout)
2. **`/jobs/{id}` placement** — confirm **no nav** (redirect sink)
3. **`/share` landing** — confirm **add as 5th item in Share foldout** (closes the asymmetry footgun)
4. **Floating-dock Quick Nav** — confirm **mirror flat pills only** (already locked; reaffirms #380 OQ4)
5. **Settings sub-menu** — confirm **stay flat** (no foldout; page is the hub)
6. **`POST /events/{id}/sources/attach`** — confirm **delete** per my #381 slice 3 drift correction (no `.ddshare` replay)
7. **"Search/Quick View" → "Search" label** — confirm (URL stays `/soldiers`)
