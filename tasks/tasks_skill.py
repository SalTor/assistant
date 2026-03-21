#!/usr/bin/env python3
"""Simple Tasks domain logic with SQLite persistence."""

from __future__ import annotations

import argparse
import json
import os
import re
import sqlite3
import sys
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo


SCHEMA_SQL = """
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  details TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  due_at TEXT,
  priority INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS task_events (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_due ON tasks(status, due_at);
CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at);
CREATE INDEX IF NOT EXISTS idx_task_events_task_time ON task_events(task_id, created_at);
"""


@dataclass
class ParsedIntent:
  intent: str
  confidence: float
  task_id: str | None = None
  title: str | None = None
  when_text: str | None = None


def now_iso(tz: ZoneInfo) -> str:
  return datetime.now(tz).isoformat(timespec="seconds")


def connect(db_path: str) -> sqlite3.Connection:
  conn = sqlite3.connect(db_path)
  conn.row_factory = sqlite3.Row
  return conn


def init_db(conn: sqlite3.Connection) -> None:
  conn.executescript(SCHEMA_SQL)
  conn.commit()


def add_event(conn: sqlite3.Connection, *, task_id: str, event_type: str, event_text: str, payload: dict, tz: ZoneInfo) -> None:
  conn.execute(
    """
    INSERT INTO task_events (id, task_id, event_type, event_text, payload_json, created_at)
    VALUES (?, ?, ?, ?, ?, ?)
    """,
    (str(uuid.uuid4()), task_id, event_type, event_text, json.dumps(payload, ensure_ascii=False), now_iso(tz)),
  )


def create_task(conn: sqlite3.Connection, title: str, tz: ZoneInfo) -> str:
  task_id = str(uuid.uuid4())
  ts = now_iso(tz)
  conn.execute(
    """
    INSERT INTO tasks (id, title, created_at, updated_at, status, priority)
    VALUES (?, ?, ?, ?, 'open', 0)
    """,
    (task_id, title.strip(), ts, ts),
  )
  add_event(conn, task_id=task_id, event_type="created", event_text="Task created", payload={"title": title.strip()}, tz=tz)
  conn.commit()
  return task_id


def get_task(conn: sqlite3.Connection, task_id: str) -> sqlite3.Row | None:
  return conn.execute("SELECT * FROM tasks WHERE id = ?", (task_id,)).fetchone()


def latest_open_task(conn: sqlite3.Connection) -> sqlite3.Row | None:
  return conn.execute(
    """
    SELECT * FROM tasks
    WHERE status IN ('open', 'snoozed')
    ORDER BY updated_at DESC
    LIMIT 1
    """
  ).fetchone()


def list_actionable(conn: sqlite3.Connection, tz: ZoneInfo) -> list[sqlite3.Row]:
  now = now_iso(tz)
  return conn.execute(
    """
    SELECT id, title, details, status, due_at, priority, created_at, updated_at
    FROM tasks
    WHERE status = 'open'
       OR (status = 'snoozed' AND due_at IS NOT NULL AND due_at <= ?)
    ORDER BY priority DESC, created_at ASC
    """,
    (now,),
  ).fetchall()


