// Package htmxattr provides a strongly-typed builder for HTMX
// attributes. It exists to remove the wrong-selector bug class that
// haunts DixieData feature work: every time a template wrote
// `hx-target="#some-id"` or `hx-get="/some/route"` as a bare string
// literal, a future rename in another file silently broke the click.
//
// Mux collects the six attributes an HTMX element usually needs (URL,
// target, swap, trigger, select, confirm) and emits them as a single
// templ.Attributes map. URL values are wrapped in templ.SafeURL so
// routes stay escaped. Swap values are validated against an allowlist.
// Target values that look like CSS selectors (`#...` or `....`) are
// checked against the uiids surface registry and panic at startup if
// they don't resolve to a known surface; warn (don't panic) for ad-hoc
// selectors that opt out of the registry check.
//
// Typical use:
//
//	import "github.com/valueforvalue/DixieData/internal/htmxattr"
//	import "github.com/valueforvalue/DixieData/internal/routebuilder"
//	import "github.com/valueforvalue/DixieData/internal/uiids"
//
//	<div { htmxattr.Mux{
//	    Get:    routebuilder.JobStatus(jobID),
//	    Target: "#" + uiids.PanelJobStatus,
//	    Swap:   "outerHTML",
//	    Trigger: "every 2s",
//	}.Attrs()... }>...</div>
//
// The Attrs() return is a templ.Attributes map. Spread it with `{ m.Attrs()... }`
// in a templ element so the generated HTML carries the right
// `hx-*` attributes.
package htmxattr

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
	"github.com/valueforvalue/DixieData/internal/htmlids"
)

// allowedSwap lists the hx-swap values DixieData uses. Centralised so
// additions are deliberate and the test can keep the inventory tight.
// See https://htmx.org/attributes/hx-swap/ for the full grammar.
var allowedSwap = map[string]bool{
	"":            true, // default; absent attribute means innerHTML
	"innerHTML":   true,
	"outerHTML":   true,
	"beforebegin": true,
	"afterbegin":  true,
	"beforeend":   true,
	"afterend":    true,
	"delete":      true,
	"none":        true,
}

// Mux collects the HTMX attributes for one element. Zero value is
// usable but emits nothing; populate the fields you need.
type Mux struct {
	// Get is the hx-get URL. Wrapped in templ.SafeURL when emitted.
	Get string
	// Post is the hx-post URL. Wrapped in templ.SafeURL when emitted.
	Post string
	// Target is the hx-target selector. If it starts with "#" the
	// remainder is checked against the htmlids registry; non-matches
	// PANIC in dev builds (issue #316 slice 4) so the typo is caught
	// before the page ships. Ad-hoc selectors are no longer accepted
	// — add the id to internal/htmlids if a new Mux target needs it.
	Target string
	// Select is the hx-select selector. Same validation as Target.
	Select string
	// Swap is the hx-swap value. Validated against the allowlist;
	// invalid values panic at template render time so the bug is
	// caught early.
	Swap string
	// Trigger is the hx-trigger value. Emitted verbatim; no
	// validation (the htmx trigger grammar is too rich to whitelist).
	Trigger string
	// Confirm is the hx-confirm message. Emitted verbatim.
	Confirm string
}

// Attrs renders Mux into a templ.Attributes map containing only the
// fields that are non-empty. Empty fields emit no attribute, so the
// caller can safely spread the result on any element regardless of
// which subset of HTMX features it uses.
//
// Validation rules:
//
//   - If both Get and Post are set, the result is unspecified
//     (htmx itself ignores one of them; we emit both, htmx picks).
//   - Swap must be in the allowlist (or empty).
//   - Target and Select, if they start with "#", must match a
//     htmlids registry entry; otherwise a dev-build panic fires
//     with the offending selector.
func (m Mux) Attrs() templ.Attributes {
	validateSwap(m.Swap)
	validateTarget(m.Target)
	validateTarget(m.Select)

	out := templ.Attributes{}
	if strings.TrimSpace(m.Get) != "" {
		// Use plain string, NOT templ.SafeURL. templ.SafeURL is
		// a typed string that the templ runtime's RenderAttributes
		// doesn't have a case for in its type switch — when an
		// attribute value is a SafeURL, RenderAttributes silently
		// drops the entire attribute. This was the root cause of
		// every hx-get / hx-post button silently doing nothing
		// after PR #1 + PR #2 of the stabilization sprint. The
		// templ.SafeURL wrapper is only meaningful inside templ's
		// expression context (not in spread attributes).
		out["hx-get"] = m.Get
	}
	if strings.TrimSpace(m.Post) != "" {
		out["hx-post"] = m.Post
	}
	if strings.TrimSpace(m.Target) != "" {
		out["hx-target"] = m.Target
	}
	if strings.TrimSpace(m.Select) != "" {
		out["hx-select"] = m.Select
	}
	if strings.TrimSpace(m.Swap) != "" {
		out["hx-swap"] = m.Swap
	}
	if strings.TrimSpace(m.Trigger) != "" {
		out["hx-trigger"] = m.Trigger
	}
	if strings.TrimSpace(m.Confirm) != "" {
		out["hx-confirm"] = m.Confirm
	}
	return out
}

func validateSwap(swap string) {
	if swap == "" {
		return
	}
	if !allowedSwap[swap] {
		panic(fmt.Sprintf("htmxattr: invalid hx-swap value %q (allowed: innerHTML, outerHTML, beforebegin, afterbegin, beforeend, afterend, delete, none)", swap))
	}
}

func validateTarget(target string) {
	if target == "" {
		return
	}
	// Only validate registry-style selectors that begin with "#".
	// Class selectors (".foo"), attribute selectors ("[data-...]"),
	// the htmx self-targeting pseudo ("this"), and bare element
	// names ("body") are valid CSS that does not require an entry
	// in the htmlids registry.
	if !strings.HasPrefix(target, "#") {
		return
	}
	id := strings.TrimPrefix(target, "#")
	if id == "" {
		return
	}
	if !htmlids.Has(id) {
		// Dev-build panic (issue #316 slice 4). Catches typos like
		// #browze-results before the page ships. Every Mux{Target}
		// call site must have its id registered in
		// internal/htmlids/htmlids.go.
		panic(fmt.Sprintf("htmxattr: target %q is not in the htmlids registry", target))
	}
}