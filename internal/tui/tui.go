package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// DumpView is a non-interactive renderer used for visual debugging: open the
// stores, force a window size, run a single View(), print the result. No
// alt-screen, no key dispatch — just shows exactly what the view function
// would produce in a real terminal of size (w, h).
func DumpView(notesDB, tasksDB, problemsDB, tz string, w, h int) error {
	notes, tasks, problems, err := OpenStores(notesDB, tasksDB, problemsDB, tz)
	if err != nil {
		return fmt.Errorf("open stores: %w", err)
	}
	defer notes.Close()
	defer tasks.Close()
	defer problems.Close()

	m := NewModel(notes, tasks, problems, "")
	m.refresh()
	m.reloadOpLog()
	m.setStatus("Welcome — press ? for help", true)
	m.SetSize(w, h)
	fmt.Print(m.View())
	fmt.Println()
	return nil
}

// Run is the entry point used by cmd/assistant-tui. It opens the three stores,
// boots the model, and blocks until the user quits. backupDir = "" disables
// the automatic backup loop.
func Run(notesDB, tasksDB, problemsDB, tz, backupDir string) error {
	notes, tasks, problems, err := OpenStores(notesDB, tasksDB, problemsDB, tz)
	if err != nil {
		return fmt.Errorf("open stores: %w", err)
	}
	defer notes.Close()
	defer tasks.Close()
	defer problems.Close()

	m := NewModel(notes, tasks, problems, backupDir)
	m.refresh()
	m.reloadOpLog()
	welcome := "Welcome — press ? for help"
	if backupDir != "" {
		welcome += " (backups → " + backupDir + ")"
	}
	m.setStatus(welcome, true)

	prog := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
