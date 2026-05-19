# Glossary Alignment Audit

This audit compares the canonical language in `CONTEXT.md` against the current codebase, especially `internal\records`, `internal\archive`, `internal\viewmodel`, `internal\presentation`, `internal\appshell`, and `internal\templates`.

## Direct Hits

These identifiers or labels already align well with the glossary and should be preserved as anchor points.

| Canonical term | Current usage | Evidence |
| --- | --- | --- |
| **Review Queue** | Domain, viewmodel, and UI all use `ReviewQueue` / `Review Queue` consistently | `internal\appshell\app_facades.go:34-36`, `internal\viewmodel\types.go:203-206`, `internal\templates\review_queue.templ:12-22` |
| **Duplicate Audit** | Domain, viewmodel, and UI use `DuplicateAudit*` consistently | `internal\records\audit_service.go:39-60`, `internal\viewmodel\mappers.go:179-204`, `internal\templates\insights.templ:78-101` |
| **Merge Review** | Persistence and UI already center the merge workflow on `merge_review_*` and “Merge Review” | `internal\db\schema.go:85-108`, `internal\templates\share.templ:160-223` |
| **Research Log** | Domain, viewmodel, presentation, and UI already expose `ResearchLog` / “Research Log” | `internal\records\soldier_service.go:88-94`, `internal\presentation\views.go:44-46`, `internal\templates\research_log.templ:10-27` |
| **Research Collection** | Domain, viewmodel, presentation, and UI already use `ResearchCollection` | `internal\records\soldier_service.go:107-125`, `internal\presentation\views.go:60-66`, `internal\templates\research_collections.templ:102-114` |
| **Research Pack** | Domain, viewmodel, presentation, and UI already use `ResearchPack` / “Research Pack” | `internal\records\soldier_service.go:96-105`, `internal\presentation\views.go:52-54`, `internal\templates\research_pack.templ:9-25` |
| **Unit Camaraderie Graph** | Domain type and UI heading match the glossary exactly | `internal\records\soldier_service.go:31-46`, `internal\templates\camaraderie.templ:11-25` |
| **Service Timeline** | Domain, viewmodel, presentation, and UI already use `ServiceTimeline` / “Service Timeline” | `internal\records\soldier_service.go:48-68`, `internal\presentation\views.go:40-42`, `internal\templates\timeline.templ:10-27` |
| **Scratch Pad** | UI uses the canonical “Scratch Pad” label | `internal\templates\layout.templ:292-295` |
| **Display ID** | Code identifiers already prefer `DisplayID` even where labels still lag | `internal\models\models.go:5`, `internal\viewmodel\types.go:5`, `internal\archive\export_service.go:78` |
| **Spouse Record umbrella in analytics** | UI explicitly says “Spouses (Wives & Widows)” | `internal\templates\insights.templ:37-40` |

## Semantic Mismatches

### Tier 1 (Surface/UI)

Low-risk, high-clarity changes in templates and viewmodels.

