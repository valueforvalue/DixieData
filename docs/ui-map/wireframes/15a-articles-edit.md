# 15a — Article Edit

- **Route**: `/articles/{id}/edit` (GET, POST)
- **Builder**: `routebuilder.ArticleEdit(id)`
- **Handler**: `internal/appshell/articles_handlers.go:handleEditArticle`
- **Template**: `internal/templates/article_edit.templ:ArticleEditShell` (thin wrapper around `ArticleArticleForm`)
- **Layout**: both (full-page form, body constraints at `max-w-6xl`)
- **Issues**: #321 (slice 3.5 + 3.7), #565 (cheatsheet), #610 (toolbar + live preview), #611 (undo/redo), #612 (image picker), #526 (preview modal)

Both `/articles/new` and `/articles/{id}/edit` render the same
`ArticleArticleForm` component. The shell file is the only
difference (the edit shell wraps the shared form with the
loaded article viewmodel; the new shell wraps it with an empty
article viewmodel). Per the AGENTS.md "implemented-but-invisible"
anti-pattern, **one probe file covers both routes** — same
controls, different `article-id` value. The wireframe table
covers both with notes on the differences.

## Regions (relaxed mode)

```
┌── Edit Article ───────────────────────────────────────────────────┐
│ h1 "Edit Article"                                     [← Back]    │
├──────────────────────────────────────────────────────────────────┤
│ Local draft persistence banner (kind="edit"):                    │
│   strong  "Committed to database"                                │
│   span    "Restore older saved local changes" (if version stale) │
│   [Delete saved local draft] (ghost button, only if draft)       │
│                                                                  │
│ [label] Title                                                    │
│   <input id="article-title" name="title" required value="...">   │
│                                                                  │
│ [label] Subtitle                                                 │
│   <input id="article-subtitle" name="subtitle" value="...">      │
│                                                                  │
│ [label] Body (Markdown)                                          │
│   [data-editor-toolbar] role="toolbar" (icon row):               │
│     [B] [I] [H] [Link] [Image] [List] [Code] [Quote] [Table]     │
│     | separator | [↶ Undo] [↷ Redo]                             │
│   <textarea id="article-body" name="body" rows="22"             │
│       data-article-editor-source data-article-id="N">           │
│   <p>Markdown syntax supported. Raw HTML is stripped at save.    │
│                                                                  │
│ Right-aligned action row:                                        │
│   [Markdown syntax ▼]    (Foldout → panel.article.markdown-cheatsheet)
│   [Preview]              (button → article-preview-modal)        │
│   [Save Article]         (submit, primary)                       │
├──────────────────────────────────────────────────────────────────┤
│ @ArticlePreviewModal (data-article-preview-modal, hidden)        │
│ @components.TableBuilderModal  (data-table-builder-modal, hidden) │
│ @components.ImagePickerModal(article.ID)                         │
│   (data-image-picker-modal, hidden)                              │
└──────────────────────────────────────────────────────────────────┘
```

## Differences from New

