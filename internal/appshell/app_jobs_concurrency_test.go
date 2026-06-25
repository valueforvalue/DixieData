package appshell

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

func TestJobsConcurrencyFromEnvHonoursValue(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"unset defaults", "", jobs.DefaultConcurrency},
		{"empty defaults", "   ", jobs.DefaultConcurrency},
		{"non-numeric defaults", "twelve", jobs.DefaultConcurrency},
		{"zero defaults", "0", jobs.DefaultConcurrency},
		{"negative defaults", "-3", jobs.DefaultConcurrency},
		{"valid value", "4", 4},
		{"clamps upper bound", "999", 16},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DIXIEDATA_JOBS_CONCURRENCY", c.raw)
			if got := jobsConcurrencyFromEnv(); got != c.want {
				t.Fatalf("jobsConcurrencyFromEnv(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}