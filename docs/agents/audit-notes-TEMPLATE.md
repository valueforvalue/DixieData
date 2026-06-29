# Manual UI Audit Notes — <DATE>

> **How to use this file.** Walk each surface in order. For every
> finding, fill in the matching block. Use ONE block per finding
> — do not bundle multiple bugs into one. Delete the
> `[placeholder]` lines as you go. If a surface has no findings,
> leave the section empty.
>
> See [`docs/agents/manual-audit-playbook.md`](manual-audit-playbook.md)
> for the full protocol. See
> [`docs/COMMON_BUGS.md`](../COMMON_BUGS.md) for the pattern
> reference. Run `node audit/run-interactive.mjs` to walk the
> surfaces — this file is for capturing what the script can't.

## Meta

- **Date**: <YYYY-MM-DD>
- **Walker**: <your name>
- **Build**: `dev` @ `<commit SHA>` (run `git rev-parse HEAD`)
- **Server**: `dixiedata-web.exe` at `http://127.0.0.1:8765`
- **Scratch dir**: `.scratch/webmode` (seeded with 25 soldiers)
- **Script result**: <paste the final "X pass, Y fail, Z manual" line from the script>

---

## Surface: calendar (3 auto + 1 manual)

> Calendar grid + month navigation + export menu.
> **Manual prompt**: click a day in the grid. Verify a details
> pane loads with the day number, holidays, and matching
> anniversaries.

### Findings

<!-- FILL IN BELOW. DELETE THIS COMMENT. -->

---

## Surface: soldier-new (1 auto + 1 manual)

> Add Person Record form — required fields, submit, redirect.
> **Manual prompt**: submit the empty form. Note which field
> is required.

### Findings

---

## Surface: browse (2 auto + 1 manual)

> Browse view — list of all soldiers, filter, pagination.
> **Manual prompt**: click a soldier name and verify the
> detail page loads.

### Findings

---

## Surface: share (3 auto)

> Share exports — print config modal, all export buttons
> navigate to /jobs/{id}.

### Findings

---

## Surface: settings (3 auto)

> Settings — scan/quality results render, debug mode toggle.

### Findings

---

## Surface: feedback-modal (3 auto)

> Floating dock Feedback button — save flow + confirmation
> toast + form clears.

### Findings

---

## Surface: floating-dock-layout (1 auto + 1 manual)

> Floating dock positioning, mobile overflow, no content
> overlap. **Manual prompt**: open the floating nav and the
> Feedback modal. Verify the dock stays below the modal
> backdrop.

### Findings

---

## Surface: jobs-page (2 auto)

> `/jobs/active` endpoint + auto-poll on every page that
> has the layout shell.

### Findings

---

## Summary

| Severity | Count | Issues |
|---|---|---|
| Blocker | 0 | — |
| Concern | 0 | — |
| Suggestion | 0 | — |
| Feature | 0 | — |
| Correction | 0 | — |

**Total findings**: 0

### Issues to file

<!-- Copy each [BUG] / [FEATURE] / [CORRECTION] block into a separate
     GitHub issue via `gh issue create --body-file`. Update the
     table above as you file them. -->

### Patterns to add to COMMON_BUGS.md

<!-- If you noticed a pattern that recurs across multiple findings,
     add it to the §11 "Bug class → first place to look" table in
     `docs/COMMON_BUGS.md` and to the grep cookbook in
     `docs/agents/bug-pattern-grep.md`. -->

### Cross-round observations

<!-- Anything that doesn't fit a single finding. E.g. "the audit
     process is slow on X" or "the manual prompts could be more
     specific on Y". -->
