# DixieData Manual UI Audit Playbook

A guided, interactive audit protocol for walking every UI surface
of DixieData by hand, capturing findings as you go, and turning
those findings into well-formed GitHub issues.

This is **not** a replacement for `audit/smoke.mjs` or the round 1–3
sweep. It's the next layer: a human walks the surfaces, the script
handles setup + deterministic assertions, and the human reports
findings into a running notes file.

---

## How to run

### Pre-flight

```bash
# Build + seed + start the web server
make web seed
rm -rf .scratch/webmode && ./build/bin/seed-data.exe -data-dir .scratch/webmode -soldiers 25 -reset
nohup ./build/bin/dixiedata-web.exe -addr 127.0.0.1:8765 -scratch-dir .scratch/webmode > /tmp/web.log 2>&1 &

# Wait for the server
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8765/

# Copy the notes template to a working file
cp docs/agents/audit-notes-TEMPLATE.md audit/notes-$(date +%Y-%m-%d).md
```

### Walk

```bash
# Walk all surfaces
node audit/run-interactive.mjs

# Walk one surface
node audit/run-interactive.mjs --surface=feedback-modal
node audit/run-interactive.mjs --surface=settings

# Write notes to a specific file
node audit/run-interactive.mjs --report=audit/notes-2026-06-29.md
```

The script:
1. Walks each surface in order, runs the auto checks, and prints
   `? (manual)` for the human-only checks.
2. Takes a `before` and `after` screenshot of every surface into
   `audit/screenshots-interactive/`.
3. Writes a machine-readable summary to
   `audit/audit-interactive-report.json`.
