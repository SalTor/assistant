package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SalTor/assistant/internal/model"
)

const helpText = `Global
  q          quit
  ?          toggle this help
  o          operation log
  r          refresh
  n / t / p  focus notes / tasks / problems
  h / l      cycle focus left / right
  j / k      move selection within focused list
  Esc        back to dashboard / close overlay

In a focused panel
  a          add new item
  A          add sub-problem (problems only)
  e          edit selected item in $EDITOR
  Enter      open detail overlay
  L          link selected note/task to a problem
  s          mark selected problem solved (problems only)
  d, then d  soft-delete selected item

In add mode
  Enter      submit
  Esc        cancel
  Ctrl-U     clear input
  Ctrl-W     delete last word

In problem detail
  j / k      move between linked items
  u          unlink selected link
  Esc / q    close

In operation log
  j / k      move between operations
  u          undelete selected delete
  Esc / q    close

In link picker
  j / k      pick problem
  h / l      pick relation
  1-4        jump to relation
  Enter      submit, Esc cancel`

// renderOverlay returns the active overlay (if any) already styled inside a
// modal box. The view layer composes the result over the panels.
func (m *Model) renderOverlay() string {
	switch m.overlay {
	case overlayHelp:
		return styleOverlayBox.Render(styleOverlayTitle.Render("Keybinds") + "\n\n" + helpText)
	case overlayOpLog:
		return m.renderOpLogOverlay()
	case overlayNoteDetail:
		return m.renderNoteDetailOverlay()
	case overlayTaskDetail:
		return m.renderTaskDetailOverlay()
	case overlayProblemDetail:
		return m.renderProblemDetailOverlay()
	case overlayLinkPicker:
		return m.renderLinkPickerOverlay()
	}
	return ""
}

// updateHelp closes the help overlay; any of the documented keys works.
func (m *Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q":
		m.overlay = overlayNone
	}
	return m, nil
}

// updateOpLog handles navigation and undelete-from-log.
func (m *Model) updateOpLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "o":
		m.overlay = overlayNone
	case "j", "down":
		m.opLogIdx = clamp(m.opLogIdx+1, len(m.opLog))
	case "k", "up":
		m.opLogIdx = clamp(m.opLogIdx-1, len(m.opLog))
	case "u":
		m.undeleteFromOpLog()
	}
	return m, nil
}

// undeleteFromOpLog inverts a delete entry from the log. Non-delete entries
// are ignored with an error status.
func (m *Model) undeleteFromOpLog() {
	if len(m.opLog) == 0 {
		m.setStatus("Operation log is empty", false)
		return
	}
	e := m.opLog[m.opLogIdx]
	if e.Action != "delete" {
		m.setStatus("Selected operation is not a delete", false)
		return
	}
	var err error
	switch e.Domain {
	case "note":
		err = m.notesStore.UndeleteNote(e.ItemID, "tui_undelete")
	case "task":
		err = m.tasksStore.UndeleteTask(e.ItemID, "tui_undelete")
	case "problem":
		err = m.problemsStore.UndeleteProblem(e.ItemID, "tui_undelete")
	default:
		m.setStatus("Unknown domain in op log: "+e.Domain, false)
		return
	}
	if err != nil {
		m.setStatus("Failed restore: "+err.Error(), false)
		return
	}
	m.recordOp("undelete", e.Domain, e.ItemID, e.Detail)
	m.setStatus("Restored "+e.Domain+" "+e.ItemID, true)
	m.refresh()
}

func (m *Model) renderOpLogOverlay() string {
	if len(m.opLog) == 0 {
		return styleOverlayBox.Render(styleOverlayTitle.Render("Operation log") + "\n\n" + styleEmpty.Render("(empty)"))
	}
	var rows []string
	for i, e := range m.opLog {
		line := fmt.Sprintf("%s  %-9s %-7s %s  %s", e.Timestamp, e.Action, e.Domain, e.ItemID, e.Detail)
		if i == m.opLogIdx {
			line = styleSelected.Render(line)
		} else {
			line = styleText.Render(line)
		}
		rows = append(rows, line)
		if i >= 20 {
			rows = append(rows, styleMuted.Render("…"))
			break
		}
	}
	hint := styleHint.Render("j/k navigate · u undelete selected delete · o/esc/q close")
	return styleOverlayBox.Render(
		styleOverlayTitle.Render("Operation log") + "\n\n" +
			strings.Join(rows, "\n") + "\n\n" + hint,
	)
}

