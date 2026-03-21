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
- Inspect full problem history.

## Command interface (preferred)

```bash
assistant problems <command> [args]
```

Underlying runner: `problems/skill_runner.py`.

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
