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

const tasksSchema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  details TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  due_at TEXT,
  priority INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS task_events (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_due ON tasks(status, due_at);
CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at);
CREATE INDEX IF NOT EXISTS idx_task_events_task_time ON task_events(task_id, created_at);
`

func (s *Store) addTaskEvent(tx *sql.Tx, taskID, eventType, eventText string, payload map[string]any) error {
	pj, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal task event payload: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO task_events (id, task_id, event_type, event_text, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), taskID, eventType, eventText, string(pj), s.nowISO(),
	)
	return err
}

func (s *Store) CreateTask(title string) (*model.Task, error) {
	title = strings.TrimSpace(title)
	id := uuid.NewString()
	ts := s.nowISO()
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO tasks (id, title, created_at, updated_at, status, priority)
		 VALUES (?, ?, ?, ?, 'open', 0)`,
		id, title, ts, ts,
	); err != nil {
		return nil, err
	}
	if err := s.addTaskEvent(tx, id, "created", "Task created", map[string]any{"title": title}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(id)
}

func (s *Store) GetTask(id string) (*model.Task, error) {
	row := s.DB.QueryRow(
		`SELECT id, title, details, status, due_at, priority, created_at, updated_at
		 FROM tasks WHERE id = ?`,
		id,
	)
	return scanTask(row)
}

func scanTask(row interface{ Scan(...any) error }) (*model.Task, error) {
	var t model.Task
	var details, due sql.NullString
	if err := row.Scan(&t.ID, &t.Title, &details, &t.Status, &due, &t.Priority, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Details = nullableString(details)
	t.DueAt = nullableString(due)
	return &t, nil
}

// FindLatestActionableTask returns the most-recently-updated task whose status
// is 'open' or 'snoozed'.
func (s *Store) FindLatestActionableTask() (*model.Task, error) {
	row := s.DB.QueryRow(
		`SELECT id, title, details, status, due_at, priority, created_at, updated_at
		 FROM tasks
		 WHERE status IN ('open', 'snoozed')
		 ORDER BY updated_at DESC
		 LIMIT 1`,
	)
	return scanTask(row)
}

func (s *Store) ListActionableTasks() ([]model.Task, error) {
	now := s.nowISO()
	rows, err := s.DB.Query(
		`SELECT id, title, details, status, due_at, priority, created_at, updated_at
		 FROM tasks
		 WHERE status = 'open'
		   OR (status = 'snoozed' AND due_at IS NOT NULL AND due_at <= ?)
		 ORDER BY priority DESC, created_at ASC`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) CompleteTask(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE tasks SET status = 'done', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addTaskEvent(tx, id, "completed", "Task marked done", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SoftDeleteTask(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE tasks SET status = 'deleted', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addTaskEvent(tx, id, "deleted", "Task soft-deleted", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UndeleteTask(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE tasks SET status = 'open', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addTaskEvent(tx, id, "undeleted", "Task restored", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SnoozeTask(id, whenText string, until time.Time) error {
	untilISO := until.In(s.TZ).Format(IsoFormat)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE tasks SET status = 'snoozed', due_at = ?, updated_at = ? WHERE id = ?`,
		untilISO, s.nowISO(), id,
	); err != nil {
		return err
	}
	if err := s.addTaskEvent(tx, id, "snoozed", "Task snoozed until "+untilISO, map[string]any{
		"raw_time_text":   whenText,
		"resolved_due_at": untilISO,
		"timezone":        s.TZ.String(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// EditTask replaces a task's title (required, non-empty) and details (nil
// stores SQL NULL).
func (s *Store) EditTask(id, title string, details *string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	old, err := s.GetTask(id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("task %s not found", id)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if details != nil {
		_, err = tx.Exec(`UPDATE tasks SET title = ?, details = ?, updated_at = ? WHERE id = ?`,
			title, *details, s.nowISO(), id)
	} else {
		_, err = tx.Exec(`UPDATE tasks SET title = ?, details = NULL, updated_at = ? WHERE id = ?`,
			title, s.nowISO(), id)
	}
	if err != nil {
		return err
	}
	oldDetails := ""
	if old.Details != nil {
		oldDetails = *old.Details
	}
	newDetails := ""
	if details != nil {
		newDetails = *details
	}
	if err := s.addTaskEvent(tx, id, "edited", "Task edited", map[string]any{
		"old_title": old.Title, "new_title": title,
		"old_details": oldDetails, "new_details": newDetails,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) TaskEvents(id string) ([]model.Event, error) {
	rows, err := s.DB.Query(
		`SELECT id, event_type, event_text, payload_json, created_at
		 FROM task_events WHERE task_id = ? ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}
