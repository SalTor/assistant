// Package classify mirrors the regex-based intent classifier each domain has
// in the reference implementation. The intent vocabulary is fixed by the spec;
// the patterns and order match notes_skill.py, tasks_skill.py, and
// problems_skill.py byte-for-byte so the CLI behaves identically.
package classify

import (
	"regexp"
	"strings"

	"github.com/SalTor/assistant/internal/timephrase"
)

type NoteIntent struct {
	Intent     string
	Confidence float64
	NoteID     string
	WhenText   string
	Body       string
}

type TaskIntent struct {
	Intent     string
	Confidence float64
	TaskID     string
	WhenText   string
	Title      string
	Body       string
}

type ProblemIntent struct {
	Intent     string
	Confidence float64
	ProblemID  string
	ParentID   string
	Title      string
	Statement  string
	Body       string
}

var (
	noteListFollowupRe = regexp.MustCompile(`(?i)\b(anything|what).*(follow\s*up)\b|\bfollow\s*up worthy\b`)
	noteCompleteRe     = regexp.MustCompile(`(?i)\b(done|completed|no need to follow up|resolved)\b`)
	noteSnoozeRe       = regexp.MustCompile(`(?i)\b(postpone|snooze|defer|push\s+back|until|after|later)\b`)

	taskListRe     = regexp.MustCompile(`(?i)\b(anything|what).*(task|todo)|\bmy tasks\b`)
	taskCompleteRe = regexp.MustCompile(`(?i)\b(done|complete|completed|finished)\b`)
	taskSnoozeRe   = regexp.MustCompile(`(?i)\b(postpone|snooze|defer|until|after|later)\b`)

	problemSolveRe = regexp.MustCompile(`(?i)\b(done|solved|resolved|close)\b`)
)

func ParseNote(message, noteID string) NoteIntent {
	m := strings.TrimSpace(message)
	lower := strings.ToLower(m)

	if noteListFollowupRe.MatchString(lower) {
		return NoteIntent{Intent: "list_followups", Confidence: 0.95}
	}
	if noteCompleteRe.MatchString(lower) {
		return NoteIntent{Intent: "complete_note", Confidence: 0.85, NoteID: noteID}
	}
	if noteSnoozeRe.MatchString(lower) {
		return NoteIntent{
			Intent: "snooze_note", Confidence: 0.75, NoteID: noteID,
			WhenText: timephrase.Extract(m),
		}
	}
	if strings.HasPrefix(lower, "edit note:") {
		return NoteIntent{
			Intent: "edit_note", Confidence: 0.8, NoteID: noteID,
			Body: strings.TrimSpace(m[len("edit note:"):]),
		}
	}
	return NoteIntent{Intent: "create_note", Confidence: 0.7, Body: m}
}

func ParseTask(message, taskID string) TaskIntent {
	m := strings.TrimSpace(message)
	lower := strings.ToLower(m)

	if taskListRe.MatchString(lower) {
		return TaskIntent{Intent: "list_tasks", Confidence: 0.95}
	}
	if taskCompleteRe.MatchString(lower) {
		return TaskIntent{Intent: "complete_task", Confidence: 0.85, TaskID: taskID}
	}
	if taskSnoozeRe.MatchString(lower) {
		return TaskIntent{
			Intent: "snooze_task", Confidence: 0.75, TaskID: taskID,
			WhenText: timephrase.Extract(m),
		}
	}
	if strings.HasPrefix(lower, "edit task:") {
		return TaskIntent{
			Intent: "edit_task", Confidence: 0.8, TaskID: taskID,
			Body: strings.TrimSpace(m[len("edit task:"):]),
		}
	}
	return TaskIntent{Intent: "create_task", Confidence: 0.7, Title: m}
}

func ParseProblem(message, problemID, parentID string) ProblemIntent {
	m := strings.TrimSpace(message)
	lower := strings.ToLower(m)

	switch lower {
	case "list", "show problems", "what problems", "what are my problems":
		return ProblemIntent{Intent: "list_problems", Confidence: 0.95}
	case "tree", "problem tree", "show tree":
		return ProblemIntent{Intent: "tree_problems", Confidence: 0.95}
	}

	if problemSolveRe.MatchString(lower) {
		return ProblemIntent{Intent: "solve_problem", Confidence: 0.85, ProblemID: problemID}
	}

	if strings.HasPrefix(lower, "edit problem:") {
		return ProblemIntent{
			Intent: "edit_problem", Confidence: 0.8, ProblemID: problemID,
			Body: strings.TrimSpace(m[len("edit problem:"):]),
		}
	}

	statement := m
	if strings.HasPrefix(lower, "add ") {
		statement = strings.TrimSpace(m[4:])
	}
	return ProblemIntent{
		Intent: "create_problem", Confidence: 0.7,
		ProblemID: problemID, ParentID: parentID, Statement: statement,
	}
}