// openDetail dispatches Enter on a focused panel's selected item to the
// appropriate detail overlay.
func (m *Model) openDetail() {
	switch m.focus {
	case panelNotes:
		if len(m.notes) == 0 {
			return
		}
		n := m.notes[m.notesIdx]
		full, err := m.notesStore.GetNote(n.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load note", false)
			return
		}
		events, _ := m.notesStore.NoteEvents(n.ID)
		m.detailNote = full
		m.detailNoteEvents = events
		m.overlay = overlayNoteDetail
	case panelTasks:
		if len(m.tasks) == 0 {
			return
		}
		t := m.tasks[m.tasksIdx]
		full, err := m.tasksStore.GetTask(t.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load task", false)
			return
		}
		events, _ := m.tasksStore.TaskEvents(t.ID)
		m.detailTask = full
		m.detailTaskEvents = events
		m.overlay = overlayTaskDetail
	case panelProblems:
		if len(m.problems) == 0 {
			return
		}
		p := m.problems[m.problemsIdx]
		full, err := m.problemsStore.GetProblem(p.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load problem", false)
			return
		}
		links, _ := m.problemsStore.ListLinks(p.ID)
		events, _ := m.problemsStore.ProblemEvents(p.ID)
		m.detailProblem = full
		m.detailProblemLinks = links
		m.detailProblemEvents = events
		m.detailLinkIdx = 0
		m.overlay = overlayProblemDetail
	}
}

func (m *Model) updateNoteDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.overlay = overlayNone
		m.detailNote = nil
	}
	return m, nil
}

func (m *Model) updateTaskDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.overlay = overlayNone
		m.detailTask = nil
	}
	return m, nil
}

func (m *Model) updateProblemDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.overlay = overlayNone
		m.detailProblem = nil
	case "j", "down":
		m.detailLinkIdx = clamp(m.detailLinkIdx+1, len(m.detailProblemLinks))
	case "k", "up":
		m.detailLinkIdx = clamp(m.detailLinkIdx-1, len(m.detailProblemLinks))
	case "u":
		m.unlinkSelected()
	}
	return m, nil
}

func (m *Model) unlinkSelected() {
	if m.detailProblem == nil || len(m.detailProblemLinks) == 0 {
		return
	}
	link := m.detailProblemLinks[m.detailLinkIdx]
	removed, err := m.problemsStore.UnlinkEntity(m.detailProblem.ID, link.EntityType, link.EntityID, link.Relation)
	if err != nil {
		m.setStatus("Failed unlink: "+err.Error(), false)
		return
	}
	m.recordOp("unlink", "problem", m.detailProblem.ID,
		fmt.Sprintf("%s %s (%s)", link.EntityType, link.EntityID, link.Relation))
	m.setStatus(fmt.Sprintf("Unlinked %d row(s)", removed), true)
	// Refresh the open overlay so the list reflects the change.
	links, _ := m.problemsStore.ListLinks(m.detailProblem.ID)
	m.detailProblemLinks = links
	m.detailLinkIdx = clamp(m.detailLinkIdx, len(links))
}

func (m *Model) renderNoteDetailOverlay() string {
	if m.detailNote == nil {
		return ""
	}
	n := m.detailNote
	header := styleOverlayTitle.Render("Note "+n.ID) + "  " + styleMuted.Render(n.Status+"/"+n.FollowupState)
	body := styleText.Render(n.Body)
	events := renderEventLog(m.detailNoteEvents)
	hint := styleHint.Render("Esc / q / Enter to close")
	return styleOverlayBox.Render(strings.Join([]string{header, "", body, "", events, "", hint}, "\n"))
}

func (m *Model) renderTaskDetailOverlay() string {
	if m.detailTask == nil {
		return ""
	}
	t := m.detailTask
	header := styleOverlayTitle.Render("Task "+t.ID) + "  " + styleMuted.Render(t.Status)
	body := styleText.Render(t.Title)
	if t.Details != nil && *t.Details != "" {
		body += "\n" + styleText.Render(*t.Details)
	}
	events := renderEventLog(m.detailTaskEvents)
	hint := styleHint.Render("Esc / q / Enter to close")
	return styleOverlayBox.Render(strings.Join([]string{header, "", body, "", events, "", hint}, "\n"))
}

func (m *Model) renderProblemDetailOverlay() string {
	if m.detailProblem == nil {
		return ""
	}
	p := m.detailProblem
	header := styleOverlayTitle.Render("Problem "+p.ID) + "  " + styleMuted.Render(p.Status)
	body := styleText.Render(p.Title) + "\n" + styleMuted.Render(p.Statement)

	var linkRows []string
	if len(m.detailProblemLinks) == 0 {
		linkRows = append(linkRows, styleEmpty.Render("(no links)"))
	} else {
		for i, l := range m.detailProblemLinks {
			line := fmt.Sprintf("%s %s [%s]", l.EntityType, l.EntityID, l.Relation)
			if i == m.detailLinkIdx {
				line = styleSelected.Render(line)
			} else {
				line = styleText.Render(line)
			}
			linkRows = append(linkRows, line)
		}
	}
	events := renderEventLog(m.detailProblemEvents)
	hint := styleHint.Render("j/k navigate · u unlink selected · Esc/q close")
	return styleOverlayBox.Render(strings.Join([]string{
		header, "", body, "",
		styleOverlayTitle.Render("Links"),
		strings.Join(linkRows, "\n"),
		"", events, "", hint,
	}, "\n"))
}

