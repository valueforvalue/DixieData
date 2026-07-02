// errors.go — centralised CLI error writer.
//
// Every subcommand runner in main.go funnels its error path
// through writeError so the format stays consistent. The
// convention: 'error: <msg>\n' to stderr, with a trailing
// newline. Matches the Unix convention (git, apt, go).
//
// Issue #274 / cli-plan.md Open follow-up #6.

package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// writeError writes a single 'error: <msg>\n' line to w.
// The msg MUST NOT include a trailing newline; writeError
// adds one. If the caller has an `error` value, use
// writeError(w, err.Error()).
func writeError(w io.Writer, msg string) {
	io.WriteString(w, "error: ")
	io.WriteString(w, msg)
	io.WriteString(w, "\n")
}

// recoverExit5 wraps fn() with a recover() that converts any
// panic into exit code 5 ('internal error'). The panic
// value + stack trace are written to stderr with the
// 'internal error:' prefix. Issue #275 / cli-plan.md Open
// follow-up #7.
//
// Use:
//
//     return recoverExit5(func() (int, error) {
//         return runMySubcommand()
//     })
func recoverExit5(fn func() (int, error)) (code int, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			io.WriteString(os.Stderr, "internal error: ")
			fmt.Fprintf(os.Stderr, "%v\n", r)
			io.WriteString(os.Stderr, "stack trace:\n")
			os.Stderr.Write(stack)
			code = 5
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return fn()
}