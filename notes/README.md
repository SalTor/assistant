# Notes skill prototype

This folder contains a runnable notes-agent prototype with SQLite persistence.

## Files

- `notes_skill.py` – human-readable CLI that initializes DB, ingests natural language note/follow-up updates, and lists follow-up-worthy notes.
- `skill_runner.py` – JSON wrapper with stable output for agent/skill orchestration.
- `skill_spec.json` – machine-readable command and I/O manifest for tool/agent discovery.

## Quick start

Preferred (from repo root):

```bash
assistant notes init --db ~/.local/share/assistant/notes.db
```

Direct script usage:

```bash
cd notes
python3 notes_skill.py --db notes.db init
```

Create a note:

```bash
python3 notes_skill.py --db notes.db run --message "I wonder if Jeremy could show me which sources I need to update for feature_x that I proposed"
```

Ask follow-ups:

```bash
python3 notes_skill.py --db notes.db run --message "Is there anything I can follow up on?"
```

Snooze with fuzzy time:

```bash
python3 notes_skill.py --db notes.db run --message "I've decided to postpone talking about this with Jeremy until after q3 ends"
```

List follow-ups directly:

```bash
python3 notes_skill.py --db notes.db list
```

View full audit trail for a note:

```bash
python3 notes_skill.py --db notes.db history --note-id <note_id>
```

## Supported intents

- Create note (default fallback)
- List follow-ups
- Snooze/postpone note
- Mark note done
- Edit note (`edit note: ...`)

## Notes on targeting

If `--note-id` is not provided for snooze/done/edit, the script picks the most recently updated actionable note.

## Time parsing examples

- `after q3 ends`
- `tomorrow`
- `next week`
- `next month`
- `in 3 days`
- `in 2 weeks`
- `2026-10-01`

If parsing is unclear, it falls back to 7 days from now.

## JSON skill wrapper (`skill_runner.py`)

Designed for agent skills/tools that need machine-readable output.

Initialize DB:

```bash
python3 skill_runner.py --db notes.db init
```

Invoke on a message:

```bash
python3 skill_runner.py --db notes.db --pretty invoke --message "Is there anything I can follow up on?"
```

Snooze/update a specific note:

```bash
python3 skill_runner.py --db notes.db --pretty invoke --note-id <note_id> --message "postpone this until after q3 ends"
```

List due/open follow-ups:

```bash
python3 skill_runner.py --db notes.db --pretty list
```

Show note + event history:

```bash
python3 skill_runner.py --db notes.db --pretty history --note-id <note_id>
```

### Output contract (summary)

- `ok` (bool)
- `action` (string)
- `intent` (for `invoke`)
- `human_message` (friendly summary)
- `note` (single note object, when applicable)
- `data` (list results, for list/follow-up queries)
- `error` + `message` on failure
