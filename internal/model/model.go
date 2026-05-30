// Package model holds the canonical record shapes that flow through the
// storage layer, JSON envelopes, and the TUI.
package model

type Note struct {
	ID            string  `json:"id"`
	Body          string  `json:"body"`
	Status        string  `json:"status"`
	FollowupState string  `json:"followup_state"`
	FollowupAfter *string `json:"followup_after"`
	Priority      int     `json:"priority"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type Task struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Details   *string `json:"details"`
	Status    string  `json:"status"`
	DueAt     *string `json:"due_at"`
	Priority  int     `json:"priority"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type Problem struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Statement string  `json:"statement"`
	ParentID  *string `json:"parent_id"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type Event struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	EventText string `json:"event_text"`
	Payload   any    `json:"payload"`
	CreatedAt string `json:"created_at"`
}

type Link struct {
	ID         string `json:"id"`
	ProblemID  string `json:"problem_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Relation   string `json:"relation"`
	CreatedAt  string `json:"created_at"`
}

type ProblemTreeRow struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Statement string  `json:"statement"`
	ParentID  *string `json:"parent_id"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Depth     int     `json:"depth"`
}
