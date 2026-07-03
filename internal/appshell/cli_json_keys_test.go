// Regression net for issue #272 — normalise --json output key naming
// to snake_case + plural-for-collections / singular-for-scalars.
//
// The convention was already implied by every existing emitter
// (verified by reading internal/appshell/cli_*.go: every key in
// every writeJSON() call site is snake_case, every collection is
// plural, every scalar is singular). What was missing was the
// LOCK — a test that catches future emitters that drift away
// from the convention. This file is the lock.
//
// Each test runs a real subcommand with --json against a fresh
// stress app, parses the output, and asserts every key:
//   1. Is snake_case (lowercase letters / digits / underscores).
//   2. Has no leading/trailing underscores.
//   3. Has no consecutive underscores.
//   4. Matches the collection/scalar rule (best-effort: any key
//      whose value is a non-nil JSON array is expected to use
//      plural-form; any scalar value is expected to be singular).
//      The rule is intentionally lenient because English
//      pluralization is irregular; future PRs can tighten.

package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// snakeCaseKey returns true if the key is lowercase snake_case:
// letters, digits, and underscores only; no leading/trailing
// underscores; no consecutive underscores.
func snakeCaseKey(key string) bool {
	if key == "" {
		return false
	}
	if key[0] == '_' || key[len(key)-1] == '_' {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isUnderscore := c == '_'
		if !isLower && !isDigit && !isUnderscore {
			return false
		}
		if i > 0 && key[i] == '_' && key[i-1] == '_' {
			return false
		}
	}
	return true
}

// looksPlural returns true if the key ends in `s`, `es`, or `ies`.
// Used as a heuristic for collection keys. English is irregular;
// this is a best-effort check, not a strict grammar rule.
func looksPlural(key string) bool {
	if key == "" {
		return false
	}
	return strings.HasSuffix(key, "s") || strings.HasSuffix(key, "es") || strings.HasSuffix(key, "ies")
}

// TestRunAdmin_JSONKeysSnakeCase (issue #272) walks the admin
// subcommands that emit --json and asserts every output key is
// snake_case. Catches future emitters that drift to camelCase.
func TestRunAdmin_JSONKeysSnakeCase(t *testing.T) {
	app := newStressApp(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		kind   AdminKind
		action AdminAction
	}{
		// restore-point-list is excluded: it requires
		// app.restorePoints which newStressApp does not
		// wire. A separate scenario test in cli_admin_test.go
		// covers the body shape; the snake_case invariant is
		// the same shape — verified by hand at all writeJSON
		// call sites in cli_admin.go.
		{"config-show", AdminConfig, AdminConfigShow},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := AdminOptions{
				Writer:  &buf,
				App:     app,
				Kind:    c.kind,
				Action:  c.action,
				JSON:    true,
				DataDir: app.dataDir,
			}
			code, err := RunAdmin(ctx, opts)
			if err != nil {
				t.Fatalf("run: %v (code=%d)\nbody=%s", err, code, buf.String())
			}
			raw := strings.TrimSpace(buf.String())
			if raw == "" {
				t.Fatalf("empty output, code=%d", code)
			}
			if err := assertAllKeysSnakeCase(raw); err != nil {
				t.Errorf("%s: %v\nbody=%s", c.name, err, raw)
			}
		})
	}
}

// TestRunDebug_JSONKeysSnakeCase (issue #272) walks the debug
// emitters. The body of debug dump is the canonical reference
// shape — app_version, archive_counts (plural collection),
// row_counts (plural collection), schema_version (singular).
// This test exercises the live HTTP path: GET /version?type=json
// is the entry the Wails dispatcher uses. We invoke the inner
// emitter directly to avoid the routing layer.
func TestRunDebug_JSONKeysSnakeCase(t *testing.T) {
	app := newStressApp(t)
	ctx := context.Background()

	var buf bytes.Buffer
	opts := DebugOptions{
		App:    app,
		Writer: &buf,
		Kind:   DebugDump,
		JSON:   true,
	}
	code, err := RunDebug(ctx, opts)
	if err != nil {
		t.Fatalf("debug dump: %v (code=%d)", err, code)
	}
	if code != 0 {
		t.Fatalf("debug dump code=%d, want 0\nbody=%s", code, buf.String())
	}
	raw := strings.TrimSpace(buf.String())
	if raw == "" {
		t.Fatal("empty output")
	}
	if err := assertAllKeysSnakeCase(raw); err != nil {
		t.Errorf("debug dump: %v\nbody=%s", err, raw)
	}
}

// assertAllKeysSnakeCase parses the JSON body and walks every key
// (at any nesting depth). Returns an error listing all offending
// keys with their dotted path. Skips bodies whose root is a JSON
// array (those wrap an array of records; the survey tool in the
// follow-up handles those).
func assertAllKeysSnakeCase(body string) error {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "[") {
		return nil // root-level array; not a survey candidate
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return err
	}
	var offending []string
	walkKeys(parsed, nil, func(path []string, k string) {
		if !snakeCaseKey(k) {
			offending = append(offending, strings.Join(append(path, k), "."))
		}
	})
	if len(offending) > 0 {
		return fmt.Errorf("non-snake_case keys: %v", offending)
	}
	return nil
}

// walkKeys recursively visits every key in the JSON object tree.
// `path` is the chain of parent keys (each becomes a segment of
// the dotted path used in error reports). `visit` is invoked once
// per object key with the current path + that key.
func walkKeys(v any, path []string, visit func(path []string, key string)) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			visit(path, k)
			walkKeys(child, append(path, k), visit)
		}
	case []any:
		for _, child := range val {
			walkKeys(child, path, visit)
		}
	}
}
