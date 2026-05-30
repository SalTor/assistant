package store

import (
	"database/sql"
	"encoding/json"

	"github.com/SalTor/assistant/internal/model"
)

// scanEvents reads the common (id, event_type, event_text, payload_json,
// created_at) tuple shared by note_events, task_events, and problem_events.
// payload_json is parsed as arbitrary JSON; an unparseable string falls back
// to {"raw": <string>} to match the Python wrapper's behavior.
func scanEvents(rows *sql.Rows) ([]model.Event, error) {
	var out []model.Event
	for rows.Next() {
		var e model.Event
		var text, payload sql.NullString
		if err := rows.Scan(&e.ID, &e.EventType, &text, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		if text.Valid {
			e.EventText = text.String
		}
		if payload.Valid && payload.String != "" {
			var v any
			if err := json.Unmarshal([]byte(payload.String), &v); err != nil {
				e.Payload = map[string]string{"raw": payload.String}
			} else {
				e.Payload = v
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
