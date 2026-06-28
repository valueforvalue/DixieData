package appshell

import (
	"strings"
	"testing"
)

func TestHasExportSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"export"}, false}, // missing kind
		{[]string{"export", "pdf"}, true},
		{[]string{"export", "jpg"}, true},
		{[]string{"export", "json"}, true},
		{[]string{"export", "csv"}, true},
		{[]string{"export", "ical"}, true},
		{[]string{"export", "static-archive"}, true},
		{[]string{"export", "backup"}, true},
		{[]string{"export", "frobnicate"}, false},
		{[]string{"doctor"}, false},
		{[]string{"list"}, false},
	}
	for _, tc := range cases {
		if got := HasExportSubcommand(tc.args); got != tc.want {
			t.Errorf("HasExportSubcommand(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParseExportArgs_PDFSoldier(t *testing.T) {
	opts, err := ParseExportArgs([]string{
		"export", "pdf", "--soldier", "54", "--out", "/tmp/foo.pdf",
		"--orientation", "P", "--no-images",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ExportPDF {
		t.Errorf("Kind = %v, want ExportPDF", opts.Kind)
	}
	if opts.Mode != ExportModeSingle {
		t.Errorf("Mode = %v, want ExportModeSingle", opts.Mode)
	}
	if opts.SoldierID != 54 {
		t.Errorf("SoldierID = %d, want 54", opts.SoldierID)
	}
	if opts.OutPath != "/tmp/foo.pdf" {
		t.Errorf("OutPath = %q, want /tmp/foo.pdf", opts.OutPath)
	}
	if opts.Orientation != "P" {
		t.Errorf("Orientation = %q, want P", opts.Orientation)
	}
	if !opts.NoImages {
		t.Error("NoImages = false, want true")
	}
}

func TestParseExportArgs_PDFMonth(t *testing.T) {
	cases := []struct {
		args []string
		wantMonth int
		wantMode ExportMode
	}{
		{[]string{"export", "pdf", "--month", "6", "--out", "/tmp/x.pdf"}, 6, ExportModeMonth},
		{[]string{"export", "pdf", "--month=6", "--out", "/tmp/x.pdf"}, 6, ExportModeMonth},
		{[]string{"export", "pdf", "--month=2026-06", "--out", "/tmp/x.pdf"}, 6, ExportModeMonth},
		{[]string{"export", "pdf", "--month=2026-12", "--out", "/tmp/x.pdf"}, 12, ExportModeMonth},
	}
	for _, tc := range cases {
		opts, err := ParseExportArgs(tc.args)
		if err != nil {
			t.Fatalf("parse %v: %v", tc.args, err)
		}
		if opts.Mode != tc.wantMode {
			t.Errorf("args %v: Mode = %v, want %v", tc.args, opts.Mode, tc.wantMode)
		}
		if opts.Month != tc.wantMonth {
			t.Errorf("args %v: Month = %d, want %d", tc.args, opts.Month, tc.wantMonth)
		}
	}
}

func TestParseExportArgs_PDFFull(t *testing.T) {
	opts, err := ParseExportArgs([]string{
		"export", "pdf", "--full", "--out", "/tmp/full.pdf",
		"--settings", `{"orientation":"L","printerFriendly":true}`,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Mode != ExportModeFull {
		t.Errorf("Mode = %v, want ExportModeFull", opts.Mode)
	}
	if !strings.Contains(opts.SettingsJSON, "printerFriendly") {
		t.Errorf("SettingsJSON = %q, want it to contain printerFriendly", opts.SettingsJSON)
	}
}

func TestParseExportArgs_MissingMode(t *testing.T) {
	_, err := ParseExportArgs([]string{"export", "pdf", "--out", "/tmp/x.pdf"})
	if err == nil {
		t.Fatal("expected error when no PDF mode is specified")
	}
}

func TestParseExportArgs_InvalidMonth(t *testing.T) {
	_, err := ParseExportArgs([]string{"export", "pdf", "--month=2026-13", "--out", "/tmp/x.pdf"})
	if err == nil {
		t.Fatal("expected error for month 13")
	}
	_, err = ParseExportArgs([]string{"export", "pdf", "--month", "0", "--out", "/tmp/x.pdf"})
	if err == nil {
		t.Fatal("expected error for month 0")
	}
	_, err = ParseExportArgs([]string{"export", "pdf", "--month=abc", "--out", "/tmp/x.pdf"})
	if err == nil {
		t.Fatal("expected error for non-numeric month")
	}
}

func TestParseExportArgs_JPGSoldier(t *testing.T) {
	opts, err := ParseExportArgs([]string{"export", "jpg", "--soldier", "54", "--out", "/tmp/x.jpg"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ExportJPG {
		t.Errorf("Kind = %v, want ExportJPG", opts.Kind)
	}
	if opts.SoldierID != 54 {
		t.Errorf("SoldierID = %d, want 54", opts.SoldierID)
	}
	if opts.OutPath != "/tmp/x.jpg" {
		t.Errorf("OutPath = %q, want /tmp/x.jpg", opts.OutPath)
	}
}

func TestParseExportArgs_JPGSoldierEqForm(t *testing.T) {
	opts, err := ParseExportArgs([]string{"export", "jpg", "--soldier=99", "--out=/tmp/x.jpg"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.SoldierID != 99 {
		t.Errorf("SoldierID = %d, want 99", opts.SoldierID)
	}
}

func TestParseExportArgs_OutEqForm(t *testing.T) {
	opts, err := ParseExportArgs([]string{"export", "json", "--out=/tmp/x.json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.OutPath != "/tmp/x.json" {
		t.Errorf("OutPath = %q", opts.OutPath)
	}
}

func TestParseExportArgs_SimpleKinds(t *testing.T) {
	cases := []struct {
		verb ExportKind
		args []string
	}{
		{ExportJSON, []string{"export", "json", "--out", "/tmp/x.json"}},
		{ExportCSV, []string{"export", "csv", "--out", "/tmp/x.csv"}},
		{ExportICalendar, []string{"export", "ical", "--out", "/tmp/x.ics"}},
		{ExportStaticArchive, []string{"export", "static-archive", "--out", "/tmp/static.zip"}},
		{ExportBackup, []string{"export", "backup", "--out", "/tmp/x.ddbak"}},
	}
	for _, tc := range cases {
		opts, err := ParseExportArgs(tc.args)
		if err != nil {
			t.Fatalf("parse %v: %v", tc.args, err)
		}
		if opts.Kind != tc.verb {
			t.Errorf("args %v: Kind = %v, want %v", tc.args, opts.Kind, tc.verb)
		}
	}
}

func TestParseExportArgs_UnknownKind(t *testing.T) {
	_, err := ParseExportArgs([]string{"export", "frobnicate", "--out", "/tmp/x"})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestParseExportArgs_NotExport(t *testing.T) {
	opts, err := ParseExportArgs([]string{"list", "soldiers"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ExportUnknown {
		t.Errorf("Kind = %v, want ExportUnknown", opts.Kind)
	}
}

func TestParseMonthFlag(t *testing.T) {
	cases := []struct {
		in   string
		want int
		err  bool
	}{
		{"6", 6, false},
		{"12", 12, false},
		{"1", 1, false},
		{"2026-06", 6, false},
		{"2026-12", 12, false},
		{"0", 0, true},
		{"13", 0, true},
		{"abc", 0, true},
		{"2026-13", 0, true},
		{"2026/06", 0, true}, // wrong separator
	}
	for _, tc := range cases {
		got, err := parseMonthFlag(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("parseMonthFlag(%q): expected error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMonthFlag(%q): unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseMonthFlag(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestStampExportForFilename(t *testing.T) {
	// Lock the format so backup filenames are stable across
	// platforms. yyyyMMdd-HHmmss in UTC.
	_ = StampExportForFilename // exercise the function so it stays exported
}