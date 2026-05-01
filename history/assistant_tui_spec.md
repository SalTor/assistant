# Assistant — CLI Tool Spec

A specification for a personal-productivity CLI that manages **notes**, **tasks**,
and **problems**, with cross-domain linking, soft-delete + undelete, and a
natural-language `invoke` verb that classifies intent (create / complete / snooze /
edit / list).

This spec captures *what the tool does* — not the language, framework, datastore
shape, or front-end form. An implementer may build it in any language with any
storage backend, and may layer any UI (interactive shell, TUI, web, etc.) on top of
the CLI surface defined here.

---

## 1. Domains

The tool manages three domains. Each domain has its own persistent store and its
own subcommand namespace under the top-level CLI.

### 1.1 Notes

A note is a free-form snippet captured for later follow-up. Lifecycle states:
`open`, `snoozed` (with a `followup_after` timestamp), `done`, `deleted`.

Persisted fields (minimum):

| field            | type     | notes                                             |
|------------------|----------|---------------------------------------------------|
| `id`             | string   | stable opaque id (e.g. UUID).                     |
| `body`           | string   | the note text.                                    |
| `status`         | string   | `open` / `snoozed` / `done` / `deleted`.          |
| `followup_state` | string?  | additional state when snoozed.                    |
| `followup_after` | iso ts?  | when the note becomes actionable again.           |
| `priority`       | int?     | optional; reserved for future use.                |
| `created_at`     | iso ts   |                                                   |
| `updated_at`     | iso ts   |                                                   |

Each note carries an event log: every mutation appends an event row with
`event_type`, `event_text`, optional structured payload, and `created_at`.

### 1.2 Tasks

A task is a thing to be done, with optional details and a due time. Same lifecycle
states as notes, plus an event log.

Persisted fields:

| field         | type    | notes                                |
|---------------|---------|--------------------------------------|
| `id`          | string  |                                      |
| `title`       | string  | short summary.                       |
| `details`     | string? | longer description.                  |
| `status`      | string  | `open` / `snoozed` / `done` / `deleted`. |
| `priority`    | int?    |                                      |
| `due_at`      | iso ts? |                                      |
| `created_at`  | iso ts  |                                      |
| `updated_at`  | iso ts  |                                      |

### 1.3 Problems

A problem is a hierarchical work item: it has a title, a longer statement, and may
have a parent problem. Problems can be linked from notes, tasks, or other problems
with a typed relation.

Persisted fields:

| field         | type    | notes                                |
|---------------|---------|--------------------------------------|
| `id`          | string  |                                      |
| `title`       | string  | short summary.                       |
| `statement`   | string  | full problem description.            |
| `status`      | string  | `open` / `solved` / `deleted`.       |
| `parent_id`   | string? | another problem's id, or null.       |
| `created_at`  | iso ts  |                                      |
| `updated_at`  | iso ts  |                                      |

A problem also carries a list of **links**: zero or more `(entity_type, entity_id,
relation)` triples where `entity_type ∈ {note, task, problem}` and `relation` is
one of `addresses`, `evidence`, `critique`, `depends_on` (custom relations are
permitted but not first-class). Each problem has an event log like notes/tasks.

---

## 2. CLI surface

Top-level invocation:

```
assistant <domain> <verb> [flags...]
assistant chat "<slash-command>"
assistant domains
assistant migrate-dbs [--copy] [--dry-run]
```

Where `<domain> ∈ {notes, tasks, problems}` (the tool may add more later).

### 2.1 Common flags (all domain verbs)

| flag        | default                | meaning                                        |
|-------------|------------------------|------------------------------------------------|
| `--db`      | `<data_dir>/<dom>.db`  | path to that domain's persistent store.        |
| `--tz`      | system / `$TZ`         | IANA timezone for time math + timestamps.      |
| `--pretty`  | off                    | pretty-print JSON output.                      |

`<data_dir>` is `$XDG_DATA_HOME/assistant` if set, else `~/.local/share/assistant`.
The directory and parent of each `--db` are created on demand.

### 2.2 Output contract

Every verb prints exactly one JSON object to stdout and exits with a status code.
The envelope:

