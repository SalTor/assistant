// Command assistant-tui launches the keyboard-first TUI defined in
// internal/tui. The same per-domain SQLite databases used by `assistant`
// CLI are opened directly — no subprocess shell-out per keystroke.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SalTor/assistant/internal/tui"
)

func main() {
	fs := flag.NewFlagSet("assistant-tui", flag.ContinueOnError)
	var notesDB, tasksDB, problemsDB, tz, backupDir string
	var dumpView bool
	var dumpW, dumpH int
	fs.StringVar(&notesDB, "db-notes", "", "Override notes DB path")
	fs.StringVar(&tasksDB, "db-tasks", "", "Override tasks DB path")
	fs.StringVar(&problemsDB, "db-problems", "", "Override problems DB path")
	fs.StringVar(&tz, "tz", "", "IANA timezone (defaults to system local)")
	fs.StringVar(&backupDir, "backup-dir", os.Getenv("ASSISTANT_BACKUP_DIR"),
		"Directory for timestamped DB backups; flag wins over $ASSISTANT_BACKUP_DIR, empty disables")
	fs.BoolVar(&dumpView, "dump-view", false, "Print one render of the View() with synthetic window size, then exit (debug)")
	fs.IntVar(&dumpW, "dump-w", 120, "Width to use with --dump-view")
	fs.IntVar(&dumpH, "dump-h", 28, "Height to use with --dump-view")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if dumpView {
		if err := tui.DumpView(notesDB, tasksDB, problemsDB, tz, dumpW, dumpH); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := tui.Run(notesDB, tasksDB, problemsDB, tz, backupDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
