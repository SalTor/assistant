// Package operationlog reads and appends to <data_dir>/tui_operations.log.
// The log is a UI affordance powering the TUI's "undo deletes" overlay; it is
// not the per-item event log that lives in each domain's SQLite store.
//
// Format: tab-separated lines `HH:MM:SS\taction\tdomain\titem_id\tdetail`.
package operationlog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one row of the log.
type Entry struct {
	Timestamp string // HH:MM:SS in the writer's local zone
	Action    string // create | delete | undelete | link | unlink
	Domain    string // note | task | problem
	ItemID    string
	Detail    string
}

// Path is the standard log location, derived from XDG_DATA_HOME or ~/.local.
func Path() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "assistant", "tui_operations.log")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "share", "assistant", "tui_operations.log")
	}
	return filepath.Join(home, ".local", "share", "assistant", "tui_operations.log")
}

// Append writes a new entry. The detail field has tabs and newlines replaced
// with spaces so a single line always parses cleanly.
func Append(action, domain, itemID, detail string) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	cleaned := strings.NewReplacer("\t", " ", "\n", " ").Replace(detail)
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n",
		time.Now().Format("15:04:05"), action, domain, itemID, cleaned)
	return err
}

// Load returns the most recent `limit` entries, newest first.
func Load(limit int) ([]Entry, error) {
	f, err := os.Open(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	// Simple ring buffer to keep memory bounded; the log can grow over time
	// but the TUI only ever wants the tail.
	buf := make([]string, 0, limit)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if len(buf) == limit {
			buf = append(buf[1:], line)
		} else {
			buf = append(buf, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	out := make([]Entry, 0, len(buf))
	for i := len(buf) - 1; i >= 0; i-- {
		parts := strings.SplitN(buf[i], "\t", 5)
		for len(parts) < 5 {
			parts = append(parts, "")
		}
		out = append(out, Entry{
			Timestamp: parts[0], Action: parts[1], Domain: parts[2],
			ItemID: parts[3], Detail: parts[4],
		})
	}
	return out, nil
}
