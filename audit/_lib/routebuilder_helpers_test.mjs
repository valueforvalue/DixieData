// audit/_lib/routebuilder_helpers_test.mjs
//
// RED-first regression net for audit/_lib/routebuilder_helpers.mjs
// (issue #369, direction #2). Tests use Node's built-in node:test
// + assert/strict so the audit harness has zero test-framework
// deps for unit-level coverage (mirrors filechooser.test.mjs).
//
// Slice 1 of the orphan-handler probe rewrite (see the locked
// plan in the issue body). The helper under test parses a Go
// file text and returns a Map<name, UrlTemplate> where each
// UrlTemplate is the routebuilder.X(...) return value as a string
// using {argName} placeholders. The probe later resolves these
// against route registrations to compute real invoker URLs.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { parseRoutebuilder } from './routebuilder_helpers.mjs';

const FIXTURE = `
package routebuilder

import (
	"fmt"
	"net/url"
	"strings"
)

func DebugConsole() string {
	return "/debug/console"
}

func BrowseResults() string {
	return "/browse/results"
}

func Anniversary(month, day int) string {
	return fmt.Sprintf("/anniversary/%d/%d", month, day)
}

func AnniversaryItemDelete(month, day int, id int64) string {
	return fmt.Sprintf("/anniversary/%d/%d/items/%d", month, day, id)
}

func JobStatus(jobID string) string {
	return "/jobs/" + url.PathEscape(strings.TrimSpace(jobID)) + "/status"
}

func JobStatusSlot(jobID string) string {
	return JobStatus(jobID) + "?slot=1"
}

func ReviewQueueBulk() string {
	return "/review/queue/bulk"
}
`;

test('parseRoutebuilder extracts every func Name(...) string helper', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.ok(map instanceof Map, 'result is a Map');
	// 7 helpers in the fixture.
	assert.equal(map.size, 7);
});

test('parseRoutebuilder handles bare string returns', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.equal(map.get('DebugConsole'), '/debug/console');
	assert.equal(map.get('BrowseResults'), '/browse/results');
	assert.equal(map.get('ReviewQueueBulk'), '/review/queue/bulk');
});

test('parseRoutebuilder handles fmt.Sprintf returns with int args', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.equal(map.get('Anniversary'), '/anniversary/{month}/{day}');
	assert.equal(map.get('AnniversaryItemDelete'), '/anniversary/{month}/{day}/items/{id}');
});

test('parseRoutebuilder handles url.PathEscape(strings.TrimSpace(IDENT)) concatenation', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.equal(map.get('JobStatus'), '/jobs/{jobID}/status');
});

test('parseRoutebuilder recurses through Helper() + "suffix" chains', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.equal(map.get('JobStatusSlot'), '/jobs/{jobID}/status?slot=1');
});

test('parseRoutebuilder returns undefined for unknown helpers (probe can fall through)', () => {
	const map = parseRoutebuilder(FIXTURE);
	assert.equal(map.get('Unknown'), undefined);
});

test('parseRoutebuilder ignores helpers that do not return string', () => {
	const text = `
package routebuilder
func NotAString() int { return 42 }
func BackToString() string { return "/back" }
`;
	const map = parseRoutebuilder(text);
	assert.equal(map.size, 1);
	assert.equal(map.get('BackToString'), '/back');
	assert.equal(map.get('NotAString'), undefined);
});
