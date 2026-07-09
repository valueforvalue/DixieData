// Share Queue HTTP handlers (issue #193 + #310). The queue itself
// lives in the browser's localStorage; the only server endpoint
// is the /share/queue management page (handleShareQueuePage), which
// takes a comma-separated ?ids=1,2,3 query string the client
// populates from localStorage on page load + after every queue
// mutation. The Share Build modal that lived at /share/queue/modal
// (issue #182) and its supporting /share/queue/preview +
// /share/queue/clear endpoints were deleted in issue #310 PR 2;
// the page absorbs every modal capability (Saved Queues presets
// land on the page in issue #310 PR 3).
//
// The subset export path itself lives in exports_handlers.go
// (handleExportSharedArchive gains a `?subset=1` branch). Saved
// Queue preset endpoints live in share_queue_presets_handlers.go
// and are reused by PR 3 without change.
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
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// parseShareQueueIDs extracts the integer ids from a repeated
// `selected_ids` form field or from a comma-separated query
// string. TrimSpace + ErrNoRows tolerated so a stale id from a
// deleted row doesn't fail the page render.
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
	// Issue #384 / Slice 8: wrap Render.
	if err := presentation.ShareQueuePage(rows).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the share queue page.", err)
	}
}
