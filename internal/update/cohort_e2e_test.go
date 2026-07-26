// cohort_e2e_test.go — live end-to-end test that the DixieData
// updater can fetch + parse the cohort's hosted manifest
// (issue #654). Pin the wiring on every CI run so a future
// refactor of the manifest parser or the config-store key
// surfaces immediately.
//
// Skipped if the network is unavailable so this test doesn't
// gate CI on a flaky connection. Run with `go test
// -run TestCohortManifestEndToEnd -v` to force the live check.
package update

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const cohortManifestURL = "https://raw.githubusercontent.com/valueforvalue/dixiedata-rc-manifest/main/manifest.json"

func TestCohortManifestEndToEnd(t *testing.T) {
	if os.Getenv("DIXIEDATA_SKIP_NETWORK_TESTS") != "" {
		t.Skip("DIXIEDATA_SKIP_NETWORK_TESTS set; skipping live cohort manifest check")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cohortManifestURL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "DixieData-cohort-e2e/1.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("network unavailable (CI without outbound); skipping live check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manifest fetch returned HTTP %d; want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Round-trip through the same parser the updater uses.
	// The test asserts the manifest field names + version
	// shape the cohort publishes are accepted by
	// manifestReleaseFromJSON.
	u, err := url.Parse(cohortManifestURL)
	if err != nil {
		t.Fatalf("parse manifest URL: %v", err)
	}
	release, err := manifestReleaseFromJSON(body, u.String())
	if err != nil {
		t.Fatalf("manifestReleaseFromJSON: %v (manifest may have a field name drift)", err)
	}
	if !strings.HasSuffix(release.downloadURL, ".zip") {
		t.Errorf("asset_kind not detected as zip; downloadURL = %q", release.downloadURL)
	}
	// The cohort manifest advertises 1.1.5-rc1 (or whatever
	// the operator has published). The version parse strips
	// the suffix; we assert the bare numeric is 1.1.5 (the
	// cohort's base version). This catches a manifest
	// published with a wrong-version typo.
	if release.version != "1.1.16" {
		t.Errorf("parsed version = %q; want %q (suffix should strip to base numeric)", release.version, "1.1.16")
	}
}
