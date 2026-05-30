---
name: problems
description: Manage nested problem statements, problem trees, and solve state tracking
---

# Problems Skill

Use this skill to frame work as nested problems.

## When to use

- Capture a project as a problem statement.
- Add nested sub-problems.
- List open problems or view a full problem tree.
- Mark a problem solved.
- Edit a problem's statement (`edit problem: <new statement>`), or press `e` on the focused problem in the TUI to open title + statement in `$EDITOR`. The CLI form re-derives the title from the new statement.
- Inspect full problem history.

## Command interface (preferred)

```bash
assistant problems <command> [args]
```

Underlying runner: the Go CLI in `internal/cli/problems.go`.

## Commands

```bash
assistant problems init --db ~/.local/share/assistant/problems.db
assistant problems invoke --db ~/.local/share/assistant/problems.db --pretty --message "Problem: PR review scope is too large"
assistant problems invoke --db ~/.local/share/assistant/problems.db --pretty --parent-problem-id <problem_id> --message "Problem: no per-feature toggles"
assistant problems list --db ~/.local/share/assistant/problems.db --pretty
assistant problems tree --db ~/.local/share/assistant/problems.db --pretty
assistant problems show --db ~/.local/share/assistant/problems.db --problem-id <problem_id> --pretty
assistant problems link --db ~/.local/share/assistant/problems.db --problem-id <problem_id> --entity-type task --entity-id <task_id> --relation addresses --pretty
assistant problems history --db ~/.local/share/assistant/problems.db --problem-id <problem_id> --pretty
```

## Operational guidance

- Prefer problem statements that are falsifiable and specific.
- Use nesting to split broad problems into testable sub-problems.
- Treat JSON `ok=false` as an error requiring clarification.
