#!/usr/bin/env python3
"""JSON wrapper for notes_skill.py suitable for agent/skill orchestration.

All commands print JSON to stdout and exit non-zero on failure.
"""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

import notes_skill as core


def row_to_note(row: sqlite3.Row | None) -> dict[str, Any] | None:
  if row is None:
    return None
  data = dict(row)
  return {
    "id": data.get("id"),
    "body": data.get("body"),
    "status": data.get("status"),
    "followup_state": data.get("followup_state"),
    "followup_after": data.get("followup_after"),
    "priority": data.get("priority"),
    "created_at": data.get("created_at"),
    "updated_at": data.get("updated_at"),
  }


def fetch_note(conn: sqlite3.Connection, note_id: str) -> dict[str, Any] | None:
  return row_to_note(core.get_note(conn, note_id))


def fetch_events(conn: sqlite3.Connection, note_id: str) -> list[dict[str, Any]]:
  rows = conn.execute(
    """
    SELECT id, event_type, event_text, payload_json, created_at
    FROM note_events
    WHERE note_id = ?
    ORDER BY created_at ASC
    """,
    (note_id,),
  ).fetchall()

  out: list[dict[str, Any]] = []
  for r in rows:
    payload = r["payload_json"]
    try:
      payload_obj = json.loads(payload) if payload else None
    except json.JSONDecodeError:
      payload_obj = {"raw": payload}
    out.append(
      {
        "id": r["id"],
        "event_type": r["event_type"],
        "event_text": r["event_text"],
        "payload": payload_obj,
        "created_at": r["created_at"],
      }
    )
  return out


def resolve_target_note_id(conn: sqlite3.Connection, note_id: str | None) -> str | None:
  if note_id:
    return note_id
  latest = core.find_latest_actionable_note(conn)
  if latest is None:
    return None
  return latest["id"]


def invoke_message(conn: sqlite3.Connection, message: str, note_id: str | None, tz) -> dict[str, Any]:
  parsed = core.parse_intent(message, note_id)
  now = datetime.now(tz)

  result: dict[str, Any] = {
    "ok": True,
    "intent": parsed.intent,
    "confidence": parsed.confidence,
    "input": {
      "message": message,
      "note_id": note_id,
      "timezone": str(tz),
    },
  }

  if parsed.intent == "list_followups":
    rows = core.list_followups(conn, tz)
    result["action"] = "list_followups"
    result["data"] = [row_to_note(r) for r in rows]
    result["human_message"] = (
      "No follow-up items right now." if not rows else f"Found {len(rows)} follow-up item(s)."
    )
    return result

  if parsed.intent == "create_note":
    if not parsed.body:
      return {
        "ok": False,
        "error": "empty_body",
        "human_message": "Nothing to save.",
        **result,
      }
    new_id = core.create_note(conn, parsed.body, tz)
    result["action"] = "create_note"
    result["note"] = fetch_note(conn, new_id)
    result["human_message"] = f"Created note {new_id}."
    return result

  target_id = resolve_target_note_id(conn, parsed.note_id)
  if not target_id:
    return {
      "ok": False,
      "error": "no_target_note",
      "human_message": "No actionable note found. Create a note first or pass --note-id.",
      **result,
    }

  if parsed.intent == "snooze_note":
    when_text = parsed.when_text or message
    dt = core.resolve_time_phrase(when_text, now, tz)
    core.snooze_note(conn, target_id, when_text, dt, tz)
    result["action"] = "snooze_note"
    result["note"] = fetch_note(conn, target_id)
    result["resolved_time"] = dt.isoformat(timespec="seconds")
    result["human_message"] = f"Snoozed note {target_id} until {dt.isoformat(timespec='seconds')}."
    return result

  if parsed.intent == "complete_note":
    core.mark_done(conn, target_id, message, tz)
    result["action"] = "complete_note"
    result["note"] = fetch_note(conn, target_id)
    result["human_message"] = f"Marked note {target_id} as done."
    return result

  if parsed.intent == "edit_note":
    if not parsed.body:
      return {
        "ok": False,
        "error": "empty_edit_body",
        "human_message": "No updated note body provided.",
        **result,
      }
    core.edit_note(conn, target_id, parsed.body, tz)
    result["action"] = "edit_note"
    result["note"] = fetch_note(conn, target_id)
    result["human_message"] = f"Edited note {target_id}."
    return result

  return {
    "ok": False,
    "error": "unknown_intent",
    "human_message": "Could not determine intent.",
    **result,
  }


