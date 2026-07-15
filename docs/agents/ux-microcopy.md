# UX microcopy conventions

This doc describes the user-facing-text conventions that the audit-issue
backlog (issue #561) and every future UI commit must satisfy. The
`audit/smoke_microcopy.mjs` probe is the executor; this doc is the spec.
Keep them in sync.

## Why this exists

The 2026-07 UI text audit (issue #561) found 73 instances of the same
three failure classes across 37 `.templ` files + `frontend/app.js`:

- **Verbose explanatory text** — paragraphs or subheaders saying what a
  single button or single field already implies.
- **Unnecessary titles** — section headings on UI areas whose content is
  self-explanatory. The canonical example is the `/calendar` page's
  `Rotating Local Archive Quote` panel: a quote panel needs no heading
  explaining that it is a quote panel.
- **Duplicated information** — the same fact stated in a heading, in a
  body paragraph, AND in a button label.

Without a written rule, every UI commit re-introduces these. This doc
pins the rules so the probe has a spec to enforce.

## The five rules

### 1. Concise

Cut filler. One short sentence beats a paragraph. A button that does
not need a description paragraph does not get one.

```templ
<!-- ❌ verbose -->
<p>This panel lets you review what changed in your archive since your
last backup. Click the button below to start a review.</p>
<button>Start review</button>

<!-- ✅ concise -->
<button>Review changes since last backup</button>
```

### 2. Don't label what is self-explanatory

A single-button group needs no heading. A single-field group needs no
subheader. The user can see what is there.

```templ
<!-- ❌ unnecessary title -->
<section>
  <h3>Rotating Local Archive Quote</h3>
  <blockquote>{ quote.Text }</blockquote>
</section>

<!-- ✅ self-explanatory -->
<section>
  <blockquote>{ quote.Text }</blockquote>
</section>
```

### 3. Action-oriented

Buttons say what they do. Modals name what they collect.

```templ
<!-- ❌ noun-form label -->
<button>Save button</button>
<button>PDF export action</button>

<!-- ✅ verb-form label -->
<button>Save</button>
<button>Export PDF</button>
```

### 4. No duplication

If the button label says it, the heading doesn't repeat it. If the
toast says it, the page header doesn't. If a field's label says it, a
sibling `<p class="help">` doesn't restate it.

```templ
<!-- ❌ header + body say the same thing -->
<h2>Export PDF</h2>
<p>Export this Person Record as a PDF file you can print or share.</p>
<button>Export PDF</button>

<!-- ✅ pick one -->
<h2>Export</h2>
<button>Export PDF</button>
```

### 5. Context-relevant

Empty states say what the user CAN do, not what the system IS. Error
messages say what failed + what to do next.

```templ
<!-- ❌ status-form empty state -->
<p>No records yet.</p>

<!-- ✅ action-form empty state -->
<p>Add your first Person Record to start your archive.</p>

<!-- ❌ error with no path forward -->
<p>An error occurred.</p>

<!-- ✅ error with path forward -->
<p>Could not save the Person Record. Check the required fields above
and try again.</p>
```

## DixieData-specific guidance

These rules are repo-specific. They are enforced the same way as the
five universal rules.

### Glossary terms win

Use the canonical terms from [`CONTEXT.md`](../../CONTEXT.md):
**Person Record**, **Source Record**, **Display ID**, **Local Archive**,
**Shared Archive**, **Backup Archive**, **Restore Point**, **Static
Archive**, **Tag**, **Finding**, **Claim**. Never invent synonyms
("record", "attachment", "label", "export").

### Case

- Sentence case for headings and labels. Not Title Case. Not ALL CAPS.
- The exceptions are well-established proper nouns that ship with their
  own casing: `Person Record`, `Local Archive`, `Find a Grave`.

### Toast text

- One line, ≤ ~80 chars.
- Lead with the verb. `"Saved {Display ID}"` beats `"The Person Record
  with Display ID {X} has been saved."`.
- Include the affected identifier when it is short (`D-00123`). For long
  identifiers, name the affected thing (`person`, `event`, `article`).

### Modal titles

- Noun phrase. No period. No trailing emoji.
- Mirror the verb on the primary button when there is one
  (`Delete Person Record` modal + `Delete` button).

### Helper copy

Only when the field's purpose is not obvious from the label. Drop
otherwise. If a label says `Birth date`, no helper copy.

### Empty-state copy

Action-oriented. The user is at the empty state because they have not
done the thing yet. Tell them the thing they can do next.

### Section headings

Only when the section is one of multiple siblings that need
disambiguation. Drop headings when the section is the only thing on the
page or in the panel.

## The regression probe

`audit/smoke_microcopy.mjs` walks every `.templ` file under
`internal/templates/**` and flags violations of the five rules. The
probe is the executor; this doc is the spec. When a rule changes here,
the probe changes in the same PR.

The `.templ` probe covers live Wails pages. Static Archive has a separate
format-aware gate because its UI is embedded as HTML + JavaScript inside
`internal/archive/static_archive.go`; do not apply `.templ` parsing rules
blindly to that mixed source. Run both probes when changing shared copy.

### Static Archive coverage (issue #581)

`audit/smoke_static_archive_microcopy.mjs` scans the user-facing Static
Archive shell plus strings emitted by its hash-routed renderers. It checks:

- concise hero, report, list, and page-description copy;
- glossary-aligned `Person Record`, `Source Record`, `Linked Soldier`,
  `Linked Persons`, and `Confederate Home Status` labels;
- entity-specific list actions instead of generic `View More` labels;
- concise empty states and one printable-report instruction;
- printable-report and detail-view labels.

The scan excludes CSS, JavaScript plumbing, route names, JSON/data field
names, researcher-authored archive content, and generated export contracts.
`--strict` is the CI gate; `audit/smoke_static_archive_microcopy.test.mjs`
pins clean output plus synthetic regressions for each rule family.

When changing Static Archive copy, run:

```text
node audit/smoke_static_archive_microcopy.mjs --strict
node audit/smoke_static_archive_microcopy.test.mjs
```

### Probe rules (enforced today)

The `.templ` probe currently asserts five concrete checks:

1. **Eyebrow above a self-explanatory block.** An uppercase,
   letter-spaced eyebrow above a single blockquote or table without
   form controls is flagged. Fix: drop the eyebrow.

2. **Heading repeats button label.** A heading whose text matches an
   adjacent button label is flagged. Fix: drop the heading or rewrite
   it to add new information.

3. **Duplicated visible text.** The same user-facing text appearing
   within a short source window is flagged after classes, component
   calls, attributes, and control flow are removed. Fix: deduplicate.

4. **Stacked headings.** Two short heading-style elements within three
   lines without body copy between them are flagged. Fix: keep only the
   heading that adds information.

5. **Verbose body under heading.** A body paragraph longer than 80
   characters directly under a heading-style element is flagged. Fix:
   trim or remove narration that repeats the heading.

Each violation message includes `file:line` and a suggested fix per
rule above.

### Probe rules (not yet enforced — TODO)

Two further rules from §The five rules are deferred because they need
runtime context (is this section the only thing on the page?) that
static analysis cannot reliably answer. Land them as runtime probes in
follow-up issues:

- **§2 self-explanatory blocks** — single-section-page check
  (deferred; needs page-boundary context).
- **§5 context-relevant empty state** — empty-state-action check
  (deferred; needs to know which states are empty).

## Author checklist

When you add or modify ANY of the following, run
`node audit/smoke_microcopy.mjs` locally before committing:

- A `.templ` file under `internal/templates/`.
- A user-facing string in `frontend/app.js` (toast, modal title,
  button label, alert copy).

If the probe fails, fix the violation before pushing. The probe runs
in CI; a red probe blocks merge.

The probe's baseline on the day it lands documents the pre-fix state
(today's audit findings from issue #561). Subsequent commits that
reduce the violation count are the implementation work; commits that
re-introduce violations are bugs.

## Out of scope

- Implementing any of the audit-issue #561 findings. That work ships
  in separate commits that cite this doc in the commit message.
- Renaming glossary terms. Use [`CONTEXT.md`](../../CONTEXT.md) terms
  as-is.
- Translating copy. DixieData is English-only.

## Related

- Issue #561 — the audit whose findings this doc + probe encode.
- [`CONTEXT.md`](../../CONTEXT.md) — the glossary. UI copy uses these
  terms; never invent synonyms.
- `audit/smoke_*.mjs` — sibling probes (e.g. `smoke_dialog_guard`,
  `smoke_mega_menu_nav`) for non-microcopy conventions.
- `docs/CODE_CHANGES.md` — the cross-layer working contract.
- `docs/COMMON_BUGS.md` §3 — frontend JS bug patterns; overlaps with
  this doc on the toasts / modals layer.