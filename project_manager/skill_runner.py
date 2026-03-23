#!/usr/bin/env python3
"""Project manager skill.

Reviews a Jujutsu stack diff, discovers machine-readable problem trailers,
and can log progress events to the bound problem.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sqlite3
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any


TRAILER_PROBLEM = "PM-Problem"
TRAILER_RELATION = "PM-Relation"
TRAILER_PROGRESS = "PM-Progress"


def default_data_dir() -> Path:
  xdg_data_home = os.getenv("XDG_DATA_HOME")
  if xdg_data_home:
    return Path(xdg_data_home).expanduser() / "assistant"
  return Path.home() / ".local" / "share" / "assistant"


def default_problems_db() -> str:
  d = default_data_dir()
  d.mkdir(parents=True, exist_ok=True)
  return str(d / "problems.db")


def now_iso() -> str:
  return datetime.now().astimezone().isoformat(timespec="seconds")


def run_jj(args: list[str]) -> tuple[int, str, str]:
  proc = subprocess.run(["jj", *args], capture_output=True, text=True)
  return proc.returncode, (proc.stdout or "").strip(), (proc.stderr or "").strip()


def load_commits(revset: str) -> list[dict[str, str]]:
  template = "change_id.short() ++ '\n' ++ commit_id.short() ++ '\n' ++ description ++ '\n<<END>>\n'"
  code, out, err = run_jj(["log", "-r", revset, "--no-graph", "-T", template])
  if code != 0:
    raise RuntimeError(f"jj log failed: {err or out}")

  chunks = out.split("<<END>>")
  commits: list[dict[str, str]] = []
  for raw in chunks:
    lines = [ln for ln in raw.strip("\n").splitlines()]
    if len(lines) < 2:
      continue
    change_id = lines[0].strip()
    commit_id = lines[1].strip()
    desc = "\n".join(lines[2:]).strip()
    commits.append({"change_id": change_id, "commit_id": commit_id, "description": desc})
  return commits


def load_diff_summary(revset: str) -> str:
  code, out, err = run_jj(["diff", "-r", revset, "--summary"])
  if code != 0:
    return f"(diff unavailable: {err or out})"
  return out


def parse_trailers(description: str) -> dict[str, str]:
  found: dict[str, str] = {}
  for line in description.splitlines():
    m = re.match(r"^([A-Za-z0-9_-]+):\s*(.+?)\s*$", line.strip())
    if not m:
      continue
    key, val = m.group(1), m.group(2)
    if key in {TRAILER_PROBLEM, TRAILER_RELATION, TRAILER_PROGRESS}:
      found[key] = val
  return found


def all_stack_trailers(commits: list[dict[str, str]]) -> list[dict[str, Any]]:
  out: list[dict[str, Any]] = []
  for c in commits:
    t = parse_trailers(c.get("description", ""))
    if t:
      out.append({"change_id": c["change_id"], "commit_id": c["commit_id"], "trailers": t})
  return out


def connect_problems(db_path: str) -> sqlite3.Connection:
  conn = sqlite3.connect(db_path)
  conn.row_factory = sqlite3.Row
  return conn


def resolve_problem_id(conn: sqlite3.Connection, token: str) -> str | None:
  token = token.strip()
  if not token:
    return None
  exact = conn.execute("SELECT id FROM problems WHERE id = ?", (token,)).fetchone()
  if exact:
    return exact["id"]
  rows = conn.execute("SELECT id FROM problems WHERE id LIKE ? AND status != 'deleted'", (f"{token}%",)).fetchall()
  if len(rows) == 1:
    return rows[0]["id"]
  return None


def problem_exists(conn: sqlite3.Connection, problem_id: str) -> bool:
  row = conn.execute("SELECT id FROM problems WHERE id = ? AND status != 'deleted'", (problem_id,)).fetchone()
  return row is not None


def create_problem(conn: sqlite3.Connection, statement: str) -> str:
  import uuid

  pid = str(uuid.uuid4())
  ts = now_iso()
  title = statement if len(statement.split()) <= 10 else " ".join(statement.split()[:10]) + "…"
  conn.execute(
    """
    INSERT INTO problems (id, title, statement, parent_id, created_at, updated_at, status)
    VALUES (?, ?, ?, NULL, ?, ?, 'open')
    """,
    (pid, title, statement, ts, ts),
  )
  conn.execute(
    """
    INSERT INTO problem_events (id, problem_id, event_type, event_text, payload_json, created_at)
    VALUES (?, ?, 'created', 'Problem created by project_manager', ?, ?)
    """,
    (str(uuid.uuid4()), pid, json.dumps({"statement": statement}, ensure_ascii=False), ts),
  )
  conn.commit()
  return pid


def add_progress_event(
  conn: sqlite3.Connection,
  *,
  problem_id: str,
  revset: str,
  commits: list[dict[str, str]],
  relation: str,
  progress: str,
  diff_summary: str,
) -> None:
  import uuid

  ts = now_iso()
  payload = {
    "revset": revset,
    "relation": relation,
    "progress": progress,
    "commit_ids": [c["commit_id"] for c in commits],
    "change_ids": [c["change_id"] for c in commits],
    "diff_summary": diff_summary,
  }
  conn.execute(
    """
    INSERT INTO problem_events (id, problem_id, event_type, event_text, payload_json, created_at)
    VALUES (?, ?, 'progress_update', ?, ?, ?)
    """,
    (str(uuid.uuid4()), problem_id, f"Progress update from revset {revset}", json.dumps(payload, ensure_ascii=False), ts),
  )
  conn.commit()


def suggest_problem_statement(commits: list[dict[str, str]], diff_summary: str) -> str:
  for c in commits:
    first = (c.get("description") or "").splitlines()[0].strip() if c.get("description") else ""
    if first and not first.startswith("PM-"):
      return f"Problem: {first}"
  for line in diff_summary.splitlines():
    line = line.strip()
    if line:
      return f"Problem: {line}"
  return "Problem: Work in this stack needs explicit framing"


def build_trailer_block(problem_id: str, relation: str = "addresses", progress: str | None = None) -> str:
  lines = [f"{TRAILER_PROBLEM}: {problem_id}", f"{TRAILER_RELATION}: {relation}"]
  if progress:
    lines.append(f"{TRAILER_PROGRESS}: {progress}")
  return "\n".join(lines)


def cmd_review(args) -> dict[str, Any]:
  commits = load_commits(args.revset)
  diff_summary = load_diff_summary(args.revset)
  trailers = all_stack_trailers(commits)

  out: dict[str, Any] = {
    "ok": True,
    "action": "review",
    "revset": args.revset,
    "commit_count": len(commits),
    "commits": commits,
    "trailers": trailers,
    "diff_summary": diff_summary,
  }

  conn = connect_problems(args.db_problems)

  problem_tokens = []
  for t in trailers:
    p = t.get("trailers", {}).get(TRAILER_PROBLEM)
    if p:
      problem_tokens.append(str(p))
  problem_tokens = list(dict.fromkeys(problem_tokens))

  resolved_ids: list[str] = []
  for tok in problem_tokens:
    rid = resolve_problem_id(conn, tok)
    if rid:
      resolved_ids.append(rid)

  if resolved_ids:
    out["bound_problem_ids"] = resolved_ids
    out["status"] = "bound"
    if len(resolved_ids) == 1 and args.apply:
      problem_id = resolved_ids[0]
      relation = trailers[0].get("trailers", {}).get(TRAILER_RELATION, "addresses") if trailers else "addresses"
      progress = trailers[0].get("trailers", {}).get(TRAILER_PROGRESS, "") if trailers else ""
      add_progress_event(
        conn,
        problem_id=problem_id,
        revset=args.revset,
        commits=commits,
        relation=relation,
        progress=progress,
        diff_summary=diff_summary,
      )
      out["progress_logged"] = True
      out["human_message"] = f"Logged progress against problem {problem_id}."
    elif len(resolved_ids) > 1:
      out["ok"] = False
      out["status"] = "ambiguous"
      out["error"] = "multiple_bound_problems"
      out["human_message"] = "Multiple problem bindings found in stack trailers."
  else:
    suggestion = suggest_problem_statement(commits, diff_summary)
    out["status"] = "unbound"
    out["suggested_problem_statement"] = suggestion
    out["suggested_trailer"] = build_trailer_block("<problem_id>", "addresses", "short progress note")
    if args.create_problem:
      pid = create_problem(conn, suggestion)
      out["created_problem_id"] = pid
      out["suggested_trailer"] = build_trailer_block(pid, "addresses", "short progress note")
      out["human_message"] = f"Created problem {pid}."

  return out


def cmd_trailer(args) -> dict[str, Any]:
  return {
    "ok": True,
    "action": "trailer_template",
    "trailer": build_trailer_block(args.problem_id, args.relation, args.progress),
  }


def main(argv: list[str]) -> int:
  parser = argparse.ArgumentParser(description="Project manager skill")

  sub = parser.add_subparsers(dest="cmd", required=True)

  review = sub.add_parser("review", help="Review stack diff and bind/suggest problem")
  review.add_argument("--pretty", action="store_true")
  review.add_argument("--revset", default="trunk()..@")
  review.add_argument("--db-problems", default=default_problems_db())
  review.add_argument("--apply", action="store_true", help="Write progress event when a single binding exists")
  review.add_argument("--create-problem", action="store_true", help="Create a suggested problem when no binding found")

  trailer = sub.add_parser("trailer", help="Print trailer block template")
  trailer.add_argument("--problem-id", required=True)
  trailer.add_argument("--relation", default="addresses")
  trailer.add_argument("--progress", default=None)
  trailer.add_argument("--pretty", action="store_true")

  args = parser.parse_args(argv)

  try:
    if args.cmd == "review":
      out = cmd_review(args)
    elif args.cmd == "trailer":
      out = cmd_trailer(args)
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
