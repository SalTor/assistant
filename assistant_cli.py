#!/usr/bin/env python3
"""Unified CLI router for domain skills (notes, tasks, etc.).

Domains:
- notes -> notes/skill_runner.py
- tasks -> tasks/skill_runner.py

Examples:
  assistant domains
  assistant notes init --db notes/notes.db
  assistant tasks invoke --db tasks/tasks.db --message "Draft scope for feature_x" --pretty
  assistant chat "/notes I want to follow up with Jeremy on source updates next week"
"""

from __future__ import annotations

import argparse
import shlex
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent
DOMAIN_ENTRYPOINTS = {
  "notes": ROOT / "notes" / "skill_runner.py",
  "tasks": ROOT / "tasks" / "skill_runner.py",
}


def run_domain(domain: str, args: list[str]) -> int:
  entrypoint = DOMAIN_ENTRYPOINTS.get(domain)
  if entrypoint is None:
    print(f"Unknown domain: {domain}", file=sys.stderr)
    print(f"Available domains: {', '.join(sorted(DOMAIN_ENTRYPOINTS))}", file=sys.stderr)
    return 2

  if not entrypoint.exists():
    print(f"Entrypoint not found for domain '{domain}': {entrypoint}", file=sys.stderr)
    return 2

  cmd = [sys.executable, str(entrypoint), *args]
  proc = subprocess.run(cmd)
  return proc.returncode


def _chat_help() -> int:
  msg = """Slash command format:
  /notes <free text>
  /notes add <text>
  /notes followups
  /notes list
  /notes done [<note_id>|latest]
  /notes snooze [<note_id>|latest] until <time phrase>
  /notes history <note_id>

  /tasks <free text>
  /tasks add <text>
  /tasks list
  /tasks done [<task_id>|latest]
  /tasks snooze [<task_id>|latest] until <time phrase>
  /tasks history <task_id>
"""
  print(msg)
  return 0


def _build_notes_chat_args(tail: str, db: str | None, tz: str | None, pretty: bool) -> list[str]:
  tokens = shlex.split(tail)
  args: list[str] = []
  if db:
    args.extend(["--db", db])
  if tz:
    args.extend(["--tz", tz])
  if pretty:
    args.append("--pretty")

  if not tokens:
    raise ValueError("Missing notes command text.")

  verb = tokens[0].lower()

  if verb in {"followups", "list"}:
    return args + ["list"]

  if verb == "history":
    if len(tokens) < 2:
      raise ValueError("/notes history requires <note_id>")
    return args + ["history", "--note-id", tokens[1]]

  if verb == "done":
    note_id = None
    if len(tokens) >= 2 and tokens[1].lower() != "latest":
      note_id = tokens[1]
    invoke = args + ["invoke", "--message", "done"]
    if note_id:
      invoke.extend(["--note-id", note_id])
    return invoke

  if verb == "snooze":
    idx = 1
    note_id = None
    if len(tokens) > 1 and tokens[1].lower() != "latest" and tokens[1].lower() not in {"until", "after", "in", "on", "tomorrow", "next"}:
      note_id = tokens[1]
      idx = 2
    elif len(tokens) > 1 and tokens[1].lower() == "latest":
      idx = 2

    phrase = " ".join(tokens[idx:]).strip()
    if not phrase:
      raise ValueError("/notes snooze requires a time phrase, e.g. 'until after q3 ends'")

    invoke = args + ["invoke", "--message", f"postpone {phrase}"]
    if note_id:
      invoke.extend(["--note-id", note_id])
    return invoke

  if verb == "add":
    body = " ".join(tokens[1:]).strip()
    if not body:
      raise ValueError("/notes add requires note text")
    return args + ["invoke", "--message", body]

  # free text fallback
  return args + ["invoke", "--message", tail]


