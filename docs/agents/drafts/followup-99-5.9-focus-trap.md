## Problem

The three modal dialogs added to the audit fix (#99, finding 5.14)
have `role="dialog"` and `aria-modal="true"` but no focus trap. When a
screen-reader or keyboard user opens the feedback modal, the
print-config modal, or the google-preferences modal, Tab and Shift+Tab
move focus to background controls behind the modal.

**Source:** 2026-06-24 full audit; deferred from issue #99.

## Goal

Modal dialogs trap focus while open and return focus to the trigger
on close.

## Approach

1. Convert each modal to a native `<dialog>` element with
   `showModal()` / `close()` rather than the current custom overlay
   `div`. Native `<dialog>` provides focus trap, ESC-to-close, and
   inert-background for free.
2. For any remaining custom overlay divs, add a small focus-trap
   utility to `frontend/app.js`: on dialog open, capture
   `document.activeElement`; install a keydown listener for Tab that
   wraps focus inside the dialog; on close, restore focus to the
   trigger and remove the listener.
3. Add a regression test that renders each modal in the layout test
   and asserts the dialog root is either a native `<dialog>` element
   or carries a `data-focus-trap` attribute that the new utility
   recognises.

## Files likely touched

- `internal/templates/layout.templ`
- `internal/templates/share.templ`
- `frontend/app.js`
- `internal/templates/layout_test.go` (regression)

## Out of scope

- Wails native focus-trap utilities (desktop window focus is
  OS-managed).