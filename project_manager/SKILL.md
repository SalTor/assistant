---
name: pm
description: Review JJ stack work, bind to problem trailers, and log progress updates
---

# Project Manager Skill

Use this skill to connect current stack work to an existing problem (or create a suggested problem) using machine-readable commit trailers.

## When to use

- I want to know which problem the current jj stack is bound to.
- I want to log progress against a bound problem after finishing some work.
- I have no problem yet for what I'm building and want one created from the stack subject + diff.
- I need a copy-pasteable trailer block to drop into a commit description.

## Trailer convention

```text
PM-Problem: <problem_id_or_unique_prefix>
PM-Relation: addresses
PM-Progress: short progress note
```

`PM-Problem` accepts either a full UUID or any unique non-deleted prefix; `--problem-id <prefix>` is also resolved by prefix in the Go CLI.

## Commands

```bash
assistant project_manager review --pretty
assistant project_manager review --pretty --apply
assistant project_manager review --pretty --create-problem
assistant project_manager trailer --problem-id <problem_id> --relation addresses --progress "implemented feature x"
```

Underlying runner: the Go CLI in `internal/cli/projectmanager.go`.

## Operational guidance

- Prefer a single problem binding in the active stack.
- If multiple bound problems exist, resolve ambiguity before applying progress updates.
- Keep `PM-Progress` concise and specific.
