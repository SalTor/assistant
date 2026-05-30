---
name: tasks
description: Manage tasks with actionable lists, snoozing, completion, and history
---

# Tasks Skill

Use this skill to manage project tasks and scheduling.

## When to use

- Capture a task from user text.
- List actionable tasks.
- Snooze/defer a task until a date/time phrase.
- Mark a task done.
- Inspect full task history.

## Command interface (preferred)

```bash
assistant tasks <command> [args]
```

Underlying runner: the Go CLI in `internal/cli/tasks.go`.

## Commands

```bash
assistant tasks init --db tasks/tasks.db
assistant tasks invoke --db tasks/tasks.db --pretty --message "Draft scope for feature_x"
assistant tasks list --db tasks/tasks.db --pretty
assistant tasks history --db tasks/tasks.db --task-id <task_id> --pretty
```

## Operational guidance

- Prefer `--task-id` for updates when available.
- Ask for confirmation on ambiguous date phrases.
- Treat JSON `ok=false` as an error requiring clarification.
