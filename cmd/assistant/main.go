// Command assistant is the unified CLI described in the assistant TUI spec.
// It dispatches to per-domain verb handlers, the chat slash router, and the
// cross-cutting domains/migrate-dbs commands. The TUI front-end lives at
// `assistant tui`.
package main

import (
	"fmt"
	"os"

	"github.com/SalTor/assistant/internal/cli"
)

const usage = `Usage: assistant <command> [args]

Commands:
  notes <verb> [flags]            notes domain (init|invoke|list|history|delete|undelete)
  tasks <verb> [flags]            tasks domain
  problems <verb> [flags]         problems domain (adds tree|show|link|unlink)
  project_manager <verb> [flags]  jj stack ↔ problems binding (review|trailer)
  chat "<slash text>"             slash-style command dispatcher
  domains                         list registered domains
  migrate-dbs [--copy] [--dry-run]
                                  relocate legacy in-repo DBs into the data dir
  run <domain> [args]             routing alias for <domain> [args]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "notes":
		os.Exit(cli.NotesRoute(args))
	case "tasks":
		os.Exit(cli.TasksRoute(args))
	case "problems":
		os.Exit(cli.ProblemsRoute(args))
	case "project_manager":
		os.Exit(cli.ProjectManagerRoute(args))
	case "chat":
		os.Exit(cli.Chat(args))
	case "domains":
		os.Exit(cli.Domains(args))
	case "migrate-dbs":
		os.Exit(cli.MigrateDBs(args))
	case "run":
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "run: missing domain")
			os.Exit(2)
		}
		dom, rest := args[0], args[1:]
		// Drop a leading "--" the way argparse REMAINDER would.
		if len(rest) > 0 && rest[0] == "--" {
			rest = rest[1:]
		}
		switch dom {
		case "notes":
			os.Exit(cli.NotesRoute(rest))
		case "tasks":
			os.Exit(cli.TasksRoute(rest))
		case "problems":
			os.Exit(cli.ProblemsRoute(rest))
		case "project_manager":
			os.Exit(cli.ProjectManagerRoute(rest))
		default:
			fmt.Fprintf(os.Stderr, "run: unknown domain %q\n", dom)
			os.Exit(2)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}
