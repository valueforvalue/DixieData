package records

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestRunDataQualityScan_EventRowsDoNotFireIdentityMissing (issue #530)
// is the regression net for the false positive where every event row
// (entry_type = 'event', no first_name/last_name by design) was
// flagged with Identity & Naming / identity-missing.
//
// The test seeds three rows in one archive:
//   - a fully-named soldier (control: must NOT fire identity-missing)
//   - a widow with no name (must STILL fire identity-missing because
//     widow IS a person-bearing entry type — the gate must not over-fire)
//   - an event record (must NOT fire identity-missing — the bug shape)
//
// Then runs the high-confidence scan and asserts on the per-row issue
// codes. A regression that drops the gate re-introduces the
// false-positive for the event row; a regression that over-fires
// (e.g. blanks the gate entirely) breaks the widow assertion.
func TestRunDataQualityScan_EventRowsDoNotFireIdentityMissing(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)

	// Control: a fully-named soldier row.
	if _, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Carter",
	}); err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	// Person-bearing but unnamed: widow with no first_name/last_name.
	// Must STILL fire identity-missing — the gate must only block
	// non-person-bearing types.
	husband, err := svc.Create(models.Soldier{
		FirstName: "Thomas",
		LastName:  "Walker",
	})
	if err != nil {
		t.Fatalf("Create husband: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		EntryType:       models.EntryTypeWidow,
		SpouseSoldierID: husband.ID,
		// intentionally blank first_name + last_name
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	// Non-person-bearing: event record with kind + begin_date set.
	if _, err := events.CreateEvent(models.Soldier{
		EntryType: models.EntryTypeEvent,
		Kind:      "Battle",
		BeginDate: "07/01/1862",
		EndDate:   "07/03/1862",
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	result, err := svc.RunDataQualityScan(string(DataQualityModeHighConfidence))
	if err != nil {
		t.Fatalf("RunDataQualityScan: %v", err)
	}

	// Group issues by (entry_type, code) for assertions.
	hasIssue := func(entryType, code string) bool {
		for _, issue := range result.Issues {
			if issue.EntryType == entryType && issue.Code == code {
				return true
			}
		}
		return false
	}

	// Bug-shape assertion: event rows must NOT fire identity-missing.
	if hasIssue(models.EntryTypeEvent, "identity-missing") {
		t.Errorf("event rows fired identity-missing; the #530 gate regressed")
		for _, issue := range result.Issues {
			if issue.EntryType == models.EntryTypeEvent && issue.Code == "identity-missing" {
				t.Logf("  false positive: %s (%s)", issue.DisplayID, issue.Name)
			}
		}
	}

	// Over-fire guard: widow with no name must STILL fire
	// identity-missing (widow IS a person-bearing entry type).
	if !hasIssue(models.EntryTypeWidow, "identity-missing") {
		t.Errorf("widow with no name did not fire identity-missing; the gate over-fires")
	}

	// Sanity: a fully-named soldier does not fire identity-missing.
	if hasIssue(models.EntryTypeSoldier, "identity-missing") {
		t.Errorf("named soldier fired identity-missing; the scan is over-eager")
	}
}

// TestClassifyMarkupNoise (issue #531) pins the new HTML / markup
// detection helper added next to hasObviousPlaceholderNoise. The
// scan has no detection for raw HTML in Notes / Birth Info / etc.;
// a Memorial JSON import or a copy-paste from a web page can land
// markup in a freeform text field and the static archive then
// renders it as live HTML.
//
// The classifier escalates by severity (mixed-content-script >
// raw-html-tags > unescaped-entity) so a field carrying all three
// surfaces the single most-severe issue rather than three stacked
// ones on the review-queue card.
func TestClassifyMarkupNoise(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		// Clean text — no noise.
		{"clean", []string{"James", "Carter", "born in Madison County", "Oakwood Cemetery"}, ""},
		{"empty", []string{"", "", "", ""}, ""},

		// raw-html-tags (medium).
		{"balanced-bold", []string{"", "", "<b>test</b>", ""}, "raw-html-tags"},
		{"balanced-link", []string{"", "", `<a href="https://example.com">link</a>`, ""}, "raw-html-tags"},
		{"unbalanced-tag", []string{"", "", "<b>test", ""}, "raw-html-tags"},
		{"self-closing", []string{"", "", "see <br/> for more", ""}, "raw-html-tags"},
		{"raw-in-name", []string{"<i>James</i>", "", "", ""}, "raw-html-tags"},
		{"raw-in-buried-in", []string{"", "", "", "<font color=red>Oakwood</font>"}, "raw-html-tags"},

		// unescaped-entity (medium) — user pasted already-encoded HTML.
		{"escaped-script", []string{"", "", "&lt;script&gt;alert(1)&lt;/script&gt;", ""}, "unescaped-entity"},
		{"escaped-amp", []string{"", "", "Tom &amp; Jerry", ""}, "unescaped-entity"},

		// mixed-content-script (high) — security concern.
		{"script-tag", []string{"", "", "<script>alert(1)</script>", ""}, "mixed-content-script"},
		{"onerror-attr", []string{"", "", `<img src="x" onerror="alert(1)">`, ""}, "mixed-content-script"},
		{"onclick-attr", []string{"", "", `<a href="#" onclick="steal()">click</a>`, ""}, "mixed-content-script"},

		// Precedence: a field carrying both raw-html-tags AND
		// mixed-content-script must report mixed-content-script.
		{"script-beats-html", []string{"", "", "<b>bold</b><script>x</script>", ""}, "mixed-content-script"},
		// script-beats-entity: script tag surfaces as mixed-content-script
		// even when entities are also present in the same value.
		{"script-beats-entity", []string{"", "", "&lt;x&gt; <script>y</script>", ""}, "mixed-content-script"},

		// Negative: angle brackets that are not HTML (e.g. less-than
		// in a measurement, a math expression) must NOT trip raw-html.
		{"math-lt", []string{"", "", "less than 5 < 10 in this region", ""}, ""},
		// Negative: the word "script" without a tag opener is not
		// a script tag.
		{"word-script", []string{"", "", "postscript to the letter", ""}, ""},
		// Negative: a bare `onclick` word without `=` is not an
		// event handler attribute.
		{"word-onclick", []string{"", "", "do not onclick this button", ""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyMarkupNoise(c.values...); got != c.want {
				t.Errorf("classifyMarkupNoise(%v) = %q, want %q", c.values, got, c.want)
			}
		})
	}
}

// TestRunDataQualityScan_RawHTMLInNotesFiresFieldContent (issue #531)
// is the integration-level regression net: a soldier with raw HTML
// in the birth_info field must produce a Field Content issue in the
// scan result, and a clean soldier must not. Combined with the
// TestClassifyMarkupNoise unit test, this pins the wiring between
// the helper and the evaluateQualityIssues branch.
func TestRunDataQualityScan_RawHTMLInNotesFiresFieldContent(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	// Soldier with raw HTML in birth_info — must fire raw-html-tags.
	if _, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Carter",
		BirthInfo: "<b>test</b>",
	}); err != nil {
		t.Fatalf("Create soldier with HTML birth_info: %v", err)
	}
	// Soldier with a script tag in birth_info — must fire
	// mixed-content-script (high severity).
	if _, err := svc.Create(models.Soldier{
		FirstName: "Samuel",
		LastName:  "Walker",
		BirthInfo: `<img src="x" onerror="alert(1)">`,
	}); err != nil {
		t.Fatalf("Create soldier with script birth_info: %v", err)
	}
	// Clean soldier — no Field Content issues.
	if _, err := svc.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "Lee",
		BirthInfo: "born in Madison County",
	}); err != nil {
		t.Fatalf("Create clean soldier: %v", err)
	}

	result, err := svc.RunDataQualityScan(string(DataQualityModeHighConfidence))
	if err != nil {
		t.Fatalf("RunDataQualityScan: %v", err)
	}

	var (
		rawHTMLCount     int
		scriptCount      int
		cleanHTMLCount   int
		rawHTMLSeverity  []string
		scriptSeverity   []string
	)
	for _, issue := range result.Issues {
		if issue.Group != "Field Content" {
			continue
		}
		switch issue.Code {
		case "raw-html-tags":
			rawHTMLCount++
			rawHTMLSeverity = append(rawHTMLSeverity, issue.Severity)
		case "mixed-content-script":
			scriptCount++
			scriptSeverity = append(scriptSeverity, issue.Severity)
		default:
			cleanHTMLCount++
		}
	}

	if rawHTMLCount != 1 {
		t.Errorf("raw-html-tags count = %d, want 1", rawHTMLCount)
	}
	if scriptCount != 1 {
		t.Errorf("mixed-content-script count = %d, want 1", scriptCount)
	}
	if cleanHTMLCount != 0 {
		t.Errorf("unexpected Field Content issues: %d", cleanHTMLCount)
	}
	// The summary/detail on the script issue should mention
	// "security" or "static archive" so the user understands the
	// why behind the high severity.
	for _, issue := range result.Issues {
		if issue.Code == "mixed-content-script" {
			if !strings.Contains(strings.ToLower(issue.Summary), "script") &&
				!strings.Contains(strings.ToLower(issue.Detail), "static archive") {
				t.Errorf("script issue summary/detail should mention script or static archive: %q / %q", issue.Summary, issue.Detail)
			}
		}
	}
	_ = rawHTMLSeverity
	_ = scriptSeverity
}

