#!/usr/bin/env python3
"""JSON wrapper for problems/problems_skill.py."""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
from pathlib import Path
from typing import Any

import problems_skill as core


def row_to_problem(row: sqlite3.Row | None) -> dict[str, Any] | None:
  if row is None:
    return None
  data = dict(row)
  return {
    "id": data.get("id"),
    "title": data.get("title"),
    "statement": data.get("statement"),
    "parent_id": data.get("parent_id"),
    "status": data.get("status"),
    "created_at": data.get("created_at"),
    "updated_at": data.get("updated_at"),
  }


def fetch_problem(conn: sqlite3.Connection, problem_id: str) -> dict[str, Any] | None:
  return row_to_problem(core.get_problem(conn, problem_id))


def fetch_events(conn: sqlite3.Connection, problem_id: str) -> list[dict[str, Any]]:
  rows = conn.execute(
    """
    SELECT id, event_type, event_text, payload_json, created_at
    FROM problem_events
    WHERE problem_id = ?
    ORDER BY created_at ASC
    """,
    (problem_id,),
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


def fetch_links(conn: sqlite3.Connection, problem_id: str) -> list[dict[str, Any]]:
  rows = core.list_links(conn, problem_id)
  return [
    {
      "id": r["id"],
      "problem_id": r["problem_id"],
      "entity_type": r["entity_type"],
      "entity_id": r["entity_id"],
      "relation": r["relation"],
      "created_at": r["created_at"],
    }
    for r in rows
  ]


def resolve_target_problem_id(conn: sqlite3.Connection, problem_id: str | None) -> str | None:
  if problem_id:
    return problem_id
  latest = core.latest_open_problem(conn)
  if latest is None:
    return None
  return latest["id"]


def invoke_message(
  conn: sqlite3.Connection,
  message: str,
  problem_id: str | None,
  parent_problem_id: str | None,
  tz,
) -> dict[str, Any]:
  parsed = core.parse_intent(message, problem_id, parent_problem_id)
  result: dict[str, Any] = {
    "ok": True,
    "intent": parsed.intent,
    "confidence": parsed.confidence,
    "input": {"message": message, "problem_id": problem_id, "parent_problem_id": parent_problem_id},
  }

  if parsed.intent == "list_problems":
    rows = core.list_open_problems(conn)
    result["action"] = "list_problems"
    result["count"] = len(rows)
    result["data"] = [row_to_problem(r) for r in rows]
    result["human_message"] = "No open problems." if not rows else f"Found {len(rows)} open problem(s)."
    return result

  if parsed.intent == "tree_problems":
    rows = core.tree_problems(conn)
    result["action"] = "tree_problems"
    result["count"] = len(rows)
    result["data"] = rows
    result["human_message"] = "No problems." if not rows else f"Found {len(rows)} problem node(s)."
    return result

  if parsed.intent == "create_problem":
    statement = parsed.statement or message
    if not statement.strip():
      return {"ok": False, "error": "empty_statement", "human_message": "Problem statement is empty.", **result}
    new_id = core.create_problem(conn, statement=statement, tz=tz, parent_id=parsed.parent_id)
    result["action"] = "create_problem"
    result["problem"] = fetch_problem(conn, new_id)
    result["human_message"] = f"Created problem {new_id}."
    return result

  target_id = resolve_target_problem_id(conn, parsed.problem_id)
  if not target_id:
    return {"ok": False, "error": "no_target_problem", "human_message": "No target problem found.", **result}

  if parsed.intent == "solve_problem":
    core.solve_problem(conn, target_id, message, tz)
    result["action"] = "solve_problem"
    result["problem"] = fetch_problem(conn, target_id)
    result["human_message"] = f"Marked problem {target_id} solved."
    return result

  return {"ok": False, "error": "unknown_intent", "human_message": "Could not determine intent.", **result}


def default_db_path() -> str:
  data_home = os.getenv("XDG_DATA_HOME")
  base = Path(data_home).expanduser() if data_home else Path.home() / ".local" / "share"
  path = base / "assistant" / "problems.db"
  path.parent.mkdir(parents=True, exist_ok=True)
  return str(path)


def main(argv: list[str]) -> int:
  common = argparse.ArgumentParser(add_help=False)
  common.add_argument("--db", default=default_db_path(), help="Path to SQLite DB")
  common.add_argument("--tz", default=None, help="IANA timezone")
  common.add_argument("--pretty", action="store_true", help="Pretty-print JSON")

  parser = argparse.ArgumentParser(description="JSON wrapper for problems skill", parents=[common])
  sub = parser.add_subparsers(dest="cmd", required=True)

  sub.add_parser("init", help="Initialize DB", parents=[common])

  invoke_p = sub.add_parser("invoke", help="Invoke natural-language message", parents=[common])
  invoke_p.add_argument("--message", required=True, help="User message")
  invoke_p.add_argument("--problem-id", default=None, help="Optional target problem id")
  invoke_p.add_argument("--parent-problem-id", default=None, help="Optional parent problem id for new problems")

  sub.add_parser("list", help="List open problems", parents=[common])
  sub.add_parser("tree", help="List full problem tree", parents=[common])

  delete_p = sub.add_parser("delete", help="Soft-delete a problem", parents=[common])
  delete_p.add_argument("--problem-id", required=True)

  show_p = sub.add_parser("show", help="Get problem details and links", parents=[common])
  show_p.add_argument("--problem-id", required=True, help="Problem id")

  link_p = sub.add_parser("link", help="Link a note/task/problem to a problem", parents=[common])
  link_p.add_argument("--problem-id", required=True, help="Problem id")
  link_p.add_argument("--entity-type", required=True, choices=["note", "task", "problem"])
  link_p.add_argument("--entity-id", required=True)
  link_p.add_argument("--relation", default="addresses")

  unlink_p = sub.add_parser("unlink", help="Remove link(s) from a problem", parents=[common])
  unlink_p.add_argument("--problem-id", required=True, help="Problem id")
  unlink_p.add_argument("--entity-type", required=True, choices=["note", "task", "problem"])
  unlink_p.add_argument("--entity-id", required=True)
  unlink_p.add_argument("--relation", default=None)

  history_p = sub.add_parser("history", help="Get problem and event history", parents=[common])
  history_p.add_argument("--problem-id", required=True, help="Problem id")

  args = parser.parse_args(argv)

  try:
    tz = core.detect_timezone(args.tz)
    conn = core.connect(args.db)
    core.init_db(conn)

    if args.cmd == "init":
      out = {"ok": True, "action": "init", "db": args.db}
    elif args.cmd == "list":
      rows = core.list_open_problems(conn)
      out = {"ok": True, "action": "list_problems", "count": len(rows), "data": [row_to_problem(r) for r in rows]}
    elif args.cmd == "tree":
      rows = core.tree_problems(conn)
      out = {"ok": True, "action": "tree_problems", "count": len(rows), "data": rows}
    elif args.cmd == "show":
      problem = fetch_problem(conn, args.problem_id)
      if problem is None:
        out = {"ok": False, "action": "show", "error": "problem_not_found", "problem_id": args.problem_id}
      else:
        out = {
          "ok": True,
          "action": "show",
          "problem": problem,
          "links": fetch_links(conn, args.problem_id),
        }
    elif args.cmd == "delete":
      if fetch_problem(conn, args.problem_id) is None:
        out = {"ok": False, "action": "delete", "error": "problem_not_found", "problem_id": args.problem_id}
      else:
        core.soft_delete_problem(conn, args.problem_id, "cli_delete", tz)
        out = {
          "ok": True,
          "action": "delete",
          "problem": fetch_problem(conn, args.problem_id),
          "human_message": "Problem soft-deleted.",
        }
    elif args.cmd == "link":
      if fetch_problem(conn, args.problem_id) is None:
        out = {"ok": False, "action": "link", "error": "problem_not_found", "problem_id": args.problem_id}
      else:
        core.link_entity(
          conn,
          problem_id=args.problem_id,
          entity_type=args.entity_type,
          entity_id=args.entity_id,
          relation=args.relation,
          tz=tz,
        )
        out = {
          "ok": True,
          "action": "link",
          "human_message": f"Linked {args.entity_type} {args.entity_id} to problem {args.problem_id}.",
          "problem_id": args.problem_id,
          "link": {
            "entity_type": args.entity_type,
            "entity_id": args.entity_id,
            "relation": args.relation,
          },
        }
    elif args.cmd == "unlink":
      if fetch_problem(conn, args.problem_id) is None:
        out = {"ok": False, "action": "unlink", "error": "problem_not_found", "problem_id": args.problem_id}
      else:
        removed = core.unlink_entity(
          conn,
          problem_id=args.problem_id,
          entity_type=args.entity_type,
          entity_id=args.entity_id,
          relation=args.relation,
          tz=tz,
        )
        out = {
          "ok": True,
          "action": "unlink",
          "human_message": f"Unlinked {removed} row(s).",
          "problem_id": args.problem_id,
          "removed": removed,
          "unlink": {
            "entity_type": args.entity_type,
            "entity_id": args.entity_id,
            "relation": args.relation,
          },
        }
    elif args.cmd == "history":
      problem = fetch_problem(conn, args.problem_id)
      if problem is None:
        out = {"ok": False, "action": "history", "error": "problem_not_found", "problem_id": args.problem_id}
      else:
        out = {
          "ok": True,
          "action": "history",
          "problem": problem,
          "links": fetch_links(conn, args.problem_id),
          "events": fetch_events(conn, args.problem_id),
        }
    elif args.cmd == "invoke":
      out = invoke_message(conn, args.message, args.problem_id, args.parent_problem_id, tz)
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
