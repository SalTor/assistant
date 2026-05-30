package cli

import (
	"flag"
	"fmt"
	"time"

	"github.com/SalTor/assistant/internal/classify"
	"github.com/SalTor/assistant/internal/envelope"
	"github.com/SalTor/assistant/internal/store"
	"github.com/SalTor/assistant/internal/timephrase"
)

// NotesRoute dispatches `assistant notes <verb>` to the right handler.
func NotesRoute(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "notes: missing verb (init|invoke|list|history|delete|undelete)")
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "init":
		return notesInit(rest)
	case "invoke":
		return notesInvoke(rest)
	case "list":
		return notesList(rest)
	case "history":
		return notesHistory(rest)
	case "delete":
		return notesDelete(rest)
	case "undelete":
		return notesUndelete(rest)
	default:
		fmt.Fprintf(stderr, "notes: unknown verb %q\n", verb)
		return 2
	}
}

func notesInit(args []string) int {
	fs := flag.NewFlagSet("notes init", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("notes"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	return envelope.Print(map[string]any{"ok": true, "action": "init", "db": c.DB}, c.Pretty)
}

func notesList(args []string) int {
	fs := flag.NewFlagSet("notes list", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("notes"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	rows, err := s.ListFollowups()
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok":     true,
		"action": "list_followups",
		"count":  len(rows),
		"data":   rows,
	}, c.Pretty)
}

func notesHistory(args []string) int {
	fs := flag.NewFlagSet("notes history", flag.ContinueOnError)
	var c commonFlags
	var noteID string
	registerCommon(fs, DefaultDBPath("notes"), &c)
	fs.StringVar(&noteID, "note-id", "", "Note id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if noteID == "" {
		fmt.Fprintln(stderr, "notes history: --note-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	note, err := s.GetNote(noteID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if note == nil {
		return envelope.Print(map[string]any{
			"ok":      false,
			"action":  "history",
			"error":   envelope.ErrNoteNotFound,
			"note_id": noteID,
		}, c.Pretty)
	}
	events, err := s.NoteEvents(noteID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok":     true,
		"action": "history",
		"note":   note,
		"events": events,
	}, c.Pretty)
}

func notesDelete(args []string) int {
	fs := flag.NewFlagSet("notes delete", flag.ContinueOnError)
	var c commonFlags
	var noteID string
	registerCommon(fs, DefaultDBPath("notes"), &c)
	fs.StringVar(&noteID, "note-id", "", "Note id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if noteID == "" {
		fmt.Fprintln(stderr, "notes delete: --note-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	note, err := s.GetNote(noteID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if note == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "delete", "error": envelope.ErrNoteNotFound, "note_id": noteID,
		}, c.Pretty)
	}
	if err := s.SoftDeleteNote(noteID, "cli_delete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetNote(noteID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "delete",
		"note":          updated,
		"human_message": "Note soft-deleted.",
	}, c.Pretty)
}

func notesUndelete(args []string) int {
	fs := flag.NewFlagSet("notes undelete", flag.ContinueOnError)
	var c commonFlags
	var noteID string
	registerCommon(fs, DefaultDBPath("notes"), &c)
	fs.StringVar(&noteID, "note-id", "", "Note id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if noteID == "" {
		fmt.Fprintln(stderr, "notes undelete: --note-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	note, err := s.GetNote(noteID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if note == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "undelete", "error": envelope.ErrNoteNotFound, "note_id": noteID,
		}, c.Pretty)
	}
	if err := s.UndeleteNote(noteID, "cli_undelete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetNote(noteID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "undelete",
		"note":          updated,
		"human_message": "Note restored.",
	}, c.Pretty)
}

