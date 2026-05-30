package cli

import (
	"flag"
	"fmt"

	"github.com/SalTor/assistant/internal/classify"
	"github.com/SalTor/assistant/internal/envelope"
	"github.com/SalTor/assistant/internal/store"
)

func ProblemsRoute(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "problems: missing verb (init|invoke|list|tree|show|history|delete|undelete|link|unlink)")
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "init":
		return problemsInit(rest)
	case "invoke":
		return problemsInvoke(rest)
	case "list":
		return problemsList(rest)
	case "tree":
		return problemsTree(rest)
	case "show":
		return problemsShow(rest)
	case "history":
		return problemsHistory(rest)
	case "delete":
		return problemsDelete(rest)
	case "undelete":
		return problemsUndelete(rest)
	case "link":
		return problemsLink(rest)
	case "unlink":
		return problemsUnlink(rest)
	default:
		fmt.Fprintf(stderr, "problems: unknown verb %q\n", verb)
		return 2
	}
}

func problemsInit(args []string) int {
	fs := flag.NewFlagSet("problems init", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("problems"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	return envelope.Print(map[string]any{"ok": true, "action": "init", "db": c.DB}, c.Pretty)
}

func problemsList(args []string) int {
	fs := flag.NewFlagSet("problems list", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("problems"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	rows, err := s.ListOpenProblems()
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "list_problems", "count": len(rows), "data": rows,
	}, c.Pretty)
}

func problemsTree(args []string) int {
	fs := flag.NewFlagSet("problems tree", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("problems"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	rows, err := s.TreeProblems()
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "tree_problems", "count": len(rows), "data": rows,
	}, c.Pretty)
}

func problemsShow(args []string) int {
	fs := flag.NewFlagSet("problems show", flag.ContinueOnError)
	var c commonFlags
	var problemID string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" {
		fmt.Fprintln(stderr, "problems show: --problem-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "show", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	links, err := s.ListLinks(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "show", "problem": p, "links": links,
	}, c.Pretty)
}

func problemsHistory(args []string) int {
	fs := flag.NewFlagSet("problems history", flag.ContinueOnError)
	var c commonFlags
	var problemID string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" {
		fmt.Fprintln(stderr, "problems history: --problem-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "history", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	links, err := s.ListLinks(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	events, err := s.ProblemEvents(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "history", "problem": p, "links": links, "events": events,
	}, c.Pretty)
}

func problemsDelete(args []string) int {
	fs := flag.NewFlagSet("problems delete", flag.ContinueOnError)
	var c commonFlags
	var problemID string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" {
		fmt.Fprintln(stderr, "problems delete: --problem-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "delete", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	if err := s.SoftDeleteProblem(problemID, "cli_delete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetProblem(problemID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "delete", "problem": updated,
		"human_message": "Problem soft-deleted.",
	}, c.Pretty)
}

func problemsUndelete(args []string) int {
	fs := flag.NewFlagSet("problems undelete", flag.ContinueOnError)
	var c commonFlags
	var problemID string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" {
		fmt.Fprintln(stderr, "problems undelete: --problem-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "undelete", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	if err := s.UndeleteProblem(problemID, "cli_undelete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetProblem(problemID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "undelete", "problem": updated,
		"human_message": "Problem restored.",
	}, c.Pretty)
}

func problemsLink(args []string) int {
	fs := flag.NewFlagSet("problems link", flag.ContinueOnError)
	var c commonFlags
	var problemID, entityType, entityID, relation string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	fs.StringVar(&entityType, "entity-type", "", "note|task|problem (required)")
	fs.StringVar(&entityID, "entity-id", "", "Entity id (required)")
	fs.StringVar(&relation, "relation", "addresses", "Relation type")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" || entityType == "" || entityID == "" {
		fmt.Fprintln(stderr, "problems link: --problem-id, --entity-type, --entity-id are required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "link", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	if err := s.LinkEntity(problemID, entityType, entityID, relation); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok":            true,
		"action":        "link",
		"human_message": fmt.Sprintf("Linked %s %s to problem %s.", entityType, entityID, problemID),
		"problem_id":    problemID,
		"link": map[string]any{
			"entity_type": entityType, "entity_id": entityID, "relation": relation,
		},
	}, c.Pretty)
}

