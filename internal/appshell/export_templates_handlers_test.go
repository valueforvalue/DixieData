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
	var template map[string]any
	if err := json.NewDecoder(applyResp.Body).Decode(&template); err != nil {
		t.Fatalf("decode apply response: %v", err)
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