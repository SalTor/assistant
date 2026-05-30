package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/SalTor/assistant/internal/model"
)

// View composes the top bar, three panels, status line, and input prompt.
// Overlays render on top of the existing layout (the rest of the screen
// stays visible) via cell-accurate compositing.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing…"
	}

	topBar := m.renderTopBar()
	statusLine := m.renderStatus()
	inputLine := m.renderInput()

	bottomRows := 1 // status
	if inputLine != "" {
		bottomRows++
	}
	panelsHeight := m.height - 1 - bottomRows
	if panelsHeight < 5 {
		panelsHeight = 5
	}

	notesW, tasksW, problemsW := splitWidths(m.width)
	// Body row budget: panel total height minus border (2) and title row (1).
	bodyRows := panelsHeight - 3
	if bodyRows < 1 {
		bodyRows = 1
	}
	notesPanel := m.renderPanel("Notes (n)", panelNotes, notesW, panelsHeight, m.renderNoteRows(bodyRows, panelInteriorWidth(notesW)))
	tasksPanel := m.renderPanel("Tasks (t)", panelTasks, tasksW, panelsHeight, m.renderTaskRows(bodyRows, panelInteriorWidth(tasksW)))
	problemsPanel := m.renderPanel("Problems (p)", panelProblems, problemsW, panelsHeight, m.renderProblemRows(bodyRows, panelInteriorWidth(problemsW)))
	panels := lipgloss.JoinHorizontal(lipgloss.Top, notesPanel, tasksPanel, problemsPanel)

	parts := []string{topBar, panels, statusLine}
	if inputLine != "" {
		parts = append(parts, inputLine)
	}
	base := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if overlayContent := m.renderOverlay(); overlayContent != "" {
		return overlayCenter(base, overlayContent, m.width, m.height)
	}
	return base
}

// renderTopBar paints a full-width blue background row with " Assistant TUI "
// pinned left and "? keybinds" pinned right. Matches the original curses TUI's
// reverse-video header.
func (m *Model) renderTopBar() string {
	const title = " Assistant TUI "
	const hint = "? keybinds "
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	row := title + strings.Repeat(" ", gap) + hint
	return styleTopBar.Width(m.width).Render(row)
}

// overlayCenter splices `overlay` (a multi-line styled string) over `base`
// at the geometric center. Cells outside the overlay's footprint are kept
// from base; cells inside are replaced with overlay content. Both sides go
// through ansi.Cut so styles survive the operation.
func overlayCenter(base, overlay string, width, height int) string {
	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)
	overlayW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > overlayW {
			overlayW = w
		}
	}

	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	startRow := (height - overlayH) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (width - overlayW) / 2
	if startCol < 0 {
		startCol = 0
	}

	for i, ol := range overlayLines {
		row := startRow + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		bg := baseLines[row]
		left := ansi.Cut(bg, 0, startCol)
		right := ansi.Cut(bg, startCol+overlayW, width)
		// Pad each piece so concatenation lines up — Cut returns visible
		// content but doesn't pad short sources.
		left = padRight(left, startCol)
		baseLines[row] = left + ol + right
	}
	return strings.Join(baseLines, "\n")
}

// padRight extends a styled line with plain spaces until its visual width
// reaches `target`. Spaces inherit no styling, which is what we want for the
// transparent gap between the panels and the overlay.
func padRight(s string, target int) string {
	w := lipgloss.Width(s)
	if w >= target {
		return s
	}
	return s + strings.Repeat(" ", target-w)
}

// splitWidths reproduces the original distribution: a baseline of 18 columns
// per panel, with extra space split 25%/25%/50% in favor of the problems panel.
func splitWidths(total int) (int, int, int) {
	const base = 18
	const gutter = 0 // lipgloss borders eat a column on each side already
	const panelCount = 3
	used := base*panelCount + gutter*(panelCount-1)
	extra := total - used
	if extra < 0 {
		return base, base, base
	}
	notes := base + extra/4
	tasks := base + extra/4
	problems := total - notes - tasks - gutter*(panelCount-1)
	if problems < base {
		problems = base
	}
	return notes, tasks, problems
}

// renderPanel wraps a list of pre-rendered rows in a focused/unfocused box
// with title. `width` and `height` are the desired OUTER dimensions; the
// rounded border adds 2 to each, so we pass Width(width-2) / Height(height-2)
// to lipgloss to get a rendered block that matches exactly.
func (m *Model) renderPanel(title string, p panel, width, height int, rows []string) string {
	style := stylePanelUnfocused
	if m.focus == p {
		style = stylePanelFocused
	}
	style = style.Width(width - 2).Height(height - 2)

	header := styleOverlayTitle.Render(title)
	body := strings.Join(rows, "\n")
	return style.Render(header + "\n" + body)
}

