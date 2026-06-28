package appshell

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestHasQuerySubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"list"}, true},
		{[]string{"show"}, true},
		{[]string{"search"}, true},
		{[]string{"doctor"}, false},
		{[]string{"--smoke"}, false},
		{[]string{"list", "soldiers", "--limit", "5"}, true},
		{[]string{"--json"}, false},
	}
	for _, tc := range cases {
		if got := HasQuerySubcommand(tc.args); got != tc.want {
			t.Errorf("HasQuerySubcommand(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParseQueryCommand_ListSoldiers(t *testing.T) {
	opts, err := ParseQueryCommand([]string{"list", "soldiers", "--limit=5", "--page=2", "--json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Command != QueryListSoldiers {
		t.Errorf("Command = %v, want QueryListSoldiers", opts.Command)
	}
	if opts.Limit != 5 {
		t.Errorf("Limit = %d, want 5", opts.Limit)
	}
	if opts.Page != 2 {
		t.Errorf("Page = %d, want 2", opts.Page)
	}
	if !opts.JSON {
		t.Error("JSON = false, want true")
	}
}

func TestParseQueryCommand_ShowSoldier(t *testing.T) {
	cases := []struct {
		args []string
		wantTarget string
	}{
		{[]string{"show", "soldier", "54"}, "54"},
		{[]string{"show", "soldier", "DXD-00052"}, "DXD-00052"},
		{[]string{"show", "soldier", "DXD-00052", "--json"}, "DXD-00052"},
		{[]string{"show", "soldier", "--json", "DXD-00052"}, "DXD-00052"},
	}
	for _, tc := range cases {
		opts, err := ParseQueryCommand(tc.args)
		if err != nil {
			t.Fatalf("parse %v: %v", tc.args, err)
		}
		if opts.Command != QueryShowSoldier {
			t.Errorf("Command = %v, want QueryShowSoldier", opts.Command)
		}
		if len(opts.Args) == 0 || opts.Args[0] != tc.wantTarget {
			t.Errorf("Args = %v, want [%q]", opts.Args, tc.wantTarget)
		}
	}
}

func TestParseQueryCommand_Search(t *testing.T) {
	cases := []struct {
		args []string
		wantQuery string
	}{
		{[]string{"search", "Smith"}, "Smith"},
		{[]string{"search", "Smith", "--limit=10"}, "Smith"},
		{[]string{"search", "--query=Smith"}, ""}, // --query=flag not supported here; positional wins
		{[]string{"search", "--query", "Smith"}, ""}, // --query space form not supported; positional wins
		// --query= via flag is set in the loop below; covered separately
	}
	for _, tc := range cases {
		opts, err := ParseQueryCommand(tc.args)
		if err != nil {
			t.Fatalf("parse %v: %v", tc.args, err)
		}
		if opts.Command != QuerySearchSoldiers {
			t.Errorf("Command = %v, want QuerySearchSoldiers", opts.Command)
		}
		if len(opts.Args) > 0 && opts.Args[0] != tc.wantQuery {
			t.Errorf("Args = %v, want positional %q", opts.Args, tc.wantQuery)
		}
	}
}

func TestParseQueryCommand_FlagForms(t *testing.T) {
	// Both --limit=N and --limit N forms should be honoured.
	eq := []string{"list", "soldiers", "--limit=3"}
	sp := []string{"list", "soldiers", "--limit", "3"}
	for _, args := range [][]string{eq, sp} {
		opts, err := ParseQueryCommand(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if opts.Limit != 3 {
			t.Errorf("args %v: Limit = %d, want 3", args, opts.Limit)
		}
	}

	// --query= and --query space form should both set Query.
	eq = []string{"search", "--query=smith"}
	sp = []string{"search", "--query", "smith"}
	for _, args := range [][]string{eq, sp} {
		opts, err := ParseQueryCommand(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if opts.Query != "smith" {
			t.Errorf("args %v: Query = %q, want smith", args, opts.Query)
		}
	}

	// --page= and --page space form should both set Page.
	eq = []string{"list", "soldiers", "--page=7"}
	sp = []string{"list", "soldiers", "--page", "7"}
	for _, args := range [][]string{eq, sp} {
		opts, err := ParseQueryCommand(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if opts.Page != 7 {
			t.Errorf("args %v: Page = %d, want 7", args, opts.Page)
		}
	}
}

func TestParseQueryCommand_UnknownVerb(t *testing.T) {
	opts, err := ParseQueryCommand([]string{"frobnicate"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Command != QueryUnknown {
		t.Errorf("Command = %v, want QueryUnknown", opts.Command)
	}
}

func TestPageCount(t *testing.T) {
	cases := []struct {
		total, pageSize, want int
	}{
		{0, 50, 1},
		{1, 50, 1},
		{50, 50, 1},
		{51, 50, 2},
		{501, 50, 11},
		{501, 3, 167},
	}
	for _, tc := range cases {
		if got := pageCount(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("pageCount(%d, %d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}

func TestFormatSoldierName(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		// Empty soldier -> empty string.
		{map[string]any{}, ""},
		// Just first + last.
		{map[string]any{"FirstName": "John", "LastName": "Smith"}, "John Smith"},
		// Full name with prefix shown before.
		{map[string]any{
			"Prefix": "Capt.", "ShowPrefixBeforeName": true,
			"FirstName": "John", "MiddleName": "Q", "LastName": "Smith",
		}, "Capt. John Q Smith"},
		// Prefix shown after (parenthesised) when ShowPrefixBeforeName is false.
		{map[string]any{
			"Prefix": "Capt.", "ShowPrefixBeforeName": false,
			"FirstName": "John", "LastName": "Smith",
		}, "John Smith (Capt.)"},
		// Suffix.
		{map[string]any{
			"FirstName": "John", "LastName": "Smith", "Suffix": "Jr.",
		}, "John Smith Jr."},
	}
	for i, tc := range cases {
		got := formatSoldierNameFromMap(tc.in)
		if got != tc.want {
			t.Errorf("[%d] got %q, want %q", i, got, tc.want)
		}
	}
}

// formatSoldierNameFromMap builds a models.Soldier from a map for
// the test table. Keeps the test compact without exposing internal
// field names to multiple test cases.
func formatSoldierNameFromMap(m map[string]any) string {
	s := buildTestSoldier(m)
	return formatSoldierName(s)
}

func buildTestSoldier(m map[string]any) models.Soldier {
	var s models.Soldier
	if v, ok := m["Prefix"].(string); ok {
		s.Prefix = v
	}
	if v, ok := m["ShowPrefixBeforeName"].(bool); ok {
		s.ShowPrefixBeforeName = v
	}
	if v, ok := m["FirstName"].(string); ok {
		s.FirstName = v
	}
	if v, ok := m["MiddleName"].(string); ok {
		s.MiddleName = v
	}
	if v, ok := m["LastName"].(string); ok {
		s.LastName = v
	}
	if v, ok := m["Suffix"].(string); ok {
		s.Suffix = v
	}
	return s
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate('hello', 10) = %q, want %q", got, "hello")
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate('hello world', 5) = %q, want %q", got, "hello...")
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate('', 5) = %q, want empty", got)
	}
	if !strings.HasSuffix(truncate(strings.Repeat("x", 100), 10), "...") {
		t.Error("long input should have ellipsis")
	}
}

func TestIndent(t *testing.T) {
	if got := indent("a\nb\nc", ">"); got != ">a\n>b\n>c" {
		t.Errorf("got %q, want %q", got, ">a\n>b\n>c")
	}
	if got := indent("", ">"); got != "" {
		t.Errorf("empty should stay empty, got %q", got)
	}
}