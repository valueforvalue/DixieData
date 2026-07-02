// errors.go — centralised CLI error writer.
//
// Every subcommand runner in main.go funnels its error path
// through writeError so the format stays consistent. The
// convention: 'error: <msg>\n' to stderr, with a trailing
// newline. Matches the Unix convention (git, apt, go).
//
// Issue #274 / cli-plan.md Open follow-up #6.

package main

import "io"

// writeError writes a single 'error: <msg>\n' line to w.
// The msg MUST NOT include a trailing newline; writeError
// adds one. If the caller has an `error` value, use
// writeError(w, err.Error()).
func writeError(w io.Writer, msg string) {
	io.WriteString(w, "error: ")
	io.WriteString(w, msg)
	io.WriteString(w, "\n")
}