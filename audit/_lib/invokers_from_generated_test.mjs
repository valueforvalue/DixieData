// audit/_lib/invokers_from_generated_test.mjs
//
// RED-first regression net for audit/_lib/invokers_from_generated.mjs
// (issue #369 Slice 2). The probe walks generated *_templ.go files
// and extracts every URL-shaped string that ends up in an HTML
// attribute. Three patterns supported:
//
//   1. templ.SafeURL("/literal")            — bare string.
//   2. templ.SafeURL(fmt.Sprintf("..%d..", a, b))  — format + args.
//   3. templ.SafeURL(routebuilder.X(args))  — helper call (resolved
//      via the Slice-1 helper map). Single-line OR multi-line args.
//
// The probe tests assert that each pattern contributes the right
// URL template to the invoker set. Multi-line arg lists exercise
// the line-spanning arg parser.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { parseGeneratedInvokers, _resolveHelperCall } from './invokers_from_generated.mjs';

const HELPER_MAP = new Map([
	['EventLinksAttach', '/events/{eventID}/links'],
	['EventLinksDetach', '/events/{eventID}/links/{personID}/detach'],
	['SoldierPDF', '/soldiers/{id}/pdf'],
]);

const FIXTURE = `
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL("/browse"))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(fmt.Sprintf("/soldiers/%d", s.ID)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(
	fmt.Sprintf("/events/%d/links/%d/detach",
		eventID,
		personID,
	)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.EventLinksAttach(eventID)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.EventLinksDetach(
	eventID, personID,
)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.SoldierPDF(s.ID)))
templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL("/articles/new"))
`;

test('parseGeneratedInvokers handles bare-string SafeURL', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/browse'), 'literal /browse captured');
	assert.ok(inv.has('/articles/new'), 'literal /articles/new captured');
});

test('parseGeneratedInvokers handles fmt.Sprintf with single arg', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/soldiers/{s.ID}'),
		'single-arg fmt.Sprintf emits positional placeholder');
});

test('parseGeneratedInvokers handles fmt.Sprintf with multi-line args', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/events/{eventID}/links/{personID}/detach'),
		'multi-line arg fmt.Sprintf emits all positional placeholders');
});

test('parseGeneratedInvokers resolves routebuilder.X single-line', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/events/{eventID}/links'),
		'routebuilder.EventLinksAttach single-line resolved');
});

test('parseGeneratedInvokers resolves routebuilder.X multi-line args', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/events/{eventID}/links/{personID}/detach'),
		'routebuilder.EventLinksDetach multi-line args resolved');
});

test('parseGeneratedInvokers resolves routebuilder.X with template arg (s.ID)', () => {
	const inv = parseGeneratedInvokers(FIXTURE, HELPER_MAP);
	assert.ok(inv.has('/soldiers/{id}/pdf'),
		'routebuilder.SoldierPDF(s.ID) — helper template {id} emitted as-is');
});

test('parseGeneratedInvokers silently drops invokers whose helper is unknown', () => {
	const text = `templ_xxx_Err = templ.JoinURLErrs(templ.SafeURL(routebuilder.UnknownHelper(a)))`;
	const inv = parseGeneratedInvokers(text, HELPER_MAP);
	// Unknown helper — emit nothing (caller's signal-only false-positive
	// prevention; don't make the probe louder than the existing HTML-attr scan).
	assert.equal(inv.size, 0);
});

test('_resolveHelperCall copies helper placeholders verbatim when single arg is a field access', () => {
	// routebuilder.SoldierPDF(s.ID) — SoldierPDF template is /soldiers/{id}/pdf.
	// With single positional arg `s.ID`, the probe emits {id} (helper's
	// template placeholder as-is — match against route's `{id}` segment).
	const result = _resolveHelperCall('SoldierPDF', 's.ID', HELPER_MAP);
	assert.equal(result, '/soldiers/{id}/pdf');
});
