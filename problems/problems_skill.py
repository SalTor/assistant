#!/usr/bin/env python3
"""Simple Problems domain logic with SQLite persistence.

Problem framing supports nesting via parent_id.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sqlite3
import sys
import uuid
from dataclasses import dataclass
from datetime import datetime
from zoneinfo import ZoneInfo


SCHEMA_SQL = """
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS problems (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  statement TEXT NOT NULL,
  parent_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  FOREIGN KEY(parent_id) REFERENCES problems(id)
);

CREATE TABLE IF NOT EXISTS problem_events (
  id TEXT PRIMARY KEY,
  problem_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(problem_id) REFERENCES problems(id)
);

CREATE TABLE IF NOT EXISTS problem_links (
  id TEXT PRIMARY KEY,
  problem_id TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(problem_id) REFERENCES problems(id)
);

CREATE INDEX IF NOT EXISTS idx_problems_parent ON problems(parent_id);
CREATE INDEX IF NOT EXISTS idx_problems_status_updated ON problems(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_problem_events_problem_time ON problem_events(problem_id, created_at);
CREATE INDEX IF NOT EXISTS idx_problem_links_problem ON problem_links(problem_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_problem_links_unique
  ON problem_links(problem_id, entity_type, entity_id, relation);
"""


@dataclass
class ParsedIntent:
  intent: str
  confidence: float
  problem_id: str | None = None
  parent_id: str | None = None
  title: str | None = None
  statement: str | None = None


def now_iso(tz: ZoneInfo) -> str:
  return datetime.now(tz).isoformat(timespec="seconds")


def connect(db_path: str) -> sqlite3.Connection:
  conn = sqlite3.connect(db_path)
  conn.row_factory = sqlite3.Row
  return conn


def init_db(conn: sqlite3.Connection) -> None:
  conn.executescript(SCHEMA_SQL)
  conn.commit()


def add_event(conn: sqlite3.Connection, *, problem_id: str, event_type: str, event_text: str, payload: dict, tz: ZoneInfo) -> None:
  conn.execute(
    """
    INSERT INTO problem_events (id, problem_id, event_type, event_text, payload_json, created_at)
    VALUES (?, ?, ?, ?, ?, ?)
    """,
    (str(uuid.uuid4()), problem_id, event_type, event_text, json.dumps(payload, ensure_ascii=False), now_iso(tz)),
  )


def _title_from_statement(statement: str) -> str:
  text = statement.strip()
  if not text:
    return "Untitled problem"
  words = text.split()
  if len(words) <= 10:
    return text
  return " ".join(words[:10]) + "…"


def create_problem(conn: sqlite3.Connection, statement: str, tz: ZoneInfo, parent_id: str | None = None, title: str | None = None) -> str:
  problem_id = str(uuid.uuid4())
  ts = now_iso(tz)
  title_val = (title or _title_from_statement(statement)).strip()
  statement_val = statement.strip()

  conn.execute(
    """
    INSERT INTO problems (id, title, statement, parent_id, created_at, updated_at, status)
    VALUES (?, ?, ?, ?, ?, ?, 'open')
    """,
    (problem_id, title_val, statement_val, parent_id, ts, ts),
  )

  add_event(
    conn,
    problem_id=problem_id,
    event_type="created",
    event_text="Problem created",
    payload={"title": title_val, "statement": statement_val, "parent_id": parent_id},
    tz=tz,
  )
  conn.commit()
  return problem_id


def get_problem(conn: sqlite3.Connection, problem_id: str) -> sqlite3.Row | None:
  return conn.execute("SELECT * FROM problems WHERE id = ?", (problem_id,)).fetchone()


def latest_open_problem(conn: sqlite3.Connection) -> sqlite3.Row | None:
  return conn.execute(
    """
    SELECT * FROM problems
    WHERE status = 'open'
    ORDER BY updated_at DESC
    LIMIT 1
    """
  ).fetchone()


def list_open_problems(conn: sqlite3.Connection) -> list[sqlite3.Row]:
  return conn.execute(
    """
    SELECT id, title, statement, parent_id, status, created_at, updated_at
    FROM problems
    WHERE status = 'open'
    ORDER BY created_at ASC
    """
  ).fetchall()


def list_all_problems(conn: sqlite3.Connection) -> list[sqlite3.Row]:
  return conn.execute(
    """
    SELECT id, title, statement, parent_id, status, created_at, updated_at
    FROM problems
    WHERE status != 'deleted'
    ORDER BY created_at ASC
    """
  ).fetchall()


def tree_problems(conn: sqlite3.Connection) -> list[dict[str, object]]:
  rows = list_all_problems(conn)
  by_parent: dict[str | None, list[sqlite3.Row]] = {}
  for r in rows:
    by_parent.setdefault(r["parent_id"], []).append(r)

  out: list[dict[str, object]] = []

  def walk(parent_id: str | None, depth: int) -> None:
    for r in by_parent.get(parent_id, []):
      out.append(
        {
          "id": r["id"],
          "title": r["title"],
          "statement": r["statement"],
          "parent_id": r["parent_id"],
          "status": r["status"],
          "created_at": r["created_at"],
          "updated_at": r["updated_at"],
          "depth": depth,
        }
      )
      walk(r["id"], depth + 1)

  walk(None, 0)
  return out


def solve_problem(conn: sqlite3.Connection, problem_id: str, source_message: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute("UPDATE problems SET status = 'solved', updated_at = ? WHERE id = ?", (ts, problem_id))
  add_event(
    conn,
    problem_id=problem_id,
    event_type="solved",
    event_text="Problem marked solved",
    payload={"source_message": source_message},
    tz=tz,
  )
  conn.commit()


def soft_delete_problem(conn: sqlite3.Connection, problem_id: str, source_message: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute("UPDATE problems SET status = 'deleted', updated_at = ? WHERE id = ?", (ts, problem_id))
  add_event(
    conn,
    problem_id=problem_id,
    event_type="deleted",
    event_text="Problem soft-deleted",
    payload={"source_message": source_message},
    tz=tz,
  )
  conn.commit()


def link_entity(
  conn: sqlite3.Connection,
  *,
  problem_id: str,
  entity_type: str,
  entity_id: str,
  relation: str,
  tz: ZoneInfo,
) -> None:
  entity_type = entity_type.strip().lower()
  relation = relation.strip().lower()
  if entity_type not in {"note", "task", "problem"}:
    raise ValueError("entity_type must be one of: note, task, problem")

  conn.execute(
    """
    INSERT OR IGNORE INTO problem_links (id, problem_id, entity_type, entity_id, relation, created_at)
    VALUES (?, ?, ?, ?, ?, ?)
    """,
    (str(uuid.uuid4()), problem_id, entity_type, entity_id.strip(), relation, now_iso(tz)),
  )
  add_event(
    conn,
    problem_id=problem_id,
    event_type="linked",
    event_text=f"Linked {entity_type} {entity_id} as {relation}",
    payload={"entity_type": entity_type, "entity_id": entity_id.strip(), "relation": relation},
    tz=tz,
  )
  conn.commit()


def list_links(conn: sqlite3.Connection, problem_id: str) -> list[sqlite3.Row]:
  return conn.execute(
    """
    SELECT id, problem_id, entity_type, entity_id, relation, created_at
    FROM problem_links
    WHERE problem_id = ?
    ORDER BY created_at ASC
    """,
    (problem_id,),
  ).fetchall()


def unlink_entity(
  conn: sqlite3.Connection,
  *,
  problem_id: str,
  entity_type: str,
  entity_id: str,
  relation: str | None,
  tz: ZoneInfo,
) -> int:
  entity_type = entity_type.strip().lower()
  if entity_type not in {"note", "task", "problem"}:
    raise ValueError("entity_type must be one of: note, task, problem")

  if relation and relation.strip():
    rel = relation.strip().lower()
    cur = conn.execute(
      """
      DELETE FROM problem_links
      WHERE problem_id = ? AND entity_type = ? AND entity_id = ? AND relation = ?
      """,
      (problem_id, entity_type, entity_id.strip(), rel),
    )
    removed = cur.rowcount
    payload = {"entity_type": entity_type, "entity_id": entity_id.strip(), "relation": rel, "removed": removed}
  else:
    cur = conn.execute(
      """
      DELETE FROM problem_links
      WHERE problem_id = ? AND entity_type = ? AND entity_id = ?
      """,
      (problem_id, entity_type, entity_id.strip()),
    )
    removed = cur.rowcount
    payload = {"entity_type": entity_type, "entity_id": entity_id.strip(), "relation": "*", "removed": removed}

  add_event(
    conn,
    problem_id=problem_id,
    event_type="unlinked",
    event_text=f"Unlinked {entity_type} {entity_id}",
    payload=payload,
    tz=tz,
  )
  conn.commit()
  return removed


def parse_intent(message: str, problem_id: str | None = None, parent_id: str | None = None) -> ParsedIntent:
  text = message.strip()
  lower = text.lower()

  if lower in {"list", "show problems", "what problems", "what are my problems"}:
    return ParsedIntent(intent="list_problems", confidence=0.95)

  if lower in {"tree", "problem tree", "show tree"}:
    return ParsedIntent(intent="tree_problems", confidence=0.95)

  if re.search(r"\b(done|solved|resolved|close)\b", lower):
    return ParsedIntent(intent="solve_problem", confidence=0.85, problem_id=problem_id)

  statement = text
  if lower.startswith("add "):
    statement = text[4:].strip()
  return ParsedIntent(intent="create_problem", confidence=0.7, problem_id=problem_id, parent_id=parent_id, statement=statement)


def detect_timezone(name: str | None) -> ZoneInfo:
  if name:
    return ZoneInfo(name)
  env = os.getenv("TZ")
  if env:
    return ZoneInfo(env)
  local = datetime.now().astimezone().tzinfo
  if local is not None and hasattr(local, "key"):
    return ZoneInfo(local.key)  # type: ignore[attr-defined]
  return ZoneInfo("UTC")


def main(argv: list[str]) -> int:
  parser = argparse.ArgumentParser(description="Problems skill")
  parser.add_argument("--db", default="problems.db")
  parser.add_argument("--tz", default=None)

  sub = parser.add_subparsers(dest="cmd", required=True)
  sub.add_parser("init")
  run_p = sub.add_parser("run")
  run_p.add_argument("--message", required=True)
  run_p.add_argument("--problem-id", default=None)
  run_p.add_argument("--parent-problem-id", default=None)
  sub.add_parser("list")
  sub.add_parser("tree")

  show_p = sub.add_parser("show")
  show_p.add_argument("--problem-id", required=True)

  link_p = sub.add_parser("link")
  link_p.add_argument("--problem-id", required=True)
  link_p.add_argument("--entity-type", required=True, choices=["note", "task", "problem"])
  link_p.add_argument("--entity-id", required=True)
  link_p.add_argument("--relation", default="addresses")

  unlink_p = sub.add_parser("unlink")
  unlink_p.add_argument("--problem-id", required=True)
  unlink_p.add_argument("--entity-type", required=True, choices=["note", "task", "problem"])
  unlink_p.add_argument("--entity-id", required=True)
  unlink_p.add_argument("--relation", default=None)

  args = parser.parse_args(argv)
  tz = detect_timezone(args.tz)
  conn = connect(args.db)
  init_db(conn)

  if args.cmd == "init":
    print(f"Initialized DB at {args.db}")
    return 0

  if args.cmd == "list":
    rows = list_open_problems(conn)
    if not rows:
      print("No open problems.")
      return 0
    print("Open problems:")
    for r in rows:
      print(f"- {r['id']}: {r['title']}")
    return 0

  if args.cmd == "tree":
    rows = tree_problems(conn)
    if not rows:
      print("No problems.")
      return 0
    print("Problem tree:")
    for r in rows:
      indent = "  " * int(r["depth"])
      print(f"{indent}- {r['id']}: {r['title']} ({r['status']})")
    return 0

  if args.cmd == "show":
    p = get_problem(conn, args.problem_id)
    if p is None:
      print(f"Problem not found: {args.problem_id}")
      return 1
    print(f"Problem {p['id']}: {p['title']} ({p['status']})")
    print(f"statement: {p['statement']}")
    links = list_links(conn, args.problem_id)
    if not links:
      print("links: (none)")
    else:
      print("links:")
      for l in links:
        print(f"- {l['entity_type']} {l['entity_id']} [{l['relation']}]")
    return 0

  if args.cmd == "link":
    if get_problem(conn, args.problem_id) is None:
      print(f"Problem not found: {args.problem_id}")
      return 1
    link_entity(
      conn,
      problem_id=args.problem_id,
      entity_type=args.entity_type,
      entity_id=args.entity_id,
      relation=args.relation,
      tz=tz,
    )
    print(f"Linked {args.entity_type} {args.entity_id} to problem {args.problem_id} as {args.relation}")
    return 0

  if args.cmd == "unlink":
    if get_problem(conn, args.problem_id) is None:
      print(f"Problem not found: {args.problem_id}")
      return 1
    removed = unlink_entity(
      conn,
      problem_id=args.problem_id,
      entity_type=args.entity_type,
      entity_id=args.entity_id,
      relation=args.relation,
      tz=tz,
    )
    rel_text = args.relation if args.relation else "all relations"
    print(f"Unlinked {removed} row(s): {args.entity_type} {args.entity_id} from problem {args.problem_id} ({rel_text})")
    return 0

  parsed = parse_intent(args.message, args.problem_id, args.parent_problem_id)

  if parsed.intent == "list_problems":
    rows = list_open_problems(conn)
    if not rows:
      print("No open problems.")
      return 0
    print("Open problems:")
    for r in rows:
      print(f"- {r['id']}: {r['title']}")
    return 0

  if parsed.intent == "tree_problems":
    rows = tree_problems(conn)
    if not rows:
      print("No problems.")
      return 0
    print("Problem tree:")
    for r in rows:
      indent = "  " * int(r["depth"])
      print(f"{indent}- {r['id']}: {r['title']} ({r['status']})")
    return 0

  if parsed.intent == "create_problem":
    if not parsed.statement:
      print("Problem statement is empty.")
      return 1
    pid = create_problem(conn, parsed.statement, tz, parent_id=parsed.parent_id)
    print(f"Created problem {pid}")
    return 0

  target = parsed.problem_id
  if not target:
    latest = latest_open_problem(conn)
    if latest is None:
      print("No target problem found.")
      return 1
    target = latest["id"]

  if parsed.intent == "solve_problem":
    solve_problem(conn, target, args.message, tz)
    print(f"Solved problem {target}")
    return 0

  return 1


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
