package cli

import (
	"flag"
	"fmt"

	"github.com/SalTor/assistant/internal/envelope"
	"github.com/SalTor/assistant/internal/projectmanager"
	"github.com/SalTor/assistant/internal/store"
)

// ProjectManagerRoute dispatches `assistant project_manager <verb>`. Matches
// the Python skill's subcommand layout: `review` and `trailer`.
func ProjectManagerRoute(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "project_manager: missing verb (review|trailer)")
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "review":
		return pmReview(rest)
	case "trailer":
		return pmTrailer(rest)
	default:
		fmt.Fprintf(stderr, "project_manager: unknown verb %q\n", verb)
		return 2
	}
}

func pmReview(args []string) int {
	fs := flag.NewFlagSet("project_manager review", flag.ContinueOnError)
	var dbProblems, tz, revset string
	var pretty, apply, createProblem bool
	fs.StringVar(&dbProblems, "db-problems", DefaultDBPath("problems"), "Path to problems SQLite DB")
	fs.StringVar(&tz, "tz", "", "IANA timezone")
	fs.StringVar(&revset, "revset", "trunk()..@", "JJ revset to inspect")
	fs.BoolVar(&pretty, "pretty", false, "Pretty-print JSON")
	fs.BoolVar(&apply, "apply", false, "Write progress event when a single binding exists")
	fs.BoolVar(&createProblem, "create-problem", false, "Create a suggested problem when no binding found")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	commits, err := projectmanager.LoadCommits(revset)
	if err != nil {
		return envelope.Print(envelope.Exception(err), pretty)
	}
	diffSummary := projectmanager.LoadDiffSummary(revset)
	trailers := projectmanager.StackTrailers(commits)

	out := map[string]any{
		"ok":           true,
		"action":       "review",
		"revset":       revset,
		"commit_count": len(commits),
		"commits":      commits,
		"trailers":     trailers,
		"diff_summary": diffSummary,
	}

	loc, err := DetectTimezone(tz)
	if err != nil {
		return envelope.Print(envelope.Exception(err), pretty)
	}
	s, err := store.Open(dbProblems, store.DomainProblems, loc)
	if err != nil {
		return envelope.Print(envelope.Exception(err), pretty)
	}
	defer s.Close()

	// Collect the unique PM-Problem tokens in stack order.
	seen := map[string]struct{}{}
	var tokens []string
	for _, t := range trailers {
		tok := t.Trailers[projectmanager.TrailerProblem]
		if tok == "" {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		tokens = append(tokens, tok)
	}

	var resolved []string
	for _, tok := range tokens {
		rid, err := s.ResolveProblemID(tok)
		if err != nil {
			return envelope.Print(envelope.Exception(err), pretty)
		}
		if rid != "" {
			resolved = append(resolved, rid)
		}
	}

	switch {
	case len(resolved) == 1:
		out["bound_problem_ids"] = resolved
		out["status"] = "bound"
		if apply {
			problemID := resolved[0]
			relation := "addresses"
			progress := ""
			if len(trailers) > 0 {
				if v := trailers[0].Trailers[projectmanager.TrailerRelation]; v != "" {
					relation = v
				}
				progress = trailers[0].Trailers[projectmanager.TrailerProgress]
			}
			payload := map[string]any{
				"revset":       revset,
				"relation":     relation,
				"progress":     progress,
				"commit_ids":   commitIDs(commits),
				"change_ids":   changeIDs(commits),
				"diff_summary": diffSummary,
			}
			text := fmt.Sprintf("Progress update from revset %s", revset)
			if err := s.RecordProblemProgress(problemID, text, payload); err != nil {
				return envelope.Print(envelope.Exception(err), pretty)
			}
			out["progress_logged"] = true
			out["human_message"] = fmt.Sprintf("Logged progress against problem %s.", problemID)
		}
	case len(resolved) > 1:
		out["bound_problem_ids"] = resolved
		out["ok"] = false
		out["status"] = "ambiguous"
		out["error"] = "multiple_bound_problems"
		out["human_message"] = "Multiple problem bindings found in stack trailers."
	default:
		suggestion := projectmanager.SuggestProblemStatement(commits, diffSummary)
		out["status"] = "unbound"
		out["suggested_problem_statement"] = suggestion
		out["suggested_trailer"] = projectmanager.BuildTrailerBlock("<problem_id>", "addresses", "short progress note")
		if createProblem {
			p, err := s.CreateProblem(suggestion, nil)
			if err != nil {
				return envelope.Print(envelope.Exception(err), pretty)
			}
			out["created_problem_id"] = p.ID
			out["suggested_trailer"] = projectmanager.BuildTrailerBlock(p.ID, "addresses", "short progress note")
			out["human_message"] = fmt.Sprintf("Created problem %s.", p.ID)
		}
	}

	return envelope.Print(out, pretty)
}

func pmTrailer(args []string) int {
	fs := flag.NewFlagSet("project_manager trailer", flag.ContinueOnError)
	var problemID, relation, progress string
	var pretty bool
	fs.StringVar(&problemID, "problem-id", "", "Problem id (required)")
	fs.StringVar(&relation, "relation", "addresses", "Trailer relation")
	fs.StringVar(&progress, "progress", "", "Optional PM-Progress note")
	fs.BoolVar(&pretty, "pretty", false, "Pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if problemID == "" {
		fmt.Fprintln(stderr, "project_manager trailer: --problem-id is required")
		return 2
	}
	return envelope.Print(map[string]any{
		"ok":      true,
		"action":  "trailer_template",
		"trailer": projectmanager.BuildTrailerBlock(problemID, relation, progress),
	}, pretty)
}

func commitIDs(commits []projectmanager.Commit) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.CommitID
	}
	return out
}

func changeIDs(commits []projectmanager.Commit) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.ChangeID
	}
	return out
}
