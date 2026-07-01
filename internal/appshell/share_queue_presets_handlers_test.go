package appshell

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// presetResponse mirrors the JSON shape the Save / List
// handlers return. Avoids importing records.ShareQueuePreset
// here so a future field rename stays a one-line change.
type presetResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TestShareQueuePresetsListEmpty (issue #192) asserts the
// GET /share/queue/presets endpoint returns 200 + an empty
// presets array when no presets exist.
func TestShareQueuePresetsListEmpty(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue/presets")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"presets":[]`) {
		t.Errorf("expected empty presets array; got %s", string(body))
	}
}

// TestShareQueuePresetsSaveAndList (issue #192) seeds a
// preset via POST /share/queue/presets, asserts the response
// carries {id, name}, then GETs /share/queue/presets and
// asserts the new preset shows up in the listing.
func TestShareQueuePresetsSaveAndList(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)

	form := url.Values{}
	form.Set("name", "Shiloh Cemetery")
	form.Add("soldier_ids", fmt.Sprintf("%d", pid1))
	form.Add("soldier_ids", fmt.Sprintf("%d", pid2))
	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("save status %d, want 200; body: %s", resp.StatusCode, string(body))
	}
	var saved presetResponse
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if saved.ID == 0 {
		t.Fatalf("save response has zero id")
	}
	if saved.Name != "Shiloh Cemetery" {
		t.Errorf("saved name = %q, want Shiloh Cemetery", saved.Name)
	}

	// List should now contain exactly one preset with the
	// same name.
	resp, err = http.Get(server.URL + "/share/queue/presets")
	if err != nil {
		t.Fatalf("GET list after save: %v", err)
	}
	defer resp.Body.Close()
	var listing struct {
		Presets []struct {
			ID         int64    `json:"id"`
			Name       string   `json:"name"`
			SoldierIDs []int64  `json:"soldier_ids"`
		} `json:"presets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listing.Presets) != 1 {
		t.Fatalf("list returned %d presets, want 1", len(listing.Presets))
	}
	if listing.Presets[0].Name != "Shiloh Cemetery" {
		t.Errorf("listed name = %q, want Shiloh Cemetery", listing.Presets[0].Name)
	}
	if len(listing.Presets[0].SoldierIDs) != 2 {
		t.Errorf("listed soldier_ids len = %d, want 2", len(listing.Presets[0].SoldierIDs))
	}
}

// TestShareQueuePresetsSaveDuplicateName (issue #192) asserts
// a second POST with the same name returns 409 (the UNIQUE
// constraint violation) rather than 500.
func TestShareQueuePresetsSaveDuplicateName(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid := seedPersonRecord(t, app)
	form := url.Values{}
	form.Set("name", "Shiloh Cemetery")
	form.Add("soldier_ids", fmt.Sprintf("%d", pid))

	if _, err := http.PostForm(server.URL+"/share/queue/presets", form); err != nil {
		t.Fatalf("first save: %v", err)
	}
	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate save status = %d, want 409", resp.StatusCode)
	}
}

// TestShareQueuePresetsSaveMissingName (issue #192) asserts
// the empty-name guard fires before the DB insert. The
// handler returns 400 via respondValidation.
func TestShareQueuePresetsSaveMissingName(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid := seedPersonRecord(t, app)
	form := url.Values{}
	form.Add("soldier_ids", fmt.Sprintf("%d", pid))

	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("save with empty name returned 200; want 400")
	}
}

// TestShareQueuePresetsSaveEmptyIDs (issue #192) asserts the
// handler refuses to save a preset with zero soldier_ids.
func TestShareQueuePresetsSaveEmptyIDs(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("name", "Empty")

	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("save with empty ids returned 200; want 400")
	}
}

// TestShareQueuePresetsDelete (issue #192) seeds a preset,
// then DELETEs /share/queue/presets/{id} and asserts 204 + a
// subsequent GET /share/queue/presets listing no longer
// includes it.
func TestShareQueuePresetsDelete(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid := seedPersonRecord(t, app)
	form := url.Values{}
	form.Set("name", "Shiloh Cemetery")
	form.Add("soldier_ids", fmt.Sprintf("%d", pid))
	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	var saved presetResponse
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+fmt.Sprintf("/share/queue/presets/%d", saved.ID), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", delResp.StatusCode)
	}

	// Listing should be empty again.
	resp, err = http.Get(server.URL + "/share/queue/presets")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer resp.Body.Close()
	var listing struct {
		Presets []struct {
			ID int64 `json:"id"`
		} `json:"presets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listing.Presets) != 0 {
		t.Errorf("list after delete returned %d presets, want 0", len(listing.Presets))
	}
}

// TestShareQueuePresetsDeleteMissing (issue #192) asserts a
// DELETE against an unknown id returns 404 (not 500).
func TestShareQueuePresetsDeleteMissing(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/share/queue/presets/99999", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing status = %d, want 404", resp.StatusCode)
	}
}

// TestShareQueuePresetsApply (issue #192) seeds a preset,
// then GETs /share/queue/presets/{id}/apply and asserts the
// returned JSON carries the saved soldier_ids array the
// modal's Load handler writes back to localStorage.
func TestShareQueuePresetsApply(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)
	form := url.Values{}
	form.Set("name", "Shiloh Cemetery")
	form.Add("soldier_ids", fmt.Sprintf("%d", pid1))
	form.Add("soldier_ids", fmt.Sprintf("%d", pid2))
	resp, err := http.PostForm(server.URL+"/share/queue/presets", form)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	var saved presetResponse
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Get(server.URL + fmt.Sprintf("/share/queue/presets/%d/apply", saved.ID))
	if err != nil {
		t.Fatalf("GET apply: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply status %d, want 200", resp.StatusCode)
	}
	var applied struct {
		Name       string  `json:"name"`
		SoldierIDs []int64 `json:"soldier_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&applied); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if applied.Name != "Shiloh Cemetery" {
		t.Errorf("applied name = %q, want Shiloh Cemetery", applied.Name)
	}
	if len(applied.SoldierIDs) != 2 {
		t.Errorf("applied soldier_ids len = %d, want 2", len(applied.SoldierIDs))
	}
}

// TestShareQueuePresetsApplyMissing (issue #192) asserts a
// GET /share/queue/presets/{id}/apply against an unknown id
// returns 404 (not 500).
func TestShareQueuePresetsApplyMissing(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue/presets/99999/apply")
	if err != nil {
		t.Fatalf("GET apply missing: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("apply missing status = %d, want 404", resp.StatusCode)
	}
}

// TestShareQueuePresetsRouteWildcardNotShadowed (issue #192)
// asserts the literal /share/queue/presets routes are not
// shadowed by the wildcard /share/queue/presets/{id:[0-9]+}
// routes. The existing route_wildcard_test harness would not
// catch a future refactor that re-orders registration, so
// this test pins the pattern with an explicit GET that
// returns 200 + an empty presets array (literal), not the
// {id:[0-9]+} apply handler's 404 for the same path.
func TestShareQueuePresetsRouteWildcardNotShadowed(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue/presets")
	if err != nil {
		t.Fatalf("GET literal: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("literal route status = %d, want 200 (wildcard shadow?)", resp.StatusCode)
	}
}