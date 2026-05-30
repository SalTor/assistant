package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SalTor/assistant/internal/model"
)

const notesSchema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  followup_state TEXT NOT NULL DEFAULT 'open',
  followup_after TEXT,
  priority INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS note_events (
  id TEXT PRIMARY KEY,
  note_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(note_id) REFERENCES notes(id)
);

CREATE INDEX IF NOT EXISTS idx_notes_followup ON notes(followup_state, followup_after);
CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_note_events_note_time ON note_events(note_id, created_at);
`

func (s *Store) addNoteEvent(tx *sql.Tx, noteID, eventType, eventText string, payload map[string]any) error {
	pj, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal note event payload: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO note_events (id, note_id, event_type, event_text, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), noteID, eventType, eventText, string(pj), s.nowISO(),
	)
	return err
}

func (s *Store) CreateNote(body string) (*model.Note, error) {
	body = strings.TrimSpace(body)
	id := uuid.NewString()
	ts := s.nowISO()
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO notes (id, body, created_at, updated_at, status, followup_state)
		 VALUES (?, ?, ?, ?, 'active', 'open')`,
		id, body, ts, ts,
	); err != nil {
		return nil, err
	}
	if err := s.addNoteEvent(tx, id, "created", "Note created", map[string]any{"body": body}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetNote(id)
}

func (s *Store) GetNote(id string) (*model.Note, error) {
	row := s.DB.QueryRow(
		`SELECT id, body, status, followup_state, followup_after, priority, created_at, updated_at
		 FROM notes WHERE id = ?`,
		id,
	)
	return scanNote(row)
}

func scanNote(row interface{ Scan(...any) error }) (*model.Note, error) {
	var n model.Note
	var followup sql.NullString
	if err := row.Scan(&n.ID, &n.Body, &n.Status, &n.FollowupState, &followup, &n.Priority, &n.CreatedAt, &n.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	n.FollowupAfter = nullableString(followup)
	return &n, nil
}

// FindLatestActionableNote returns the most-recently-updated note whose
// status is 'active' and followup_state is 'open' or 'snoozed'. The Python
// implementation does not gate on followup_after, so neither do we.
func (s *Store) FindLatestActionableNote() (*model.Note, error) {
	row := s.DB.QueryRow(
		`SELECT id, body, status, followup_state, followup_after, priority, created_at, updated_at
		 FROM notes
		 WHERE status = 'active'
		   AND followup_state IN ('open', 'snoozed')
		 ORDER BY updated_at DESC
		 LIMIT 1`,
	)
	return scanNote(row)
}

// ListFollowups returns active notes that are open, plus snoozed notes whose
// followup_after has elapsed.
func (s *Store) ListFollowups() ([]model.Note, error) {
	now := s.nowISO()
	rows, err := s.DB.Query(
		`SELECT id, body, status, followup_state, followup_after, priority, created_at, updated_at
		 FROM notes
		 WHERE status = 'active'
		   AND (
		     followup_state = 'open'
		     OR (followup_state = 'snoozed' AND followup_after IS NOT NULL AND followup_after <= ?)
		   )
		 ORDER BY priority DESC, created_at ASC`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (s *Store) MarkNoteDone(id, sourceMessage string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE notes SET followup_state = 'done', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addNoteEvent(tx, id, "completed", "Marked as done", map[string]any{"source_message": sourceMessage}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SoftDeleteNote(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE notes SET status = 'deleted', followup_state = 'done', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addNoteEvent(tx, id, "deleted", "Soft-deleted note", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UndeleteNote(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE notes SET status = 'active', followup_state = 'open', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addNoteEvent(tx, id, "undeleted", "Note restored", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SnoozeNote(id, whenText string, until time.Time) error {
	untilISO := until.In(s.TZ).Format(IsoFormat)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE notes SET followup_state = 'snoozed', followup_after = ?, updated_at = ? WHERE id = ?`,
		untilISO, s.nowISO(), id,
	); err != nil {
		return err
	}
	if err := s.addNoteEvent(tx, id, "snoozed", "Snoozed until "+untilISO, map[string]any{
		"raw_time_text":           whenText,
		"resolved_followup_after": untilISO,
		"timezone":                s.TZ.String(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EditNoteBody(id, newBody string) error {
	old, err := s.GetNote(id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("note %s not found", id)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE notes SET body = ?, updated_at = ? WHERE id = ?`, newBody, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addNoteEvent(tx, id, "edited", "Body updated", map[string]any{
		"old_body": old.Body, "new_body": newBody,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) NoteEvents(id string) ([]model.Event, error) {
	rows, err := s.DB.Query(
		`SELECT id, event_type, event_text, payload_json, created_at
		 FROM note_events WHERE note_id = ? ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}
