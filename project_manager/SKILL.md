---
name: pm
description: Review JJ stack work, bind to problem trailers, and log progress updates
---

# Project Manager Skill

Use this skill to connect current stack work to an existing problem (or create a suggested problem) using machine-readable commit trailers.

## Trailer convention

```text
PM-Problem: <problem_id_or_unique_prefix>
PM-Relation: addresses
PM-Progress: short progress note
```

## Commands

```bash
assistant project_manager review --pretty
assistant project_manager review --pretty --apply
assistant project_manager review --pretty --create-problem
assistant project_manager trailer --problem-id <problem_id> --relation addresses --progress "implemented feature x"
```

## Operational guidance

- Prefer a single problem binding in the active stack.
- If multiple bound problems exist, resolve ambiguity before applying progress updates.
- Keep `PM-Progress` concise and specific.
