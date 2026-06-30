package appshell

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

func TestExportTemplateHandlers_ListEmpty(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/export/templates")
	if err != nil {
		t.Fatalf("GET /export/templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"templates":[]`) {
		t.Errorf("expected empty list in body: %s", body)
	}
}

func TestExportTemplateHandlers_SaveAndApply(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("template_name", "Co. A 5VA")
	form.Set("scope", "filtered")
	form.Set("sort_by", "death_year")
	form.Set("orientation", "L")
	form.Set("filter_unit", "5th Virginia Infantry")
	form.Set("group_by_buried_in", "1")

	resp, err := http.PostForm(server.URL+"/export/templates", form)
	if err != nil {
		t.Fatalf("POST /export/templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /export/templates status %d, want 200; body: %s", resp.StatusCode, body)
	}
	var saveResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&saveResp); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	id, ok := saveResp["id"].(float64)
	if !ok {
		t.Fatalf("save response missing numeric id: %v", saveResp)
	}
	tid := int64(id)

	applyResp, err := http.PostForm(server.URL+"/export/templates/"+strconv.FormatInt(tid, 10)+"/apply", url.Values{})
	if err != nil {
		t.Fatalf("POST apply: %v", err)
	}
	defer applyResp.Body.Close()
	if applyResp.StatusCode != http.StatusOK {
		t.Fatalf("apply status %d, want 200", applyResp.StatusCode)
	}
	var applyEnvelope struct {
		Template map[string]any `json:"template"`
		Warnings []string      `json:"warnings"`
	}
	if err := json.NewDecoder(applyResp.Body).Decode(&applyEnvelope); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	template := applyEnvelope.Template
	if applyEnvelope.Warnings == nil {
		t.Errorf("warnings field missing from response (should be empty array)")
	}
	if template["name"] != "Co. A 5VA" {
		t.Errorf("apply name = %v, want Co. A 5VA", template["name"])
	}
	if template["scope"] != "filtered" {
		t.Errorf("apply scope = %v, want filtered", template["scope"])
	}
	filters, ok := template["filters"].(map[string]any)
	if !ok {
		t.Fatalf("apply filters = %T, want map", template["filters"])
	}
	units, _ := filters["unit"].([]any)
	if len(units) != 1 || units[0] != "5th Virginia Infantry" {
		t.Errorf("apply filters[unit] = %v, want [5th Virginia Infantry]", units)
	}
	groupBy, _ := template["group_by"].([]any)
	if len(groupBy) != 1 || groupBy[0] != "buried_in" {
		t.Errorf("apply group_by = %v, want [buried_in]", groupBy)
	}
}

