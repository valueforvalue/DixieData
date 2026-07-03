// Package records owns the persistence-layer services that back the
// DixieData Local Archive: CRUD on Soldiers (Person Records),
// Source Records, Claims, Findings, Tags, Calendar Items,
// Research Logs, Merge-Review Ledgers, and the duplicate-detection
// audit. Each service is constructed by a New* function, takes a
// *db.DB handle, and exposes methods that map to SQL operations
// on the underlying tables.
//
// The records package does NOT import internal/appshell, internal/
// presentation, or any Templ/HTMX package; the dependency direction
// is records → db → sqlite. UI-facing types live in internal/
// viewmodel and are produced by the records methods' return values
// (which are domain types from internal/models).
//
// Conventions:
//   - Service types are constructed via New*Service(db).
//   - Errors are exported as `Err<Kind>NotFound` package vars.
//   - Methods that return domain types carry no doc comment per
//     identifier — the package-doc block above documents the
//     convention. Methods with non-obvious side effects
//     (background scans, schema migrations) carry their own doc.
package records