```jsonc
{
  "ok": true | false,
  "action": "<verb-or-intent-name>",
  "human_message": "<one-line summary suitable for a status bar>",

  // domain-specific payload, e.g.:
  "note":    { ... },         // for note verbs that target one note
  "task":    { ... },
  "problem": { ... },
  "events":  [ ... ],
  "links":   [ ... ],
  "data":    [ ... ],         // for list/tree verbs
  "count":   12,

  // when ok=false:
  "error": "<machine-readable code>"
}
```

Exit code is `0` on `ok: true`, non-zero otherwise. Stderr is reserved for
unexpected failures (uncaught exceptions still produce a JSON envelope with
`error: "exception"`).

### 2.3 Verbs (per domain)

The same shape applies to all three domains, with `<id-flag>` being `--note-id`,
`--task-id`, or `--problem-id` respectively.

| verb        | flags                                    | purpose                                                    |
|-------------|------------------------------------------|------------------------------------------------------------|
| `init`      | (common)                                 | create/upgrade the store. Idempotent.                      |
| `invoke`    | `--message <text>` `[<id-flag> <id>]`    | natural-language entry point — see §3.                     |
| `list`      | (common)                                 | notes/tasks: list follow-up-worthy items. problems: list open problems. |
| `tree`      | (common)                                 | **problems only** — full hierarchy as a flat list with `depth`. |
| `show`      | `<id-flag> <id>`                         | **problems only** — return problem + links.                |
| `history`   | `<id-flag> <id>`                         | return the item plus its event log.                        |
| `delete`    | `<id-flag> <id>`                         | soft-delete: set `status = "deleted"`, record an event.    |
| `undelete`  | `<id-flag> <id>`                         | restore from deleted to its prior status.                  |
| `link`      | `--problem-id <p>` `--entity-type <t>` `--entity-id <e>` `--relation <r>` | **problems only** — add a link. `<r>` defaults to `addresses`. |
| `unlink`    | same as `link`, `--relation` optional    | **problems only** — remove matching link(s).               |

`list` for notes/tasks returns items where `status` is "actionable" — `open`, plus
`snoozed` whose `followup_after <= now`.

`tree` returns rows with at minimum `{id, title, status, parent_id, depth}`, sorted
so parents precede children.

`history` returns `{<domain>: {...}, events: [...]}`. `events` is sorted ascending
by `created_at`.

### 2.4 Cross-cutting commands

- `assistant domains` — print the list of registered domains.
- `assistant migrate-dbs [--copy] [--dry-run]` — one-time helper to relocate
  per-domain DBs from older in-repo locations into `<data_dir>`. Default is
  `move`; `--copy` preserves originals; `--dry-run` prints planned actions only.
- `assistant chat "<text>"` — see §6.

---

## 3. Natural-language `invoke`

`invoke` accepts a free-text message and classifies the user's intent against a
small grammar. The classifier may be regex-based or LLM-based; the spec only
constrains the *intents* and *outputs*.

### 3.1 Intents

| intent            | trigger phrases (examples)                                                 | applies to         |
|-------------------|-----------------------------------------------------------------------------|--------------------|
| `list_followups`  | "anything to follow up on", "what's follow-up worthy"                       | notes              |
| `complete_note`   | "done", "completed", "no need to follow up", "resolved"                     | notes              |
| `snooze_note`     | "postpone", "snooze", "defer", "push back", "until …", "after …", "later"   | notes              |
| `edit_note`       | message starting with `edit note:`                                          | notes              |
| `create_note`     | (default fallback)                                                          | notes              |
| `complete_task`   | analogous to notes                                                          | tasks              |
| `snooze_task`     | analogous to notes                                                          | tasks              |
| `create_task`     | (default fallback)                                                          | tasks              |
| `solve_problem`   | "solved", "solution is …"                                                   | problems           |
| `create_problem`  | (default fallback)                                                          | problems           |
| `tree_problems`   | (chat verb) — `/problems tree`                                              | problems           |

### 3.2 Target resolution