def _build_tasks_chat_args(tail: str, db: str | None, tz: str | None, pretty: bool) -> list[str]:
  tokens = shlex.split(tail)
  args: list[str] = []
  if db:
    args.extend(["--db", db])
  if tz:
    args.extend(["--tz", tz])
  if pretty:
    args.append("--pretty")

  if not tokens:
    raise ValueError("Missing tasks command text.")

  verb = tokens[0].lower()

  if verb == "list":
    return args + ["list"]

  if verb == "history":
    if len(tokens) < 2:
      raise ValueError("/tasks history requires <task_id>")
    return args + ["history", "--task-id", tokens[1]]

  if verb == "done":
    task_id = None
    if len(tokens) >= 2 and tokens[1].lower() != "latest":
      task_id = tokens[1]
    invoke = args + ["invoke", "--message", "done"]
    if task_id:
      invoke.extend(["--task-id", task_id])
    return invoke

  if verb == "snooze":
    idx = 1
    task_id = None
    if len(tokens) > 1 and tokens[1].lower() != "latest" and tokens[1].lower() not in {"until", "after", "in", "on", "tomorrow", "next"}:
      task_id = tokens[1]
      idx = 2
    elif len(tokens) > 1 and tokens[1].lower() == "latest":
      idx = 2

    phrase = " ".join(tokens[idx:]).strip()
    if not phrase:
      raise ValueError("/tasks snooze requires a time phrase, e.g. 'until after q3 ends'")

    invoke = args + ["invoke", "--message", f"postpone {phrase}"]
    if task_id:
      invoke.extend(["--task-id", task_id])
    return invoke

  if verb == "add":
    body = " ".join(tokens[1:]).strip()
    if not body:
      raise ValueError("/tasks add requires task text")
    return args + ["invoke", "--message", body]

  # free text fallback
  return args + ["invoke", "--message", tail]


def run_chat_command(text: str, db_notes: str | None, db_tasks: str | None, tz: str | None, pretty: bool) -> int:
  raw = text.strip()
  if raw in {"/help", "help", "/chat help"}:
    return _chat_help()

  if not raw.startswith("/"):
    print("Chat command must start with '/'. Example: /notes followups", file=sys.stderr)
    return 2

  parts = raw[1:].split(None, 1)
  if not parts:
    return _chat_help()

  domain = parts[0].lower()
  tail = parts[1] if len(parts) > 1 else ""

  try:
    if domain == "notes":
      if not tail:
        raise ValueError("/notes requires text or a subcommand")
      args = _build_notes_chat_args(tail, db_notes, tz, pretty)
      return run_domain("notes", args)

    if domain == "tasks":
      if not tail:
        raise ValueError("/tasks requires text or a subcommand")
      args = _build_tasks_chat_args(tail, db_tasks, tz, pretty)
      return run_domain("tasks", args)

    if domain in {"chat", "commands"}:
      return _chat_help()

    print(f"Unknown slash domain: /{domain}", file=sys.stderr)
    return _chat_help()
  except ValueError as e:
    print(str(e), file=sys.stderr)
    return 2


def main(argv: list[str]) -> int:
  parser = argparse.ArgumentParser(description="Unified skill CLI router")
  sub = parser.add_subparsers(dest="cmd", required=True)

  sub.add_parser("domains", help="List registered domains")

  route = sub.add_parser("run", help="Run a domain command via router")
  route.add_argument("domain", choices=sorted(DOMAIN_ENTRYPOINTS.keys()))
  route.add_argument("args", nargs=argparse.REMAINDER, help="Arguments passed to domain skill runner")

  for domain in sorted(DOMAIN_ENTRYPOINTS.keys()):
    p = sub.add_parser(domain, help=f"Shortcut for: run {domain} ...")
    p.add_argument("args", nargs=argparse.REMAINDER, help="Arguments passed to domain skill runner")

  chat = sub.add_parser("chat", help="Run a slash-style command (e.g. '/notes followups')")
  chat.add_argument("text", help="Slash command text")
  chat.add_argument("--db-notes", default=None, help="Default notes DB path for /notes")
  chat.add_argument("--db-tasks", default=None, help="Default tasks DB path for /tasks")
  chat.add_argument("--tz", default=None, help="IANA timezone")
  chat.add_argument("--pretty", action="store_true", help="Pretty JSON output from domain skill")

  args = parser.parse_args(argv)

  if args.cmd == "domains":
    for name, path in sorted(DOMAIN_ENTRYPOINTS.items()):
      print(f"{name}\t{path.relative_to(ROOT)}")
    return 0

  if args.cmd == "chat":
    return run_chat_command(args.text, args.db_notes, args.db_tasks, args.tz, args.pretty)

  if args.cmd == "run":
    passthrough = args.args
    if passthrough and passthrough[0] == "--":
      passthrough = passthrough[1:]
    return run_domain(args.domain, passthrough)

  passthrough = args.args
  if passthrough and passthrough[0] == "--":
    passthrough = passthrough[1:]
  return run_domain(args.cmd, passthrough)


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
