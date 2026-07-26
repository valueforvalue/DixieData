// seed_flag_test.go — tests for the --seed in-app flag handler
// (issue #667 follow-up). Pins ParseSeedOptions so a future
// refactor doesn't accidentally break the flag surface that
// scripts/seed-fixture.ps1 depends on.
package appshell

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

func TestParseSeedOptions_Defaults(t *testing.T) {
	opts := ParseSeedOptions([]string{"--seed"})
	if opts.DataDir != appdata.DefaultDir() {
		t.Errorf("DataDir = %q, want %q (default)", opts.DataDir, appdata.DefaultDir())
	}
	if opts.Soldiers != 250 {
		t.Errorf("Soldiers = %d, want 250", opts.Soldiers)
	}
	if opts.Seed != 1865 {
		t.Errorf("Seed = %d, want 1865", opts.Seed)
	}
	if opts.Reset {
		t.Errorf("Reset = true, want false (default)")
	}
	if opts.SkipSoldiers {
		t.Errorf("SkipSoldiers = true, want false (default)")
	}
	if opts.ArticlesFormat != "markdown" {
		t.Errorf("ArticlesFormat = %q, want %q (default showcases goldmark)", opts.ArticlesFormat, "markdown")
	}
}

func TestParseSeedOptions_AllFlags(t *testing.T) {
	opts := ParseSeedOptions([]string{
		"--seed",
		"--data-dir", "C:\\test\\data",
		"--soldiers", "100",
		"--rng-seed", "42",
		"--reset",
		"--skip-soldiers",
		"--tags", "30",
		"--articles", "5",
		"--articles-format", "plain",
		"--events", "20",
	})
	if opts.DataDir != "C:\\test\\data" {
		t.Errorf("DataDir = %q, want %q", opts.DataDir, "C:\\test\\data")
	}
	if opts.Soldiers != 100 {
		t.Errorf("Soldiers = %d, want 100", opts.Soldiers)
	}
	if opts.Seed != 42 {
		t.Errorf("Seed = %d, want 42", opts.Seed)
	}
	if !opts.Reset {
		t.Errorf("Reset = false, want true")
	}
	if !opts.SkipSoldiers {
		t.Errorf("SkipSoldiers = false, want true")
	}
	if opts.Tags != 30 {
		t.Errorf("Tags = %d, want 30", opts.Tags)
	}
	if opts.Articles != 5 {
		t.Errorf("Articles = %d, want 5", opts.Articles)
	}
	if opts.ArticlesFormat != "plain" {
		t.Errorf("ArticlesFormat = %q, want plain", opts.ArticlesFormat)
	}
	if opts.Events != 20 {
		t.Errorf("Events = %d, want 20", opts.Events)
	}
}

func TestParseSeedOptions_FlagInterleaved(t *testing.T) {
	// Issue: flag values can appear before OR after the --seed
	// trigger. The PS wrapper script emits the args in a specific
	// order but future scripts (or hand-typed commands) may
	// interleave. ParseSeedOptions must accept any ordering.
	opts := ParseSeedOptions([]string{
		"--soldiers", "50",
		"--seed",
		"--articles", "3",
		"--reset",
	})
	if opts.Soldiers != 50 {
		t.Errorf("Soldiers = %d, want 50 (before --seed)", opts.Soldiers)
	}
	if opts.Articles != 3 {
		t.Errorf("Articles = %d, want 3 (after --seed)", opts.Articles)
	}
	if !opts.Reset {
		t.Errorf("Reset = false, want true (after --seed)")
	}
}

func TestParseSeedOptions_ArticlesFormatCaseInsensitive(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"markdown", "markdown"},
		{"MARKDOWN", "markdown"},
		{"Markdown", "markdown"},
		{"plain", "plain"},
		{"PLAIN", "plain"},
		{"Plain", "plain"},
	}
	for _, c := range cases {
		opts := ParseSeedOptions([]string{"--seed", "--articles-format", c.in})
		if opts.ArticlesFormat != c.want {
			t.Errorf("--articles-format %q -> %q, want %q", c.in, opts.ArticlesFormat, c.want)
		}
	}
}

func TestParseSeedOptions_UnknownFlagsIgnored(t *testing.T) {
	// Unknown flags should not panic and should not stomp known
	// options. The PS wrapper may add new flags in the future;
	// old binaries should still parse the known subset.
	opts := ParseSeedOptions([]string{
		"--seed",
		"--future-flag", "value",
		"--soldiers", "75",
	})
	if opts.Soldiers != 75 {
		t.Errorf("Soldiers = %d, want 75", opts.Soldiers)
	}
}

func TestHasSeedFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--seed"}, true},
		{[]string{"--seed-json"}, true},
		{[]string{"--soldiers", "100", "--seed"}, true},
		{[]string{}, false},
		{[]string{"--smoke"}, false},
		{[]string{"--data-dir", "x"}, false},
	}
	for _, c := range cases {
		got := HasSeedFlag(c.args)
		if got != c.want {
			t.Errorf("HasSeedFlag(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestWantsSeedJSON(t *testing.T) {
	if !WantsSeedJSON([]string{"--seed-json"}) {
		t.Errorf("WantsSeedJSON(--seed-json) = false, want true")
	}
	if WantsSeedJSON([]string{"--seed"}) {
		t.Errorf("WantsSeedJSON(--seed) = true, want false")
	}
}
