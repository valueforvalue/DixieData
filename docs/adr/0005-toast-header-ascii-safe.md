# Toast header text must be ASCII-safe before crossing the HTTP boundary

## Status

Accepted 2026-06-29.

## Context

Toast text for every DixieData action (export, import, settings change, feedback submit, etc.) is shipped to the frontend via the `X-DixieData-Toast` response header. The contract was established in ADR 0004 (Option C dispatcher) and is used by ~80 call sites that call `setToastHeader` / `setToastHeaderWithType` / `setInfoToastHeader` in `internal/appshell/exports_handlers.go`.

Issue #135 (`d91e32c`) previously fixed a source-level bug where Go's seven-char ASCII literal `\u2026` was being shipped verbatim — Go does not interpret `\uXXXX` escapes inside ordinary double-quoted strings. The fix replaced the literal with the actual U+2026 HORIZONTAL ELLIPSIS rune in 10 production files and added a source-level regression net (`TestInProgressToastStringsContainActualEllipsis`).

After the #135 fix shipped, the toast `Shared archive import started…` still renders as `Shared archive import startedâ¦` in the browser. The bytes over the wire are correct UTF-8 (`e2 80 a6`) — `curl -i` confirms — but Chromium decodes HTTP/1.x response headers as **Windows-1252**, not UTF-8, per the WHATWG Fetch spec. Only HTTP/2 has explicit UTF-8 header support. Each UTF-8 byte above `0x7F` is reinterpreted as a Windows-1252 codepoint, producing the visible mojibake.

This affects every non-ASCII character in any toast text: ellipsis (`…`), em-dash (`—`), en-dash (`–`), smart quotes (`'` `"` `'` `"`), non-breaking space, accented Latin letters, etc. The user-visible bug is identical for all of them: the bytes look correct on the server, but the toast shows garbage.

## Decision

Add a small ASCII-safe substitution table at the boundary where toast text enters the response header. The table maps every common non-ASCII punctuation mark to its ASCII equivalent (ellipsis → `...`, em-dash → `--`, etc.). The substitution happens inside `setToastHeader` / `setToastHeaderWithType`, so every existing caller benefits without changing the call sites. Source code keeps the polished Unicode characters; only the wire payload is ASCII.

The substitution table is deliberately not a "strip everything above 0x7F" blanket rule. Stripping would silently mangle user data that legitimately contains non-ASCII (a future "Saved record for José" toast, accented place names in the calendar, etc.). The table only covers punctuation that has a clean ASCII twin — the class of characters that has zero information loss in the substitution.

## Alternatives considered

- **Move toast text into the JSON response body.** Touches every handler + dispatcher; introduces a second contract alongside the existing header. Rejected as disproportionate to the bug (the LLM session protocol in AGENTS.md says: match the weight of the process to the weight of the task).
- **Strip all non-ASCII bytes before sending the header.** One-line fix, but silently mangles any future toast that quotes user input. Rejected as too aggressive.
- **Base64-encode the toast payload in the header.** Survives the Windows-1252 decoding (bytes are all `< 0x80` after encoding), but obscures the wire format and forces every debug dump of the header to be decoded by hand. Rejected as opaque.

## Consequences

- All 80 existing `setToastHeader*` call sites continue to write source-level Unicode characters. The user sees ASCII twins in toasts that contain punctuation, but no information loss.
- Future toast additions automatically pick up the substitution. Authors do not need to remember to ASCII-ify their strings.
- The source-level regression net `TestInProgressToastStringsContainActualEllipsis` is extended with a sibling that exercises `sanitiseToastForHeader` on every punctuation character and asserts the round-trip is loss-free.
- The substitution table is a single source of truth. Future additions go through review because adding a new entry changes the user-visible toast text for every caller that uses the character.

## Implementation notes

- `sanitiseToastForHeader(string) string` lives next to `setToastHeader` in `internal/appshell/exports_handlers.go`. The dispatcher does not need to know about encoding — the contract boundary is the header write.
- The substitution table is a `map[rune]string` keyed on the Unicode codepoint, not a regex. A regex would be slower and harder to audit one entry at a time.
- The current list: `…` → `...`, `—` → `--`, `–` → `-`, `'` `'` → `'`, `"` `"` → `"`, ` ` (NBSP) → ` ` (regular space), `…` (single-char ellipsis) → `...`. Extend as new punctuation appears in toasts; user-data characters (accented Latin, CJK, etc.) intentionally pass through unchanged so future toasts that quote user input are not mangled.