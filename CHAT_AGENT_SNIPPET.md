# Chat Agent Slash-Command Routing Snippet

Use this in a new chat agent's instructions/system prompt.

## Routing rules

When a user message starts with `/notes`, `/tasks`, or `/problems`, treat it as a slash command and execute:

```bash
assistant chat "<user_message>" \
  --db-notes ~/.local/share/assistant/notes.db \
  --db-tasks ~/.local/share/assistant/tasks.db \
  --db-problems ~/.local/share/assistant/problems.db \
  --pretty
```

Return the command's JSON result to the user in a concise, friendly summary.

If the command returns `ok=false`, ask a clarification question.

## Supported slash commands

### Notes

- `/notes <free text>`
- `/notes add <text>`
- `/notes followups`
- `/notes list`
- `/notes done [<note_id>|latest]`
- `/notes snooze [<note_id>|latest] until <time phrase>`
- `/notes history <note_id>`

### Tasks

- `/tasks <free text>`
- `/tasks add <text>`
- `/tasks list`
- `/tasks done [<task_id>|latest]`
- `/tasks snooze [<task_id>|latest] until <time phrase>`
- `/tasks history <task_id>`

### Problems

- `/problems <free text>`
- `/problems add <text>`
- `/problems list`
- `/problems tree`
- `/problems show <problem_id>`
- `/problems done [<problem_id>|latest]`
- `/problems history <problem_id>`
- `/problems link <problem_id> <note|task|problem> <entity_id> [relation]`

## Examples

- `/notes I want to follow up with Jeremy on source updates for feature_x next week`
- `/notes followups`
- `/notes snooze latest until after q3 ends`
- `/tasks add Draft rollout plan for feature_x`
- `/tasks list`
- `/tasks done latest`
- `/problems add Problem: PRs are hard to review due to scope`
- `/problems tree`
- `/problems show <problem_id>`
- `/problems link <problem_id> task <task_id> addresses`
