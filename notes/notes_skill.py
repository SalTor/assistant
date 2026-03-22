#!/usr/bin/env python3
"""Simple Notes Agent Skill with SQLite persistence.

Features:
- Initializes notes DB schema.
- Creates notes from natural-language messages.
- Lists follow-up-worthy notes.
- Snoozes notes with fuzzy time phrases (e.g., "after q3 ends").
- Marks notes done.
- Stores full note event history.

Usage examples:
  python notes_skill.py init --db notes.db
  python notes_skill.py run --db notes.db --message "I wonder if Jeremy could show me sources for feature_x"
  python notes_skill.py run --db notes.db --message "Is there anything I can follow up on?"
  python notes_skill.py run --db notes.db --message "I've decided to postpone talking about this with Jeremy until after q3 ends"
  python notes_skill.py history --db notes.db --note-id <note_id>
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
from datetime import datetime, timedelta, timezone
from zoneinfo import ZoneInfo


SCHEMA_SQL = """
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  followup_state TEXT NOT NULL DEFAULT 'open',
  followup_after TEXT,
  priority INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS note_events (
  id TEXT PRIMARY KEY,
  note_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_text TEXT,
  payload_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(note_id) REFERENCES notes(id)
);

CREATE INDEX IF NOT EXISTS idx_notes_followup ON notes(followup_state, followup_after);
CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_note_events_note_time ON note_events(note_id, created_at);
"""


@dataclass
class ParsedIntent:
  intent: str
  confidence: float
  note_id: str | None = None
  when_text: str | None = None
  body: str | None = None


def now_iso(tz: ZoneInfo) -> str:
  return datetime.now(tz).isoformat(timespec="seconds")


def connect(db_path: str) -> sqlite3.Connection:
  conn = sqlite3.connect(db_path)
  conn.row_factory = sqlite3.Row
  return conn


def init_db(conn: sqlite3.Connection) -> None:
  conn.executescript(SCHEMA_SQL)
  conn.commit()


def add_event(
  conn: sqlite3.Connection,
  *,
  note_id: str,
  event_type: str,
  event_text: str,
  payload: dict,
  tz: ZoneInfo,
) -> str:
  event_id = str(uuid.uuid4())
  conn.execute(
    """
    INSERT INTO note_events (id, note_id, event_type, event_text, payload_json, created_at)
    VALUES (?, ?, ?, ?, ?, ?)
    """,
    (
      event_id,
      note_id,
      event_type,
      event_text,
      json.dumps(payload, ensure_ascii=False),
      now_iso(tz),
    ),
  )
  return event_id


def create_note(conn: sqlite3.Connection, body: str, tz: ZoneInfo) -> str:
  note_id = str(uuid.uuid4())
  ts = now_iso(tz)
  conn.execute(
    """
    INSERT INTO notes (id, body, created_at, updated_at, status, followup_state)
    VALUES (?, ?, ?, ?, 'active', 'open')
    """,
    (note_id, body.strip(), ts, ts),
  )
  add_event(
    conn,
    note_id=note_id,
    event_type="created",
    event_text="Note created",
    payload={"body": body.strip()},
    tz=tz,
  )
  conn.commit()
  return note_id


def find_latest_actionable_note(conn: sqlite3.Connection) -> sqlite3.Row | None:
  return conn.execute(
    """
    SELECT *
    FROM notes
    WHERE status = 'active'
      AND followup_state IN ('open', 'snoozed')
    ORDER BY updated_at DESC
    LIMIT 1
    """
  ).fetchone()


def get_note(conn: sqlite3.Connection, note_id: str) -> sqlite3.Row | None:
  return conn.execute("SELECT * FROM notes WHERE id = ?", (note_id,)).fetchone()


def list_followups(conn: sqlite3.Connection, tz: ZoneInfo) -> list[sqlite3.Row]:
  now = now_iso(tz)
  return conn.execute(
    """
    SELECT id, body, followup_state, followup_after, priority, created_at, updated_at
    FROM notes
    WHERE status = 'active'
      AND (
        followup_state = 'open'
        OR (followup_state = 'snoozed' AND followup_after IS NOT NULL AND followup_after <= ?)
      )
    ORDER BY priority DESC, created_at ASC
    """,
    (now,),
  ).fetchall()


def mark_done(conn: sqlite3.Connection, note_id: str, message: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute(
    """
    UPDATE notes
    SET followup_state = 'done', updated_at = ?
    WHERE id = ?
    """,
    (ts, note_id),
  )
  add_event(
    conn,
    note_id=note_id,
    event_type="completed",
    event_text="Marked as done",
    payload={"source_message": message},
    tz=tz,
  )
  conn.commit()


def soft_delete_note(conn: sqlite3.Connection, note_id: str, source: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute(
    """
    UPDATE notes
    SET status = 'deleted', followup_state = 'done', updated_at = ?
    WHERE id = ?
    """,
    (ts, note_id),
  )
  add_event(
    conn,
    note_id=note_id,
    event_type="deleted",
    event_text="Soft-deleted note",
    payload={"source_message": source},
    tz=tz,
  )
  conn.commit()


def undelete_note(conn: sqlite3.Connection, note_id: str, source: str, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute(
    """
    UPDATE notes
    SET status = 'active', followup_state = 'open', updated_at = ?
    WHERE id = ?
    """,
    (ts, note_id),
  )
  add_event(
    conn,
    note_id=note_id,
    event_type="undeleted",
    event_text="Note restored",
    payload={"source_message": source},
    tz=tz,
  )
  conn.commit()


def snooze_note(conn: sqlite3.Connection, note_id: str, when_text: str, dt: datetime, tz: ZoneInfo) -> None:
  ts = now_iso(tz)
  conn.execute(
    """
    UPDATE notes
    SET followup_state = 'snoozed', followup_after = ?, updated_at = ?
    WHERE id = ?
    """,
    (dt.isoformat(timespec="seconds"), ts, note_id),
  )
  add_event(
    conn,
    note_id=note_id,
    event_type="snoozed",
    event_text=f"Snoozed until {dt.isoformat(timespec='seconds')}",
    payload={
      "raw_time_text": when_text,
      "resolved_followup_after": dt.isoformat(timespec="seconds"),
      "timezone": str(tz),
    },
    tz=tz,
  )
  conn.commit()


def parse_intent(message: str, note_id: str | None = None) -> ParsedIntent:
  m = message.strip()
  lower = m.lower()

  if re.search(r"\b(anything|what).*(follow\s*up)\b|\bfollow\s*up worthy\b", lower):
    return ParsedIntent(intent="list_followups", confidence=0.95)

  if re.search(r"\b(done|completed|no need to follow up|resolved)\b", lower):
    return ParsedIntent(intent="complete_note", confidence=0.85, note_id=note_id)

  if re.search(r"\b(postpone|snooze|defer|push\s+back|until|after|later)\b", lower):
    when_text = extract_time_phrase(m)
    return ParsedIntent(intent="snooze_note", confidence=0.75, note_id=note_id, when_text=when_text)

  if lower.startswith("edit note:"):
    return ParsedIntent(intent="edit_note", confidence=0.8, note_id=note_id, body=m[len("edit note:"):].strip())

  # default: treat as note creation
  return ParsedIntent(intent="create_note", confidence=0.7, body=m)


def extract_time_phrase(message: str) -> str:
  m = re.search(r"\b(?:until|after|on|in)\b(.+)$", message, flags=re.IGNORECASE)
  if m:
    return m.group(0).strip()
  return message.strip()


def quarter_end(year: int, quarter: int, tz: ZoneInfo) -> datetime:
  # Return first instant after quarter end
  if quarter == 1:
    return datetime(year, 4, 1, 0, 0, 0, tzinfo=tz)
  if quarter == 2:
    return datetime(year, 7, 1, 0, 0, 0, tzinfo=tz)
  if quarter == 3:
    return datetime(year, 10, 1, 0, 0, 0, tzinfo=tz)
  return datetime(year + 1, 1, 1, 0, 0, 0, tzinfo=tz)


def resolve_time_phrase(text: str, now: datetime, tz: ZoneInfo) -> datetime:
  lower = text.lower().strip()

  # ISO date e.g. 2026-10-01
  m = re.search(r"(\d{4}-\d{2}-\d{2})", lower)
  if m:
    y, mo, d = map(int, m.group(1).split("-"))
    return datetime(y, mo, d, 9, 0, 0, tzinfo=tz)

  # after q3 ends [2026]
  m = re.search(r"after\s+q([1-4])(?:\s+ends?)?(?:\s+(\d{4}))?", lower)
  if m:
    q = int(m.group(1))
    year = int(m.group(2)) if m.group(2) else now.year
    dt = quarter_end(year, q, tz)
    if dt <= now:
      dt = quarter_end(year + 1, q, tz)
    return dt

  if "tomorrow" in lower:
    d = (now + timedelta(days=1)).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  if "next week" in lower:
    # next Monday at 9am
    days_until_monday = (7 - now.weekday()) % 7
    if days_until_monday == 0:
      days_until_monday = 7
    d = (now + timedelta(days=days_until_monday)).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  if "next month" in lower:
    y = now.year + (1 if now.month == 12 else 0)
    mo = 1 if now.month == 12 else now.month + 1
    return datetime(y, mo, 1, 9, 0, 0, tzinfo=tz)

  m = re.search(r"in\s+(\d+)\s+day", lower)
  if m:
    d = (now + timedelta(days=int(m.group(1)))).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  m = re.search(r"in\s+(\d+)\s+week", lower)
  if m:
    d = (now + timedelta(weeks=int(m.group(1)))).date()
    return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)

  # Fallback: 7 days
  d = (now + timedelta(days=7)).date()
  return datetime(d.year, d.month, d.day, 9, 0, 0, tzinfo=tz)


def edit_note(conn: sqlite3.Connection, note_id: str, new_body: str, tz: ZoneInfo) -> None:
  old = get_note(conn, note_id)
  if old is None:
    raise ValueError(f"Note {note_id} not found")
  ts = now_iso(tz)
  conn.execute("UPDATE notes SET body = ?, updated_at = ? WHERE id = ?", (new_body, ts, note_id))
  add_event(
    conn,
    note_id=note_id,
    event_type="edited",
    event_text="Body updated",
    payload={"old_body": old["body"], "new_body": new_body},
    tz=tz,
  )
  conn.commit()


def print_followups(rows: list[sqlite3.Row]) -> None:
  if not rows:
    print("No follow-up items right now.")
    return
  print("Follow-up items:")
  for r in rows:
    snooze = f" [snoozed until {r['followup_after']}]" if r["followup_state"] == "snoozed" else ""
    print(f"- {r['id']}: {r['body']}{snooze}")


def show_history(conn: sqlite3.Connection, note_id: str) -> None:
  note = get_note(conn, note_id)
  if note is None:
    print(f"Note not found: {note_id}")
    return

  print(f"Note {note_id}")
  print(f"body: {note['body']}")
  print(f"state: {note['followup_state']}, followup_after: {note['followup_after']}")
  print("events:")

  rows = conn.execute(
    """
    SELECT event_type, event_text, payload_json, created_at
    FROM note_events
    WHERE note_id = ?
    ORDER BY created_at ASC
    """,
    (note_id,),
  ).fetchall()

  for r in rows:
    print(f"- {r['created_at']} [{r['event_type']}] {r['event_text']} :: {r['payload_json']}")


def run_agent(conn: sqlite3.Connection, message: str, note_id: str | None, tz: ZoneInfo) -> None:
  parsed = parse_intent(message, note_id)
  now = datetime.now(tz)

  if parsed.intent == "list_followups":
    rows = list_followups(conn, tz)
    print_followups(rows)
    return

  if parsed.intent == "create_note":
    if not parsed.body:
      print("Nothing to save.")
      return
    new_id = create_note(conn, parsed.body, tz)
    print(f"Created note {new_id}")
    return

  target = parsed.note_id
  if not target:
    latest = find_latest_actionable_note(conn)
    if latest is None:
      print("No actionable note found. Create a note first or pass --note-id.")
      return
    target = latest["id"]

  if parsed.intent == "snooze_note":
    when_text = parsed.when_text or message
    dt = resolve_time_phrase(when_text, now, tz)
    snooze_note(conn, target, when_text, dt, tz)
    print(f"Snoozed note {target} until {dt.isoformat(timespec='seconds')} ({tz})")
    return

  if parsed.intent == "complete_note":
    mark_done(conn, target, message, tz)
    print(f"Marked note {target} as done")
    return

  if parsed.intent == "edit_note":
    if not parsed.body:
      print("No updated note body provided.")
      return
    edit_note(conn, target, parsed.body, tz)
    print(f"Edited note {target}")
    return

  print("Could not determine intent.")


def detect_timezone(name: str | None) -> ZoneInfo:
  if name:
    return ZoneInfo(name)
  tz_env = os.getenv("TZ")
  if tz_env:
    return ZoneInfo(tz_env)
  # local fallback
  local_tz = datetime.now().astimezone().tzinfo
  if local_tz is not None and hasattr(local_tz, "key"):
    return ZoneInfo(local_tz.key)  # type: ignore[attr-defined]
  return ZoneInfo("UTC")


def main(argv: list[str]) -> int:
  parser = argparse.ArgumentParser(description="Notes agent skill")
  parser.add_argument("--db", default="notes.db", help="Path to SQLite DB")
  parser.add_argument("--tz", default=None, help="IANA timezone, e.g., America/New_York")

  sub = parser.add_subparsers(dest="cmd", required=True)

  sub.add_parser("init", help="Initialize database schema")

  run_p = sub.add_parser("run", help="Run agent on a natural-language message")
  run_p.add_argument("--message", required=True, help="User message")
  run_p.add_argument("--note-id", default=None, help="Target note id")

  hist_p = sub.add_parser("history", help="Show full history for a note")
  hist_p.add_argument("--note-id", required=True, help="Note id")

  sub.add_parser("list", help="List follow-up-worthy notes")

  args = parser.parse_args(argv)
  tz = detect_timezone(args.tz)

  conn = connect(args.db)
  init_db(conn)

  if args.cmd == "init":
    print(f"Initialized DB at {args.db}")
    return 0

  if args.cmd == "run":
    run_agent(conn, args.message, args.note_id, tz)
    return 0

  if args.cmd == "history":
    show_history(conn, args.note_id)
    return 0

  if args.cmd == "list":
    print_followups(list_followups(conn, tz))
    return 0

  return 1


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
