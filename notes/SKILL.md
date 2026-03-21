---
name: notes
description: Manage notes with follow-up tracking, snoozing, and event history
---

# Notes Skill

Use this skill to manage user notes and follow-up tracking.

## When to use

- Capture a note from user text.
- Ask for follow-up-worthy notes.
- Snooze/defer a note until a date/time phrase (e.g., "after q3 ends").
- Mark a note as done.
- Inspect full note history/audit trail.

## Command interface (preferred)

Use the unified router from repo root:

```bash
assistant notes <command> [args]
```

Underlying runner: `notes/skill_runner.py` (JSON output contract).

## Commands

### 1) Initialize DB

```bash
assistant notes init --db notes/notes.db
```

### 2) Invoke from natural language

```bash
assistant notes invoke --db notes/notes.db --pretty --message "I wonder if Jeremy could show me which sources I need to update for feature_x that I proposed"
```

### 3) List follow-up-worthy notes

```bash
assistant notes list --db notes/notes.db --pretty
```

### 4) Show history for one note

```bash
assistant notes history --db notes/notes.db --note-id <note_id> --pretty
```

## Operational guidance

- Prefer passing `--note-id` for mutation intents when available.
- For ambiguous time phrases, follow up with user confirmation.
- Treat JSON `ok=false` as a hard error and ask for clarification.
- `human_message` can be surfaced directly to users.

## Data files

- Notes DB path is caller-provided (`--db`).
- SQLite WAL side files (`.db-wal`, `.db-shm`) are expected behavior.
