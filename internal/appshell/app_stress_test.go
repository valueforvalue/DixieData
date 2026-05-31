package appshell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func newStressApp(t *testing.T) *App {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	app := NewApp()
	app.WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	return app
}

func TestStressBridgeConcurrentSearchAndSave(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 10; i++ {
		if _, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("SEED-%03d", i),
			FirstName: "Seed",
			LastName:  fmt.Sprintf("Soldier-%d", i),
			Unit:      "Stress Regiment",
		}); err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	const workers = 60
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := server.URL + "/soldiers/search?q=Stress"
			if i%2 == 1 {
				target = server.URL + "/soldiers/search/advanced?unit=Stress%20Regiment&last_name=Soldier"
			}
			resp, err := client.Get(target)
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				errCh <- fmt.Errorf("bridge stress status=%d body=%q", resp.StatusCode, string(body))
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		form := url.Values{
			"display_id":              {fmt.Sprintf("STRESS-%03d", i)},
			"entry_type":              {"soldier"},
			"first_name":              {fmt.Sprintf("Stress-%d", i)},
			"last_name":               {"Hammer"},
			"unit":                    {"Stress Regiment"},
			"pension_state":           {"NA"},
			"confederate_home_status": {"None"},
		}
		resp, err := client.PostForm(server.URL+"/soldiers", form)
		if err != nil {
			t.Fatalf("PostForm sequential save %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("sequential save status=%d", resp.StatusCode)
		}
	}
}

func TestStressFilesystemChaosGracefulFailure(t *testing.T) {
	app := newStressApp(t)

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CHAOS-001",
		FirstName: "Chaos",
		LastName:  "Target",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := app.database.Close(); err != nil {
		t.Fatalf("Close database: %v", err)
	}
	app.database = nil
	_ = os.RemoveAll(app.dataDir)

	searchReq := httptest.NewRequest(http.MethodGet, "/soldiers/search?q=Chaos", nil)
	searchRec := httptest.NewRecorder()
	app.ServeHTTP(searchRec, searchReq)
	if searchRec.Code < http.StatusInternalServerError {
		t.Fatalf("expected graceful 5xx after data loss, got %d", searchRec.Code)
	}

	form := url.Values{
		"display_id":              {created.DisplayID},
		"entry_type":              {"soldier"},
		"first_name":              {created.FirstName},
		"last_name":               {created.LastName},
		"pension_state":           {"NA"},
		"confederate_home_status": {"None"},
	}
	saveReq := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(form.Encode()))
	saveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	saveRec := httptest.NewRecorder()
	app.ServeHTTP(saveRec, saveReq)
	if saveRec.Code < http.StatusInternalServerError {
		t.Fatalf("expected graceful 5xx on save after data loss, got %d", saveRec.Code)
	}
}

func TestStressAppLoggingToFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "stress_test.log")
	t.Setenv("DIXIEDATA_STRESS_LOG", logPath)
	t.Setenv("DIXIEDATA_DATA_DIR", filepath.Join(t.TempDir(), ".dixiedata"))
	defer resetStressLoggingForTests()

	app := NewApp()
	app.startup(context.Background())
	defer app.shutdown(context.Background())

	fmt.Fprintln(os.Stdout, "stress stdout probe")
	fmt.Fprintln(os.Stderr, "stress stderr probe")

	body, contentType := stressMultipartBody(t)
	req := httptest.NewRequest(http.MethodPost, "/soldiers", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile stress log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "stress stdout probe") || !strings.Contains(text, "stress stderr probe") {
		t.Fatalf("stress log missing stdout/stderr probes: %q", text)
	}
}

func stressMultipartBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"display_id":              "LOG-001",
		"entry_type":              "soldier",
		"first_name":              "Logger",
		"last_name":               "Probe",
		"pension_state":           "NA",
		"confederate_home_status": "None",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}
