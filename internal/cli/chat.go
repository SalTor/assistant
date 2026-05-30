package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/google/shlex"
)

// chatHelp is the body printed by `chat /help`. It mirrors the Python
// implementation's grammar reference verbatim.
const chatHelp = `Slash command format:
  /notes <free text>
  /notes add <text>
  /notes followups
  /notes list
  /notes done [<note_id>|latest]
  /notes snooze [<note_id>|latest] until <time phrase>
  /notes history <note_id>

  /tasks <free text>
  /tasks add <text>
  /tasks list
  /tasks done [<task_id>|latest]
  /tasks snooze [<task_id>|latest] until <time phrase>
  /tasks history <task_id>

  /problems <free text>
  /problems add <text>
  /problems list
  /problems tree
  /problems show <problem_id>
  /problems done [<problem_id>|latest]
  /problems history <problem_id>
  /problems link <problem_id> <note|task|problem> <entity_id> [relation]
  /problems unlink <problem_id> <note|task|problem> <entity_id> [relation]
`

// snoozeStopWords are tokens that, when they appear immediately after the
// snooze verb, mean the user did NOT supply an explicit id — they jumped
// straight into the time phrase.
var snoozeStopWords = map[string]struct{}{
	"until": {}, "after": {}, "in": {}, "on": {}, "tomorrow": {}, "next": {},
}

// Chat parses `assistant chat "<text>"` plus the per-domain DB and tz
// overrides, then dispatches into the same per-verb handlers used by the
// direct CLI surface.
func Chat(args []string) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	var dbNotes, dbTasks, dbProblems, tz string
	var pretty bool
	fs.StringVar(&dbNotes, "db-notes", "", "Default notes DB path for /notes")
	fs.StringVar(&dbTasks, "db-tasks", "", "Default tasks DB path for /tasks")
	fs.StringVar(&dbProblems, "db-problems", "", "Default problems DB path for /problems")
	fs.StringVar(&tz, "tz", "", "IANA timezone")
	fs.BoolVar(&pretty, "pretty", false, "Pretty JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "chat: missing slash text")
		return 2
	}
	raw := strings.TrimSpace(rest[0])
	switch raw {
	case "/help", "help", "/chat help":
		fmt.Print(chatHelp)
		return 0
	}
	if !strings.HasPrefix(raw, "/") {
		fmt.Fprintln(stderr, "Chat command must start with '/'. Example: /notes followups")
		return 2
	}

	parts := strings.SplitN(raw[1:], " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		fmt.Print(chatHelp)
		return 0
	}
	domain := strings.ToLower(parts[0])
	tail := ""
	if len(parts) > 1 {
		tail = strings.TrimSpace(parts[1])
	}

	switch domain {
	case "notes":
		if tail == "" {
			fmt.Fprintln(stderr, "/notes requires text or a subcommand")
			return 2
		}
		out, err := buildNotesChatArgs(tail, dbNotes, tz, pretty)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 2
		}
		return NotesRoute(out)
	case "tasks":
		if tail == "" {
			fmt.Fprintln(stderr, "/tasks requires text or a subcommand")
			return 2
		}
		out, err := buildTasksChatArgs(tail, dbTasks, tz, pretty)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 2
		}
		return TasksRoute(out)
	case "problems":
		if tail == "" {
			fmt.Fprintln(stderr, "/problems requires text or a subcommand")
			return 2
		}
		out, err := buildProblemsChatArgs(tail, dbProblems, tz, pretty)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 2
		}
		return ProblemsRoute(out)
	case "chat", "commands":
		fmt.Print(chatHelp)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown slash domain: /%s\n", domain)
		fmt.Print(chatHelp)
		return 2
	}
}

// commonChatPrefix builds the leading --db/--tz/--pretty argv slice shared by
// every chat-derived call.
func commonChatPrefix(db, tz string, pretty bool) []string {
	var args []string
	if db != "" {
		args = append(args, "--db", db)
	}
	if tz != "" {
		args = append(args, "--tz", tz)
	}
	if pretty {
		args = append(args, "--pretty")
	}
	return args
}

func buildNotesChatArgs(tail, db, tz string, pretty bool) ([]string, error) {
	tokens, err := shlex.Split(tail)
	if err != nil {
		return nil, err
	}
	common := commonChatPrefix(db, tz, pretty)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("Missing notes command text.")
	}
	verb := strings.ToLower(tokens[0])
	switch verb {
	case "followups", "list":
		return append([]string{"list"}, common...), nil
	case "history":
		if len(tokens) < 2 {
			return nil, fmt.Errorf("/notes history requires <note_id>")
		}
		return append([]string{"history", "--note-id", tokens[1]}, common...), nil
	case "done":
		var noteID string
		if len(tokens) >= 2 && strings.ToLower(tokens[1]) != "latest" {
			noteID = tokens[1]
		}
		out := append([]string{"invoke", "--message", "done"}, common...)
		if noteID != "" {
			out = append(out, "--note-id", noteID)
		}
		return out, nil
	case "snooze":
		idx, noteID := snoozeIDIndex(tokens)
		phrase := strings.TrimSpace(strings.Join(tokens[idx:], " "))
		if phrase == "" {
			return nil, fmt.Errorf("/notes snooze requires a time phrase, e.g. 'until after q3 ends'")
		}
		out := append([]string{"invoke", "--message", "postpone " + phrase}, common...)
		if noteID != "" {
			out = append(out, "--note-id", noteID)
		}
		return out, nil
	case "add":
		body := strings.TrimSpace(strings.Join(tokens[1:], " "))
		if body == "" {
			return nil, fmt.Errorf("/notes add requires note text")
		}
		return append([]string{"invoke", "--message", body}, common...), nil
	}
	return append([]string{"invoke", "--message", tail}, common...), nil
}

