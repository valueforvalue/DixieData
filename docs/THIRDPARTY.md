# Third-party dependencies

DixieData uses the following third-party components that are not
vendored as Go modules.

## Typst

- **Version:** 0.15.0 (released 2026-06-15)
- **License:** Apache-2.0
- **Source:** https://github.com/typst/typst/releases/tag/v0.15.0
- **Bundled binaries:**
  - `bin/typst-windows.exe` (Windows x86_64)
  - `bin/typst-macos` (macOS Apple Silicon, aarch64)
  - `bin/typst-linux` (Linux x86_64, musl)
- **Used by:** `pkg/render` (via direct `os/exec` calls) to compile
  `.typ` templates to PDF.

The Typst binary is invoked via `exec.Command`, which shells out to
the bundled `typst-windows.exe` (or `typst` on PATH if the bundled
binary is absent). On Windows the child process is created with
`CREATE_NO_WINDOW` / `HideWindow` so the user does not see a black
console window during PDF export.

The license text and notice from the Typst upstream are committed at
`bin/LICENSE` and `bin/NOTICE`.

## Liberation Fonts

- **Version:** (bundled with Typst 0.15)
- **License:** SIL Open Font License 1.1
- **Source:** shipped with Typst as the default font family

The `Libertinus Serif` font is used as the default Typst font for
templates that don't specify a font family. Templates can override
this in their `#set text(font: ...)` call.

## Update procedure

1. Watch the Typst release page for new versions.
2. Download the new binaries from the matching assets.
3. Replace the files in `bin/`.
4. Update the version in this file.
5. Re-run the smoke test (`go test ./pkg/render/...`) to confirm
   the new binary still works.

## Formspark

- **Vendor:** Trampoline Software SRL
- **Privacy policy:** https://formspark.io/legal/privacy-policy/
- **Terms of service:** https://formspark.io/legal/terms-of-service/
- **GDPR:** https://formspark.io/legal/gdpr/
- **Endpoint (DixieData-owned form):** `https://submit-form.com/vJSONT1nB`
- **Plan:** Free (250 submissions/month, 10 forms, 1 workspace,
  per the vendor's pricing page; EU operation with Irish data
  storage per the privacy policy)
- **Used by:** `internal/supportuploader.UploadFeedbackFormspark`
  to deliver the **Save & Send to Support** action in the
  global feedback modal. The form is publicly identified (the
  URL is a public form ID, not a secret) and the desktop
  binary hardcodes the endpoint via
  `internal/supportuploader.DefaultFormsparkEndpoint`.

### Wire-format contract

The helper POSTs a single JSON body
(`Content-Type: application/json`, `Accept: application/json`)
to the endpoint:

| Field            | Type    | Notes                                            |
| ---------------- | ------- | ------------------------------------------------ |
| `subject`        | string  | synthesised `<category> · <page_path> · <contact-or-anonymous>` |
| `message`        | string  | verbatim user message                            |
| `page_path`      | string  | page the user was on when the modal opened       |
| `contact_name`   | string  | optional, omitempty                              |
| `contact_email`  | string  | optional, omitempty                              |
| `category`       | string  | one of bug / feature / research / general        |
| `app_version`    | string  | `buildinfo.AppVersion` (e.g. `1.1.1`)            |
| `build_identity` | string  | `buildinfo.BuildIdentity()` (e.g. `dev · commit dev`) |
| `schema_version` | number  | `buildinfo.SchemaVersion` (e.g. `67`)            |
| `submitted_at`   | string  | optional, omitempty, RFC 3339                    |

A 2xx response is treated as accepted. The vendor does not
promise a `ticket_id` field; the dashboard and email
notification are the delivery surfaces.

### Disclosed to the user

The feedback modal's PII disclosure paragraph (rendered above
the Send button) names every transmitted field with its
actual value, so the user sees exactly what the binary sends
before clicking. Values render from `buildinfo` (the canonical
chain per CONTEXT.md "Release counter N ≠ schema version"
law). The local JSONL write is always first; a remote failure
never loses the report.

### Update procedure

1. If the DixieData-owned form URL changes (vendor migration,
   or we move to a paid plan with a different endpoint):
   update `DefaultFormsparkEndpoint` in
   `internal/supportuploader/default_endpoint.go`.
2. Add a bullet to CHANGELOG `[Unreleased]` under `### Changed`
   naming the new URL.
3. Re-run `node audit/smoke_feedback_send.mjs` with
   `SMOKE_FEEDBACK_SEND_BROWSER=1 BASE=http://127.0.0.1:8901`
   to verify the modal disclosure + button wiring still match
   the new endpoint.
4. The `audit/smoke_feedback_send.mjs` network probe
   automatically validates the new endpoint returns 2xx + JSON
   for the exact payload shape.

