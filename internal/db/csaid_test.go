package db

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestNextDXDID consolidates the per-shape NextDXDID tests into one
// table-driven case. Each row seeds the database with a different
// starting state and asserts the next-minted id. Seeds are SQL
// fragments so the rows stay close to the production schema.
func TestNextDXDID(t *testing.T) {
	cases := []struct {
		name  string
		seeds []string
		want  string
	}{
		{
			name:  "format",
			seeds: nil,
			want:  "DXD-00001",
		},
		{
			name: "increment",
			seeds: []string{
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00001', 1)`,
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00002', 1)`,
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00003', 1)`,
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00004', 1)`,
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00005', 1)`,
			},
			want: "DXD-00006",
		},
		{
			// Non-generated soldier (pension ID) — must not affect count.
			name: "non_generated_ignored",
			seeds: []string{
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('PENSION-12345', 0)`,
			},
			want: "DXD-00001",
		},
		{
			// Existing DXD- rows without the generated flag still
			// count for the next-id calculation.
			name: "uses_existing_dxd_ids_without_generated_flag",
			seeds: []string{
				`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00007', 0)`,
			},
			want: "DXD-00008",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Open(testtemp.New(t).Path())
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer d.Close()

			for _, seed := range c.seeds {
				if _, err := d.conn.Exec(seed); err != nil {
					t.Fatalf("seed %q: %v", seed, err)
				}
			}

			id, err := d.NextDXDID()
			if err != nil {
				t.Fatalf("NextDXDID: %v", err)
			}
			if id != c.want {
				t.Errorf("got %s, want %s", id, c.want)
			}
		})
	}
}

func TestBuildUserNodePrefix(t *testing.T) {
	prefix, err := BuildUserNodePrefix("Samuel", "Thomas", "Carter", 1838)
	if err != nil {
		t.Fatalf("BuildUserNodePrefix: %v", err)
	}
	if prefix != "STC38" {
		t.Fatalf("prefix = %q", prefix)
	}
}

func TestIdentitySetupRequiredForFreshDatabase(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	required, err := d.IdentitySetupRequired()
	if err != nil {
		t.Fatalf("IdentitySetupRequired: %v", err)
	}
	if !required {
		t.Fatal("expected fresh database to require identity setup")
	}

	identity, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838)
	if err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if identity.NodePrefix != "STC38" {
		t.Fatalf("node prefix = %q", identity.NodePrefix)
	}

	required, err = d.IdentitySetupRequired()
	if err != nil {
		t.Fatalf("IdentitySetupRequired after configure: %v", err)
	}
	if required {
		t.Fatal("expected configured database not to require identity setup")
	}
}

// TestNextEventID (issue #320) consolidates the per-shape NextEventID
// tests into one table-driven case. Each row seeds the database with a
// different starting state and asserts the next-minted id. The EVT-
// namespace counter must stay independent of the DXD- counter
// (mirrors TestNextDXDID_NonGeneratedIgnored for cross-namespace
// isolation).
func TestNextEventID(t *testing.T) {
	cases := []struct {
		name  string
		seeds []string
		want  string
	}{
		{
			name:  "format",
			seeds: nil,
			want:  "EVT-00001",
		},
		{
			name: "increment",
			seeds: []string{
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('EVT-00001', 1, 'event')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('EVT-00002', 1, 'event')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('EVT-00003', 1, 'event')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('EVT-00004', 1, 'event')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('EVT-00005', 1, 'event')`,
			},
			want: "EVT-00006",
		},
		{
			// Person Records in the DXD- namespace must not bump the
			// EVT- counter.
			name: "namespace_independent",
			seeds: []string{
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('DXD-00001', 1, 'soldier')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('DXD-00002', 1, 'soldier')`,
				`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES ('DXD-00003', 1, 'soldier')`,
			},
			want: "EVT-00001",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Open(testtemp.New(t).Path())
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer d.Close()

			for _, seed := range c.seeds {
				if _, err := d.conn.Exec(seed); err != nil {
					t.Fatalf("seed %q: %v", seed, err)
				}
			}

			id, err := d.NextEventID()
			if err != nil {
				t.Fatalf("NextEventID: %v", err)
			}
			if id != c.want {
				t.Errorf("got %s, want %s", id, c.want)
			}
		})
	}
}

// TestNextEventID_RoundTrip verifies the SanitizeID / CanonicalDisplayID
// helpers handle the EVT- namespace correctly.
func TestNextEventID_RoundTrip(t *testing.T) {
	// SanitizeID preserves the EVT- prefix
	if got := SanitizeID("EVT-00042", ""); got != "EVT-00042" {
		t.Errorf("SanitizeID(EVT-00042) = %q, want EVT-00042", got)
	}
	// CanonicalDisplayID parses EVT-00042 → ("EVT", 42, true)
	namespace, seq, ok := CanonicalDisplayID("EVT-00042")
	if !ok {
		t.Fatal("CanonicalDisplayID(EVT-00042) returned ok=false")
	}
	if namespace != "EVT" {
		t.Errorf("namespace = %q, want EVT", namespace)
	}
	if seq != 42 {
		t.Errorf("seq = %d, want 42", seq)
	}
}
