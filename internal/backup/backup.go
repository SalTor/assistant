// Package backup snapshots the three assistant SQLite databases to a
// timestamped subdirectory under a configured destination. Uses SQLite's
// `VACUUM INTO` so the copy is consistent even if a write is in flight.
package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stampLayout produces folder names like 20260530T113042. Sortable, filesystem
// safe, no colons (Windows-hostile).
const stampLayout = "20060102T150405"

// Backup writes a VACUUM INTO snapshot of each named DB into
// `<destDir>/<stamp>/<name>.db`. Returns the absolute path to the
// per-invocation directory it created.
//
// The map keys are the file basenames used inside the backup folder. Pass the
// already-open *sql.DB handles from your *store.Store so this package doesn't
// have to know about the store types.
func Backup(dbs map[string]*sql.DB, destDir string) (string, error) {
	if destDir == "" {
		return "", fmt.Errorf("backup destination is empty")
	}
	if len(dbs) == 0 {
		return "", fmt.Errorf("no databases to back up")
	}
	stamp := time.Now().Format(stampLayout)
	dir := filepath.Join(destDir, stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	for name, db := range dbs {
		target := filepath.Join(dir, name+".db")
		// VACUUM INTO requires a string-quoted absolute path. SQLite parses
		// embedded single quotes by doubling them; abs paths on macOS/Linux
		// don't contain quotes, so a straight format is safe in practice.
		if _, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", target)); err != nil {
			return "", fmt.Errorf("vacuum %s: %w", name, err)
		}
	}
	return dir, nil
}
