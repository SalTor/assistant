// Package envelope writes the JSON response object documented in spec §2.2.
// The envelope shape varies enough by verb that we use a map[string]any
// rather than a fixed struct; this also matches the Python wrapper's behavior
// (it uses `**result` spreads to merge intent/confidence/input into both
// success and error responses).
package envelope

import (
	"encoding/json"
	"fmt"
	"os"
)

// Standard error codes used in `error`. See spec §9.
const (
	ErrEmptyBody          = "empty_body"
	ErrEmptyTitle         = "empty_title"
	ErrEmptyStatement     = "empty_statement"
	ErrEmptyEditBody      = "empty_edit_body"
	ErrNoTargetNote       = "no_target_note"
	ErrNoTargetTask       = "no_target_task"
	ErrNoTargetProblem    = "no_target_problem"
	ErrNoteNotFound       = "note_not_found"
	ErrTaskNotFound       = "task_not_found"
	ErrProblemNotFound    = "problem_not_found"
	ErrUnknownIntent      = "unknown_intent"
	ErrUnsupportedCommand = "unsupported_command"
	ErrException          = "exception"
)

// Print writes the envelope to stdout as compact JSON unless pretty is true,
// then returns the appropriate exit code: 0 if env["ok"] is true, 1 otherwise.
func Print(env map[string]any, pretty bool) int {
	var (
		buf []byte
		err error
	)
	if pretty {
		buf, err = json.MarshalIndent(env, "", "  ")
	} else {
		buf, err = json.Marshal(env)
	}
	if err != nil {
		// We can't render the envelope itself — fall back to a stderr line so
		// the caller still sees something. This should be impossible in
		// practice (every value we put in is JSON-safe).
		fmt.Fprintf(os.Stderr, "envelope marshal failed: %v\n", err)
		return 1
	}
	os.Stdout.Write(buf)
	os.Stdout.Write([]byte("\n"))

	if ok, _ := env["ok"].(bool); ok {
		return 0
	}
	return 1
}

// Exception builds the exception envelope used at the top level when an
// unexpected error escapes a verb handler.
func Exception(err error) map[string]any {
	return map[string]any{
		"ok":      false,
		"error":   ErrException,
		"message": err.Error(),
	}
}