def default_db_path() -> str:
  data_home = os.getenv("XDG_DATA_HOME")
  base = Path(data_home).expanduser() if data_home else Path.home() / ".local" / "share"
  path = base / "assistant" / "notes.db"
  path.parent.mkdir(parents=True, exist_ok=True)
  return str(path)


def main(argv: list[str]) -> int:
  common = argparse.ArgumentParser(add_help=False)
  common.add_argument("--db", default=default_db_path(), help="Path to SQLite DB")
  common.add_argument("--tz", default=None, help="IANA timezone")
  common.add_argument("--pretty", action="store_true", help="Pretty-print JSON")

  parser = argparse.ArgumentParser(description="JSON wrapper for notes skill", parents=[common])
  sub = parser.add_subparsers(dest="cmd", required=True)

  sub.add_parser("init", help="Initialize DB", parents=[common])

  invoke_p = sub.add_parser("invoke", help="Invoke natural-language message", parents=[common])
  invoke_p.add_argument("--message", required=True, help="User message")
  invoke_p.add_argument("--note-id", default=None, help="Optional note id target")

  history_p = sub.add_parser("history", help="Get note and event history", parents=[common])
  history_p.add_argument("--note-id", required=True, help="Note id")

  delete_p = sub.add_parser("delete", help="Soft-delete a note", parents=[common])
  delete_p.add_argument("--note-id", required=True, help="Note id")

  undelete_p = sub.add_parser("undelete", help="Restore a soft-deleted note", parents=[common])
  undelete_p.add_argument("--note-id", required=True, help="Note id")

  sub.add_parser("list", help="List follow-up-worthy notes", parents=[common])

  args = parser.parse_args(argv)

  try:
    tz = core.detect_timezone(args.tz)
    conn = core.connect(args.db)
    core.init_db(conn)

    if args.cmd == "init":
      out = {"ok": True, "action": "init", "db": args.db}
    elif args.cmd == "invoke":
      out = invoke_message(conn, args.message, args.note_id, tz)
    elif args.cmd == "list":
      rows = core.list_followups(conn, tz)
      out = {
        "ok": True,
        "action": "list_followups",
        "count": len(rows),
        "data": [row_to_note(r) for r in rows],
      }
    elif args.cmd == "history":
      note = fetch_note(conn, args.note_id)
      if note is None:
        out = {
          "ok": False,
          "action": "history",
          "error": "note_not_found",
          "note_id": args.note_id,
        }
      else:
        out = {
          "ok": True,
          "action": "history",
          "note": note,
          "events": fetch_events(conn, args.note_id),
        }
    elif args.cmd == "delete":
      note = fetch_note(conn, args.note_id)
      if note is None:
        out = {"ok": False, "action": "delete", "error": "note_not_found", "note_id": args.note_id}
      else:
        core.soft_delete_note(conn, args.note_id, "cli_delete", tz)
        out = {"ok": True, "action": "delete", "note": fetch_note(conn, args.note_id), "human_message": "Note soft-deleted."}
    elif args.cmd == "undelete":
      note = fetch_note(conn, args.note_id)
      if note is None:
        out = {"ok": False, "action": "undelete", "error": "note_not_found", "note_id": args.note_id}
      else:
        core.undelete_note(conn, args.note_id, "cli_undelete", tz)
        out = {"ok": True, "action": "undelete", "note": fetch_note(conn, args.note_id), "human_message": "Note restored."}
    else:
      out = {"ok": False, "error": "unsupported_command"}

    print(json.dumps(out, indent=2 if args.pretty else None, ensure_ascii=False))
    return 0 if out.get("ok") else 1
  except Exception as e:
    out = {"ok": False, "error": "exception", "message": str(e)}
    print(json.dumps(out, indent=2 if args.pretty else None, ensure_ascii=False))
    return 1


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