4. Exits 1 if any auto check failed, 0 otherwise. (Manual checks
   do not affect the exit code — that's your job.)

### Per surface

For each surface:

1. **Read the surface description** (printed as the `[name]`
   header). It's a one-liner reminder of what this surface is
   for.
2. **Read each `? (manual)` line** — the prompt tells you what
   to verify by hand. Note: the script has already taken the
   `before` screenshot.
3. **Open the browser** at `http://127.0.0.1:8765/<path>`. Walk
   the surface yourself, ignoring the script's auto checks —
   you want to see what a user sees, not what the assertions
   think.
4. **Find a finding?** Open `audit/notes-<date>.md`, find the
   surface section, fill in the [BUG] or [FEATURE] or
   [CORRECTION] block. Use the templates below.
5. **Find nothing?** Move on. Don't pad the notes.

### After the walk

```bash
# File one issue per finding
gh issue create --label needs-triage --label bug --body-file <(sed -n '/^## BUG: /,/^---/p' audit/notes-2026-06-29.md)

# Or batch:
# - Each `## BUG: <title>` block in the notes file becomes one
#   GitHub issue. The script does NOT file issues for you — that
#   is your judgement. Some findings are duplicates of
#   open issues; some are workflow nitpicks; some are
#   features.
```

---

## Surfaces covered (today)

| Surface | Auto checks | Manual checks | Notes |
|---|---|---|---|
| `calendar` | 3 | 1 | Day click + anniversary render |
| `soldier-new` | 1 | 1 | Required-field validation |
| `browse` | 2 | 1 | Soldier link → detail navigation |
| `share` | 3 | 0 | Print modal + all export buttons |
| `settings` | 3 | 0 | Scan/quality results, debug toggle |
| `feedback-modal` | 3 | 0 | Save flow + confirmation toast |
| `floating-dock-layout` | 1 | 1 | Dock positioning, no overlap |
| `jobs-page` | 2 | 0 | `/jobs/active` polling, `/jobs/{id}` detail |

To add a surface, append a new entry to the `SURFACES` array in
`audit/run-interactive.mjs`. Each surface is `{ name, path,
description, checks: [] }` and each check is either `{ name,
kind: 'auto', run: async (page) => ({ ok, reason }) }` or
`{ name, kind: 'manual', run: async () => ({ ok: 'manual',
prompt: '...' }) }`.

---

## What to look for (the "I see something off" checklist)

When you're walking a surface, run this mental list in the
background. It's not exhaustive — the point is to train your eye
to notice *patterns* not just *bugs*.

### Visual

- **Floating dock overlap**: does the dock cover any content
  (anniversary list, modal footer, toast region)? [§4.14]
- **Truncation**: any text overflowing its container, especially
  long names, sources, comments?
- **Spacing**: any element visibly misaligned vs the rest of the
  page? (Often a missing `mt-` / `space-y-` class.)
- **Empty state**: every list view should have a meaningful empty
  state. If you see a blank list, the empty state is missing.
- **Modal centering**: on different viewport widths, do modals
  center correctly? Mobile + tablet especially.
- **Focus outline**: tab through the page. Every focusable
  element should show a visible focus outline.

### Behaviour

- **Toast appears**: every save / import / export / delete
  should show a toast. If you don't see one, that's a bug. (The
  feedback fix is the most recent instance of this class.)
- **Toast says the right thing**: "Saved." is not enough; the
  toast should say what was saved and what happens next.
- **Form clears after save**: any form that posts and stays on
  the same page should clear the input fields. (Another
  instance of the feedback class.)
- **Modal closes after save**: any modal that hosts a save
  button should close the modal on success. Otherwise the user
  sees a closed modal with no confirmation.
- **Redirects land you on the right page**: click "Save Person
  Record" and you should land on that person's detail page, not
  on the form.
- **Pagination is bidirectional**: if you can go next, you can
  go prev. If you can go next from page 5, the URL changes.
- **Filter survives reload**: if I filter by a status, the URL
  reflects the filter, and reloading keeps the filter.
- **Delete confirms**: every destructive action should ask
  "Are you sure?" before running.

### Accessibility

- **Tab order**: tab through the page. The order should match
  the visual order.
- **Screen reader labels**: every icon-only button should have
  an `aria-label`. Every form input should have a label.
- **Focus trap in modals**: open a modal, press Tab. Focus
  should stay inside the modal until you close it.
- **Escape closes modals**: open a modal, press Esc. The modal
  should close and focus should return to the trigger.
- **Color contrast**: any text you can barely read? Especially
  the small text in the corner (e.g. build identity in the
  footer).
- **Keyboard-only flow**: can you do everything in the app
  with the keyboard? No? That's a bug.

### Performance

- **Time to interactive**: does the page feel responsive
  immediately, or is there a flash of blank / unstyled content?
- **Background polling**: the jobs-progress overlay should poll
  every 3s, but the request should be cheap. If you see
  `/jobs/active` requests every 3s when there are no jobs,
  that's expected. If you see it every 1s, that's a bug.
- **Image load**: any image that takes >1s to load? Especially
  the soldier photos on the detail page.

### Data integrity

- **Edit → save → reload**: edit a soldier, save, refresh the
  page. The edit should persist. (Most common bug class: edit
  saves to the DB but the form's prefill is wrong, so the user
  thinks the edit was lost.)
- **Delete → reload**: delete a soldier, refresh. They should
  be gone.
- **Backup → restore**: take a backup, restore it on a fresh
  scratch dir. The data should match.

---

## What to do with a finding

Fill in the appropriate block in `audit/notes-<date>.md`. Use
**one block per finding**. Do not bundle multiple findings into
one block — the issue tracker will reject them.

### [BUG] — confirmed misbehaviour

```markdown
## BUG: <one-line summary>

- **Surface**: `browse` / `share` / etc.
- **Steps to reproduce**:
  1. ...
  2. ...
  3. ...
- **Expected**: ...
- **Actual**: ...
- **Screenshot**: `audit/screenshots-interactive/<surface>-after.png`
  (or a fresh screenshot if the bug is timing-dependent)
- **Pattern class**: redirect-contract-drift / dialog-guard /
  stale-status-panel / duplicate-job / toast-mojibake /
  feedback-silent / other (see COMMON_BUGS.md)
- **Severity**: blocker / concern / suggestion
  - blocker: feature is unusable
  - concern: feature is wrong but workaround exists
  - suggestion: cosmetic or workflow improvement
- **Notes**: any extra context. If the bug is a regression from
  a known fix, link the commit SHA.
```

### [FEATURE] — missing functionality

```markdown
## FEATURE: <one-line summary>

- **Surface**: ...
- **User story**: as a <who>, I want <what>, so I can <why>.
- **Current behaviour**: ...
- **Desired behaviour**: ...
- **Acceptance criteria**:
  - [ ] ...
  - [ ] ...
- **Notes**: ...
```

### [CORRECTION] — wording, copy, terminology

```markdown
## CORRECTION: <one-line summary>

- **Surface**: ...
- **Current text**: "..."
- **Suggested text**: "..."
- **Why**: link to CONTEXT.md glossary entry, or explain
  why the current term is wrong.
```

---

## Filing the issues

After the walk, run:

```bash
# Triage (per-finding; you decide which is a real issue)
gh issue create --label needs-triage --label bug --title "..." --body-file <(extract one block)
```

The repo's `ready-for-agent` label is the right label for issues
you want a coding agent to pick up. The `ready-for-human` label
is for issues that need your judgement. See
[`docs/agents/triage-labels.md`](triage-labels.md).

---

## When to grow this playbook

- After every UI fix lands, add a `## <pattern>` section to
  `docs/COMMON_BUGS.md` if the fix is a recurring class.
- After every manual audit round, add the new findings to the
  grep cookbook in
  [`docs/agents/bug-pattern-grep.md`](bug-pattern-grep.md) so
  the next round of CI catches them automatically.
- After the surface list grows past 12, split the playbook into
  per-domain files (`playbook-calendar.md`, `playbook-archive.md`,
  etc.) and link them from here.

---

## What's NOT in this playbook

- **Axe-core accessibility sweep**: that's the
  `audit/run-round3.mjs` script. It runs axe-core across every
  route and writes findings to `audit/reports-r3/`.
- **Visual screenshot diff**: that's the
  `audit/run.mjs` script. It takes a screenshot of every route
  at every viewport and diffs against the previous round.
- **Wails-specific flows**: native dialogs, file:// open, Quit,
  BrowserOpenURL. See §"PATH FORWARD" in
  [`docs/COMMON_BUGS.md`](../COMMON_BUGS.md#12-checklist-for-new-features)
  for the plan to bring these under the audit harness.

This playbook is the **layer above** those automated sweeps. It
exists to find what automation can't: the user's experience.
