// audit/discover_orphan_handlers_test.mjs
//
// RED-first regression net for the wired-up probe after Slices
// 1 + 2 + 3 (issue #369 direction #2). End-to-end logic test:
// synthetic routes.go + synthetic templ files exercise the
// matching pipeline without touching the real project files.
//
// The real probe (`audit/discover_orphan_handlers.mjs`) is the
// production entry point; this file pins its logic in isolation.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { findOrphans as _findOrphans, parseRoutes, normalizeForMatch } from './discover_orphan_handlers.mjs';
import { parseGeneratedInvokers } from './_lib/invokers_from_generated.mjs';
import { parseRoutebuilder } from './_lib/routebuilder_helpers.mjs';

// Routes registered in the synthetic project.
const SYNTHETIC_ROUTES = `package routes

func setupRoutes(r chi.Router) {
	r.Get("/alpha", hAlpha)
	r.Post("/alpha", hAlpha)
	r.Get("/alpha/{id:[0-9]+}", hAlphaByID)
	r.Get("/bravo/{path:.*}", hBravo)
}
`;

// Generated _templ.go text for the synthetic project.
const SYNTHETIC_GENERATED = `
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL("/alpha"))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.Alpha()))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(fmt.Sprintf("/alpha/%d", s.ID)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.AlphaByID(s.ID)))
`;

// _templ.go that has NO invoker for /bravo/* — that route is the orphan.
const SYNTHETIC_GENERATED_ORPHAN = SYNTHETIC_GENERATED;

const SYNTHETIC_ROUTEBUILDER = `
package routebuilder
import "fmt"
func Alpha() string { return "/alpha" }
func AlphaByID(id int64) string { return fmt.Sprintf("/alpha/%d", id) }
`;

test('parseRoutes extracts every registered route', () => {
	const routes = parseRoutes(SYNTHETIC_ROUTES);
	assert.equal(routes.length, 4);
	const paths = routes.map((r) => r.method + ' ' + r.path);
	assert.ok(paths.includes('GET /alpha'));
	assert.ok(paths.includes('POST /alpha'));
	assert.ok(paths.includes('GET /alpha/{id:[0-9]+}'));
	assert.ok(paths.includes('GET /bravo/{path:.*}'));
});

test('end-to-end: routebuilder-wired routes are NOT orphans', () => {
	const routes = parseRoutes(SYNTHETIC_ROUTES);
	const helperMap = parseRoutebuilder(SYNTHETIC_ROUTEBUILDER);
	const inv = parseGeneratedInvokers(SYNTHETIC_GENERATED, helperMap);
	const orphans = _findOrphans(routes, [...inv]);
	// /bravo/* has no invoker anywhere — that's the only orphan.
	assert.equal(orphans.length, 1);
	assert.equal(orphans[0].path, '/bravo/{path:.*}');
});

test('normalizeForMatch strips Go placeholders uniformly', () => {
	assert.equal(normalizeForMatch('/soldiers/{id}/edit'), '/soldiers//edit');
	assert.equal(normalizeForMatch('/soldiers/{s.ID}/edit'), '/soldiers//edit');
	assert.equal(normalizeForMatch('/jobs/{jobID}/log'), '/jobs//log');
});

test('findOrphans leaves the always-reachable prefixes alone', () => {
	// Assert that the ALWAYS_REACHABLE list in the probe includes
	// the known front-controller + static prefixes. This is a soft
	// contract — the probe's /recovery, /setup, /layout/* patterns
	// MUST exist so the probe stays usable in CI.
	const probeText = readFileSync('./audit/discover_orphan_handlers.mjs', 'utf8');
	for (const prefix of ['/app\\.', '/debug\\.', '/htmx\\.', '/recovery', '/setup']) {
		assert.ok(probeText.includes(prefix),
			`discover_orphan_handlers.mjs MUST include ALWAYS_REACHABLE for ${prefix}`);
	}
});
