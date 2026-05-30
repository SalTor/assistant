package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/SalTor/assistant/internal/model"
)

const problemsSchema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS problems (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  statement TEXT NOT NULL,
  parent_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  FOREIGN KEY(parent_id) REFERENCES problems(id)
);

CREATE TABLE IF NOT EXISTS problem_events (
  id TEXT PRIMARY KEY,
  problem_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(problem_id) REFERENCES problems(id)
);

CREATE TABLE IF NOT EXISTS problem_links (
  id TEXT PRIMARY KEY,
  problem_id TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(problem_id) REFERENCES problems(id)
);

CREATE INDEX IF NOT EXISTS idx_problems_parent ON problems(parent_id);
CREATE INDEX IF NOT EXISTS idx_problems_status_updated ON problems(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_problem_events_problem_time ON problem_events(problem_id, created_at);
CREATE INDEX IF NOT EXISTS idx_problem_links_problem ON problem_links(problem_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_problem_links_unique
  ON problem_links(problem_id, entity_type, entity_id, relation);
`

func (s *Store) addProblemEvent(tx *sql.Tx, problemID, eventType, eventText string, payload map[string]any) error {
	pj, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal problem event payload: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO problem_events (id, problem_id, event_type, event_text, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), problemID, eventType, eventText, string(pj), s.nowISO(),
	)
	return err
}

// titleFromStatement derives a problem title from its statement: first 10
// words, with "…" appended if the statement is longer.
func titleFromStatement(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return "Untitled problem"
	}
	words := strings.Fields(statement)
	if len(words) <= 10 {
		return statement
	}
	return strings.Join(words[:10], " ") + "…"
}

func (s *Store) CreateProblem(statement string, parentID *string) (*model.Problem, error) {
	statement = strings.TrimSpace(statement)
	id := uuid.NewString()
	ts := s.nowISO()
	title := titleFromStatement(statement)

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var parentArg any
	if parentID != nil {
		parentArg = *parentID
	}
	if _, err := tx.Exec(
		`INSERT INTO problems (id, title, statement, parent_id, created_at, updated_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'open')`,
		id, title, statement, parentArg, ts, ts,
	); err != nil {
		return nil, err
	}
	payload := map[string]any{"title": title, "statement": statement, "parent_id": parentID}
	if err := s.addProblemEvent(tx, id, "created", "Problem created", payload); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetProblem(id)
}

func (s *Store) GetProblem(id string) (*model.Problem, error) {
	row := s.DB.QueryRow(
		`SELECT id, title, statement, parent_id, status, created_at, updated_at
		 FROM problems WHERE id = ?`,
		id,
	)
	return scanProblem(row)
}

func scanProblem(row interface{ Scan(...any) error }) (*model.Problem, error) {
	var p model.Problem
	var parent sql.NullString
	if err := row.Scan(&p.ID, &p.Title, &p.Statement, &parent, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.ParentID = nullableString(parent)
	return &p, nil
}

func (s *Store) FindLatestOpenProblem() (*model.Problem, error) {
	row := s.DB.QueryRow(
		`SELECT id, title, statement, parent_id, status, created_at, updated_at
		 FROM problems WHERE status = 'open'
		 ORDER BY updated_at DESC LIMIT 1`,
	)
	return scanProblem(row)
}

func (s *Store) ListOpenProblems() ([]model.Problem, error) {
	rows, err := s.DB.Query(
		`SELECT id, title, statement, parent_id, status, created_at, updated_at
		 FROM problems WHERE status = 'open'
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// TreeProblems returns all non-deleted problems flattened into a depth-first
// list so that parents always precede their children. Status is preserved so
// the consumer can choose to render solved nodes differently.
func (s *Store) TreeProblems() ([]model.ProblemTreeRow, error) {
	rows, err := s.DB.Query(
		`SELECT id, title, statement, parent_id, status, created_at, updated_at
		 FROM problems WHERE status != 'deleted'
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []model.Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byParent := map[string][]model.Problem{}
	const rootKey = ""
	for _, p := range all {
		key := rootKey
		if p.ParentID != nil {
			key = *p.ParentID
		}
		byParent[key] = append(byParent[key], p)
	}

	out := make([]model.ProblemTreeRow, 0, len(all))
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, p := range byParent[parent] {
			out = append(out, model.ProblemTreeRow{
				ID: p.ID, Title: p.Title, Statement: p.Statement,
				ParentID: p.ParentID, Status: p.Status,
				CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
				Depth: depth,
			})
			walk(p.ID, depth+1)
		}
	}
	walk(rootKey, 0)
	return out, nil
}

func (s *Store) SolveProblem(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE problems SET status = 'solved', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addProblemEvent(tx, id, "solved", "Problem marked solved", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SoftDeleteProblem(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE problems SET status = 'deleted', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addProblemEvent(tx, id, "deleted", "Problem soft-deleted", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UndeleteProblem(id, source string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE problems SET status = 'open', updated_at = ? WHERE id = ?`, s.nowISO(), id); err != nil {
		return err
	}
	if err := s.addProblemEvent(tx, id, "undeleted", "Problem restored", map[string]any{"source_message": source}); err != nil {
		return err
	}
	return tx.Commit()
}

// LinkEntity is idempotent: the unique index on (problem_id, entity_type,
// entity_id, relation) makes duplicate inserts no-op.
func (s *Store) LinkEntity(problemID, entityType, entityID, relation string) error {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	relation = strings.ToLower(strings.TrimSpace(relation))
	entityID = strings.TrimSpace(entityID)
	if entityType != "note" && entityType != "task" && entityType != "problem" {
		return fmt.Errorf("entity_type must be one of: note, task, problem")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO problem_links (id, problem_id, entity_type, entity_id, relation, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), problemID, entityType, entityID, relation, s.nowISO(),
	); err != nil {
		return err
	}
	if err := s.addProblemEvent(tx, problemID, "linked",
		fmt.Sprintf("Linked %s %s as %s", entityType, entityID, relation),
		map[string]any{"entity_type": entityType, "entity_id": entityID, "relation": relation},
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UnlinkEntity removes all links matching (problem_id, entity_type, entity_id);
// if relation is non-empty, only that relation is removed. Returns the number
// of rows deleted.
func (s *Store) UnlinkEntity(problemID, entityType, entityID, relation string) (int, error) {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	entityID = strings.TrimSpace(entityID)
	if entityType != "note" && entityType != "task" && entityType != "problem" {
		return 0, fmt.Errorf("entity_type must be one of: note, task, problem")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var res sql.Result
	rel := strings.ToLower(strings.TrimSpace(relation))
	payloadRel := "*"
	if rel != "" {
		payloadRel = rel
		res, err = tx.Exec(
			`DELETE FROM problem_links
			 WHERE problem_id = ? AND entity_type = ? AND entity_id = ? AND relation = ?`,
			problemID, entityType, entityID, rel,
		)
	} else {
		res, err = tx.Exec(
			`DELETE FROM problem_links
			 WHERE problem_id = ? AND entity_type = ? AND entity_id = ?`,
			problemID, entityType, entityID,
		)
	}
	if err != nil {
		return 0, err
	}
	removed64, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	removed := int(removed64)
	if err := s.addProblemEvent(tx, problemID, "unlinked",
		fmt.Sprintf("Unlinked %s %s", entityType, entityID),
		map[string]any{"entity_type": entityType, "entity_id": entityID, "relation": payloadRel, "removed": removed},
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

func (s *Store) ListLinks(problemID string) ([]model.Link, error) {
	rows, err := s.DB.Query(
		`SELECT id, problem_id, entity_type, entity_id, relation, created_at
		 FROM problem_links WHERE problem_id = ?
		 ORDER BY created_at ASC`,
		problemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Link
	for rows.Next() {
		var l model.Link
		if err := rows.Scan(&l.ID, &l.ProblemID, &l.EntityType, &l.EntityID, &l.Relation, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// RecordProblemProgress appends a `progress_update` event to a problem. Used
// by the project_manager review flow to log stack-binding evidence.
func (s *Store) RecordProblemProgress(problemID, eventText string, payload map[string]any) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.addProblemEvent(tx, problemID, "progress_update", eventText, payload); err != nil {
		return err
	}
	return tx.Commit()
}

// ResolveProblemID returns a problem id matching `token` exactly, or — failing
// that — a single id whose prefix matches and that isn't soft-deleted. When
// `token` is ambiguous (matches more than one non-deleted prefix) it returns
// ("", nil). The empty token returns ("", nil) without a query.
func (s *Store) ResolveProblemID(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	var id string
	if err := s.DB.QueryRow(`SELECT id FROM problems WHERE id = ?`, token).Scan(&id); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	rows, err := s.DB.Query(
		`SELECT id FROM problems WHERE id LIKE ? AND status != 'deleted'`,
		token+"%",
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var matches []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return "", err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}

func (s *Store) ProblemEvents(id string) ([]model.Event, error) {
	rows, err := s.DB.Query(
		`SELECT id, event_type, event_text, payload_json, created_at
		 FROM problem_events WHERE problem_id = ? ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}