func buildTasksChatArgs(tail, db, tz string, pretty bool) ([]string, error) {
	tokens, err := shlex.Split(tail)
	if err != nil {
		return nil, err
	}
	common := commonChatPrefix(db, tz, pretty)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("Missing tasks command text.")
	}
	verb := strings.ToLower(tokens[0])
	switch verb {
	case "list":
		return append([]string{"list"}, common...), nil
	case "history":
		if len(tokens) < 2 {
			return nil, fmt.Errorf("/tasks history requires <task_id>")
		}
		return append([]string{"history", "--task-id", tokens[1]}, common...), nil
	case "done":
		var taskID string
		if len(tokens) >= 2 && strings.ToLower(tokens[1]) != "latest" {
			taskID = tokens[1]
		}
		out := append([]string{"invoke", "--message", "done"}, common...)
		if taskID != "" {
			out = append(out, "--task-id", taskID)
		}
		return out, nil
	case "snooze":
		idx, taskID := snoozeIDIndex(tokens)
		phrase := strings.TrimSpace(strings.Join(tokens[idx:], " "))
		if phrase == "" {
			return nil, fmt.Errorf("/tasks snooze requires a time phrase, e.g. 'until after q3 ends'")
		}
		out := append([]string{"invoke", "--message", "postpone " + phrase}, common...)
		if taskID != "" {
			out = append(out, "--task-id", taskID)
		}
		return out, nil
	case "add":
		body := strings.TrimSpace(strings.Join(tokens[1:], " "))
		if body == "" {
			return nil, fmt.Errorf("/tasks add requires task text")
		}
		return append([]string{"invoke", "--message", body}, common...), nil
	}
	return append([]string{"invoke", "--message", tail}, common...), nil
}

func buildProblemsChatArgs(tail, db, tz string, pretty bool) ([]string, error) {
	tokens, err := shlex.Split(tail)
	if err != nil {
		return nil, err
	}
	common := commonChatPrefix(db, tz, pretty)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("Missing problems command text.")
	}
	verb := strings.ToLower(tokens[0])
	switch verb {
	case "list":
		return append([]string{"list"}, common...), nil
	case "tree":
		return append([]string{"tree"}, common...), nil
	case "history":
		if len(tokens) < 2 {
			return nil, fmt.Errorf("/problems history requires <problem_id>")
		}
		return append([]string{"history", "--problem-id", tokens[1]}, common...), nil
	case "show":
		if len(tokens) < 2 {
			return nil, fmt.Errorf("/problems show requires <problem_id>")
		}
		return append([]string{"show", "--problem-id", tokens[1]}, common...), nil
	case "link":
		if len(tokens) < 4 {
			return nil, fmt.Errorf("/problems link requires <problem_id> <note|task|problem> <entity_id> [relation]")
		}
		relation := "addresses"
		if len(tokens) >= 5 {
			relation = tokens[4]
		}
		return append([]string{
			"link",
			"--problem-id", tokens[1],
			"--entity-type", tokens[2],
			"--entity-id", tokens[3],
			"--relation", relation,
		}, common...), nil
	case "unlink":
		if len(tokens) < 4 {
			return nil, fmt.Errorf("/problems unlink requires <problem_id> <note|task|problem> <entity_id> [relation]")
		}
		out := append([]string{
			"unlink",
			"--problem-id", tokens[1],
			"--entity-type", tokens[2],
			"--entity-id", tokens[3],
		}, common...)
		if len(tokens) >= 5 {
			out = append(out, "--relation", tokens[4])
		}
		return out, nil
	case "done":
		var problemID string
		if len(tokens) >= 2 && strings.ToLower(tokens[1]) != "latest" {
			problemID = tokens[1]
		}
		out := append([]string{"invoke", "--message", "solved"}, common...)
		if problemID != "" {
			out = append(out, "--problem-id", problemID)
		}
		return out, nil
	case "add":
		body := strings.TrimSpace(strings.Join(tokens[1:], " "))
		if body == "" {
			return nil, fmt.Errorf("/problems add requires problem text")
		}
		return append([]string{"invoke", "--message", body}, common...), nil
	}
	return append([]string{"invoke", "--message", tail}, common...), nil
}

// snoozeIDIndex inspects tokens after the snooze verb. If the next token
// looks like an explicit id (not "latest", not a phrase stop-word) it's
// treated as the id and the phrase starts at index 2; "latest" also advances
// to index 2 with no id; anything else is the start of the phrase.
func snoozeIDIndex(tokens []string) (idx int, id string) {
	if len(tokens) <= 1 {
		return 1, ""
	}
	first := strings.ToLower(tokens[1])
	if first == "latest" {
		return 2, ""
	}
	if _, isStop := snoozeStopWords[first]; isStop {
		return 1, ""
	}
	return 2, tokens[1]
}
