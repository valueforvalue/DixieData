// Share Queue HTTP handlers (issue #182). The queue itself
// lives in the browser's localStorage; these endpoints serve
// the server-side look-ups the queue needs:
//
//   - GET  /share/queue/modal   — render the Share Build modal
//                                 fragment (the modal body is
//                                 templ-rendered with a stub list;
//                                 the JS hydrates it from
//                                 localStorage on open).
//   - POST /share/queue/preview — given a `selected_ids` form,
//                                 return a tiny HTML summary
//                                 (Soldiers / Source Records /
//                                 Images counts) the modal
//                                 renders as the live preview.
//   - POST /share/queue/clear   — explicit Clear Queue handler;
//                                 the queue lives in localStorage
//                                 so the server has nothing to
//                                 delete, but a confirm() on the
//                                 client expects a 200 to dismiss.
//                                 Returns 204 to keep semantics
//                                 simple.
//
// The subset export path itself lives in exports_handlers.go
// (handleExportSharedArchive gains a `?subset=1` branch). All
// POST handlers write `X-DixieData-Redirect` per Option C
// (issue #130). The preview is a fragment, not a redirect.
package appshell

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// handleShareQueueModal renders the Share Build modal fragment.
// The modal's tbody is populated client-side from the
// localStorage queue; the server side just needs a page body
// the dispatcher can inject. The 501 fallback path is a safety
// net so the route exists before c5 lands the templ.
func (a *App) handleShareQueueModal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tpl := templates.ShareQueueModal()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Render(r.Context(), w); err != nil {
		respondInternal(w, r, "Could not render the Share Queue modal.", err)
	}
}

// handleShareQueuePreview accepts a `selected_ids` repeated
// form field and returns a tiny HTML summary (count-by-table)
// the modal renders as the live preview pane.
//
// Response shape: a single <div data-share-queue-preview>
// carrying three spans (`Soldiers: N`, `Source Records: N`,
// `Images: N`) so the modal can swap the inner content
// without re-rendering the surrounding modal chrome.
func (a *App) handleShareQueuePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the share-queue preview form.", err)
		return
	}
	raw := r.Form["selected_ids"]
	ids := parseShareQueueIDs(raw)
	if len(ids) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<div data-share-queue-preview class="text-sm text-slate-500">Pick at least one Person Record.</div>`))
		return
	}
	soldiers, err := a.soldiers.ByIDs(ids)
	if err != nil {
		respondInternal(w, r, "Could not load the staged soldiers.", err)
		return
	}
	// Image + Source Record counts: piggyback on the soldier list
	// payload (soldierListSelectColumns already pulls
	// (SELECT COUNT(*) FROM records/images ...)). No second round
	// trip.
	images := 0
	records := 0
	for _, s := range soldiers {
		images += s.ImageCount
		records += s.RecordCount
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := buildShareQueuePreviewFragment(len(soldiers), records, images)
	_, _ = w.Write([]byte(body))
}

// handleShareQueueClear is intentionally a no-op 204. The
// queue lives in localStorage, not on the server, so clearing
// is a client-side concern. The endpoint exists to give the
// dispatchDixieDataForm its 200 OK + X-DixieData-Redirect
// landing target so the UI has a single anchor for the action.
func (a *App) handleShareQueueClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DixieData-Redirect", "/share")
	w.WriteHeader(http.StatusOK)
}

// parseShareQueueIDs extracts the integer ids from a
// repeated `selected_ids` form field. TrimSpace + ErrNoRows
// tolerated so a stale id from a deleted row doesn't fail the
// preview fetch.
func parseShareQueueIDs(raw []string) []int64 {
	ids := make([]int64, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		if n <= 0 {
			continue
		}
		ids = append(ids, n)
	}
	return ids
}

// buildShareQueuePreviewFragment is a tiny HTML helper kept in
// its own function so the handler body stays readable.
func buildShareQueuePreviewFragment(soldiers, records, images int) string {
	var b strings.Builder
	b.WriteString(`<div data-share-queue-preview class="space-y-1 text-sm text-slate-700">`)
	b.WriteString(`<p class="font-semibold text-[#22303d]">`)
	b.WriteString(strconv.Itoa(soldiers))
	if soldiers != 1 {
		b.WriteString(" soldiers")
	} else {
		b.WriteString(" soldier")
	}
	b.WriteString(` will ship in this export.</p>`)
	b.WriteString(`<p class="text-xs text-slate-500">Source Records: `)
	b.WriteString(strconv.Itoa(records))
	b.WriteString(`</p>`)
	b.WriteString(`<p class="text-xs text-slate-500">Images: `)
	b.WriteString(strconv.Itoa(images))
	b.WriteString(`</p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// unused but kept to anchor the records import if future
// handlers extend with detach, etc. (issue #191 follow-up).
var _ = errors.New
var _ = context.Background
var _ = records.ExportTemplate{}

// handleShareQueuePage (issue #193) renders the
// /share/queue management page. The queue itself lives in
// the browser's localStorage; the client sends the staged
// ids as a comma-separated `?ids=1,2,3` query string so the
// server can look up the soldier rows + counts. Unknown ids
// are dropped silently (mirrors ByIDs's ErrNoRows-tolerance).
// If `?ids=` is missing or empty, the page renders with no
// rows + an empty-state message -- the JS hydrates the page
// from localStorage on load and re-fetches /share/queue?ids=
// with the live queue.
func (a *App) handleShareQueuePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawIDs := strings.Split(r.URL.Query().Get("ids"), ",")
	ids := parseShareQueueIDs(rawIDs)
	var rows []viewmodel.ShareQueueRow
	if len(ids) > 0 {
		soldiers, err := a.soldiers.ByIDs(ids)
		if err != nil {
			respondInternal(w, r, "Could not load the staged soldiers.", err)
			return
		}
		// Rebuild the rows in caller order so the table
		// mirrors the localStorage queue. ByIDs preserves
		// caller order (issue #182 contract).
		byID := make(map[int64]models.Soldier, len(soldiers))
		for _, s := range soldiers {
			byID[s.ID] = s
		}
		rows = make([]viewmodel.ShareQueueRow, 0, len(ids))
		for i, id := range ids {
			s, ok := byID[id]
			if !ok {
				continue
			}
			rows = append(rows, viewmodel.ShareQueueRow{
				Order:        i + 1,
				PersonRecord: viewmodel.PersonRecordFromModel(s),
			})
		}
	}
	presentation.ShareQueuePage(rows).Render(r.Context(), w)
}
