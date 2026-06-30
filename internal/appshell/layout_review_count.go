package appshell

import (
	"html"
	"net/http"
)

// handleLayoutReviewCount returns the small HTML fragment rendered
// inside the Review Queue nav-link badge. Polled by the layout at
// 30s cadence (see layout.templ's data-layout-review-count hx-get).
// Issue #180.
//
// On error or zero count the fragment is empty so the badge
// collapses to nothing — no JS state to manage, the badge simply
// disappears between polls.
//
// Response shape: a single <span> with classes matching the layout
// badge style, or empty string when count is zero.
func (a *App) handleLayoutReviewCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	count, err := a.soldiers.CountNeedsReview()
	if err != nil || count <= 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(""))
		return
	}
	display := ""
	if count > 99 {
		display = "99+"
	} else {
		display = itoaSmall(count)
	}
	fragment := `<span class="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-[#6f2c26] px-1.5 text-xs font-bold text-[#fff8e7]" aria-label="` +
		html.EscapeString(itoaSmall(count)) + ` records pending review">` +
		display + `</span>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fragment))
}

// itoaSmall converts a small non-negative int to its decimal string
// without importing strconv just for one call site. Matches the
// layout badge's int range (0..99+ capped), so no overflow concern.
func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}