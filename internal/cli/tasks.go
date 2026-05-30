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

func TasksRoute(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "tasks: missing verb (init|invoke|list|history|delete|undelete)")
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "init":
		return tasksInit(rest)
	case "invoke":
		return tasksInvoke(rest)
	case "list":
		return tasksList(rest)
	case "history":
		return tasksHistory(rest)
	case "delete":
		return tasksDelete(rest)
	case "undelete":
		return tasksUndelete(rest)
	default:
		fmt.Fprintf(stderr, "tasks: unknown verb %q\n", verb)
		return 2
	}
}

func tasksInit(args []string) int {
	fs := flag.NewFlagSet("tasks init", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	return envelope.Print(map[string]any{"ok": true, "action": "init", "db": c.DB}, c.Pretty)
}

func tasksList(args []string) int {
	fs := flag.NewFlagSet("tasks list", flag.ContinueOnError)
	var c commonFlags
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	rows, err := s.ListActionableTasks()
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "list_tasks", "count": len(rows), "data": rows,
	}, c.Pretty)
}

func tasksHistory(args []string) int {
	fs := flag.NewFlagSet("tasks history", flag.ContinueOnError)
	var c commonFlags
	var taskID string
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	fs.StringVar(&taskID, "task-id", "", "Task id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if taskID == "" {
		fmt.Fprintln(stderr, "tasks history: --task-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	task, err := s.GetTask(taskID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if task == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "history", "error": envelope.ErrTaskNotFound, "task_id": taskID,
		}, c.Pretty)
	}
	events, err := s.TaskEvents(taskID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	return envelope.Print(map[string]any{
		"ok": true, "action": "history", "task": task, "events": events,
	}, c.Pretty)
}

func tasksDelete(args []string) int {
	fs := flag.NewFlagSet("tasks delete", flag.ContinueOnError)
	var c commonFlags
	var taskID string
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	fs.StringVar(&taskID, "task-id", "", "Task id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if taskID == "" {
		fmt.Fprintln(stderr, "tasks delete: --task-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	task, err := s.GetTask(taskID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if task == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "delete", "error": envelope.ErrTaskNotFound, "task_id": taskID,
		}, c.Pretty)
	}
	if err := s.SoftDeleteTask(taskID, "cli_delete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetTask(taskID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "delete",
		"task":          updated,
		"human_message": "Task soft-deleted.",
	}, c.Pretty)
}

func tasksUndelete(args []string) int {
	fs := flag.NewFlagSet("tasks undelete", flag.ContinueOnError)
	var c commonFlags
	var taskID string
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	fs.StringVar(&taskID, "task-id", "", "Task id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if taskID == "" {
		fmt.Fprintln(stderr, "tasks undelete: --task-id is required")
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()

	task, err := s.GetTask(taskID)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	if task == nil {
		return envelope.Print(map[string]any{
			"ok": false, "action": "undelete", "error": envelope.ErrTaskNotFound, "task_id": taskID,
		}, c.Pretty)
	}
	if err := s.UndeleteTask(taskID, "cli_undelete"); err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	updated, _ := s.GetTask(taskID)
	return envelope.Print(map[string]any{
		"ok": true, "action": "undelete",
		"task":          updated,
		"human_message": "Task restored.",
	}, c.Pretty)
}

func tasksInvoke(args []string) int {
	fs := flag.NewFlagSet("tasks invoke", flag.ContinueOnError)
	var c commonFlags
	var message, taskID string
	registerCommon(fs, DefaultDBPath("tasks"), &c)
	fs.StringVar(&message, "message", "", "User message (required)")
	fs.StringVar(&taskID, "task-id", "", "Optional target task id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if message == "" {
		fmt.Fprintln(stderr, "tasks invoke: --message is required")
		return 2
	}
	s, err := openStore(c, store.DomainTasks)
	if err != nil {
		return envelope.Print(envelope.Exception(err), c.Pretty)
	}
	defer s.Close()
	return envelope.Print(InvokeTasks(s, message, taskID), c.Pretty)
}

// InvokeTasks is the typed-store + intent-classify path used by both the CLI
// `tasks invoke` verb and the TUI's add-task input.
func InvokeTasks(s *store.Store, message, taskID string) map[string]any {
	intent := classify.ParseTask(message, taskID)
	now := time.Now()

	base := map[string]any{
		"ok":         true,
		"intent":     intent.Intent,
		"confidence": intent.Confidence,
		"input": map[string]any{
			"message":  message,
			"task_id":  stringPtr(taskID),
			"timezone": s.TZ.String(),
		},
	}

	switch intent.Intent {
	case "list_tasks":
		rows, err := s.ListActionableTasks()
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "list_tasks"
		base["count"] = len(rows)
		base["data"] = rows
		if len(rows) == 0 {
			base["human_message"] = "No actionable tasks."
		} else {
			base["human_message"] = fmt.Sprintf("Found %d actionable task(s).", len(rows))
		}
		return base

	case "create_task":
		title := intent.Title
		if title == "" {
			title = message
		}
		if title == "" {
			return mergeError(base, envelope.ErrEmptyTitle, "Task title is empty.")
		}
		task, err := s.CreateTask(title)
		if err != nil {
			return envelope.Exception(err)
		}
		base["action"] = "create_task"
		base["task"] = task
		base["human_message"] = fmt.Sprintf("Created task %s.", task.ID)
		return base
	}

	target := intent.TaskID
	if target == "" {
		latest, err := s.FindLatestActionableTask()
		if err != nil {
			return envelope.Exception(err)
		}
		if latest == nil {
			return mergeError(base, envelope.ErrNoTargetTask, "No target task found.")
		}
		target = latest.ID
	}

	switch intent.Intent {
	case "complete_task":
		if err := s.CompleteTask(target, message); err != nil {
			return envelope.Exception(err)
		}
		task, _ := s.GetTask(target)
		base["action"] = "complete_task"
		base["task"] = task
		base["human_message"] = fmt.Sprintf("Marked task %s as done.", target)
		return base

	case "edit_task":
		if intent.Body == "" {
			return mergeError(base, envelope.ErrEmptyEditBody, "No updated task title provided.")
		}
		existing, err := s.GetTask(target)
		if err != nil {
			return envelope.Exception(err)
		}
		if existing == nil {
			return mergeError(base, envelope.ErrTaskNotFound, fmt.Sprintf("Task %s not found.", target))
		}
		if err := s.EditTask(target, intent.Body, existing.Details); err != nil {
			return envelope.Exception(err)
		}
		task, _ := s.GetTask(target)
		base["action"] = "edit_task"
		base["task"] = task
		base["human_message"] = fmt.Sprintf("Edited task %s.", target)
		return base

	case "snooze_task":
		whenText := intent.WhenText
		if whenText == "" {
			whenText = message
		}
		due := timephrase.Resolve(whenText, now, s.TZ)
		if err := s.SnoozeTask(target, whenText, due); err != nil {
			return envelope.Exception(err)
		}
		task, _ := s.GetTask(target)
		base["action"] = "snooze_task"
		base["task"] = task
		base["resolved_time"] = due.In(s.TZ).Format(store.IsoFormat)
		base["human_message"] = fmt.Sprintf("Snoozed task %s until %s.", target,
			due.In(s.TZ).Format(store.IsoFormat))
		return base
	}

	return mergeError(base, envelope.ErrUnknownIntent, "Could not determine intent.")
}