When an `<id-flag>` is supplied, the intent applies to that record. When omitted
and the intent is something other than `create_*` / `list_followups` /
`tree_problems`, the tool selects the **latest actionable item** (most recently
updated open or due-now snoozed item) and applies the operation to it. If no such
item exists, return `ok: false, error: "no_target_<domain>"`.

### 3.3 Time-phrase resolution

Snooze / due-time phrases are resolved to an absolute timestamp. Required forms:

| phrase                                | resolution                                                |
|---------------------------------------|-----------------------------------------------------------|
| `YYYY-MM-DD`                          | that date at 09:00 in the configured timezone.            |
| `after q<N> [ends] [<year>]`          | first instant after the named quarter; if past, next year.|
| `tomorrow`                            | next day at 09:00.                                        |
| `next week`                           | next Monday at 09:00.                                     |
| `next month`                          | first day of next month at 09:00.                         |
| `in <n> day(s)`                       | now + n days, at 09:00 of that date.                      |
| `in <n> week(s)`                      | now + n weeks, at 09:00 of that date.                     |
| (anything else)                       | fallback: now + 7 days at 09:00.                          |

The "extracted phrase" is whatever follows `until|after|on|in`; if none of those
prepositions appear, the whole message is the phrase.

### 3.4 `invoke` envelope additions

In addition to the standard envelope, `invoke` includes:

```jsonc
{
  "intent": "<intent-name>",
  "confidence": 0.7,
  "input": { "message": "...", "<x>_id": "...", "timezone": "..." },
  "resolved_time": "2026-07-01T00:00:00-07:00"   // when intent is snooze_*
}
```

---

## 4. Soft-delete & undelete

`delete` never removes data — it transitions `status` to `deleted` and records an
event with source label (e.g. `cli_delete`). `undelete` restores the previous
status (or `open` if unknown) and records a `cli_undelete` event.

`list` and `tree` exclude items whose `status == "deleted"`.

`history` returns deleted items as well, so consumers can recover ids.

---

## 5. Linking (problems)

A problem can be linked to any number of notes, tasks, and other problems. Each
link has a `relation` string. The first-class relations in display and traversal
are, in order:

1. `addresses`
2. `evidence`
3. `critique`
4. `depends_on`

Other relation strings are permitted but appear after the first-class ones in any
ordered display.

`link` is idempotent on the triple `(problem_id, entity_type, entity_id, relation)`.
`unlink` removes all links matching the supplied `(problem_id, entity_type,
entity_id)`; if `--relation` is supplied, only that relation is removed.

`problems show` returns `{problem, links}` where each link is
`{entity_type, entity_id, relation}`.

---

## 6. Slash-command form (`chat`)

`assistant chat "<text>"` accepts a slash-prefixed string and dispatches to the
appropriate domain verb. This is sugar for non-interactive callers (chat agents,
keybinds in editors, etc.).

Grammar:

```
/notes      <free-text>                             → notes invoke --message <free-text>
/notes      add <text>                              → notes invoke --message <text>
/notes      followups | list                        → notes list
/notes      done [<id>|latest]                      → notes invoke --message done [--note-id <id>]
/notes      snooze [<id>|latest] until <phrase>     → notes invoke --message "postpone <phrase>" [--note-id <id>]
/notes      history <id>                            → notes history --note-id <id>

/tasks      <free-text>                             → tasks invoke --message <free-text>
/tasks      add <text>                              → tasks invoke --message <text>
/tasks      list                                    → tasks list
/tasks      done [<id>|latest]                      → tasks invoke --message done [--task-id <id>]
/tasks      snooze [<id>|latest] until <phrase>     → tasks invoke --message "postpone <phrase>" [--task-id <id>]
/tasks      history <id>                            → tasks history --task-id <id>

/problems   <free-text>                             → problems invoke --message <free-text>
/problems   add <text>                              → problems invoke --message <text>
/problems   list | tree                             → problems list / tree
/problems   show <id>                               → problems show --problem-id <id>
/problems   done [<id>|latest]                      → problems invoke --message solved [--problem-id <id>]
/problems   history <id>                            → problems history --problem-id <id>
/problems   link   <p> <note|task|problem> <e> [rel] → problems link ...
/problems   unlink <p> <note|task|problem> <e> [rel] → problems unlink ...

/help | help | /chat help                           → print this grammar
```

