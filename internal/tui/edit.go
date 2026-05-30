package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorDoneMsg is delivered by tea.ExecProcess after the user's $EDITOR
// exits. The TUI applies the change in its handler.
type editorDoneMsg struct {
	domain  string // "note" | "task" | "problem"
	id      string
	tmpPath string
	err     error
}

// startEdit opens the focused panel's selected item in $EDITOR via
// tea.ExecProcess. The cmd it returns suspends the TUI until the editor exits;
// the result is then funneled back through Update as an editorDoneMsg.
func (m *Model) startEdit() tea.Cmd {
	var (
		domain   string
		id       string
		template string
	)
	switch m.focus {
	case panelNotes:
		if len(m.notes) == 0 {
			m.setStatus("Nothing selected", false)
			return nil
		}
		n := m.notes[m.notesIdx]
		full, err := m.notesStore.GetNote(n.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load note", false)
			return nil
		}
		domain, id = "note", full.ID
		template = noteEditTemplate(full.Body)
	case panelTasks:
		if len(m.tasks) == 0 {
			m.setStatus("Nothing selected", false)
			return nil
		}
		t := m.tasks[m.tasksIdx]
		full, err := m.tasksStore.GetTask(t.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load task", false)
			return nil
		}
		details := ""
		if full.Details != nil {
			details = *full.Details
		}
		domain, id = "task", full.ID
		template = taskEditTemplate(full.Title, details)
	case panelProblems:
		if len(m.problems) == 0 {
			m.setStatus("Nothing selected", false)
			return nil
		}
		p := m.problems[m.problemsIdx]
		full, err := m.problemsStore.GetProblem(p.ID)
		if err != nil || full == nil {
			m.setStatus("Could not load problem", false)
			return nil
		}
		domain, id = "problem", full.ID
		template = problemEditTemplate(full.Title, full.Statement)
	default:
		m.setStatus("Pick a panel first (n/t/p)", false)
		return nil
	}

	tmp, err := os.CreateTemp("", "assistant-edit-*.txt")
	if err != nil {
		m.setStatus("Could not create temp file: "+err.Error(), false)
		return nil
	}
	if _, err := tmp.WriteString(template); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		m.setStatus("Could not write temp file: "+err.Error(), false)
		return nil
	}
	tmp.Close()
	tmpPath := tmp.Name()

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Shell out so $EDITOR can contain args ("nvim -p", "code --wait"). Leave
	// stdin/stdout/stderr unset — bubbletea's ExecProcess wires them to the
	// program's own input/output when it suspends.
	cmd := exec.Command("sh", "-c", fmt.Sprintf("%s %q", editor, tmpPath))

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{domain: domain, id: id, tmpPath: tmpPath, err: err}
	})
}

// applyEditorResult reads the temp file the editor wrote, parses it by
// domain, and persists the result. Always removes the temp file.
func (m *Model) applyEditorResult(msg editorDoneMsg) {
	defer os.Remove(msg.tmpPath)
	if msg.err != nil {
		m.setStatus("Editor exited with error: "+msg.err.Error(), false)
		return
	}
	data, err := os.ReadFile(msg.tmpPath)
	if err != nil {
		m.setStatus("Could not read editor output: "+err.Error(), false)
		return
	}
	content := stripCommentLines(string(data))

	switch msg.domain {
	case "note":
		body := strings.TrimRight(content, "\n")
		if strings.TrimSpace(body) == "" {
			m.setStatus("Empty body; edit cancelled", false)
			return
		}
		if err := m.notesStore.EditNoteBody(msg.id, body); err != nil {
			m.setStatus("Failed edit: "+err.Error(), false)
			return
		}
		m.recordOp("edit", "note", msg.id, body)
		m.setStatus("Edited note "+msg.id, true)
	case "task":
		title, details := splitTitleBody(content)
		if title == "" {
			m.setStatus("Empty title; edit cancelled", false)
			return
		}
		var detailsPtr *string
		if strings.TrimSpace(details) != "" {
			detailsPtr = &details
		}
		if err := m.tasksStore.EditTask(msg.id, title, detailsPtr); err != nil {
			m.setStatus("Failed edit: "+err.Error(), false)
			return
		}
		m.recordOp("edit", "task", msg.id, title)
		m.setStatus("Edited task "+msg.id, true)
	case "problem":
		title, statement := splitTitleBody(content)
		if title == "" {
			m.setStatus("Empty title; edit cancelled", false)
			return
		}
		if err := m.problemsStore.EditProblem(msg.id, title, statement); err != nil {
			m.setStatus("Failed edit: "+err.Error(), false)
			return
		}
		m.recordOp("edit", "problem", msg.id, title)
		m.setStatus("Edited problem "+msg.id, true)
	}
	m.refresh()
}

const editCommentHeader = "# Lines starting with '#' are ignored.\n" +
	"# First non-blank line is the title; blank line then body.\n" +
	"# Save and exit to apply, or leave the title empty to cancel.\n"

const noteCommentHeader = "# Lines starting with '#' are ignored.\n" +
	"# Edit the note body below. Save empty to cancel.\n"

func noteEditTemplate(body string) string {
	return body + "\n\n" + noteCommentHeader
}

func taskEditTemplate(title, details string) string {
	return title + "\n\n" + details + "\n\n" + editCommentHeader
}

func problemEditTemplate(title, statement string) string {
	return title + "\n\n" + statement + "\n\n" + editCommentHeader
}

// stripCommentLines drops lines whose first non-whitespace character is '#'.
// Other lines are preserved verbatim. Used so the on-screen guidance in the
// editor template doesn't leak into stored content.
func stripCommentLines(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// splitTitleBody returns (title, body) using git-commit-style parsing: the
// first non-blank line is the title; everything after the first blank line
// following it is the body. Leading/trailing whitespace is trimmed off both.
func splitTitleBody(s string) (string, string) {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return "", ""
	}
	title := strings.TrimSpace(lines[i])
	i++
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	body := strings.TrimRight(strings.Join(lines[i:], "\n"), "\n")
	body = strings.TrimLeft(body, "\n")
	return title, body
}
