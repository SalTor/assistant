# Problems skill prototype

This folder contains a runnable problems-agent prototype with SQLite persistence.

## Quick start

From repo root:

```bash
assistant problems init --db ~/.local/share/assistant/problems.db
assistant problems invoke --db ~/.local/share/assistant/problems.db --pretty --message "Problem: PRs are too hard to review"
assistant problems list --db ~/.local/share/assistant/problems.db --pretty
assistant problems tree --db ~/.local/share/assistant/problems.db --pretty
assistant problems show --db ~/.local/share/assistant/problems.db --pretty --problem-id <problem_id>
assistant problems link --db ~/.local/share/assistant/problems.db --pretty --problem-id <problem_id> --entity-type task --entity-id <task_id> --relation addresses
```

## Supported intents

- Create problem (default)
- List open problems
- Show problem tree
- Mark problem solved
- Soft-delete / undelete problem
- Edit problem title + statement (TUI: press `e` on a focused problem; opens `$EDITOR`)

## Notes

- Nested problems are supported via `--parent-problem-id` on `invoke`.
- `tree` returns all problems with a `depth` field for rendering hierarchy.
- `show` includes linked entities.
- `link` associates notes/tasks/problems to a problem with a relation (`addresses`, `evidence`, `critique`, etc.).