func notesInvoke(args []string) int {
	fs := flag.NewFlagSet("notes invoke", flag.ContinueOnError)
	var c commonFlags
	var message, noteID string
	registerCommon(fs, DefaultDBPath("notes"), &c)
	fs.StringVar(&message, "message", "", "User message (required)")
	fs.StringVar(&noteID, "note-id", "", "Optional target note id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if message == "" {
		fmt.Fprintln(stderr, "notes invoke: --message is required")
		return 2
	}
	s, err := openStore(c, store.DomainNotes)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	env := InvokeNotes(s, message, noteID)
	return envelope.Print(env, c.Pretty)
}

// InvokeNotes mirrors the Python invoke_message() for notes. The base map
// contains intent/confidence/input and is merged into both success and error
// envelopes (matching the Python `**result` spread). Exported so the TUI
// can invoke the same code path as the CLI without going through subprocesses.
func InvokeNotes(s *store.Store, message, noteID string) map[string]any {
	intent := classify.ParseNote(message, noteID)
	now := time.Now()

	base := map[string]any{
		"ok":         true,
		"intent":     intent.Intent,
		"confidence": intent.Confidence,
		"input": map[string]any{
			"message":  message,
			"note_id":  stringPtr(noteID),
			"timezone": s.TZ.String(),
		},
	}

	switch intent.Intent {
	case "list_followups":
		rows, err := s.ListFollowups()
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "list_followups"
		base["data"] = rows
		if len(rows) == 0 {
			base["human_message"] = "No follow-up items right now."
		} else {
			base["human_message"] = fmt.Sprintf("Found %d follow-up item(s).", len(rows))
		}
		return base

	case "create_note":
		body := intent.Body
		if body == "" {
			return mergeError(base, envelope.ErrEmptyBody, "Nothing to save.")
		}
		note, err := s.CreateNote(body)
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "create_note"
		base["note"] = note
		base["human_message"] = fmt.Sprintf("Created note %s.", note.ID)
		return base
	}

	// All remaining intents need a target note.
	target := intent.NoteID
	if target == "" {
		latest, err := s.FindLatestActionableNote()
		if err != nil {
			return envelope.Exception(err)
		}
		if latest == nil {
			return mergeError(base, envelope.ErrNoTargetNote,
				"No actionable note found. Create a note first or pass --note-id.")
		}
		target = latest.ID
	}

	switch intent.Intent {
	case "snooze_note":
		whenText := intent.WhenText
		if whenText == "" {
			whenText = message
		}
		dt := timephrase.Resolve(whenText, now, s.TZ)
		if err := s.SnoozeNote(target, whenText, dt); err != nil {
			return envelope.Exception(err)
		}
		note, _ := s.GetNote(target)
		base["action"] = "snooze_note"
		base["note"] = note
		base["resolved_time"] = dt.In(s.TZ).Format(store.IsoFormat)
		base["human_message"] = fmt.Sprintf("Snoozed note %s until %s.", target,
			dt.In(s.TZ).Format(store.IsoFormat))
		return base

	case "complete_note":
		if err := s.MarkNoteDone(target, message); err != nil {
			return envelope.Exception(err)
		}
		note, _ := s.GetNote(target)
		base["action"] = "complete_note"
		base["note"] = note
		base["human_message"] = fmt.Sprintf("Marked note %s as done.", target)
		return base

	case "edit_note":
		if intent.Body == "" {
			return mergeError(base, envelope.ErrEmptyEditBody, "No updated note body provided.")
		}
		if err := s.EditNoteBody(target, intent.Body); err != nil {
			return envelope.Exception(err)
		}
		note, _ := s.GetNote(target)
		base["action"] = "edit_note"
		base["note"] = note
		base["human_message"] = fmt.Sprintf("Edited note %s.", target)
		return base
	}

	return mergeError(base, envelope.ErrUnknownIntent, "Could not determine intent.")
}

// mergeError mutates `base` into the error envelope shape used by every
// `invoke` handler: ok=false, an error code, a human_message, plus the
// intent/confidence/input fields the Python wrapper splats in via **result.
func mergeError(base map[string]any, code, human string) map[string]any {
	base["ok"] = false
	base["error"] = code
	base["human_message"] = human
	return base
}
