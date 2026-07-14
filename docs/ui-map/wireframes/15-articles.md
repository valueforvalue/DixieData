# 15 — Articles

- **Route**: `/articles` (GET)
- **Builder**: `routebuilder.Articles()` (lazy-derived from `ArticleByID` + `/articles` literal in the route package)
- **Template**: `internal/templates/articles.templ`
- **Layout**: relaxed only (single-column long-form list)
- **Owner**: package `templates`
- **Issues**: #321 (slice 1 + 2, list + detail surfaces), #532 (slice 2 added the body excerpt, slice 1 widened the article body theme to cover the preview modal)

## Regions (relaxed mode)

```
┌── Articles ────────────────────────────────────────────────────────┐
│ h1 "Articles"                                          [+ Create] │
├───────────────────────────────────────────────────────────────────┤
│ if len(articles) == 0:                                            │
│   EmptyState("No articles yet", ...)                              │
│   card "Articles are long-form essays ..."                        │
│ else:                                                             │
│   [data-articles-list] vertical stack of cards (gap-3):           │
│     per article:                                                  │
│       <a href="/articles/{id}" data-article-row>                  │
│         ┌──────────────────────────────────────────────────┐      │
│         │ h2 (data-article-row-title)        span DisplayID │      │
│         │ if subtitle: p (data-article-row-subtitle)        │      │
│         │ if BodyExcerpt: p (data-article-preview)          │      │
│         │   mt-2 text-sm leading-relaxed text-slate-700    │      │
│         │   line-clamp-3 (cap at 3 lines via CSS)          │      │
│         └──────────────────────────────────────────────────┘      │
└───────────────────────────────────────────────────────────────────┘
```

## Notes

- The list page (this wireframe) is the relaxed-mode-only entry point. The detail page lives at `/articles/{id}` and is rendered by `internal/templates/article_detail.templ` (not wireframed separately here -- the detail page is a long-form prose surface, identical shape to the per-page scratchpad panel; see `internal/templates/components/article_detail.templ` for the marker vocabulary).
- The `data-article-preview` element is the slice-2 (#532) addition. It carries the plain-text excerpt computed in `viewmodel.ArticleFromModel` via `buildBodyExcerpt`. The excerpt is stripped of HTML, whitespace-collapsed, and capped at 280 runes with a trailing `\u2026` ellipsis when the body exceeds the cap. See `internal/viewmodel/article.go:30-51`.
- The article body theme CSS (`[data-article-body]` + `[data-article-preview-body]` selectors in `frontend/tailwind.css:1108-1290`) applies to the detail page AND the editor preview modal. The list-page excerpt is plain text (already stripped of HTML), so the prose theme doesn't apply -- the excerpt renders in the same `text-slate-700` ink as the subtitle and the row's `line-clamp-3` Tailwind class caps the visual height at 3 lines.
- `[data-articles-list]`, `[data-article-row]`, `[data-article-row-title]`, `[data-article-row-display-id]`, `[data-article-row-subtitle]`, `[data-article-preview]` are the canonical marker names for the list page. Audit probes (issue #532 slice 3) and the `browse_frontend_harness.js` smoke test assert on these markers.

## Cross-references

- Issue #321: Articles list + detail + editor surfaces (v62).
- Issue #525: GFM extension (markdown tables, task lists, strikethrough).
- Issue #526: Editor preview redesign (modal pattern; preview-body uses `data-article-preview-body`).
- Issue #532: Article body theme + list-page preview excerpt (this wireframe).
- `internal/templates/article_detail.templ`: detail page (long-form prose surface, `data-article-body`).
- `internal/templates/article_preview_modal.templ`: editor preview modal (`data-article-preview-body`).