func problemsUnlink(args []string) int {
	fs := flag.NewFlagSet("problems unlink", flag.ContinueOnError)
	var c commonFlags
	var problemID, entityType, entityID, relation string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	fs.StringVar(&entityType, "entity-type", "", "note|task|problem (required)")
	fs.StringVar(&entityID, "entity-id", "", "Entity id (required)")
	fs.StringVar(&relation, "relation", "", "Optional relation; if blank, all relations are removed")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" || entityType == "" || entityID == "" {
		fmt.Fprintln(stderr, "problems unlink: --problem-id, --entity-type, --entity-id are required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	p, err := s.GetProblem(problemID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if p == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "unlink", "error": envelope.ErrProblemNotFound, "problem_id": problemID,
		}, c.Pretty)
	}
	removed, err := s.UnlinkEntity(problemID, entityType, entityID, relation)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok":            true,
		"action":        "unlink",
		"human_message": fmt.Sprintf("Unlinked %d row(s).", removed),
		"problem_id":    problemID,
		"removed":       removed,
		"unlink": map[string]any{
			"entity_type": entityType, "entity_id": entityID, "relation": stringPtr(relation),
		},
	}, c.Pretty)
}

func problemsInvoke(args []string) int {
	fs := flag.NewFlagSet("problems invoke", flag.ContinueOnError)
	var c commonFlags
	var message, problemID, parentProblemID string
	registerCommon(fs, DefaultDBPath("problems"), &c)
	fs.StringVar(&message, "message", "", "User message (required)")
	fs.StringVar(&problemID, "problem-id", "", "Optional target problem id")
	fs.StringVar(&parentProblemID, "parent-problem-id", "", "Optional parent problem id for new problems")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if message == "" {
		fmt.Fprintln(stderr, "problems invoke: --message is required")
		return 2
	}
	s, err := openStore(c, store.DomainProblems)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	return envelope.Print(InvokeProblems(s, message, problemID, parentProblemID), c.Pretty)
}

// InvokeProblems is the typed-store + intent-classify path used by both the
// CLI `problems invoke` verb and the TUI's add-problem / add-subproblem input.
func InvokeProblems(s *store.Store, message, problemID, parentProblemID string) map[string]any {
	intent := classify.ParseProblem(message, problemID, parentProblemID)

	base := map[string]any{
		"ok":         true,
		"intent":     intent.Intent,
		"confidence": intent.Confidence,
		"input": map[string]any{
			"message":           message,
			"problem_id":        stringPtr(problemID),
			"parent_problem_id": stringPtr(parentProblemID),
		},
	}

	switch intent.Intent {
	case "list_problems":
		rows, err := s.ListOpenProblems()
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "list_problems"
		base["count"] = len(rows)
		base["data"] = rows
		if len(rows) == 0 {
			base["human_message"] = "No open problems."
		} else {
			base["human_message"] = fmt.Sprintf("Found %d open problem(s).", len(rows))
		}
		return base

	case "tree_problems":
		rows, err := s.TreeProblems()
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "tree_problems"
		base["count"] = len(rows)
		base["data"] = rows
		if len(rows) == 0 {
			base["human_message"] = "No problems."
		} else {
			base["human_message"] = fmt.Sprintf("Found %d problem node(s).", len(rows))
		}
		return base

	case "create_problem":
		statement := intent.Statement
		if statement == "" {
			statement = message
		}
		if statement == "" {
			return mergeError(base, envelope.ErrEmptyStatement, "Problem statement is empty.")
		}
		var parent *string
		if intent.ParentID != "" {
			pid := intent.ParentID
			parent = &pid
		}
		p, err := s.CreateProblem(statement, parent)
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "create_problem"
		base["problem"] = p
		base["human_message"] = fmt.Sprintf("Created problem %s.", p.ID)
		return base
	}

	target := intent.ProblemID
	if target == "" {
		latest, err := s.FindLatestOpenProblem()
		if err != nil {
			return envelope.Exception(err)
		}
		if latest == nil {
			return mergeError(base, envelope.ErrNoTargetProblem, "No target problem found.")
		}
		target = latest.ID
	}

	switch intent.Intent {
	case "solve_problem":
		if err := s.SolveProblem(target, message); err != nil {
			return envelope.Exception(err)
		}
		p, _ := s.GetProblem(target)
		base["action"] = "solve_problem"
		base["problem"] = p
		base["human_message"] = fmt.Sprintf("Marked problem %s solved.", target)
		return base

	case "edit_problem":
		if intent.Body == "" {
			return mergeError(base, envelope.ErrEmptyEditBody, "No updated problem statement provided.")
		}
		title := store.TitleFromStatement(intent.Body)
		if err := s.EditProblem(target, title, intent.Body); err != nil {
			return envelope.Exception(err)
		}
		p, _ := s.GetProblem(target)
		base["action"] = "edit_problem"
		base["problem"] = p
		base["human_message"] = fmt.Sprintf("Edited problem %s.", target)
		return base
	}

	return mergeError(base, envelope.ErrUnknownIntent, "Could not determine intent.")
}