| Old term | Canonical term | Why it conflicts | Evidence |
| --- | --- | --- | --- |
| **Record ID** | **Display ID** | The glossary explicitly reserves the user-facing identifier term as Display ID; “Record ID” reintroduces the overloaded word “record.” | `internal\templates\entry_form.templ:146-149`, `internal\templates\soldier_card.templ:166-167`, `internal\templates\soldier_card.templ:344-345`, `internal\templates\soldier_card.templ:692` |
| **Add Record / Edit Record / Back to Record / Open Record** | **Add Person Record / Edit Person Record / Back to Person Record / Open Person Record** | Generic “record” is now ambiguous between Person Record and Source Record. | `internal\templates\layout.templ:286`, `internal\templates\soldier_card.templ:309`, `internal\templates\research_log.templ:18-21`, `internal\templates\research_pack.templ:17-20`, `internal\templates\research_pack.templ:75-76` |
| **Supporting Records / Add Record / Records shown here** | **Source Records / Add Source Record / Source Records shown here** | This section manages attached evidence items, which the glossary now names Source Records. | `internal\templates\entry_form.templ:300-317` |
| **Record Type** (for person subtype selector) | **Person Record Type** or **Entry Type** | The label is currently used for `entry_type`, but “record type” is also used for source-record categorization. | `internal\templates\entry_form.templ:152-157`, `internal\templates\soldier_card.templ:343-345` |
| **Archive Insights / Rotating Archive Quote / archive** | **Local Archive Insights / Rotating Local Archive Quote / local archive** | The glossary split archive into Local / Shared / Backup / Static Archive. Plain “archive” is now underspecified. | `internal\templates\insights.templ:18-19`, `internal\templates\calendar.templ:40-43`, `internal\templates\entry_form.templ:142`, `internal\templates\share.templ:17`, `internal\templates\share.templ:24`, `internal\templates\share.templ:61` |
| **Soldier Archive** | **Local Archive** | The top-level archive contains Soldiers, Wives, and Widows, not only Soldiers. | `internal\templates\layout.templ:276-277` |
| **Shared Merge Review / Local / Shared archive / Keep Shared** | **Merge Review / Local Record / Incoming Record / Keep Incoming** | The glossary now names the merge-review sides as Local Record and Incoming Record. “Shared” is the package, not the record side. | `internal\templates\share.templ:163-223` |
| **Archive** evidence type in Research Log | likely **Local Archive** or a narrower evidence term | “Archive” is now a family of distinct terms; the evidence-type dropdown keeps the overloaded generic label. | `internal\templates\research_log.templ:61-70`, `internal\templates\research_log.templ:151-167` |
| **Undated Archive Sources / Archive Context** | **Undated Source Records / Source Context** | These timeline elements refer to attached evidence items, not the archive as a whole. | `internal\templates\timeline.templ:97-105`, `internal\templates\timeline.templ:127-141` |
| **Spouse memorials found** | depends on subtype context: **Wife memorials**, **Widow memorials**, or **Spouse memorials** only when subtype is unknown | The glossary permits Spouse Record as an umbrella, but prefers Wife/Widow when the distinction matters. This UI may be correct only when subtype is genuinely unknown. | `internal\templates\entry_form.templ:44`, `internal\templates\entry_form.templ:78-87` |

### Tier 2 (Domain/Logic)

Medium-risk refactors inside Go code, facades, DTOs, and private APIs.

| Old term | Canonical term | Why it conflicts | Evidence |
| --- | --- | --- | --- |
| `models.Soldier`, `viewmodel.Soldier`, `[]models.Soldier` as the umbrella type | **Person Record** | The core type is used for soldiers, wives, and widows. The glossary reserves Soldier for one subtype only. | `internal\models\models.go:3-50`, `internal\viewmodel\types.go:3-50`, `internal\records\soldier_service.go:18-22`, `internal\appshell\app_facades.go:16-24` |
| `models.Record`, `viewmodel.Record`, `Records []Record`, `RecordFromModel`, `RecordsFromModels` | **Source Record** | This type models attached evidence items, not generic records. | `internal\models\models.go:122-130`, `internal\viewmodel\types.go:53-61`, `internal\viewmodel\mappers.go:77-82` |
| `soldiersFacade` and its method names (`Create`, `List`, `GetByID`, etc.) | **Person Record–oriented facade naming** | The main AI-agent-facing contract still speaks in Soldier terms even though it serves all person subtypes. | `internal\appshell\app_facades.go:13-44` |
| `ResearchPackForSoldier`, `AddSoldierToResearchCollection`, `Current *Soldier`, `Members []Soldier` | **Person Record** / possibly broader archive-material terms | Collections and packs are glossary concepts broader than “soldier-only,” but the current APIs hard-code Soldier as the container/member type. | `internal\appshell\app_facades.go:29-33`, `internal\records\soldier_service.go:96-125`, `internal\viewmodel\types.go:309-339` |
| `ServiceTimelineEvent` | **Timeline Event** | The timeline includes life, burial, pension, and death categories, so not every event is specifically a Service Event. | `internal\records\soldier_service.go:48-68`, `internal\viewmodel\types.go:263-281` |
| `UndatedRecords []Record` | **Undated Source Records** | These are attached evidence items without parseable dates, not generic records. | `internal\records\soldier_service.go:48-56`, `internal\viewmodel\types.go:263-271`, `internal\templates\timeline.templ:97-117` |
| Merge-review fields `LocalSoldier`, `SourceSoldier`, `LocalSoldierID`, `SourceDisplayID`, `SourceSnapshot` | **Local Record / Incoming Record** | “Source” is now reserved for Source Record evidence; in merge review it means incoming shared data, not evidence. | `internal\models\models.go:169-180`, `internal\viewmodel\types.go:174-186`, `internal\archive\backup_service.go:72-95` |
| `SourceConflictLedger` | likely **Incoming Conflict Ledger** or **Merge Conflict Ledger** | The term “Source” here means incoming shared-archive data, which collides with the new Source Record meaning. | `internal\archive\backup_service.go:77-95`, `internal\presentation\views.go:48-50`, `internal\viewmodel\mappers.go:349-371` |
| `SpouseSoldierID` | needs a more explicit relationship term | The field points from a spouse record to a linked soldier. “SpouseSoldier” is legacy table language, not glossary language. This needs a small glossary extension before rename. | `internal\models\models.go:8`, `internal\viewmodel\types.go:8`, `internal\records\soldier_service.go:18-22` |
| `ArchiveCounts.TotalSoldiers`, `TotalWivesWidows`, `TotalRecords()` | **Person Record**-aware counting terms | The counting model is still table-era/legacy-era language, not glossary-first language. | `internal\models\models.go:53-60`, `internal\viewmodel\types.go:75-78` |