def complete_task(conn: sqlite3.Connection, task_id: str, source: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute("UPDATE tasks SET status = 'done', updated_at = ? WHERE id = ?", (ts, task_id))
  add_event(conn, task_id=task_id, event_type="completed", event_text="Task marked done", payload={"source_message": source}, tz=tz)
  conn.commit()


def snooze_task(conn: sqlite3.Connection, task_id: str, when_text: str, due_at: datetime, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute(
    "UPDATE tasks SET status = 'snoozed', due_at = ?, updated_at = ? WHERE id = ?",
    (due_at.isoformat(timespec="seconds"), ts, task_id),
  )
  add_event(
    conn,
    task_id=task_id,
    event_type="snoozed",
    event_text=f"Task snoozed until {due_at.isoformat(timespec='seconds')}",
    payload={"raw_time_text": when_text, "resolved_due_at": due_at.isoformat(timespec="seconds"), "timezone": str(tz)},
    tz=tz,
  )
  conn.commit()


def parse_intent(message: str, task_id: str | None = None) -> ParsedIntent:
  text = message.strip()
  lower = text.lower()

  if re.search(r"\b(anything|what).*(task|todo)|\bmy tasks\b", lower):
    return ParsedIntent(intent="list_tasks", confidence=0.95)

  if re.search(r"\b(done|complete|completed|finished)\b", lower):
    return ParsedIntent(intent="complete_task", confidence=0.85, task_id=task_id)

  if re.search(r"\b(postpone|snooze|defer|until|after|later)\b", lower):
    return ParsedIntent(intent="snooze_task", confidence=0.75, task_id=task_id, when_text=extract_time_phrase(text))

  return ParsedIntent(intent="create_task", confidence=0.7, title=text)


def extract_time_phrase(message: str) -> str:
  m = re.search(r"\b(?:until|after|on|in)\b(.+)$", message, flags=re.IGNORECASE)
  return m.group(0).strip() if m else message.strip()


def quarter_rollover(year: int, quarter: int, tz: ZoneInfo) -> datetime:
  if quarter == 1:
    return datetime(year, 4, 1, 0, 0, 0, tzinfo=tz)
  if quarter == 2:
    return datetime(year, 7, 1, 0, 0, 0, tzinfo=tz)
  if quarter == 3:
    return datetime(year, 10, 1, 0, 0, 0, tzinfo=tz)
  return datetime(year + 1, 1, 1, 0, 0, 0, tzinfo=tz)


def resolve_time_phrase(text: str, now: datetime, tz: ZoneInfo) -> datetime:
  lower = text.lower().strip()

  m = re.search(r"(\d{4}-\d{2}-\d{2})", lower)
  if m:
    y, mo, d = map(int, m.group(1).split("-"))
    return datetime(y, mo, d, 9, 0, 0, tzinfo=tz)

  m = re.search(r"after\s+q([1-4])(?:\s+ends?)?(?:\s+(\d{4}))?", lower)
  if m:
    q = int(m.group(1))
    y = int(m.group(2)) if m.group(2) else now.year
    dt = quarter_rollover(y, q, tz)
    if dt <= now:
      dt = quarter_rollover(y + 1, q, tz)
    return dt

  if "tomorrow" in lower:
    d = (now + timedelta(days=1)).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  m = re.search(r"in\s+(\d+)\s+day", lower)
  if m:
    d = (now + timedelta(days=int(m.group(1)))).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  m = re.search(r"in\s+(\d+)\s+week", lower)
  if m:
    d = (now + timedelta(weeks=int(m.group(1)))).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  d = (now + timedelta(days=7)).date()
  return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)


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
  parser = argparse.ArgumentParser(description="Tasks skill")
  parser.add_argument("--db", default="tasks.db")
  parser.add_argument("--tz", default=None)

  sub = parser.add_subparsers(dest="cmd", required=True)
  sub.add_parser("init")
  run_p = sub.add_parser("run")
  run_p.add_argument("--message", required=True)
  run_p.add_argument("--task-id", default=None)
  sub.add_parser("list")

  args = parser.parse_args(argv)
  tz = detect_timezone(args.tz)

  conn = connect(args.db)
  init_db(conn)

  if args.cmd == "init":
    print(f"Initialized DB at {args.db}")
    return 0

  if args.cmd == "list":
    rows = list_actionable(conn, tz)
    if not rows:
      print("No actionable tasks.")
      return 0
    print("Actionable tasks:")
    for r in rows:
      print(f"- {r['id']}: {r['title']}")
    return 0

  parsed = parse_intent(args.message, args.task_id)
  if parsed.intent == "create_task":
    tid = create_task(conn, parsed.title or args.message, tz)
    print(f"Created task {tid}")
    return 0

  if parsed.intent == "list_tasks":
    rows = list_actionable(conn, tz)
    if not rows:
      print("No actionable tasks.")
      return 0
    print("Actionable tasks:")
    for r in rows:
      print(f"- {r['id']}: {r['title']}")
    return 0

  target = parsed.task_id
  if not target:
    latest = latest_open_task(conn)
    if latest is None:
      print("No target task found.")
      return 1
    target = latest["id"]

  if parsed.intent == "complete_task":
    complete_task(conn, target, args.message, tz)
    print(f"Completed task {target}")
    return 0

  if parsed.intent == "snooze_task":
    due = resolve_time_phrase(parsed.when_text or args.message, datetime.now(tz), tz)
    snooze_task(conn, target, parsed.when_text or args.message, due, tz)
    print(f"Snoozed task {target} until {due.isoformat(timespec='seconds')}")
    return 0

  return 1


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
