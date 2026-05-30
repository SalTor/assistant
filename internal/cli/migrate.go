package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MigrateDBs is the one-time helper that relocates per-domain DBs from the
// legacy in-repo locations into the canonical data dir. Output is text — this
// command predates the JSON envelope contract.
func MigrateDBs(args []string) int {
	fs := flag.NewFlagSet("migrate-dbs", flag.ContinueOnError)
	var copyMode, dryRun bool
	fs.BoolVar(&copyMode, "copy", false, "Copy instead of move")
	fs.BoolVar(&dryRun, "dry-run", false, "Show planned actions without writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	move := !copyMode

	cwd, _ := os.Getwd()
	dataDir := DefaultDataDir()

	candidates := map[string][]string{
		"notes":    {filepath.Join(cwd, "notes", "notes.db"), filepath.Join(cwd, "notes.db")},
		"tasks":    {filepath.Join(cwd, "tasks", "tasks.db"), filepath.Join(cwd, "tasks.db")},
		"problems": {filepath.Join(cwd, "problems", "problems.db"), filepath.Join(cwd, "problems.db")},
	}

	type plan struct{ src, dest string }
	var planned []plan
	for _, domain := range []string{"notes", "tasks", "problems"} {
		dest := filepath.Join(dataDir, domain+".db")
		for _, src := range candidates[domain] {
			if _, err := os.Stat(src); err == nil {
				planned = append(planned, plan{src, dest})
				for _, ext := range []string{"-shm", "-wal"} {
					sideSrc := src + ext
					if _, err := os.Stat(sideSrc); err == nil {
						planned = append(planned, plan{sideSrc, dest + ext})
					}
				}
				break
			}
		}
	}

	if len(planned) == 0 {
		fmt.Println("No in-repo DBs found to migrate.")
		return 0
	}

	fmt.Printf("Target data dir: %s\n", dataDir)
	mode := "copy"
	if move {
		mode = "move"
	}
	suffix := ""
	if dryRun {
		suffix = " (dry-run)"
	}
	fmt.Printf("Mode: %s%s\n", mode, suffix)

	if !dryRun {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "mkdir %s: %v\n", dataDir, err)
			return 1
		}
	}

	migrated, skipped, plannedActions := 0, 0, 0
	for _, p := range planned {
		if _, err := os.Stat(p.dest); err == nil {
			rel, _ := filepath.Rel(cwd, p.src)
			fmt.Printf("skip  %s -> %s (destination exists)\n", rel, p.dest)
			skipped++
			continue
		}
		rel, _ := filepath.Rel(cwd, p.src)
		fmt.Printf("%s  %s -> %s\n", mode, rel, p.dest)
		plannedActions++
		if dryRun {
			continue
		}
		if err := transfer(p.src, p.dest, move); err != nil {
			fmt.Fprintf(stderr, "%s %s -> %s: %v\n", mode, p.src, p.dest, err)
			return 1
		}
		migrated++
	}

	if dryRun {
		fmt.Printf("Done. planned=%d skipped=%d\n", plannedActions, skipped)
	} else {
		fmt.Printf("Done. migrated=%d skipped=%d\n", migrated, skipped)
	}
	return 0
}

func transfer(src, dest string, move bool) error {
	if move {
		return os.Rename(src, dest)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
