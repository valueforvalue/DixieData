package supportuploader

// DefaultFormsparkEndpoint is the production Formspark form
// owned by DixieData (Formspark free plan, 250 submissions /
// month per the vendor's pricing page; this constant exists
// so the wire-format contract is reviewable in one place and
// a future swap is a single-line change). The value is a
// public form ID — Formspark form IDs are not secrets, so
// hardcoding the URL in the desktop binary does not expose
// any private material. The user supplies no token; the
// receiver identifies submissions by the Formspark dashboard
// login only.
//
// Issue #566 locked decision 1 (provider), locked decision 2
// (hardcode in binary), and locked decision 9 (no third-party
// Go module required). To change the endpoint, also update
// docs/THIRDPARTY.md and the CHANGELOG.
const DefaultFormsparkEndpoint = "https://submit-form.com/vJSONT1nB"