### Tier 3 (Persistence / External Contracts)

High-risk changes that need migrations or compatibility layers.

| Old term | Canonical term | Why it conflicts | Evidence |
| --- | --- | --- | --- |
| `soldiers` table | **person_records** (or compat-mapped legacy table) | The main table stores Soldiers, Wives, and Widows, not only Soldiers. | `internal\db\schema.go:24-62` |
| `records` table | **source_records** (or compat-mapped legacy table) | This table holds attached evidence items, which the glossary now names Source Records. | `internal\db\schema.go:64-72` |
| `soldier_id`, `soldier_sync_id`, `spouse_soldier_id`, `left_soldier_id`, `right_soldier_id`, `local_soldier_id` | **person_record_id / linked_soldier_id / local_record_id / incoming_record_id** | Persistence names still leak the old umbrella term into every contract. | `internal\db\schema.go:29`, `internal\db\schema.go:67-68`, `internal\db\schema.go:99-103`, `internal\db\schema.go:113-114`, `internal\db\schema.go:126`, `internal\db\schema.go:146` |
| JSON tags `json:"records"` on person payloads | **source_records** | Attached evidence should travel as Source Records, not generic records. | `internal\models\models.go:49`, `internal\archive\export_service.go:113`, `internal\archive\backup_service.go:41`, `internal\archive\diagnostics_service.go:29` |
| JSON/document roots `Soldiers []models.Soldier`, `json:"soldiers"` | **person_records** (or a compatibility alias) | External export/import payloads still encode the old umbrella type. | `internal\archive\export_service.go:72-75`, `internal\archive\backup_service.go:53`, `internal\archive\backup_service.go:40`, `internal\archive\diagnostics_service.go:28` |
| Merge-review JSON `json:"soldier"`, `spouse_sync_id`, `local_soldier_id`, `source_display_id` | **local_record / incoming_record** vocabulary | Merge payloads currently overload Soldier/Source language in persisted review state. | `internal\archive\backup_service.go:72-75`, `internal\db\schema.go:94-108` |
| `scratchpad_cache.soldier_id` and FTS `soldier_id` columns | **person_record_id** | Scratch Pad is tied to a Person Record, not specifically a Soldier. | `internal\db\schema.go:126`, `internal\db\schema.go:409-529` |

## Canonical Terms Not Yet Represented in Code

These are glossary concepts with little or no first-class implementation presence yet.

| Canonical term | Current state | Evidence |
| --- | --- | --- |
| **Person Record** | Not present as a first-class type or UI term | no matches in `internal\records` for `PersonRecord`; umbrella type is still `Soldier` |
| **Source Record** | Not present as a first-class type; implemented as `Record` | `internal\models\models.go:122-130`, `internal\viewmodel\types.go:53-61` |
| **Claim** | No first-class domain type yet | no matches in `internal\records` for `Claim` |
| **Finding** (research conclusion, not duplicate finding) | No research-domain type yet; only duplicate-audit findings exist | `internal\records\audit_service.go:39-60`; no research-domain `Finding` type in `internal\records` |
| **Timeline Event** | Timeline item exists, but as `ServiceTimelineEvent` rather than the glossary’s more general term | `internal\records\soldier_service.go:58-68` |
| **Service Event** | No dedicated subtype or category type yet | timeline currently uses category strings in `ServiceTimelineEvent` |
| **Local Record / Incoming Record** | Merge-review domain still uses `LocalSoldier` / `SourceSoldier` | `internal\models\models.go:169-180`, `internal\archive\backup_service.go:84-95` |