func renderEventLog(events []model.Event) string {
	if len(events) == 0 {
		return styleEmpty.Render("(no events)")
	}
	var rows []string
	rows = append(rows, styleOverlayTitle.Render("Events"))
	for _, e := range events {
		rows = append(rows, styleMuted.Render(e.CreatedAt+"  ")+styleText.Render("["+e.EventType+"] "+e.EventText))
	}
	return strings.Join(rows, "\n")
}

// openLinkPicker prepares the link-picker state from whichever panel is
// focused. Notes and tasks are linkable sources; the problems panel itself
// isn't (use the problem detail's u-key to unlink instead).
func (m *Model) openLinkPicker() {
	switch m.focus {
	case panelNotes:
		if len(m.notes) == 0 {
			return
		}
		m.linkPickerSourceDomain = "note"
		m.linkPickerSourceID = m.notes[m.notesIdx].ID
	case panelTasks:
		if len(m.tasks) == 0 {
			return
		}
		m.linkPickerSourceDomain = "task"
		m.linkPickerSourceID = m.tasks[m.tasksIdx].ID
	default:
		m.setStatus("Select a note or task first", false)
		return
	}
	open, err := m.problemsStore.ListOpenProblems()
	if err != nil {
		m.setStatus("Could not load problems: "+err.Error(), false)
		return
	}
	if len(open) == 0 {
		m.setStatus("No open problems to link to", false)
		return
	}
	m.linkPickerProblems = open
	m.linkPickerProblemIdx = 0
	m.linkPickerRelationIdx = 0
	m.overlay = overlayLinkPicker
}

func (m *Model) updateLinkPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.overlay = overlayNone
	case "j", "down":
		m.linkPickerProblemIdx = clamp(m.linkPickerProblemIdx+1, len(m.linkPickerProblems))
	case "k", "up":
		m.linkPickerProblemIdx = clamp(m.linkPickerProblemIdx-1, len(m.linkPickerProblems))
	case "h", "left":
		m.linkPickerRelationIdx = clamp(m.linkPickerRelationIdx-1, len(firstClassRelations))
	case "l", "right":
		m.linkPickerRelationIdx = clamp(m.linkPickerRelationIdx+1, len(firstClassRelations))
	case "1":
		m.linkPickerRelationIdx = 0
	case "2":
		m.linkPickerRelationIdx = 1
	case "3":
		m.linkPickerRelationIdx = 2
	case "4":
		m.linkPickerRelationIdx = 3
	case "enter":
		m.commitLink()
	}
	return m, nil
}

func (m *Model) commitLink() {
	if len(m.linkPickerProblems) == 0 {
		return
	}
	problem := m.linkPickerProblems[m.linkPickerProblemIdx]
	relation := firstClassRelations[m.linkPickerRelationIdx]
	if err := m.problemsStore.LinkEntity(problem.ID, m.linkPickerSourceDomain, m.linkPickerSourceID, relation); err != nil {
		m.setStatus("Failed link: "+err.Error(), false)
		return
	}
	m.recordOp("link", "problem", problem.ID,
		fmt.Sprintf("%s %s (%s)", m.linkPickerSourceDomain, m.linkPickerSourceID, relation))
	m.setStatus(fmt.Sprintf("Linked (%s) to [%s]", relation, problem.ID), true)
	m.overlay = overlayNone
}

func (m *Model) renderLinkPickerOverlay() string {
	if len(m.linkPickerProblems) == 0 {
		return ""
	}
	header := styleOverlayTitle.Render(
		fmt.Sprintf("Link %s %s", m.linkPickerSourceDomain, m.linkPickerSourceID))

	var probRows []string
	for i, p := range m.linkPickerProblems {
		line := p.ID + "  " + p.Title
		if i == m.linkPickerProblemIdx {
			line = styleSelected.Render(line)
		} else {
			line = styleText.Render(line)
		}
		probRows = append(probRows, line)
	}

	var relCells []string
	for i, r := range firstClassRelations {
		label := fmt.Sprintf(" %d %s ", i+1, r)
		if i == m.linkPickerRelationIdx {
			relCells = append(relCells, styleSelected.Render(label))
		} else {
			relCells = append(relCells, styleText.Render(label))
		}
	}

	hint := styleHint.Render("j/k pick problem · h/l or 1-4 pick relation · Enter submit · Esc cancel")
	return styleOverlayBox.Render(strings.Join([]string{
		header, "",
		styleOverlayTitle.Render("Problems"),
		strings.Join(probRows, "\n"),
		"",
		styleOverlayTitle.Render("Relation"),
		strings.Join(relCells, "  "),
		"", hint,
	}, "\n"))
}
