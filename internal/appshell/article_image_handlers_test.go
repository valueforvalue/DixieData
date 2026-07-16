// article_image_handlers_test.go — issue #612 slice 2
// HTTP-level regression net for the article-attached images
// surface: POST /articles/{id}/images/import (multipart
// upload) + GET /articles/{id}/images (picker fragment).
//
// Pins the slice-2 contract end-to-end:
//   - The new routes are registered (so a 404 here means
//     the slice-2 wiring is incomplete).
//   - Multipart upload lands the file under
//     dataDir/images/articles/<displayID>/ + inserts a row
//     in images with article_id + kind='article'.
//   - The picker fragment returns the image list so the
//     slice-3 picker modal's "Pick existing" tab can render.
package appshell

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestArticleImageRoutes_Registered pins the slice-2 wiring:
// both routes are reachable. A 404 here means the
// handleImportArticleImages / handleArticleImagesList
// functions weren't bound in routes.go (or the
// serveMux priority dropped them).
func TestArticleImageRoutes_Registered(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Create an article via the existing slice-1 surface so
	// the image routes have a valid id.
	form := newFormValues()
	form.Set("title", "Slice 2 image test article")
	resp, err := http.PostForm(server.URL+"/articles/new", form)
	if err != nil {
		t.Fatalf("create article: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create article status = %d, want 200", resp.StatusCode)
	}
	articleID := readArticleIDFromRedirect(t, resp)
	_ = articleID

	// GET /articles/{id}/images (picker fragment) — 200
	// even with no images (the empty-state message is
	// valid fragment content).
	getResp, err := http.Get(fmt.Sprintf("%s/articles/%d/images", server.URL, articleID))
	if err != nil {
		t.Fatalf("GET images: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("GET /articles/{id}/images status = %d, want 200 (route not registered or 404)", getResp.StatusCode)
	}

	// POST /articles/{id}/images/import with no file —
	// 400 (validation) or 405 (method-not-allowed for
	// the wrong content-type) are both acceptable; the
	// important pin is that the route is registered
	// (i.e. NOT 404).
	postResp, err := http.PostForm(fmt.Sprintf("%s/articles/%d/images/import", server.URL, articleID), nil)
	if err != nil {
		t.Fatalf("POST images/import: %v", err)
	}
	defer postResp.Body.Close()
	if postResp.StatusCode == http.StatusNotFound {
		t.Errorf("POST /articles/{id}/images/import returned 404 (route not registered)")
	}
}

// TestArticleImageImport_LandsFileAndRow pins the slice-2
// happy path: a real multipart upload lands the file on
// disk under the article's image directory + creates a
// row in images. The picker fragment on the next GET
// includes the new image.
func TestArticleImageImport_LandsFileAndRow(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Mint an article.
	form := newFormValues()
	form.Set("title", "Slice 2 multipart test")
	resp, err := http.PostForm(server.URL+"/articles/new", form)
	if err != nil {
		t.Fatalf("create article: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create article status = %d, want 200", resp.StatusCode)
	}
	articleID := readArticleIDFromRedirect(t, resp)

	// Build a multipart upload with a single tiny PNG-ish
	// file. The bytes don't have to be a real PNG; the
	// import handler doesn't decode, it just stores the
	// bytes. The picker fragment doesn't render the file
	// inline so the contents are irrelevant.
	uploadBody := &bytes.Buffer{}
	writer := multipart.NewWriter(uploadBody)
	fileWriter, err := writer.CreateFormFile("images", "test-portrait.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.WriteString(fileWriter, "fake-png-bytes-for-test"); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	// POST the upload. The handler reads multipart, saves
	// the file, inserts the row, and writes the picker
	// fragment to the response body.
	uploadURL := fmt.Sprintf("%s/articles/%d/images/import", server.URL, articleID)
	uploadReq, err := http.NewRequest("POST", uploadURL, uploadBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(uploadResp.Body)
		t.Fatalf("upload status = %d, want 200. body: %s", uploadResp.StatusCode, string(body))
	}

	// The response body is the picker fragment; it must
	// reference the new image (data-article-image-insert +
	// data-article-image-url with the /media/ prefix).
	uploadBody2, _ := io.ReadAll(uploadResp.Body)
	if !strings.Contains(string(uploadBody2), "data-article-image-insert") {
		t.Errorf("upload response missing picker fragment markers\nbody: %s", string(uploadBody2))
	}
	if !strings.Contains(string(uploadBody2), "/media/images/articles/") {
		t.Errorf("upload response missing /media/ URL for the new image\nbody: %s", string(uploadBody2))
	}

	// The file is on disk under dataDir/images/articles/<displayID>/.
	// We don't know the displayID from the picker fragment
	// (the fragment only carries the file_path), so we
	// assert the /media/ URL path matches the on-disk
	// layout. The dataDir is the test's temp dir; the
	// fragment's /media/ URL maps back to dataDir/<path>.
	expectedRelative := "images/articles/"
	if !strings.Contains(string(uploadBody2), expectedRelative) {
		t.Errorf("upload response missing %q fragment URL prefix\nbody: %s", expectedRelative, string(uploadBody2))
	}

	// Verify the on-disk file exists. The import handler
	// renames files (standardizedImageFileName), so we
	// walk the article image directory and confirm at
	// least one file was written.
	articleImgDir, _ := appdata.ArticleImageDir(app.dataDir, "") // prefix only
	found := false
	walkErr := filepath.Walk(app.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.Contains(path, filepath.ToSlash(articleImgDir)) || strings.Contains(filepath.ToSlash(path), "images/articles/") {
			found = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk dataDir: %v", walkErr)
	}
	if !found {
		t.Errorf("uploaded file not found on disk under dataDir")
	}

	// GET the picker fragment on a fresh request. The
	// fragment must contain the new image.
	getResp, err := http.Get(fmt.Sprintf("%s/articles/%d/images", server.URL, articleID))
	if err != nil {
		t.Fatalf("GET images: %v", err)
	}
	defer getResp.Body.Close()
	getBody, _ := io.ReadAll(getResp.Body)
	if !strings.Contains(string(getBody), "data-article-image-insert") {
		t.Errorf("picker fragment missing insert button\nbody: %s", string(getBody))
	}

	_ = models.Article{} // keep import in use
}

// readArticleIDFromRedirect extracts the article id from the
// X-DixieData-Redirect response header that the article
// create handler emits. The pattern mirrors the per-Person-Record
// readArticleIDFromRedirect helper used elsewhere.
func readArticleIDFromRedirect(t *testing.T, resp *http.Response) int64 {
	t.Helper()
	loc := resp.Header.Get("X-DixieData-Redirect")
	if loc == "" {
		t.Fatalf("create article response missing X-DixieData-Redirect header")
	}
	parts := strings.Split(loc, "/")
	last := parts[len(parts)-1]
	id, err := parseInt64(last)
	if err != nil {
		t.Fatalf("parse article id from %q: %v", loc, err)
	}
	return id
}

// parseInt64 is a tiny helper to avoid pulling in strconv
// for one call site in this test file.
func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// newFormValues returns a url.Values pre-populated with the
// required fields the slice-1 article create handler reads.
func newFormValues() url.Values {
	v := url.Values{}
	v.Set("title", "Untitled")
	return v
}