## Recommended Refactor Plan (Grey Box First)

### 1. ViewModel Alignment First

Update the presentation-facing language before touching persistence:

1. Rename viewmodel types and fields that are only presentation contracts:
   - `viewmodel.Record` -> `viewmodel.SourceRecord`
   - `Soldier.RecordCount` -> `SourceRecordCount`
   - `ServiceTimelineEvent` -> `TimelineEvent`
   - `UndatedRecords` -> `UndatedSourceRecords`
2. Update `internal\presentation\views.go` function names and template call sites to expose canonical vocabulary without touching storage.
3. Fix hardcoded template labels first:
   - `Record ID` -> `Display ID`
   - `Supporting Records` -> `Source Records`
   - `Add Record` / `Edit Record` / `Back to Record` -> `... Person Record`
   - merge-review side labels to `Local Record` / `Incoming Record`
   - replace generic `archive` with `local archive`, `shared archive`, `backup archive`, or `static archive` where the meaning is specific

This stage gives immediate user-facing alignment while preserving existing deep-module behavior.

### 2. Facade Alignment Next

Refactor the app-shell boundary so future work stops spreading old terms:

1. Replace `soldiersFacade` with a more accurate facade name such as `personRecordsFacade`.
2. Rename exported facade methods where the old umbrella term leaks:
   - `ResearchPackForSoldier` -> `ResearchPackForPersonRecord` or `ResearchPackForSoldier` only if the feature is intentionally soldier-only
   - `AddSoldierToResearchCollection` -> `AddPersonRecordToResearchCollection`
   - review and listing methods should return DTOs named for Person Records at the app-shell boundary
3. Keep the deep module implementation stable underneath while the app shell and presentation layer adopt the glossary.

This creates a clean AI-agent-facing contract at the Grey Box boundary.

### 3. Domain Sanitization After Boundary Cleanup

Once viewmodels and facades stop leaking legacy language, rename internals deliberately:

1. In `internal\records`, introduce canonical aliases/types and phase out legacy type names:
   - `Record` -> `SourceRecord`
   - `ServiceTimelineEvent` -> `TimelineEvent`
   - merge-review “source” snapshots -> “incoming” snapshots
2. In `internal\archive`, stop using `Source*` for incoming shared-archive payloads because `Source Record` now has a precise evidence meaning.
3. Add missing research-domain concepts only when they become real behavior:
   - `Claim`
   - `Finding`
   - possibly a stronger type distinction between `TimelineEvent` and `ServiceEvent`
4. Extend `CONTEXT.md` with one extra relationship term before renaming `SpouseSoldierID`; the glossary does not yet define the canonical name for that link field.

### 4. Persistence and Contract Migration Last

Do not rename tables or JSON fields until the application boundary is already aligned.

1. Keep DB table names and JSON keys stable initially; translate them at the domain boundary.
2. If persistence is renamed later, use an explicit migration/compatibility plan:
   - SQL migration or compatibility views for `soldiers` / `records`
   - dual-read/dual-write or versioned import/export payloads for JSON archives
   - migration of merge-review persisted state from `source_*` / `*_soldier_*` names
3. Treat this as a separate migration project, not part of the first terminology cleanup.

## Recommended Order of Work

1. **Tier 1 first**: template text, viewmodel names, presentation adapters.
2. **Tier 2 second**: app-shell facades, domain type names, unexported helpers.
3. **Tier 3 last**: DB schema, import/export JSON, merge-review persisted state, static archive payload shape.

## Bottom Line

The biggest immediate wins are:

1. replace **Record ID** with **Display ID**
2. rename attached **Record(s)** to **Source Record(s)**
3. stop using **Soldier** as the umbrella term at the presentation and facade boundary
4. rename merge-review sides from **Local/Shared/Source Soldier** to **Local Record/Incoming Record**
5. reserve plain **archive** for no UI at all; choose **Local / Shared / Backup / Static Archive** explicitly
