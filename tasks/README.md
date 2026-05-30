# Tasks skill prototype

This folder contains a runnable task-agent prototype with SQLite persistence.

## Files

- `tasks_skill.py` – human-readable CLI for task capture/updates/listing.
- `skill_runner.py` – JSON wrapper with stable output for agent orchestration.
- `skill_spec.json` – machine-readable command and I/O manifest.
- `SKILL.md` – usage guidance for agent skills.

## Quick start (preferred from repo root)

```bash
assistant tasks init --db ~/.local/share/assistant/tasks.db
assistant tasks invoke --db ~/.local/share/assistant/tasks.db --pretty --message "Draft scope for feature_x"
assistant tasks invoke --db ~/.local/share/assistant/tasks.db --pretty --message "What tasks do I have?"
assistant tasks invoke --db ~/.local/share/assistant/tasks.db --pretty --message "Postpone this until after q3 ends"
assistant tasks list --db ~/.local/share/assistant/tasks.db --pretty
```

## Supported features

- Create task (default fallback intent)
- List actionable tasks
- Snooze/postpone task
- Mark task done
- Soft-delete / undelete task
- Edit task title + details (TUI: press `e` on a focused task; opens `$EDITOR`)
