#!/usr/bin/env python3
"""JSON wrapper for tasks/tasks_skill.py."""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

import tasks_skill as core


def row_to_task(row: sqlite3.Row | None) -> dict[str, Any] | None:
  if row is None:
    return None
  data = dict(row)
  return {
    "id": data.get("id"),
    "title": data.get("title"),
    "details": data.get("details"),
    "status": data.get("status"),
    "due_at": data.get("due_at"),
    "priority": data.get("priority"),
    "created_at": data.get("created_at"),
    "updated_at": data.get("updated_at"),
  }


def fetch_task(conn: sqlite3.Connection, task_id: str) -> dict[str, Any] | None:
  return row_to_task(core.get_task(conn, task_id))


def fetch_events(conn: sqlite3.Connection, task_id: str) -> list[dict[str, Any]]:
  rows = conn.execute(
    """
    SELECT id, event_type, event_text, payload_json, created_at
    FROM task_events
    WHERE task_id = ?
    ORDER BY created_at ASC
    """,
    (task_id,),
  ).fetchall()
  out: list[dict[str, Any]] = []
  for r in rows:
    payload_raw = r["payload_json"]
    try:
      payload = json.loads(payload_raw) if payload_raw else None
    except json.JSONDecodeError:
      payload = {"raw": payload_raw}
    out.append(
      {
        "id": r["id"],
        "event_type": r["event_type"],
        "event_text": r["event_text"],
        "payload": payload,
        "created_at": r["created_at"],
      }
    )
  return out


def resolve_target_task_id(conn: sqlite3.Connection, task_id: str | None) -> str | None:
  if task_id:
    return task_id
  latest = core.latest_open_task(conn)
  if latest is None:
    return None
  return latest["id"]


def invoke_message(conn: sqlite3.Connection, message: str, task_id: str | None, tz) -> dict[str, Any]:
  parsed = core.parse_intent(message, task_id)
  now = datetime.now(tz)

  result: dict[str, Any] = {
    "ok": True,
    "intent": parsed.intent,
    "confidence": parsed.confidence,
    "input": {"message": message, "task_id": task_id, "timezone": str(tz)},
  }

  if parsed.intent == "list_tasks":
    rows = core.list_actionable(conn, tz)
    result["action"] = "list_tasks"
    result["count"] = len(rows)
    result["data"] = [row_to_task(r) for r in rows]
    result["human_message"] = "No actionable tasks." if not rows else f"Found {len(rows)} actionable task(s)."
    return result

  if parsed.intent == "create_task":
    title = parsed.title or message
    if not title.strip():
      return {"ok": False, "error": "empty_title", "human_message": "Task title is empty.", **result}
    new_id = core.create_task(conn, title, tz)
    result["action"] = "create_task"
    result["task"] = fetch_task(conn, new_id)
    result["human_message"] = f"Created task {new_id}."
    return result

  target_id = resolve_target_task_id(conn, parsed.task_id)
  if not target_id:
    return {"ok": False, "error": "no_target_task", "human_message": "No target task found.", **result}

  if parsed.intent == "complete_task":
    core.complete_task(conn, target_id, message, tz)
    result["action"] = "complete_task"
    result["task"] = fetch_task(conn, target_id)
    result["human_message"] = f"Marked task {target_id} as done."
    return result

  if parsed.intent == "snooze_task":
    when_text = parsed.when_text or message
    due = core.resolve_time_phrase(when_text, now, tz)
    core.snooze_task(conn, target_id, when_text, due, tz)
    result["action"] = "snooze_task"
    result["task"] = fetch_task(conn, target_id)
    result["resolved_time"] = due.isoformat(timespec="seconds")
    result["human_message"] = f"Snoozed task {target_id} until {due.isoformat(timespec='seconds')}."
    return result

  return {"ok": False, "error": "unknown_intent", "human_message": "Could not determine intent.", **result}


def default_db_path() -> str:
  data_home = os.getenv("XDG_DATA_HOME")
  base = Path(data_home).expanduser() if data_home else Path.home() / ".local" / "share"
  path = base / "assistant" / "tasks.db"
  path.parent.mkdir(parents=True, exist_ok=True)
  return str(path)


def main(argv: list[str]) -> int:
  common = argparse.ArgumentParser(add_help=False)
  common.add_argument("--db", default=default_db_path(), help="Path to SQLite DB")
  common.add_argument("--tz", default=None, help="IANA timezone")
  common.add_argument("--pretty", action="store_true", help="Pretty-print JSON")

  parser = argparse.ArgumentParser(description="JSON wrapper for tasks skill", parents=[common])
  sub = parser.add_subparsers(dest="cmd", required=True)

  sub.add_parser("init", parents=[common], help="Initialize DB")
  invoke_p = sub.add_parser("invoke", parents=[common], help="Invoke natural-language message")
  invoke_p.add_argument("--message", required=True)
  invoke_p.add_argument("--task-id", default=None)
  sub.add_parser("list", parents=[common], help="List actionable tasks")
  delete_p = sub.add_parser("delete", parents=[common], help="Soft-delete a task")
  delete_p.add_argument("--task-id", required=True)
  history_p = sub.add_parser("history", parents=[common], help="Get task and event history")
  history_p.add_argument("--task-id", required=True)

  args = parser.parse_args(argv)

  try:
    tz = core.detect_timezone(args.tz)
    conn = core.connect(args.db)
    core.init_db(conn)

    if args.cmd == "init":
      out = {"ok": True, "action": "init", "db": args.db}
    elif args.cmd == "list":
      rows = core.list_actionable(conn, tz)
      out = {"ok": True, "action": "list_tasks", "count": len(rows), "data": [row_to_task(r) for r in rows]}
    elif args.cmd == "invoke":
      out = invoke_message(conn, args.message, args.task_id, tz)
    elif args.cmd == "delete":
      task = fetch_task(conn, args.task_id)
      if task is None:
        out = {"ok": False, "action": "delete", "error": "task_not_found", "task_id": args.task_id}
      else:
        core.soft_delete_task(conn, args.task_id, "cli_delete", tz)
        out = {"ok": True, "action": "delete", "task": fetch_task(conn, args.task_id), "human_message": "Task soft-deleted."}
    elif args.cmd == "history":
      task = fetch_task(conn, args.task_id)
      if task is None:
        out = {"ok": False, "action": "history", "error": "task_not_found", "task_id": args.task_id}
      else:
        out = {"ok": True, "action": "history", "task": task, "events": fetch_events(conn, args.task_id)}
    else:
      out = {"ok": False, "error": "unsupported_command"}

    print(json.dumps(out, indent=2 if args.pretty else None, ensure_ascii=False))
    return 0 if out.get("ok") else 1
  except Exception as e:
    print(json.dumps({"ok": False, "error": "exception", "message": str(e)}, indent=2 if args.pretty else None))
    return 1


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
