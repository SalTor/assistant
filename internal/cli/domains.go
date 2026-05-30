package cli

import (
	"fmt"
	"io"
	"os"
)

// stderr lets tests redirect; production callers use os.Stderr.
var stderr io.Writer = os.Stderr

// Domains prints the registered domain list as one tab-separated row per
// domain. Output is plain text (not JSON) to match the Python implementation.
func Domains(_ []string) int {
	rows := []struct{ name, path string }{
		{"notes", "internal/cli/notes.go"},
		{"tasks", "internal/cli/tasks.go"},
		{"problems", "internal/cli/problems.go"},
		{"project_manager", "internal/cli/projectmanager.go"},
	}
	for _, r := range rows {
		fmt.Printf("%s\t%s\n", r.name, r.path)
	}
	return 0
}