`/notes`, `/tasks`, `/problems` with no body return an error. Unknown slash
domains print the help text.

`chat` accepts `--db-notes`, `--db-tasks`, `--db-problems`, `--tz`, `--pretty`
flags as overrides; otherwise the per-domain defaults apply.

---

## 7. Operation log (optional, recommended)

Implementations targeting interactive use should maintain a per-process **operation
log** of mutating actions, suitable for an "undo deletes" UX. The log contains, per
entry:

- timestamp
- action (`create`, `delete`, `undelete`, `link`, `unlink`)
- domain (`note`, `task`, `problem`)
- target id
- short detail string

The log is append-only, persists to disk under `<data_dir>` (e.g.
`assistant_operations.log`), and is read newest-first. The store format is an
implementation detail; line-delimited tab-separated values are sufficient.

This log is *not* the per-item event log (which lives inside each domain's store
and is returned by `history`). The operation log is a UI affordance; the event log
is part of the data model.

---

## 8. Storage

- Each domain has a separate persistent store under `<data_dir>`.
- The reference implementation uses SQLite (`notes.db`, `tasks.db`, `problems.db`),
  but no consumer of the CLI needs to know that. An implementation may use any
  store that supports the required operations atomically.
- Schemas must support the per-item event log and (for problems) the link table.
- `init` is required to be idempotent and safe to re-run on every invocation; the
  reference implementation calls it implicitly inside every command.

---

## 9. Errors

Standard error codes used in `error`:

| code                  | meaning                                                  |
|-----------------------|----------------------------------------------------------|
| `note_not_found`      | id given does not resolve.                               |
| `task_not_found`      | "                                                        |
| `problem_not_found`   | "                                                        |
| `no_target_note`      | no id and no latest-actionable fallback.                 |
| `no_target_task`      | "                                                        |
| `no_target_problem`   | "                                                        |
| `empty_body`          | `create_*` intent with empty text.                       |
| `empty_statement`     | problem creation with empty text.                        |
| `empty_edit_body`     | `edit_note` intent with no new body.                     |
| `unknown_intent`      | classifier could not resolve.                            |
| `unsupported_command` | router fell through.                                     |
| `exception`           | uncaught failure; `message` carries the human string.    |

`human_message` is always populated; consumers should prefer it for display.

---

## 10. Acceptance test (manual or scripted)

A correct implementation must satisfy the following sequence end-to-end:

1. `assistant notes init` returns `ok: true`.
2. `assistant notes invoke --message "buy milk"` creates a note and returns it
   with `intent: "create_note"`, `note.status: "open"`.
3. `assistant notes list` includes that note.
4. `assistant notes invoke --message "postpone until next week"` (no id) snoozes
   the latest note; `resolved_time` is next Monday 09:00 in the active tz.
5. `assistant tasks invoke --message "ship feature x"` creates a task.
6. `assistant problems invoke --message "users are confused"` creates a problem.
7. `assistant problems link --problem-id <p> --entity-type note --entity-id <n>
   --relation addresses` succeeds; `problems show --problem-id <p>` returns the
   link under `links`.
8. `assistant problems unlink ...` removes it.
9. `assistant notes delete --note-id <n>` sets `status: "deleted"`; subsequent
   `list` excludes it.
10. `assistant notes undelete --note-id <n>` restores it.
11. `assistant chat "/problems tree"` returns the same data as
    `assistant problems tree`.
12. All envelopes parse as JSON; `ok: false` paths produce a non-zero exit code.

---

## 11. What this spec deliberately does *not* fix

- Choice of implementation language or runtime.
- Storage backend (SQLite, files, embedded KV, server, etc.).
- Whether `invoke`'s intent classifier is regex, embedding-based, or LLM-driven —
  only the intent vocabulary and outputs are specified.
- ID format (UUIDv4, ULID, slug — all fine, as long as ids are stable strings).
- Whether the tool ships a TUI / REPL / web UI on top. Any front-end is allowed
  provided it consumes the JSON envelope above.
- Concurrency model. The reference implementation is single-process; nothing in
  the spec precludes a multi-writer backend.