| Region | New (`/articles/new`) | Edit (`/articles/{id}/edit`) |
| --- | --- | --- |
| H1 | "New Article" | "Edit Article" (articleFormTitle helper) |
| Local-draft persistence | `kind="new"`, banner copy "Local draft only" | `kind="edit"`, banner copy "Committed to database", carries `data-draft-record-version={UpdatedAt\|ID}` |
| Form action | `routebuilder.ArticleNew()` → `/articles/new` (POST) | `routebuilder.ArticleEdit(id)` → `/articles/{id}/edit` (POST) |
| Image picker | Tabs disabled (`data-image-picker-no-article` shown), manual URL only | Full upload + pick-existing + manual URL |
| Pre-populated fields | Empty | title/subtitle/body pre-filled |
| Hidden `existing_*` fields | Absent | Absent (article has no equivalent of soldier's `existing_needs_review` |

## HTMX wiring

The form is `data-dixie-submit="true"` with `enctype="multipart/form-data"`.
The dispatcher (`frontend/app.js:dispatchDixieDataForm`) handles the
POST + form encoding. Bare URL on the edit form (no routebuilder
abstraction in the handler — `/articles/{id}/edit` literal).

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Form submit | POST | `/articles/{id}/edit` | body | default | 404 + 400 + 409 contracts |
| `data-article-preview-open` (button) | — | — | `#article-preview-modal` | none (modal show) | Fills body via `data-article-preview-body` |
| `data-editor-toolbar-action="image"` | — | — | `#image-picker-modal` | none (modal show) | Plus `data-editor-toolbar-opens-modal="image-picker-modal"` |
| `data-editor-toolbar-action="table"` | — | — | table-builder-modal | none (modal show) | Plus `data-editor-toolbar-opens-modal="table-builder-modal"` |
| `data-image-picker-tab="existing"` | GET (via custom event) | `/articles/{id}/images` | `[data-image-picker-existing-list][data-article-id='N']` | innerHTML | Fired on first tab activation |
| `data-image-picker-insert-url` (button) | — | — | `#article-body` (cursor) | none (insertTextAtCursor) | Inserts `![alt](url)` |
| `data-md-cheatsheet-insert-key` (menuitem) | — | — | `#article-body` (cursor) | none (insertTextAtCursor) | Inserts the example Markdown |
| `data-md-cheatsheet-copy-key` (button) | — | — | clipboard | none | Copies the example to clipboard |
| `data-article-md-cheatsheet-open` (DELETED — see correction) | — | — | — | — | **Correction (post-36d42c7 probe work):** the cheatsheet trigger uses the Foldout primitive's `data-foldout-trigger="<menuID>"` contract, where `menuID = panel.article.markdown-cheatsheet`. The literal `data-article-md-cheatsheet-open` marker is **not** present in the rendered HTML. See the Foldout primitive at `internal/templates/components/foldout.templ:54` + `:96` for the canonical attribute. Use `[data-foldout-trigger="panel.article.markdown-cheatsheet"]` instead. |
| `data-foldout-trigger="panel.article.markdown-cheatsheet"` (Foldout trigger) | — | — | `#panel.article.markdown-cheatsheet` | none (Foldout show) | aria-expanded toggles. The Foldout primitive uses `data-foldout-trigger=<menuID>` as the trigger contract; menuID = `panel.article.markdown-cheatsheet`. |
| `data-history-back` (button) | — | — | history back | none | Falls back to `routebuilder.Articles()` if no history |

## Canonical DOM surface IDs

From `internal/uiids`:

| ID | Kind | Where |
| --- | --- | --- |
| `panel.article.markdown-cheatsheet` | panel | Foldout surface for the cheatsheet popover |

Pinned by `internal/uiids` Registry. Other markers are data-attribute
conventions rather than registry IDs:

| Marker | Where |
| --- | --- |
| `data-article-editor-source` | body textarea |
| `data-article-id` | body textarea + image picker list |
| `data-article-preview-open` | Preview button |
| `data-article-preview-modal` | preview modal root |
| `data-article-preview-body` | preview modal body |
| `data-article-preview-close` | preview modal close button |
| `data-image-picker-modal` | image picker modal root |
| `data-image-picker-close` | image picker close button |
| `data-image-picker-tab="upload"` | upload tab |
| `data-image-picker-tab="existing"` | existing tab |
| `data-image-picker-panel="upload"` | upload panel |
| `data-image-picker-panel="existing"` | existing panel |
| `data-image-picker-existing-list` | existing images container |
| `data-image-picker-no-article` | new-article banner |
| `data-image-picker-url` | manual URL input |
| `data-image-picker-alt` | manual alt text input |
| `data-image-picker-insert-url` | manual URL insert button |
| `data-editor-toolbar` | toolbar root |
| `data-editor-toolbar-action="bold\|italic\|heading\|link\|image\|list\|code\|quote\|table\|undo\|redo"` | per-button |
| `data-editor-toolbar-template` | per-button (not undo/redo) |
| `data-editor-toolbar-opens-modal` | image + table buttons |
| `data-md-cheatsheet-insert-key` | per-row insert menuitem |
| `data-md-cheatsheet-insert-value` | per-row insert menuitem |
| `data-md-cheatsheet-copy-key` | per-row copy button |
| `data-md-cheatsheet-copy-value` | per-row copy button |
| `data-md-cheatsheet-preview-key` | per-row live preview cell |
| `data-record-persistence` | draft persistence banner root |
| `data-record-persistence-kind` | draft persistence mode |
| `data-record-persistence-heading` | draft persistence strong label |
| `data-record-persistence-message` | draft persistence span |
| `data-clear-draft-trigger="base"` | draft delete button |
| `data-draft-key` | form (localStorage key) |
| `data-draft-record-version` | form (stale-detection sentinel) |
| `data-draft-reset-path` | form (reset endpoint) |
| `data-history-back` | Back button |
| `data-fallback-href` | Back button history fallback |
| `data-fallback-label` | Back button history fallback |

## Footguns

- **Form action is a bare URL** (`/articles/{id}/edit` literal),
  not a routebuilder (`routebuilder.ArticleEdit(id)` exists but the
  form action is generated via `templ.SafeURL(routebuilder.ArticleEdit(...))`
  — verify in the rendered HTML). The image-upload form action
  (`/articles/{id}/images/import`) is also a bare URL.
- **Two `data-editor-toolbar-opens-modal` consumers** — the
  Image button opens the image picker, the Table button opens the
  table builder. Wiring-by-data-attr means a typo on either would
  silently open the wrong modal; the probe must verify each.
- **Markdown cheatsheet does NOT steal focus** from the textarea
  (per the issue #565 decision). The probe must verify the
  textarea retains focus after cheatsheet open.
- **New-article image picker is degraded** — `data-image-picker-no-article`
  banner shown, tabs disabled. The probe must verify the new-article
  form has only the manual URL fallback, not the upload tab.
- **Preview modal body is empty until first Preview click** — the
  JS initializer (`initializeArticlePreview`) fills it on demand.
  The probe must verify the JS path, not just the static HTML.
- **`data-draft-record-version` is the slice-3.7 stale-detection
  sentinel** — `UpdatedAt|ID`. A server-side update that bumps
  `updated_at` invalidates local drafts. Verify the version format.
- **Nested form risk** — the form contains sub-forms (the image
  upload form at `article_image_picker_modal.templ:80`). Per
  `<form>` inside `<form>` is HTML5-illegal. The image picker
  modal is a SIBLING of the article form (not nested) — verify
  the rendered DOM tree.

## See also

- [15-articles.md](15-articles.md) (list page)
- `internal/templates/article_edit.templ` — the thin shell wrapper
- `internal/templates/article_new.templ` — the shared form markup
- `internal/templates/components/editor_toolbar.templ` — the icon row
- `internal/templates/components/markdown_cheatsheet.templ` — the Foldout popover
- `internal/templates/components/image_picker_modal.templ` — the image picker
- `internal/templates/article_preview_modal.templ` — the preview modal
- `internal/uiids/registry.go` — `PanelArticleMarkdownCheatsheet` canonical ID
- `internal/routebuilder/routebuilder.go:501-606` — `ArticleNew` / `ArticleEdit` / `ArticleByID` / `ArticleDelete` / `ArticlePDF` URL builders
- `internal/appshell/articles_handlers.go:325-348` — `handleEditArticle` POST contract
- `frontend/app.js` — `dispatchDixieDataForm` (form dispatcher), `initializeEditorToolbar` (toolbar wiring), `initializeArticlePreview` (preview modal), `initializeDraftForms` (local-draft persistence)
- `MISTAKES.md` — the dispatchDixieDataForm body-construction gotcha (issue #691) that affects all `data-dixie-submit` forms
- `docs/CODE_CHANGES.md` §3.9 (class 8 — form.action mutation) and §3.10 (class 9 — empty-body dispatch) — the regression classes this probe catches
