package tui

import (
	"database/sql"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SalTor/assistant/internal/backup"
)

// backupTickMsg fires on a fixed interval (see backupTickInterval). When it
// lands we check whether the idle deadline has passed and, if so, kick off a
// backup goroutine.
type backupTickMsg time.Time

// backupDoneMsg carries the result of an asynchronous backup run.
type backupDoneMsg struct {
	dest string
	err  error
}

// backupTickCmd schedules the next periodic check. It's only useful when a
// backup destination is configured.
func (m *Model) backupTickCmd() tea.Cmd {
	if m.backupDir == "" {
		return nil
	}
	return tea.Tick(backupTickInterval, func(t time.Time) tea.Msg {
		return backupTickMsg(t)
	})
}

// markEdit is called from recordOp() whenever a mutation completes. Sets the
// pending flag and arms the idle timer.
func (m *Model) markEdit() {
	if m.backupDir == "" {
		return
	}
	m.backupPending = true
	m.backupDeadline = time.Now().Add(backupIdleDelay)
}

// resetIdleTimer is called from each KeyMsg. It only does work when a backup
// is already pending — random navigation in an unedited session should not
// arm the timer.
func (m *Model) resetIdleTimer() {
	if m.backupDir == "" || !m.backupPending {
		return
	}
	m.backupDeadline = time.Now().Add(backupIdleDelay)
}

// maybeStartBackup launches the async backup if the idle window has elapsed
// and no backup is already running. Returns nil if there's nothing to do.
func (m *Model) maybeStartBackup() tea.Cmd {
	if !m.backupPending || m.backingUp || m.backupDir == "" {
		return nil
	}
	if time.Now().Before(m.backupDeadline) {
		return nil
	}
	m.backingUp = true
	m.setStatus("Backing up databases…", true)
	return m.runBackupCmd()
}

// runBackupCmd returns a tea.Cmd that calls Backup() off the main loop and
// returns the result as a backupDoneMsg.
func (m *Model) runBackupCmd() tea.Cmd {
	dbs := map[string]*sql.DB{
		"notes":    m.notesStore.DB,
		"tasks":    m.tasksStore.DB,
		"problems": m.problemsStore.DB,
	}
	dir := m.backupDir
	return func() tea.Msg {
		dest, err := backup.Backup(dbs, dir)
		return backupDoneMsg{dest: dest, err: err}
	}
}

// runBackupSync is the on-quit path. tea.Quit stops the message loop, so we
// can't deliver a backupDoneMsg back through Update — block here, then return.
func (m *Model) runBackupSync() (string, error) {
	dbs := map[string]*sql.DB{
		"notes":    m.notesStore.DB,
		"tasks":    m.tasksStore.DB,
		"problems": m.problemsStore.DB,
	}
	return backup.Backup(dbs, m.backupDir)
}
