package templates

// reviewQueueMenuItem returns the HTML for the "Open Review
// Queue" mega-menu menuitem, choosing between the default
// neutral style and the red-border "has open reviews" style
// based on the openReview flag. Used by issue #380 slice 3's
// Share & Review mega-menu (which absorbs the pre-#380 R&R
// foldout).
//
// The flag echo logic was originally inside the foldout's
// `@components.FoldoutWithBadge` children block as an
// `if layoutHasOpenReview(ctx) { ... } else { ... }`. That
// shape works inside foldout's children templ body, but it
// can't survive as one of the elements of a Go slice literal
// in the new mega-menu's `Items: []templ.Component{ ... }`
// argument — templ parses slice literals as comma-separated
// expressions and an if/else inside a struct-literal value
// position doesn't bind cleanly. Extracting the branch into
// a helper that returns a single templ.Raw-friendly string
// keeps the slice literal flat (one element per menuitem)
// and the open-count flag logic intact.
//
// The two output strings are byte-identical to the pre-#380
// foldout menuitem HTML except for the class swap (foldout-
// menuitem -> mega-menu-item) per slice 2's mega-menu CSS
// conventions.
func reviewQueueMenuItem(openReview bool) string {
	if openReview {
		// Issue #472 follow-up: the previous `border border-review-red/60`
		// + `bg-review-red/[0.18]` combo read as pink-on-pink because
		// the alpha was too low and the default-theme text color was
		// #fbe1de (a near-pink cream). The fix is a thicker 2px solid
		// red border + 0.32 alpha background + a brighter cream text
		// (#fff5f1) so the cue reads as "this needs attention" at a
		// glance. Per-theme overrides in tailwind.css target
		// `.mega-menu-item[data-research-review-has-count]` under
		// `[data-theme="high-contrast"|"soft"]` (HC = deep red on
		// light-red bg, Soft = deep red on warm-light-red bg) so
		// the urgency cue reads in every theme. The menuitem
		// moved from the retired R&R foldout to the Share &
		// Review mega-menu during issue #380 slice 3, and the
		// per-theme overrides had to follow — the pre-#380
		// `.foldout-menuitem[data-research-review-has-count]`
		// selectors are now orphaned (defensive-only).
		return `<li role="none"><a href="/review-queue" role="menuitem" class="mega-menu-item pill-link justify-start w-full border-2 border-review-red bg-review-red/[0.32] hover:bg-review-red/[0.48]" data-research-menu-review-queue data-research-review-has-count>Open Review Queue</a></li>`
	}
	return `<li role="none"><a href="/review-queue" role="menuitem" class="mega-menu-item pill-link justify-start w-full" data-research-menu-review-queue>Open Review Queue</a></li>`
}