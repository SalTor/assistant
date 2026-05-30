package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Init is required by tea.Model; we have no startup-side commands.
func (m *Model) Init() tea.Cmd { return nil }

// Update is the central dispatch point. Order:
//   1. Window resize is always processed (so layouts adapt under overlays).
//   2. Add-mode swallows all keys until ESC/Enter.
//   3. Whichever overlay is active gets first crack.
//   4. Otherwise the global/normal keymap applies.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case editorDoneMsg:
		m.applyEditorResult(msg)
		return m, nil

	case tea.KeyMsg:
		m.disarmDelete()
		if m.add != addNone {
			return m.updateAddMode(msg)
		}
		switch m.overlay {
		case overlayHelp:
			return m.updateHelp(msg)
		case overlayOpLog:
			return m.updateOpLog(msg)
		case overlayNoteDetail:
			return m.updateNoteDetail(msg)
		case overlayTaskDetail:
			return m.updateTaskDetail(msg)
		case overlayProblemDetail:
			return m.updateProblemDetail(msg)
		case overlayLinkPicker:
			return m.updateLinkPicker(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

// updateNormal handles the panel-focused keymap when no overlay or add prompt
// is active.
func (m *Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.overlay = overlayHelp
		return m, nil
	case "o":
		m.reloadOpLog()
		m.overlay = overlayOpLog
		return m, nil
	case "r":
		m.refresh()
		m.setStatus("Refreshed", true)
		return m, nil
	case "n":
		m.focus = panelNotes
		m.setStatus("Notes focus", true)
		return m, nil
	case "t":
		m.focus = panelTasks
		m.setStatus("Tasks focus", true)
		return m, nil
	case "p":
		m.focus = panelProblems
		m.setStatus("Problems focus", true)
		return m, nil
	case "esc":
		m.focus = panelDashboard
		return m, nil
	case "h", "left":
		m.focus = cycleFocusLeft(m.focus)
		return m, nil
	case "l", "right":
		m.focus = cycleFocusRight(m.focus)
		return m, nil
	case "j", "down":
		if m.focus == panelDashboard {
			m.focus = panelNotes
			return m, nil
		}
		m.moveSelection(+1)
		return m, nil
	case "k", "up":
		m.moveSelection(-1)
		return m, nil
	case "a":
		m.startAdd(false)
		return m, nil
	case "A":
		m.startAdd(true)
		return m, nil
	case "e":
		if cmd := m.startEdit(); cmd != nil {
			return m, cmd
		}
		return m, nil
	case "enter":
		m.openDetail()
		return m, nil
	case "L":
		m.openLinkPicker()
		return m, nil
	case "d":
		return m.handleDelete(), nil
	case "s":
		return m.handleSolve(), nil
	}
	return m, nil
}

// updateAddMode collects characters into the input buffer until ESC cancels
// or Enter submits. Ctrl-U clears, Ctrl-W deletes the last word.
func (m *Model) updateAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.add = addNone
		m.inputBuf = ""
		m.addParentID = ""
		return m, nil
	case "enter":
		m.addCurrent()
		return m, nil
	case "backspace":
		if m.inputBuf != "" {
			r := []rune(m.inputBuf)
			m.inputBuf = string(r[:len(r)-1])
		}
		return m, nil
	case "ctrl+u":
		m.inputBuf = ""
		return m, nil
	case "ctrl+w":
		m.inputBuf = deleteLastWord(m.inputBuf)
		return m, nil
	}
	// All printable input — bubbletea passes msg.Runes for keys with one or
	// more typed characters.
	if len(msg.Runes) > 0 {
		m.inputBuf += string(msg.Runes)
	}
	return m, nil
}

// startAdd opens the input prompt for the focused panel. Pressing Shift-A in
// the problems panel with a selection opens "add subproblem" mode.
func (m *Model) startAdd(subProblem bool) {
	m.inputBuf = ""
	m.addParentID = ""
	switch m.focus {
	case panelNotes:
		m.add = addNote
	case panelTasks:
		m.add = addTask
	case panelProblems:
		if subProblem && len(m.problems) > 0 {
			m.add = addSubProblem
			m.addParentID = m.problems[m.problemsIdx].ID
		} else {
			m.add = addProblem
		}
	default:
		m.setStatus("Pick a panel first (n/t/p)", false)
	}
}

