// Package store wraps a SQLite database for one assistant domain
// (notes, tasks, or problems). Each domain uses its own *.db file but the
// connection lifecycle is identical.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// IsoFormat matches Python's datetime.isoformat(timespec="seconds"):
// numeric offset for UTC ("+00:00"), not the "Z" form Go's RFC3339 emits.
const IsoFormat = "2006-01-02T15:04:05-07:00"

// Domain selects which schema is initialized when Open() is called.
type Domain string

const (
	DomainNotes    Domain = "notes"
	DomainTasks    Domain = "tasks"
	DomainProblems Domain = "problems"
)

// Store owns a *sql.DB and the timezone the caller wants timestamps written in.
type Store struct {
	DB     *sql.DB
	TZ     *time.Location
	domain Domain
}

// Open creates the parent directory, opens the SQLite file, runs the schema
// for the given domain, and returns a ready Store.
func Open(path string, domain Domain, tz *time.Location) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	s := &Store{DB: db, TZ: tz, domain: domain}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// nowISO returns the current time in the store's timezone, formatted as the
// Python implementation does.
func (s *Store) nowISO() string { return time.Now().In(s.TZ).Format(IsoFormat) }

func (s *Store) initSchema() error {
	var script string
	switch s.domain {
	case DomainNotes:
		script = notesSchema
	case DomainTasks:
		script = tasksSchema
	case DomainProblems:
		script = problemsSchema
	default:
		return fmt.Errorf("unknown domain %q", s.domain)
	}
	if _, err := s.DB.Exec(script); err != nil {
		return fmt.Errorf("init schema for %s: %w", s.domain, err)
	}
	return nil
}

// nullableString returns nil for an empty string so callers can pass scan
// targets that distinguish NULL from "".
func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