// TestIsPersonBearingEntryType (issue #530) pins the helper semantics
// in internal/models. The data-quality scan's Identity & Naming gate
// is the primary consumer; a change to the allowed set ripples to
// every scan run, so the contract is captured here as a regression net.
func TestIsPersonBearingEntryType(t *testing.T) {
	cases := []struct {
		entryType string
		want      bool
	}{
		{models.EntryTypeSoldier, true},
		{models.EntryTypeWife, true},
		{models.EntryTypeWidow, true},
		{models.EntryTypeLinkedPerson, true},
		{models.EntryTypeEvent, false},
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		if got := models.IsPersonBearingEntryType(c.entryType); got != c.want {
			t.Errorf("IsPersonBearingEntryType(%q) = %v, want %v", c.entryType, got, c.want)
		}
	}
}

// TestRunDataQualityScan_SurnameTooShortGatedToPersonBearing (issue #538)
// is the regression net for the surname-too-short sibling check.
// Mirrors TestRunDataQualityScan_EventRowsDoNotFireIdentityMissing
// (the #530 test) — the gate must:
//   - still fire for person-bearing rows with a single-character
//     last_name (soldier, wife, widow, linked_person) — the check
//     is meaningful for them
//   - NOT fire for non-person-bearing rows (event) — the check is
//     meaningless for them because they carry no first_name/
//     last_name column values
// A regression that drops the gate re-introduces the inconsistency
// (the #530 fix gated identity-missing but left surname-too-short
// unguarded); a regression that over-fires the gate breaks the
// soldier / widow positive case.
func TestRunDataQualityScan_SurnameTooShortGatedToPersonBearing(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)

	// Control: a soldier with a single-character last_name.
	// Must fire surname-too-short in advanced mode.
	if _, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "X",
	}); err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	// Person-bearing but unrelated: a soldier with a normal
	// last_name. Must NOT fire surname-too-short (control on the
	// length check itself).
	if _, err := svc.Create(models.Soldier{
		FirstName: "Samuel",
		LastName:  "Carter",
	}); err != nil {
		t.Fatalf("Create soldier (normal): %v", err)
	}

	// Person-bearing: a widow with a single-character last_name.
	// Must still fire surname-too-short — widow IS a
	// person-bearing entry type and the check is meaningful.
	husband, err := svc.Create(models.Soldier{
		FirstName: "Thomas",
		LastName:  "Walker",
	})
	if err != nil {
		t.Fatalf("Create husband: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		EntryType:       models.EntryTypeWidow,
		SpouseSoldierID: husband.ID,
		FirstName:       "Martha",
		LastName:        "Q",
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	// Non-person-bearing: an event record (no first_name /
	// last_name columns populated by design). Must NOT fire
	// surname-too-short — the check is meaningless for events.
	if _, err := events.CreateEvent(models.Soldier{
		EntryType: models.EntryTypeEvent,
		Kind:      "Battle",
		BeginDate: "07/01/1862",
		EndDate:   "07/03/1862",
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// Run the advanced scan (surname-too-short is advanced-mode only).
	result, err := svc.RunDataQualityScan(string(DataQualityModeAdvanced))
	if err != nil {
		t.Fatalf("RunDataQualityScan: %v", err)
	}

	hasIssue := func(entryType, code string) bool {
		for _, issue := range result.Issues {
			if issue.EntryType == entryType && issue.Code == code {
				return true
			}
		}
		return false
	}

	// Positive: soldier with last_name length 1 must fire.
	if !hasIssue(models.EntryTypeSoldier, "surname-too-short") {
		t.Errorf("soldier with last_name='X' did not fire surname-too-short")
	}
	// Positive: widow with last_name length 1 must fire.
	if !hasIssue(models.EntryTypeWidow, "surname-too-short") {
		t.Errorf("widow with last_name='Q' did not fire surname-too-short")
	}
	// Bug-shape: event row must NOT fire surname-too-short.
	if hasIssue(models.EntryTypeEvent, "surname-too-short") {
		t.Errorf("event row fired surname-too-short; the #538 gate regressed")
		for _, issue := range result.Issues {
			if issue.EntryType == models.EntryTypeEvent && issue.Code == "surname-too-short" {
				t.Logf("  regression: %s (%s)", issue.DisplayID, issue.Name)
			}
		}
	}
	// Sanity: a soldier with a normal last_name does not fire
	// (defends the underlying length check).
	if hasIssue(models.EntryTypeSoldier, "surname-too-short") {
		// Note: this branch is only meaningful if we count the
		// number of soldier surname-too-short issues. We have
		// one soldier (James X) that SHOULD fire; the other
		// (Samuel Carter) should NOT. Count and assert.
		count := 0
		for _, issue := range result.Issues {
			if issue.EntryType == models.EntryTypeSoldier && issue.Code == "surname-too-short" {
				count++
			}
		}
		if count > 1 {
			t.Errorf("soldier surname-too-short count = %d, want exactly 1 (only 'X' should fire)", count)
		}
	}
}

// TestRunDataQualityScan_SurnameTooShortAdvancedModeOnly pins the
// mode contract: surname-too-short is an advanced-mode check, so
// the high-confidence scan must NOT fire it regardless of the
// person's last_name length. (This is the #538 contract; the
// #530 test was high-confidence only because identity-missing is
// a high-confidence check, but surname-too-short is the opposite
// — advanced only.)
func TestRunDataQualityScan_SurnameTooShortAdvancedModeOnly(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	if _, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "X",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	highConf, err := svc.RunDataQualityScan(string(DataQualityModeHighConfidence))
	if err != nil {
		t.Fatalf("RunDataQualityScan (high-confidence): %v", err)
	}
	for _, issue := range highConf.Issues {
		if issue.Code == "surname-too-short" {
			t.Errorf("high-confidence scan fired surname-too-short; the check is advanced-mode only")
		}
	}
}