func cycleFocusLeft(p panel) panel {
	switch p {
	case panelProblems:
		return panelTasks
	case panelTasks:
		return panelNotes
	case panelNotes:
		return panelDashboard
	default:
		return panelProblems
	}
}

func cycleFocusRight(p panel) panel {
	switch p {
	case panelDashboard:
		return panelNotes
	case panelNotes:
		return panelTasks
	case panelTasks:
		return panelProblems
	default:
		return panelDashboard
	}
}

// moveSelection adjusts the selected index within whichever panel is focused.
// dashboard ignores movement entirely (handled by caller for j-from-dashboard).
func (m *Model) moveSelection(delta int) {
	switch m.focus {
	case panelNotes:
		m.notesIdx = clamp(m.notesIdx+delta, len(m.notes))
	case panelTasks:
		m.tasksIdx = clamp(m.tasksIdx+delta, len(m.tasks))
	case panelProblems:
		m.problemsIdx = clamp(m.problemsIdx+delta, len(m.problems))
	}
}

// handleDelete implements the d-then-d-again confirmation. The first `d` arms
// the action; the second within ~1.5s commits the soft-delete on the focused
// panel's selected item.
func (m *Model) handleDelete() tea.Model {
	if !m.pendingDelete {
		m.pendingDelete = true
		m.pendingDeleted = time.Now()
		m.setStatus("Press d again to soft-delete selected item", false)
		return m
	}
	m.pendingDelete = false

	switch m.focus {
	case panelNotes:
		if len(m.notes) == 0 {
			m.setStatus("Nothing selected", false)
			return m
		}
		n := m.notes[m.notesIdx]
		if err := m.notesStore.SoftDeleteNote(n.ID, "tui_delete"); err != nil {
			m.setStatus("Failed delete: "+err.Error(), false)
			return m
		}
		m.recordOp("delete", "note", n.ID, n.Body)
		m.setStatus("Deleted note "+n.ID, true)
	case panelTasks:
		if len(m.tasks) == 0 {
			m.setStatus("Nothing selected", false)
			return m
		}
		t := m.tasks[m.tasksIdx]
		if err := m.tasksStore.SoftDeleteTask(t.ID, "tui_delete"); err != nil {
			m.setStatus("Failed delete: "+err.Error(), false)
			return m
		}
		m.recordOp("delete", "task", t.ID, t.Title)
		m.setStatus("Deleted task "+t.ID, true)
	case panelProblems:
		if len(m.problems) == 0 {
			m.setStatus("Nothing selected", false)
			return m
		}
		p := m.problems[m.problemsIdx]
		if err := m.problemsStore.SoftDeleteProblem(p.ID, "tui_delete"); err != nil {
			m.setStatus("Failed delete: "+err.Error(), false)
			return m
		}
		m.recordOp("delete", "problem", p.ID, p.Title)
		m.setStatus("Deleted problem "+p.ID, true)
	default:
		m.setStatus("Nothing selected", false)
		return m
	}
	m.refresh()
	return m
}

// handleSolve marks the focused problem solved. Only valid in the problems
// panel — other focuses produce a status hint instead.
func (m *Model) handleSolve() tea.Model {
	if m.focus != panelProblems {
		m.setStatus("Solve only applies to problems (press p first)", false)
		return m
	}
	if len(m.problems) == 0 {
		m.setStatus("Nothing selected", false)
		return m
	}
	p := m.problems[m.problemsIdx]
	if err := m.problemsStore.SolveProblem(p.ID, "tui_solve"); err != nil {
		m.setStatus("Failed solve: "+err.Error(), false)
		return m
	}
	m.recordOp("solve", "problem", p.ID, p.Title)
	m.setStatus("Solved problem "+p.ID, true)
	m.refresh()
	return m
}

// deleteLastWord removes the trailing run of non-space characters and any
// preceding spaces, matching Ctrl-W's behavior in most readline implementations.
func deleteLastWord(s string) string {
	s = strings.TrimRight(s, " ")
	if i := strings.LastIndex(s, " "); i >= 0 {
		return s[:i+1]
	}
	return ""
}