// panelInteriorWidth returns the cell budget per row inside a panel of given
// outer width. The panel uses border (1 cell each side) and Padding(0, 1)
// (1 cell each side), so each row gets `outer - 4` columns.
func panelInteriorWidth(outer int) int {
	w := outer - 4
	if w < 1 {
		return 1
	}
	return w
}

// windowSlice returns a slice of `total` items starting at offset such that
// `selected` is always within the returned range. Pure helper — no panel
// knowledge.
func windowSlice(total, selected, max int) (start, end int) {
	if total <= max {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	start = selected - max/2
	if start < 0 {
		start = 0
	}
	end = start + max
	if end > total {
		end = total
		start = end - max
	}
	return start, end
}

func (m *Model) renderNoteRows(maxRows, maxW int) []string {
	if len(m.notes) == 0 {
		return []string{styleEmpty.Render("(empty)")}
	}
	start, end := windowSlice(len(m.notes), m.notesIdx, maxRows)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		n := m.notes[i]
		out = append(out, m.renderRow(i == m.notesIdx && m.focus == panelNotes, "", n.ID, n.Body, noteSuffix(n), maxW))
	}
	return out
}

func noteSuffix(n model.Note) string {
	if n.FollowupState == "snoozed" && n.FollowupAfter != nil {
		return " ⏳" + *n.FollowupAfter
	}
	return ""
}

func (m *Model) renderTaskRows(maxRows, maxW int) []string {
	if len(m.tasks) == 0 {
		return []string{styleEmpty.Render("(empty)")}
	}
	start, end := windowSlice(len(m.tasks), m.tasksIdx, maxRows)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := m.tasks[i]
		suffix := ""
		if t.Status == "snoozed" && t.DueAt != nil {
			suffix = " ⏳" + *t.DueAt
		}
		out = append(out, m.renderRow(i == m.tasksIdx && m.focus == panelTasks, "", t.ID, t.Title, suffix, maxW))
	}
	return out
}

func (m *Model) renderProblemRows(maxRows, maxW int) []string {
	if len(m.problems) == 0 {
		return []string{styleEmpty.Render("(empty)")}
	}
	start, end := windowSlice(len(m.problems), m.problemsIdx, maxRows)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		p := m.problems[i]
		indent := strings.Repeat("  ", p.Depth)
		selected := i == m.problemsIdx && m.focus == panelProblems
		short := p.ID
		if len(short) > 4 {
			short = short[:4]
		}
		switch p.Status {
		case "open":
			out = append(out, m.renderRow(selected, indent, p.ID, p.Title, "", maxW))
		case "solved":
			full := indent + "[" + short + "] ✓ " + p.Title
			full = ansi.Truncate(full, maxW, "…")
			if selected {
				out = append(out, styleSelected.Render(full))
			} else {
				out = append(out, styleDone.Render(full))
			}
		default:
			suffix := fmt.Sprintf(" (%s)", p.Status)
			out = append(out, m.renderRow(selected, indent, p.ID, p.Title, suffix, maxW))
		}
	}
	return out
}

// renderRow renders one panel item as `<indent>[xxxx] <body><suffix>` (matching
// the original curses TUI), then truncates with an ellipsis to fit in maxW
// visual columns.
func (m *Model) renderRow(selected bool, indent, id, body, suffix string, maxW int) string {
	short := id
	if len(short) > 4 {
		short = short[:4]
	}
	full := indent + "[" + short + "] " + body + suffix
	full = ansi.Truncate(full, maxW, "…")
	if selected {
		return styleSelected.Render(full)
	}
	return styleText.Render(full)
}

// renderStatus chooses the green vs red style and renders the current status
// string. An empty status is rendered as a single space so the layout doesn't
// collapse.
func (m *Model) renderStatus() string {
	if m.status == "" {
		return " "
	}
	if m.statusErr {
		return styleStatusErr.Render(m.status)
	}
	return styleStatusOK.Render(m.status)
}

// renderInput shows the bottom-line "add foo> " prompt while the input mode
// is active. The cursor is rendered as a trailing block character so the user
// always sees where typing lands.
func (m *Model) renderInput() string {
	var prompt string
	switch m.add {
	case addNote:
		prompt = "add note> "
	case addTask:
		prompt = "add task> "
	case addProblem:
		prompt = "add problem> "
	case addSubProblem:
		prompt = "add subproblem> "
	default:
		return ""
	}
	return stylePrompt.Render(prompt) + styleText.Render(m.inputBuf+"▌")
}
