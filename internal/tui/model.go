package tui

import (
	"time"

	"github.com/SalTor/assistant/internal/model"
	"github.com/SalTor/assistant/internal/operationlog"
	"github.com/SalTor/assistant/internal/store"
)

// panel identifies which of the three lists (or the dashboard pseudo-focus)
// has keyboard focus.
type panel int

const (
	panelDashboard panel = iota
	panelNotes
	panelTasks
	panelProblems
)

// overlay indicates which (mutually exclusive) modal is active. When non-zero
// the underlying panels still render but key dispatch is routed to the
// overlay's handler first.
type overlay int

const (
	overlayNone overlay = iota
	overlayHelp
	overlayOpLog
	overlayNoteDetail
	overlayTaskDetail
	overlayProblemDetail
	overlayLinkPicker
)

// addMode tracks whether the bottom-line input prompt is collecting text and
// what to do with it on submit.
type addMode int

const (
	addNone addMode = iota
	addNote
	addTask
	addProblem
	addSubProblem
)

// firstClassRelations are rendered at the top of any link relation list. Other
// relations are still permitted but appear after these in the picker.
var firstClassRelations = []string{"addresses", "evidence", "critique", "depends_on"}

// backupIdleDelay is how long after the last edit/interaction the deferred
// backup fires. The Python reference had no analogue — this matches the spec
// in the user request.
const backupIdleDelay = 2 * time.Minute

// backupTickInterval is how often the model checks whether the idle deadline
// has passed. Small enough that the user perceives the backup as prompt,
// large enough not to wake the runtime constantly.
const backupTickInterval = 10 * time.Second

// Model is the Bubble Tea model: every piece of state the TUI mutates.
type Model struct {
	notesStore    *store.Store
	tasksStore    *store.Store
	problemsStore *store.Store

	focus   panel
	overlay overlay
	add     addMode

	// Backup state. backupDir is empty when no destination is configured, in
	// which case all backup logic is short-circuited.
	backupDir      string
	backupPending  bool      // an edit has occurred since the last backup
	backupDeadline time.Time // when the idle timer expires; zero if no pending edit
	backingUp      bool      // a backup goroutine is currently running

	notes    []model.Note
	tasks    []model.Task
	problems []model.ProblemTreeRow

	notesIdx, tasksIdx, problemsIdx int

	inputBuf       string
	addParentID    string
	pendingDelete  bool
	pendingDeleted time.Time

	width, height int

	status    string
	statusErr bool

	// Detail overlay state.
	detailNote          *model.Note
	detailNoteEvents    []model.Event
	detailTask          *model.Task
	detailTaskEvents    []model.Event
	detailProblem       *model.Problem
	detailProblemLinks  []model.Link
	detailProblemEvents []model.Event
	detailLinkIdx       int

	// Operation log overlay state.
	opLog    []operationlog.Entry
	opLogIdx int

	// Link picker state.
	linkPickerSourceDomain string // "note" | "task"
	linkPickerSourceID     string
	linkPickerProblems     []model.Problem
	linkPickerProblemIdx   int
	linkPickerRelationIdx  int
}

// NewModel wires up the three stores. Stores are owned by the model and
// closed when Run() returns. Pass backupDir = "" to disable the backup loop.
func NewModel(notes, tasks, problems *store.Store, backupDir string) *Model {
	return &Model{
		notesStore:    notes,
		tasksStore:    tasks,
		problemsStore: problems,
		focus:         panelDashboard,
		backupDir:     backupDir,
	}
}

// setStatus stores a status-bar message; ok=false routes it through the red
// style. Callers don't need to think about clearing — the next status update
// replaces the previous one.
func (m *Model) setStatus(msg string, ok bool) {
	m.status = msg
	m.statusErr = !ok
}

// clamp ensures an index stays inside [0, len-1] even when a list shrinks
// after a refresh.
func clamp(i, length int) int {
	if length == 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= length {
		return length - 1
	}
	return i
}

func (m *Model) clampSelections() {
	m.notesIdx = clamp(m.notesIdx, len(m.notes))
	m.tasksIdx = clamp(m.tasksIdx, len(m.tasks))
	m.problemsIdx = clamp(m.problemsIdx, len(m.problems))
}

// SetSize is exported so non-interactive callers (DumpView) can inject a
// terminal size without going through the bubbletea event loop.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// disarmDelete cancels a pending d-press if more than ~1.5s passed since the
// arm. The Python version disarmed on any other key; we use a time-based
// fallback so brief mis-presses don't strand the user in a half-armed state.
func (m *Model) disarmDelete() {
	if m.pendingDelete && time.Since(m.pendingDeleted) > 1500*time.Millisecond {
		m.pendingDelete = false
	}
}
