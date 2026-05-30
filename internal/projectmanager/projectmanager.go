// Package projectmanager implements the `assistant project_manager` skill:
// reads a Jujutsu stack diff, parses machine-readable problem trailers from
// commit descriptions, and offers operations that resolve bindings into the
// problems DB or suggest new problem statements.
package projectmanager

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Trailer keys recognised in commit descriptions.
const (
	TrailerProblem  = "PM-Problem"
	TrailerRelation = "PM-Relation"
	TrailerProgress = "PM-Progress"
)

// Commit is one entry in the jj stack: change_id, commit_id, full description.
type Commit struct {
	ChangeID    string `json:"change_id"`
	CommitID    string `json:"commit_id"`
	Description string `json:"description"`
}

// TrailerBlock pairs a commit identity with the trailer keys parsed from its
// description. Used so the review envelope can show which commit in the stack
// supplied the binding.
type TrailerBlock struct {
	ChangeID string            `json:"change_id"`
	CommitID string            `json:"commit_id"`
	Trailers map[string]string `json:"trailers"`
}

var trailerLineRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.+?)\s*$`)

// LoadCommits runs `jj log -r <revset>` with a fixed template and parses the
// output into Commit values. The template uses an "<<END>>" sentinel so
// multi-line descriptions stay intact.
func LoadCommits(revset string) ([]Commit, error) {
	template := "change_id.short() ++ \"\\n\" ++ commit_id.short() ++ \"\\n\" ++ description ++ \"\\n<<END>>\\n\""
	out, err := runJJ([]string{"log", "-r", revset, "--no-graph", "-T", template})
	if err != nil {
		return nil, fmt.Errorf("jj log: %w", err)
	}
	var commits []Commit
	for _, raw := range strings.Split(out, "<<END>>") {
		lines := strings.Split(strings.Trim(raw, "\n"), "\n")
		if len(lines) < 2 {
			continue
		}
		commits = append(commits, Commit{
			ChangeID:    strings.TrimSpace(lines[0]),
			CommitID:    strings.TrimSpace(lines[1]),
			Description: strings.TrimSpace(strings.Join(lines[2:], "\n")),
		})
	}
	return commits, nil
}

// LoadDiffSummary returns `jj diff -r <revset> --summary`'s stdout, or a
// best-effort error string the caller can include in the review envelope. The
// Python reference treats a failure here as soft (the rest of review still
// runs), so we mirror that by returning a "(diff unavailable: …)" string.
func LoadDiffSummary(revset string) string {
	out, err := runJJ([]string{"diff", "-r", revset, "--summary"})
	if err != nil {
		return fmt.Sprintf("(diff unavailable: %v)", err)
	}
	return out
}

// ParseTrailers pulls recognised PM-* keys out of a commit description. Lines
// that don't match the trailer pattern (or use keys we don't recognise) are
// ignored.
func ParseTrailers(description string) map[string]string {
	found := map[string]string{}
	for _, line := range strings.Split(description, "\n") {
		m := trailerLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		key, val := m[1], m[2]
		switch key {
		case TrailerProblem, TrailerRelation, TrailerProgress:
			found[key] = val
		}
	}
	return found
}

// StackTrailers walks a slice of commits and returns one TrailerBlock per
// commit that supplied at least one recognised trailer.
func StackTrailers(commits []Commit) []TrailerBlock {
	var out []TrailerBlock
	for _, c := range commits {
		t := ParseTrailers(c.Description)
		if len(t) == 0 {
			continue
		}
		out = append(out, TrailerBlock{ChangeID: c.ChangeID, CommitID: c.CommitID, Trailers: t})
	}
	return out
}

// SuggestProblemStatement picks a reasonable seed for a new problem when no
// binding is found. Preference order: first non-trailer commit subject line,
// then first diff-summary line, then a generic fallback.
func SuggestProblemStatement(commits []Commit, diffSummary string) string {
	for _, c := range commits {
		desc := strings.TrimSpace(c.Description)
		if desc == "" {
			continue
		}
		first := strings.TrimSpace(strings.SplitN(desc, "\n", 2)[0])
		if first != "" && !strings.HasPrefix(first, "PM-") {
			return "Problem: " + first
		}
	}
	for _, line := range strings.Split(diffSummary, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return "Problem: " + line
		}
	}
	return "Problem: Work in this stack needs explicit framing"
}

// BuildTrailerBlock formats the standard PM-Problem / PM-Relation /
// (optional) PM-Progress lines as a commit-trailer block.
func BuildTrailerBlock(problemID, relation, progress string) string {
	if relation == "" {
		relation = "addresses"
	}
	lines := []string{
		TrailerProblem + ": " + problemID,
		TrailerRelation + ": " + relation,
	}
	if progress != "" {
		lines = append(lines, TrailerProgress+": "+progress)
	}
	return strings.Join(lines, "\n")
}

// runJJ executes jj with the given args and returns trimmed stdout. The Python
// version reads stdout even on non-zero exit; we surface the stderr in the
// error message instead.
func runJJ(args []string) (string, error) {
	cmd := exec.Command("jj", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
