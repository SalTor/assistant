package tui

import (
	"fmt"

	"github.com/SalTor/assistant/internal/cli"
	"github.com/SalTor/assistant/internal/model"
	"github.com/SalTor/assistant/internal/operationlog"
	"github.com/SalTor/assistant/internal/store"
)

// OpenStores opens the three per-domain databases at their default paths
// using the supplied tz override (or system default if empty).
func OpenStores(notesDB, tasksDB, problemsDB, tz string) (*store.Store, *store.Store, *store.Store, error) {
	loc, err := cli.DetectTimezone(tz)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("timezone: %w", err)
	}
	if notesDB == "" {
		notesDB = cli.DefaultDBPath("notes")
	}
	if tasksDB == "" {
		tasksDB = cli.DefaultDBPath("tasks")
	}
	if problemsDB == "" {
		problemsDB = cli.DefaultDBPath("problems")
	}
	ns, err := store.Open(notesDB, store.DomainNotes, loc)
	if err != nil {
		return nil, nil, nil, err
	}
	ts, err := store.Open(tasksDB, store.DomainTasks, loc)
	if err != nil {
		ns.Close()
		return nil, nil, nil, err
	}
	ps, err := store.Open(problemsDB, store.DomainProblems, loc)
	if err != nil {
		ns.Close()
		ts.Close()
		return nil, nil, nil, err
	}
	return ns, ts, ps, nil
}

// refresh reloads the three list panels and clamps selection indexes.
// Failures populate the status line but otherwise leave panels unchanged so
// the TUI stays usable through transient errors.
func (m *Model) refresh() {
	if rows, err := m.notesStore.ListFollowups(); err != nil {
		m.setStatus("Notes list issue: "+err.Error(), false)
	} else {
		m.notes = rows
	}
	if rows, err := m.tasksStore.ListActionableTasks(); err != nil {
		m.setStatus("Tasks list issue: "+err.Error(), false)
	} else {
		m.tasks = rows
	}
	if rows, err := m.problemsStore.TreeProblems(); err != nil {
		m.setStatus("Problems tree issue: "+err.Error(), false)
	} else {
		m.problems = rows
	}
	m.clampSelections()
}

// reloadOpLog rereads the most recent operation log entries (newest first).
func (m *Model) reloadOpLog() {
	if entries, err := operationlog.Load(500); err == nil {
		m.opLog = entries
		m.opLogIdx = 0
	}
}

// recordOp appends a row to the on-disk operation log and prepends it to the
// in-memory list so the overlay reflects the new state without a re-read.
func (m *Model) recordOp(action, domain, itemID, detail string) {
	if err := operationlog.Append(action, domain, itemID, detail); err != nil {
		// Log failure is non-fatal: still update the in-memory list so the
		// overlay reflects what just happened, but flag the persistence error.
		m.setStatus("Op log write failed: "+err.Error(), false)
	}
	entry := operationlog.Entry{
		Action: action, Domain: domain, ItemID: itemID, Detail: detail,
	}
	m.opLog = append([]operationlog.Entry{entry}, m.opLog...)
	m.opLogIdx = 0
}

// addCurrent submits whatever is in the input buffer to the active domain
// using the same intent-classified path that the CLI uses, so a body of
// "buy milk" still becomes a note while "postpone until Friday" snoozes the
// latest one.
func (m *Model) addCurrent() {
	body := m.inputBuf
	m.inputBuf = ""
	switch m.add {
	case addNote:
		m.add = addNone
		if body == "" {
			m.setStatus("Nothing entered", false)
			return
		}
		env := cli.InvokeNotes(m.notesStore, body, "")
		if ok, _ := env["ok"].(bool); ok {
			if note, ok := env["note"].(*model.Note); ok && note != nil {
				m.recordOp("create", "note", note.ID, note.Body)
			}
			m.setStatus("Added note. "+humanMessage(env), true)
		} else {
			m.setStatus("Failed adding note. "+humanMessage(env), false)
		}
		m.refresh()
		m.focus = panelNotes

	case addTask:
		m.add = addNone
		if body == "" {
			m.setStatus("Nothing entered", false)
			return
		}
		env := cli.InvokeTasks(m.tasksStore, body, "")
		if ok, _ := env["ok"].(bool); ok {
			if task, ok := env["task"].(*model.Task); ok && task != nil {
				m.recordOp("create", "task", task.ID, task.Title)
			}
			m.setStatus("Added task. "+humanMessage(env), true)
		} else {
			m.setStatus("Failed adding task. "+humanMessage(env), false)
		}
		m.refresh()
		m.focus = panelTasks

	case addProblem, addSubProblem:
		parent := m.addParentID
		m.addParentID = ""
		mode := m.add
		m.add = addNone
		if body == "" {
			m.setStatus("Nothing entered", false)
			return
		}
		env := cli.InvokeProblems(m.problemsStore, body, "", parent)
		if ok, _ := env["ok"].(bool); ok {
			if p, ok := env["problem"].(*model.Problem); ok && p != nil {
				label := "create"
				if mode == addSubProblem {
					label = "create_sub"
				}
				m.recordOp(label, "problem", p.ID, p.Title)
			}
			m.setStatus("Added problem. "+humanMessage(env), true)
		} else {
			m.setStatus("Failed adding problem. "+humanMessage(env), false)
		}
		m.refresh()
		m.focus = panelProblems
	}
}

// humanMessage extracts the envelope's human-readable line for status display,
// falling back to the structured error code if no human message was set.
func humanMessage(env map[string]any) string {
	if msg, ok := env["human_message"].(string); ok && msg != "" {
		return msg
	}
	if code, ok := env["error"].(string); ok && code != "" {
		return code
	}
	return ""
}
