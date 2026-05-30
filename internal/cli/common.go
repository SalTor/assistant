// Package cli implements the assistant binary's command-line surface
// described in spec §2. Each verb returns a map[string]any envelope (or text
// output for the migrate-dbs / domains commands) and an exit code.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SalTor/assistant/internal/store"
)

// DefaultDataDir resolves $XDG_DATA_HOME/assistant or ~/.local/share/assistant.
func DefaultDataDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "assistant")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// The Python code blindly expands ~; if that fails we fall back to cwd.
		return filepath.Join(".", ".local", "share", "assistant")
	}
	return filepath.Join(home, ".local", "share", "assistant")
}

// DefaultDBPath returns the per-domain default DB location under the data dir.
func DefaultDBPath(domain string) string {
	return filepath.Join(DefaultDataDir(), domain+".db")
}

// DetectTimezone resolves an IANA tz name, $TZ, or system local, falling back
// to UTC. Mirrors detect_timezone() in the Python skill code.
func DetectTimezone(name string) (*time.Location, error) {
	if name != "" {
		return time.LoadLocation(name)
	}
	if env := os.Getenv("TZ"); env != "" {
		return time.LoadLocation(env)
	}
	// time.Local already reflects the system zone.
	if time.Local != nil {
		return time.Local, nil
	}
	return time.UTC, nil
}

// commonFlags holds --db, --tz, --pretty as parsed by all per-domain verbs.
type commonFlags struct {
	DB     string
	TZ     string
	Pretty bool
}

func registerCommon(fs *flag.FlagSet, defaultDB string, c *commonFlags) {
	fs.StringVar(&c.DB, "db", defaultDB, "Path to SQLite DB")
	fs.StringVar(&c.TZ, "tz", "", "IANA timezone")
	fs.BoolVar(&c.Pretty, "pretty", false, "Pretty-print JSON")
}

// openStore is the small shared helper used by every domain verb: pick a tz,
// open the SQLite file (creating parent dirs), run the schema. Returns a
// closer the caller should defer.
func openStore(c commonFlags, domain store.Domain) (*store.Store, error) {
	tz, err := DetectTimezone(c.TZ)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", c.TZ, err)
	}
	return store.Open(c.DB, domain, tz)
}

// stringPtr returns a pointer for non-empty strings, nil otherwise. The
// Python wrapper passes through Python None for missing optional ids; we
// match that with omitted JSON or explicit null where the spec calls for it.
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