func TestExportTemplateHandlers_DuplicateName409(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("template_name", "DupTest")
	form.Set("scope", "all")
	if resp, err := http.PostForm(server.URL+"/export/templates", form); err != nil {
		t.Fatalf("first POST: %v", err)
	} else {
		resp.Body.Close()
	}

	resp2, err := http.PostForm(server.URL+"/export/templates", form)
	if err != nil {
		t.Fatalf("second POST: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("duplicate name status %d, want 409", resp2.StatusCode)
	}
}

func TestExportTemplateHandlers_Delete(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	template, err := app.exportTemplates.Create(records.ExportTemplate{Name: "ToDelete", Scope: "all"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	req, err := http.NewRequest(http.MethodDelete, server.URL+"/export/templates/"+strconv.FormatInt(template.ID, 10), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status %d, want 204", resp.StatusCode)
	}
	_, err = app.exportTemplates.Get(template.ID)
	if err == nil {
		t.Errorf("Get after Delete: nil err, want not found")
	}
}

func TestExportTemplateHandlers_ApplyNotFound404(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.PostForm(server.URL+"/export/templates/99999/apply", url.Values{})
	if err != nil {
		t.Fatalf("POST apply: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

// (no helper needed; strconv.FormatInt is used inline above)
// TestExportTemplateHandlers_StaleFilterWarning seeds a record,
// saves a template that filters on its unit, deletes the record
// (so the filter value is now stale), calls apply, and asserts the
// response carries a warning naming the stale value. Issue #181.
func TestExportTemplateHandlers_StaleFilterWarning(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	soldier, err := app.soldiers.Create(models.Soldier{
		DisplayID: "STALE-001",
		FirstName: "Stale",
		LastName:  "Record",
		Unit:      "Vanishing Regiment",
	})
	if err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	form := url.Values{}
	form.Set("template_name", "Stale Test")
	form.Set("scope", "filtered")
	form.Set("filter_unit", "Vanishing Regiment")
	saveResp, err := http.PostForm(server.URL+"/export/templates", form)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	defer saveResp.Body.Close()
	var saveResult map[string]any
	if err := json.NewDecoder(saveResp.Body).Decode(&saveResult); err != nil {
		t.Fatalf("decode save: %v", err)
	}
	tid := int64(saveResult["id"].(float64))

	// Delete the only matching record to make the filter stale.
	if err := app.soldiers.Delete(soldier.ID); err != nil {
		t.Fatalf("Delete seed: %v", err)
	}

	applyResp, err := http.PostForm(server.URL+"/export/templates/"+strconv.FormatInt(tid, 10)+"/apply", url.Values{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer applyResp.Body.Close()
	var envelope struct {
		Template map[string]any `json:"template"`
		Warnings []string      `json:"warnings"`
	}
	if err := json.NewDecoder(applyResp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 stale-unit warning", envelope.Warnings)
	}
	if !strings.Contains(envelope.Warnings[0], "Vanishing Regiment") {
		t.Errorf("warning %q should name the stale value", envelope.Warnings[0])
	}
	if !strings.Contains(envelope.Warnings[0], "Unit") {
		t.Errorf("warning %q should label the filter family", envelope.Warnings[0])
	}
}

// TestExportTemplateHandlers_CleanTemplateNoWarnings is the
// regression net — a template whose filter values still match
// records in the archive should produce no warnings.
func TestExportTemplateHandlers_CleanTemplateNoWarnings(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	_, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CLEAN-001",
		FirstName: "Clean",
		LastName:  "Tester",
		Unit:      "Enduring Regiment",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	form := url.Values{}
	form.Set("template_name", "Clean Test")
	form.Set("scope", "filtered")
	form.Set("filter_unit", "Enduring Regiment")
	saveResp, err := http.PostForm(server.URL+"/export/templates", form)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	defer saveResp.Body.Close()
	var saveResult map[string]any
	if err := json.NewDecoder(saveResp.Body).Decode(&saveResult); err != nil {
		t.Fatalf("decode save: %v", err)
	}
	tid := int64(saveResult["id"].(float64))

	applyResp, err := http.PostForm(server.URL+"/export/templates/"+strconv.FormatInt(tid, 10)+"/apply", url.Values{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer applyResp.Body.Close()
	var envelope struct {
		Template map[string]any `json:"template"`
		Warnings []string      `json:"warnings"`
	}
	if err := json.NewDecoder(applyResp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Warnings) != 0 {
		t.Errorf("warnings = %v, want empty (clean template)", envelope.Warnings)
	}
}

// TestExportTemplateHandlers_StaleSelectedIDWarning is the
// scope=selected stale-ID variant. Saves a template with explicit
// selected_ids, deletes those records, calls apply, asserts the
// response carries one warning per missing ID.
func TestExportTemplateHandlers_StaleSelectedIDWarning(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	a, err := app.soldiers.Create(models.Soldier{
		DisplayID: "SEL-001",
		FirstName: "Sel",
		LastName:  "One",
	})
	if err != nil {
		t.Fatalf("seed a: %v", err)
	}
	b, err := app.soldiers.Create(models.Soldier{
		DisplayID: "SEL-002",
		FirstName: "Sel",
		LastName:  "Two",
	})
	if err != nil {
		t.Fatalf("seed b: %v", err)
	}

	form := url.Values{}
	form.Set("template_name", "Selected Test")
	form.Set("scope", "selected")
	form.Add("selected_ids", strconv.FormatInt(a.ID, 10))
	form.Add("selected_ids", strconv.FormatInt(b.ID, 10))
	saveResp, err := http.PostForm(server.URL+"/export/templates", form)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	defer saveResp.Body.Close()
	var saveResult map[string]any
	if err := json.NewDecoder(saveResp.Body).Decode(&saveResult); err != nil {
		t.Fatalf("decode save: %v", err)
	}
	tid := int64(saveResult["id"].(float64))

	if err := app.soldiers.Delete(a.ID); err != nil {
		t.Fatalf("Delete a: %v", err)
	}

	applyResp, err := http.PostForm(server.URL+"/export/templates/"+strconv.FormatInt(tid, 10)+"/apply", url.Values{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer applyResp.Body.Close()
	var envelope struct {
		Template map[string]any `json:"template"`
		Warnings []string      `json:"warnings"`
	}
	if err := json.NewDecoder(applyResp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 stale-id warning", envelope.Warnings)
	}
	if !strings.Contains(envelope.Warnings[0], strconv.FormatInt(a.ID, 10)) {
		t.Errorf("warning %q should name the missing ID %d", envelope.Warnings[0], a.ID)
	}
}
