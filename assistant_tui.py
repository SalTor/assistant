#!/usr/bin/env python3
"""Keyboard-first TUI for assistant_cli.py with notes/tasks/problems panels."""

from __future__ import annotations

import argparse
import curses
import json
import os
import subprocess
import sys
import textwrap
from dataclasses import dataclass
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parent
CLI_PATH = ROOT / "assistant_cli.py"


def _default_data_dir() -> Path:
  xdg_data_home = os.getenv("XDG_DATA_HOME")
  if xdg_data_home:
    return Path(xdg_data_home).expanduser() / "assistant"
  return Path.home() / ".local" / "share" / "assistant"


@dataclass
class UiItem:
  item_id: str
  text: str
  status: str = ""


def _hex_to_curses_rgb(hex_color: str) -> tuple[int, int, int]:
  hex_color = hex_color.lstrip("#")
  r = int(hex_color[0:2], 16)
  g = int(hex_color[2:4], 16)
  b = int(hex_color[4:6], 16)
  return (round(r / 255 * 1000), round(g / 255 * 1000), round(b / 255 * 1000))


class AssistantTUI:
  def __init__(self, stdscr, db_notes: str, db_tasks: str, db_problems: str, tz: str | None):
    self.stdscr = stdscr
    self.db_notes = db_notes
    self.db_tasks = db_tasks
    self.db_problems = db_problems
    self.tz = tz

    self.focus = "dashboard"  # dashboard | notes | tasks | problems
    self.show_help = False
    self.input_mode: str | None = None  # add_note | add_task | add_problem
    self.input_parent_problem_id: str | None = None
    self.input_buf = ""
    self.status = "Ready"
    self.pending_d = False
    self.show_problem_detail = False
    self.problem_detail: dict[str, Any] | None = None
    self.problem_detail_link_index = 0
    self.show_link_picker = False
    self.link_source_type: str | None = None
    self.link_source_id: str | None = None
    self.link_problem_index = 0
    self.link_relations = ["addresses", "evidence", "critique", "depends_on"]
    self.link_relation_index = 0
    self.link_label_cache: dict[tuple[str, str], str] = {}

    self.notes: list[UiItem] = []
    self.tasks: list[UiItem] = []
    self.problems: list[UiItem] = []
    self.panel_boxes: dict[str, tuple[int, int, int, int]] = {}
    self.note_index = 0
    self.task_index = 0
    self.problem_index = 0

  def run(self) -> None:
    try:
      curses.set_escdelay(25)
    except Exception:
      pass
    curses.curs_set(0)
    self.stdscr.nodelay(False)
    self.stdscr.keypad(True)
    try:
      curses.mousemask(curses.ALL_MOUSE_EVENTS | curses.REPORT_MOUSE_POSITION)
      curses.mouseinterval(0)
    except Exception:
      pass
    self._setup_colors()
    self._ensure_db()
    self.refresh_data()

    while True:
      self._draw()
      ch = self.stdscr.getch()
      if not self._handle_key(ch):
        break

  def _setup_colors(self) -> None:
    curses.start_color()
    curses.use_default_colors()

    # Catppuccin Frappé palette
    # Base: #303446, Surface0: #414559, Text: #c6d0f5
    # Blue: #8caaee, Green: #a6d189, Yellow: #e5c890, Red: #e78284, Mauve: #ca9ee6
    can_redef = curses.can_change_color() and curses.COLORS >= 32

    if can_redef:
      palette = {
        16: "#303446",  # base
        17: "#414559",  # surface0
        18: "#c6d0f5",  # text
        19: "#8caaee",  # blue
        20: "#a6d189",  # green
        21: "#e5c890",  # yellow
        22: "#e78284",  # red
        23: "#ca9ee6",  # mauve
      }
      for idx, hex_code in palette.items():
        r, g, b = _hex_to_curses_rgb(hex_code)
        curses.init_color(idx, r, g, b)

      c_base, c_surface, c_text = 16, 17, 18
      c_blue, c_green, c_yellow, c_red, c_mauve = 19, 20, 21, 22, 23
    else:
      c_base, c_surface, c_text = curses.COLOR_BLACK, curses.COLOR_BLACK, curses.COLOR_WHITE
      c_blue, c_green, c_yellow, c_red, c_mauve = (
        curses.COLOR_BLUE,
        curses.COLOR_GREEN,
        curses.COLOR_YELLOW,
        curses.COLOR_RED,
        curses.COLOR_MAGENTA,
      )

    # Keep terminal background transparent (-1) so theme blends with your terminal.
    curses.init_pair(1, c_text, -1)      # default
    curses.init_pair(2, c_blue, -1)      # title
    curses.init_pair(3, c_surface, -1)   # border
    curses.init_pair(4, c_mauve, -1)     # focused border
    curses.init_pair(5, c_green, -1)     # success
    curses.init_pair(6, c_yellow, -1)    # hint
    curses.init_pair(7, c_red, -1)       # error

  def _run_cli(self, args: list[str]) -> tuple[int, dict[str, Any] | None, str]:
    full_args = list(args)
    if self.tz and "--tz" not in full_args:
      full_args.extend(["--tz", self.tz])
    cmd = [sys.executable, str(CLI_PATH), *full_args]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    stdout = (proc.stdout or "").strip()
    stderr = (proc.stderr or "").strip()

    payload = None
    if stdout:
      try:
        payload = json.loads(stdout)
      except json.JSONDecodeError:
        payload = None

    msg = ""
    if payload and isinstance(payload, dict):
      msg = payload.get("human_message") or payload.get("message") or ""
      if not msg and payload.get("ok") is False:
        msg = payload.get("error") or "Command failed"
    if not msg:
      msg = stderr or stdout or f"Exited {proc.returncode}"

    return proc.returncode, payload, msg

  def _ensure_db(self) -> None:
    for domain, db in (("notes", self.db_notes), ("tasks", self.db_tasks), ("problems", self.db_problems)):
      self._run_cli([domain, "init", "--db", db])

  def refresh_data(self) -> None:
    _, payload_n, msg_n = self._run_cli(["notes", "list", "--db", self.db_notes])
    _, payload_t, msg_t = self._run_cli(["tasks", "list", "--db", self.db_tasks])
    _, payload_p, msg_p = self._run_cli(["problems", "tree", "--db", self.db_problems])

    self.notes = []
    if isinstance(payload_n, dict) and payload_n.get("ok"):
      for n in payload_n.get("data", []):
        self.notes.append(UiItem(item_id=n.get("id", "?"), text=n.get("body", ""), status=n.get("status", "")))

    self.tasks = []
    if isinstance(payload_t, dict) and payload_t.get("ok"):
      for t in payload_t.get("data", []):
        self.tasks.append(UiItem(item_id=t.get("id", "?"), text=t.get("title", ""), status=t.get("status", "")))

    self.problems = []
    if isinstance(payload_p, dict) and payload_p.get("ok"):
      for p in payload_p.get("data", []):
        depth = int(p.get("depth", 0) or 0)
        indent = "  " * max(0, depth)
        title = p.get("title", "")
        self.problems.append(UiItem(item_id=p.get("id", "?"), text=f"{indent}{title}", status=p.get("status", "")))

    self.note_index = min(self.note_index, max(0, len(self.notes) - 1))
    self.task_index = min(self.task_index, max(0, len(self.tasks) - 1))
    self.problem_index = min(self.problem_index, max(0, len(self.problems) - 1))

    self.status = "Refreshed"
    if not isinstance(payload_n, dict) or payload_n.get("ok") is False:
      self.status = f"Notes list issue: {msg_n}"
    elif not isinstance(payload_t, dict) or payload_t.get("ok") is False:
      self.status = f"Tasks list issue: {msg_t}"
    elif not isinstance(payload_p, dict) or payload_p.get("ok") is False:
      self.status = f"Problems list issue: {msg_p}"

  def _add_current(self) -> None:
    text = self.input_buf.strip()
    if not text:
      self.status = "Nothing entered"
      return

    if self.input_mode == "add_note":
      code, _, msg = self._run_cli(["notes", "invoke", "--db", self.db_notes, "--message", text])
      self.status = ("Added note. " if code == 0 else "Failed adding note. ") + msg
      self.focus = "notes"
    elif self.input_mode == "add_task":
      code, _, msg = self._run_cli(["tasks", "invoke", "--db", self.db_tasks, "--message", text])
      self.status = ("Added task. " if code == 0 else "Failed adding task. ") + msg
      self.focus = "tasks"
    elif self.input_mode == "add_problem":
      args = ["problems", "invoke", "--db", self.db_problems, "--message", text]
      if self.input_parent_problem_id:
        args.extend(["--parent-problem-id", self.input_parent_problem_id])
      code, _, msg = self._run_cli(args)
      self.status = ("Added problem. " if code == 0 else "Failed adding problem. ") + msg
      self.focus = "problems"

    self.input_mode = None
    self.input_parent_problem_id = None
    self.input_buf = ""
    self.refresh_data()

  def _complete_selected(self) -> None:
    if self.focus == "notes" and self.notes:
      target = self.notes[self.note_index].item_id
      code, _, msg = self._run_cli(["notes", "invoke", "--db", self.db_notes, "--note-id", target, "--message", "done"])
      self.status = ("Marked note done. " if code == 0 else "Failed to mark note. ") + msg
      self.refresh_data()
      return

    if self.focus == "tasks" and self.tasks:
      target = self.tasks[self.task_index].item_id
      code, _, msg = self._run_cli(["tasks", "invoke", "--db", self.db_tasks, "--task-id", target, "--message", "done"])
      self.status = ("Marked task done. " if code == 0 else "Failed to mark task. ") + msg
      self.refresh_data()
      return

    if self.focus == "problems" and self.problems:
      target = self.problems[self.problem_index].item_id
      code, _, msg = self._run_cli(["problems", "invoke", "--db", self.db_problems, "--problem-id", target, "--message", "solved"])
      self.status = ("Marked problem solved. " if code == 0 else "Failed to solve problem. ") + msg
      self.refresh_data()
      return

    self.status = "Nothing selected"

  def _current_link_relation(self) -> str:
    if not self.link_relations:
      return "addresses"
    if self.link_relation_index < 0 or self.link_relation_index >= len(self.link_relations):
      self.link_relation_index = 0
    return self.link_relations[self.link_relation_index]

  def _selected_problem_id(self, index: int | None = None) -> str | None:
    if not self.problems:
      return None
    idx = self.problem_index if index is None else index
    if idx < 0 or idx >= len(self.problems):
      return None
    return self.problems[idx].item_id

  def _resolve_link_label(self, entity_type: str, entity_id: str) -> str:
    key = (entity_type, entity_id)
    if key in self.link_label_cache:
      return self.link_label_cache[key]

    label = ""
    if entity_type == "note":
      _, payload, _ = self._run_cli(["notes", "history", "--db", self.db_notes, "--note-id", entity_id])
      if isinstance(payload, dict) and payload.get("ok") and isinstance(payload.get("note"), dict):
        label = str(payload["note"].get("body") or "")
    elif entity_type == "task":
      _, payload, _ = self._run_cli(["tasks", "history", "--db", self.db_tasks, "--task-id", entity_id])
      if isinstance(payload, dict) and payload.get("ok") and isinstance(payload.get("task"), dict):
        label = str(payload["task"].get("title") or "")
    elif entity_type == "problem":
      _, payload, _ = self._run_cli(["problems", "show", "--db", self.db_problems, "--problem-id", entity_id])
      if isinstance(payload, dict) and payload.get("ok") and isinstance(payload.get("problem"), dict):
        label = str(payload["problem"].get("title") or "")

    if not label:
      label = "(not found)"
    self.link_label_cache[key] = label
    return label

  def _load_problem_detail(self, problem_id: str) -> bool:
    code, payload, msg = self._run_cli(["problems", "show", "--db", self.db_problems, "--problem-id", problem_id])
    if code != 0 or not isinstance(payload, dict) or payload.get("ok") is False:
      self.status = f"Failed loading problem detail: {msg}"
      return False
    self.problem_detail = payload
    return True

  def _detail_links(self) -> list[dict[str, str]]:
    if not isinstance(self.problem_detail, dict):
      return []
    links = self.problem_detail.get("links")
    out: list[dict[str, str]] = []
    if isinstance(links, list):
      for link in links:
        if not isinstance(link, dict):
          continue
        out.append(
          {
            "entity_type": str(link.get("entity_type", "")),
            "entity_id": str(link.get("entity_id", "")),
            "relation": str(link.get("relation", "addresses")).strip().lower() or "addresses",
          }
        )
    return out

  def _ordered_detail_links(self) -> list[dict[str, str]]:
    raw = self._detail_links()
    if not raw:
      return []
    relation_order = ["addresses", "evidence", "critique", "depends_on"]
    grouped: dict[str, list[dict[str, str]]] = {}
    for link in raw:
      grouped.setdefault(link["relation"], []).append(link)

    ordered: list[dict[str, str]] = []
    for rel in relation_order:
      ordered.extend(grouped.get(rel, []))
    for rel, links in grouped.items():
      if rel in relation_order:
        continue
      ordered.extend(links)
    return ordered

  def _unlink_selected_detail_link(self) -> None:
    if not isinstance(self.problem_detail, dict):
      self.status = "No problem detail loaded"
      return
    problem = self.problem_detail.get("problem")
    if not isinstance(problem, dict):
      self.status = "No problem selected"
      return

    links = self._ordered_detail_links()
    if not links:
      self.status = "No links to remove"
      return

    self.problem_detail_link_index = max(0, min(self.problem_detail_link_index, len(links) - 1))
    target = links[self.problem_detail_link_index]
    pid = str(problem.get("id", ""))
    code, _, msg = self._run_cli(
      [
        "problems",
        "unlink",
        "--db",
        self.db_problems,
        "--problem-id",
        pid,
        "--entity-type",
        target["entity_type"],
        "--entity-id",
        target["entity_id"],
        "--relation",
        target["relation"],
      ]
    )
    if code != 0:
      self.status = f"Failed unlink: {msg}"
      return

    self.status = f"Unlinked {target['entity_type']} {target['entity_id'][:4]} [{target['relation']}]"
    if self._load_problem_detail(pid):
      new_links = self._ordered_detail_links()
      if new_links:
        self.problem_detail_link_index = min(self.problem_detail_link_index, len(new_links) - 1)
      else:
        self.problem_detail_link_index = 0

  def _problem_descendants(self, problem_id: str) -> list[tuple[int, str, str, str]]:
    code, payload, _ = self._run_cli(["problems", "tree", "--db", self.db_problems])
    if code != 0 or not isinstance(payload, dict) or payload.get("ok") is False:
      return []
    rows = payload.get("data")
    if not isinstance(rows, list):
      return []

    by_parent: dict[str | None, list[dict[str, Any]]] = {}
    for r in rows:
      if isinstance(r, dict):
        by_parent.setdefault(r.get("parent_id"), []).append(r)

    out: list[tuple[int, str, str, str]] = []

    def walk(parent_id: str, depth: int) -> None:
      for r in by_parent.get(parent_id, []):
        rid = str(r.get("id", "?"))
        title = str(r.get("title", ""))
        status = str(r.get("status", ""))
        out.append((depth, rid, title, status))
        walk(rid, depth + 1)

    walk(problem_id, 0)
    return out

  def _open_problem_detail(self) -> None:
    pid = self._selected_problem_id()
    if not pid:
      self.status = "No problem selected"
      return
    if not self._load_problem_detail(pid):
      return
    self.problem_detail_link_index = 0
    self.show_problem_detail = True

  def _open_link_picker(self) -> None:
    if not self.problems:
      self.status = "No problems available to link against"
      return

    if self.focus == "notes" and self.notes:
      self.link_source_type = "note"
      self.link_source_id = self.notes[self.note_index].item_id
    elif self.focus == "tasks" and self.tasks:
      self.link_source_type = "task"
      self.link_source_id = self.tasks[self.task_index].item_id
    else:
      self.status = "Focus a note/task row to link"
      return

    self.link_problem_index = min(self.problem_index, max(0, len(self.problems) - 1))
    self.show_link_picker = True

  def _submit_link_from_picker(self) -> None:
    pid = self._selected_problem_id(self.link_problem_index)
    if not pid or not self.link_source_type or not self.link_source_id:
      self.status = "Unable to link: missing selection"
      self.show_link_picker = False
      return

    relation = self._current_link_relation()
    code, _, msg = self._run_cli(
      [
        "problems",
        "link",
        "--db",
        self.db_problems,
        "--problem-id",
        pid,
        "--entity-type",
        self.link_source_type,
        "--entity-id",
        self.link_source_id,
        "--relation",
        relation,
      ]
    )
    prefix = f"Linked ({relation}) to [{pid[:4]}]. " if code == 0 else f"Failed link ({relation}). "
    self.status = prefix + msg
    self.show_link_picker = False
    self.link_source_type = None
    self.link_source_id = None

  def _handle_mouse(self) -> bool:
    try:
      _, mx, my, _, bstate = curses.getmouse()
    except Exception:
      return True

    if not (bstate & (curses.BUTTON1_PRESSED | curses.BUTTON1_CLICKED | curses.BUTTON1_RELEASED)):
      return True

    for panel, (y, x, h, w) in self.panel_boxes.items():
      if y <= my < y + h and x <= mx < x + w:
        self.focus = panel
        self.status = f"{panel.capitalize()} focus"
        return True

    return True

  def _handle_key(self, ch: int) -> bool:
    if self.show_problem_detail:
      if ch in (27, ord("q"), 10, 13):
        self.show_problem_detail = False
        self.problem_detail = None
        self.problem_detail_link_index = 0
        return True
      if ch in (ord("j"), curses.KEY_DOWN):
        links = self._ordered_detail_links()
        if links:
          self.problem_detail_link_index = min(self.problem_detail_link_index + 1, len(links) - 1)
        return True
      if ch in (ord("k"), curses.KEY_UP):
        links = self._ordered_detail_links()
        if links:
          self.problem_detail_link_index = max(self.problem_detail_link_index - 1, 0)
        return True
      if ch == ord("u"):
        self._unlink_selected_detail_link()
        return True
      return True

    if self.show_link_picker:
      if ch in (27, ord("q")):
        self.show_link_picker = False
        self.link_source_type = None
        self.link_source_id = None
        return True
      if ch in (ord("j"), curses.KEY_DOWN):
        self.link_problem_index = min(self.link_problem_index + 1, max(0, len(self.problems) - 1))
        return True
      if ch in (ord("k"), curses.KEY_UP):
        self.link_problem_index = max(self.link_problem_index - 1, 0)
        return True
      if ch in (ord("h"), curses.KEY_LEFT):
        self.link_relation_index = max(self.link_relation_index - 1, 0)
        return True
      if ch in (ord("l"), curses.KEY_RIGHT):
        self.link_relation_index = min(self.link_relation_index + 1, len(self.link_relations) - 1)
        return True
      if ch in (ord("1"), ord("2"), ord("3"), ord("4")):
        idx = ch - ord("1")
        if 0 <= idx < len(self.link_relations):
          self.link_relation_index = idx
        return True
      if ch in (10, 13):
        self._submit_link_from_picker()
        return True
      return True

    if self.show_help:
      if ch in (ord("?"), 27, ord("q")):
        self.show_help = False
      return True

    if self.input_mode:
      if ch in (27,):
        self.input_mode = None
        self.input_parent_problem_id = None
        self.input_buf = ""
        self.status = "Canceled input"
        return True
      if ch in (curses.KEY_BACKSPACE, 127, 8):
        self.input_buf = self.input_buf[:-1]
        return True
      if ch == 21:  # Ctrl-U
        self.input_buf = ""
        return True
      if ch == 23:  # Ctrl-W
        buf = self.input_buf.rstrip()
        while buf and not buf[-1].isspace():
          buf = buf[:-1]
        self.input_buf = buf.rstrip()
        return True
      if ch in (curses.KEY_ENTER, 10, 13):
        self._add_current()
        return True
      if 32 <= ch <= 126:
        self.input_buf += chr(ch)
      return True

    if self.pending_d:
      if ch == ord("d"):
        self.pending_d = False
        self._complete_selected()
        return True
      self.pending_d = False

    if ch == curses.KEY_MOUSE:
      return self._handle_mouse()

    if ch == ord("q"):
      return False
    if ch == ord("?"):
      self.show_help = True
      return True
    if ch in (ord("r"),):
      self.refresh_data()
      return True

    if ch in (ord("n"),):
      self.focus = "notes"
      self.status = "Notes focus"
      return True
    if ch in (ord("t"),):
      self.focus = "tasks"
      self.status = "Tasks focus"
      return True
    if ch in (ord("p"),):
      self.focus = "problems"
      self.status = "Problems focus"
      return True
    if ch == 27:
      self.focus = "dashboard"
      self.status = "Dashboard focus"
      return True

    if ch == ord("h"):
      if self.focus == "problems":
        self.focus = "tasks"
        self.status = "Tasks focus"
      elif self.focus == "tasks":
        self.focus = "notes"
        self.status = "Notes focus"
      elif self.focus == "notes":
        self.focus = "dashboard"
        self.status = "Dashboard focus"
      else:
        self.status = "Dashboard focus"
      return True

    if ch == ord("l"):
      if self.focus == "dashboard":
        self.focus = "notes"
        self.status = "Notes focus"
      elif self.focus == "notes":
        self.focus = "tasks"
        self.status = "Tasks focus"
      elif self.focus == "tasks":
        self.focus = "problems"
        self.status = "Problems focus"
      else:
        self.status = "Problems focus"
      return True

    if ch == ord("j"):
      if self.focus == "dashboard":
        self.focus = "notes"
        self.status = "Notes focus"
      elif self.focus == "notes" and self.notes:
        self.note_index = min(self.note_index + 1, len(self.notes) - 1)
      elif self.focus == "tasks" and self.tasks:
        self.task_index = min(self.task_index + 1, len(self.tasks) - 1)
      elif self.focus == "problems" and self.problems:
        self.problem_index = min(self.problem_index + 1, len(self.problems) - 1)
      return True

    if ch == ord("k"):
      if self.focus == "notes" and self.notes:
        self.note_index = max(self.note_index - 1, 0)
      elif self.focus == "tasks" and self.tasks:
        self.task_index = max(self.task_index - 1, 0)
      elif self.focus == "problems" and self.problems:
        self.problem_index = max(self.problem_index - 1, 0)
      return True

    if ch == ord("a"):
      if self.focus == "notes":
        self.input_mode = "add_note"
        self.input_parent_problem_id = None
        self.input_buf = ""
        self.status = "Add note: type and press Enter"
      elif self.focus == "tasks":
        self.input_mode = "add_task"
        self.input_parent_problem_id = None
        self.input_buf = ""
        self.status = "Add task: type and press Enter"
      elif self.focus == "problems":
        self.input_mode = "add_problem"
        self.input_parent_problem_id = None
        self.input_buf = ""
        self.status = "Add problem: type and press Enter"
      else:
        self.status = "Focus notes (n), tasks (t), or problems (p) before adding"
      return True

    if ch == ord("A"):
      if self.focus == "problems":
        self.input_mode = "add_problem"
        self.input_buf = ""
        self.input_parent_problem_id = self.problems[self.problem_index].item_id if self.problems else None
        if self.input_parent_problem_id:
          self.status = f"Add sub-problem under {(self.input_parent_problem_id or '')[:4]}: type and press Enter"
        else:
          self.status = "No problem selected; adding root problem"
      return True

    if ch in (10, 13) and self.focus == "problems":
      self._open_problem_detail()
      return True

    if ch == ord("L"):
      self._open_link_picker()
      return True

    if ch == ord("d"):
      if self.focus in {"notes", "tasks", "problems"}:
        self.pending_d = True
        self.status = "Press d again to mark selected item done"
      return True

    return True

  def _safe_add(self, y: int, x: int, text: str, attr: int = 0) -> None:
    h, w = self.stdscr.getmaxyx()
    if 0 <= y < h and 0 <= x < w:
      try:
        self.stdscr.addnstr(y, x, text, max(0, w - x - 1), attr)
      except curses.error:
        pass

  def _draw_box(self, y: int, x: int, h: int, w: int, title: str, focused: bool) -> None:
    border_attr = curses.color_pair(4 if focused else 3)
    title_attr = curses.color_pair(2) | curses.A_BOLD

    for i in range(w):
      self._safe_add(y, x + i, "─", border_attr)
      self._safe_add(y + h - 1, x + i, "─", border_attr)
    for i in range(h):
      self._safe_add(y + i, x, "│", border_attr)
      self._safe_add(y + i, x + w - 1, "│", border_attr)

    self._safe_add(y, x, "┌", border_attr)
    self._safe_add(y, x + w - 1, "┐", border_attr)
    self._safe_add(y + h - 1, x, "└", border_attr)
    self._safe_add(y + h - 1, x + w - 1, "┘", border_attr)

    self._safe_add(y, x + 2, f" {title} ", title_attr)

  def _render_items(self, y: int, x: int, h: int, w: int, items: list[UiItem], selected: int, focused: bool) -> None:
    body_h = h - 2
    if body_h <= 0:
      return

    if not items:
      self._safe_add(y + 1, x + 2, "(empty)", curses.color_pair(6))
      return

    start = 0
    if selected >= body_h:
      start = selected - body_h + 1
    visible = items[start : start + body_h]

    for i, it in enumerate(visible):
      row = y + 1 + i
      idx = start + i
      short_id = (it.item_id or "?")[:4]
      text = it.text or ""
      indent_len = len(text) - len(text.lstrip(" "))
      indent = text[:indent_len]
      body = text[indent_len:]
      base = f"{indent}[{short_id}] {body}"
      max_w = max(8, w - 3)
      line = base if len(base) <= max_w else (base[: max_w - 1] + "…")
      attr = curses.color_pair(1)
      if focused and idx == selected:
        attr |= curses.A_REVERSE | curses.A_BOLD
      self._safe_add(row, x + 1, line.ljust(max(1, w - 2)), attr)

  def _draw_help(self) -> None:
    h, w = self.stdscr.getmaxyx()
    box_h = min(20, h - 4)
    box_w = min(70, w - 4)
    y = (h - box_h) // 2
    x = (w - box_w) // 2

    self._draw_box(y, x, box_h, box_w, "Keybinds", True)

    # Clear interior so underlying panel content doesn't bleed through.
    for row in range(y + 1, y + box_h - 1):
      self._safe_add(row, x + 1, " " * max(1, box_w - 2), curses.color_pair(1))

    lines = [
      "Global:",
      "  ?      toggle this help",
      "  q      quit",
      "  r      refresh notes/tasks/problems",
      "  n/t/p  focus notes/tasks/problems panel",
      "  j      from dashboard -> notes",
      "  h/l    move focus left/right",
      "         (dashboard <- notes <-> tasks <-> problems)",
      "  ESC    back to dashboard focus",
      "",
      "In focused panel:",
      "  j / k  move selection down/up",
      "  a      add item in focused panel",
      "  A      in problems: add sub-problem under selection",
      "  Enter  in problems: open selected problem detail",
      "  L      in notes/tasks: open link picker",
      "  dd     mark selected item done/solved",
      "",
      "In add mode:",
      "  Enter  submit",
      "  Ctrl-W delete previous word",
      "  Ctrl-U clear prompt",
      "  ESC    cancel",
      "",
      "In problem detail:",
      "  j / k  navigate linked items",
      "  u      unlink selected item",
      "  Enter/ESC/q close",
    ]

    for i, line in enumerate(lines[: box_h - 2]):
      self._safe_add(y + 1 + i, x + 2, line, curses.color_pair(1))

  def _draw_problem_detail(self) -> None:
    if not self.problem_detail:
      return
    h, w = self.stdscr.getmaxyx()
    box_h = min(max(12, h - 6), h - 4)
    box_w = min(max(50, w - 8), w - 4)
    y = (h - box_h) // 2
    x = (w - box_w) // 2

    self._draw_box(y, x, box_h, box_w, "Problem detail", True)
    for row in range(y + 1, y + box_h - 1):
      self._safe_add(row, x + 1, " " * max(1, box_w - 2), curses.color_pair(1))

    problem = self.problem_detail.get("problem") if isinstance(self.problem_detail, dict) else None
    links = self.problem_detail.get("links") if isinstance(self.problem_detail, dict) else None
    if not isinstance(problem, dict):
      self._safe_add(y + 1, x + 2, "No problem data", curses.color_pair(7))
      return

    pid = str(problem.get("id", "?"))
    title = str(problem.get("title", ""))
    statement = str(problem.get("statement", ""))
    status = str(problem.get("status", ""))

    lines: list[str] = []
    lines.append(f"[{pid[:4]}] {title} ({status})")
    lines.append("statement:")
    wrapped_statement = textwrap.wrap(statement, width=max(12, box_w - 6)) or [""]
    lines.extend([f"  {s}" for s in wrapped_statement])

    parent_id_raw = problem.get("parent_id")
    parent_id = str(parent_id_raw).strip() if parent_id_raw else ""
    if parent_id:
      parent_title = self._resolve_link_label("problem", parent_id)
      parent_line = textwrap.shorten(f"[{parent_id[:4]}] {parent_title}", width=max(20, box_w - 8), placeholder="…")
      lines.append("")
      lines.append("Belongs to:")
      lines.append(f"  - {parent_line}")

    descendants = self._problem_descendants(pid)
    if descendants:
      lines.append("")
      lines.append("Sub-problems:")
      for depth, cid, ctitle, cstatus in descendants:
        indent = "  " * max(0, depth)
        item = textwrap.shorten(f"{indent}[{cid[:4]}] {ctitle} ({cstatus})", width=max(20, box_w - 8), placeholder="…")
        lines.append(f"  - {item}")

    relation_order = ["addresses", "evidence", "critique", "depends_on"]
    grouped: dict[str, list[tuple[str, str, str, str]]] = {}
    if isinstance(links, list):
      for link in links:
        rel = str(link.get("relation", "addresses")).strip().lower() or "addresses"
        et = str(link.get("entity_type", "?"))
        eid = str(link.get("entity_id", "?"))
        label = self._resolve_link_label(et, eid)
        item = textwrap.shorten(f"{et} {eid[:4]} — {label}", width=max(20, box_w - 8), placeholder="…")
        grouped.setdefault(rel, []).append((item, et, eid, rel))

    selectable: list[tuple[int, str, str, str]] = []  # line_idx, entity_type, entity_id, relation

    if grouped:
      lines.append("")
      for rel in relation_order:
        items = grouped.get(rel)
        if not items:
          continue
        lines.append(f"{rel.capitalize()}:")
        for item_text, et, eid, rel_val in items:
          lines.append(f"  - {item_text}")
          selectable.append((len(lines) - 1, et, eid, rel_val))
        lines.append("")

      # Include any custom relations not in the default order.
      for rel, items in grouped.items():
        if rel in relation_order:
          continue
        lines.append(f"{rel.capitalize()}:")
        for item_text, et, eid, rel_val in items:
          lines.append(f"  - {item_text}")
          selectable.append((len(lines) - 1, et, eid, rel_val))
        lines.append("")
    else:
      lines.append("")
      lines.append("No links yet.")
      lines.append("")

    lines.append("j/k select  u unlink  Enter/Esc/q close")

    if selectable:
      self.problem_detail_link_index = max(0, min(self.problem_detail_link_index, len(selectable) - 1))
      selected_line = selectable[self.problem_detail_link_index][0]
    else:
      selected_line = -1

    max_lines = box_h - 2
    for i, line in enumerate(lines[:max_lines]):
      attr = curses.color_pair(1)
      if i == selected_line:
        attr |= curses.A_REVERSE | curses.A_BOLD
      self._safe_add(y + 1 + i, x + 2, line, attr)

  def _draw_link_picker(self) -> None:
    h, w = self.stdscr.getmaxyx()
    box_h = min(max(14, h - 8), h - 4)
    box_w = min(max(56, w - 8), w - 4)
    y = (h - box_h) // 2
    x = (w - box_w) // 2

    self._draw_box(y, x, box_h, box_w, "Link picker", True)
    for row in range(y + 1, y + box_h - 1):
      self._safe_add(row, x + 1, " " * max(1, box_w - 2), curses.color_pair(1))

    src_type = self.link_source_type or "?"
    src_id = self.link_source_id or "?"
    src_label = self._resolve_link_label(src_type, src_id) if self.link_source_type and self.link_source_id else ""
    src_header = f"Source: {src_type} {src_id[:4]}"
    self._safe_add(y + 1, x + 2, src_header, curses.color_pair(1) | curses.A_BOLD)
    if src_label:
      src_body = textwrap.shorten(src_label, width=max(16, box_w - 6), placeholder="…")
      self._safe_add(y + 2, x + 2, f"  {src_body}", curses.color_pair(1))
    self._safe_add(y + 3, x + 2, "j/k select problem  h/l select relation  Enter link", curses.color_pair(6))

    rels = "  ".join(
      f"[{r}]" if i == self.link_relation_index else r
      for i, r in enumerate(self.link_relations)
    )
    self._safe_add(y + 4, x + 2, f"Relation: {rels}", curses.color_pair(1))

    self._safe_add(y + 5, x + 2, "Target problem:", curses.color_pair(1))

    list_h = max(1, box_h - 8)
    if not self.problems:
      self._safe_add(y + 6, x + 4, "(no problems)", curses.color_pair(6))
    else:
      start = 0
      if self.link_problem_index >= list_h:
        start = self.link_problem_index - list_h + 1
      visible = self.problems[start : start + list_h]
      for i, p in enumerate(visible):
        idx = start + i
        attr = curses.color_pair(1)
        if idx == self.link_problem_index:
          attr |= curses.A_REVERSE | curses.A_BOLD
        short_id = (p.item_id or "?")[:4]
        line = textwrap.shorten(f"[{short_id}] {p.text}", width=max(12, box_w - 8), placeholder="…")
        self._safe_add(y + 6 + i, x + 4, line.ljust(max(1, box_w - 8)), attr)

    self._safe_add(y + box_h - 2, x + 2, "Esc/q: cancel", curses.color_pair(6))

  def _draw(self) -> None:
    self.stdscr.erase()

    h, w = self.stdscr.getmaxyx()
    title = " Assistant TUI "

    self._safe_add(0, 0, " " * w, curses.color_pair(2) | curses.A_REVERSE)
    self._safe_add(0, 2, title, curses.color_pair(2) | curses.A_BOLD)
    self._safe_add(0, max(2, w - 14), "? keybinds", curses.color_pair(6) | curses.A_BOLD)

    top = 2
    bottom = h - 4
    panel_h = max(6, bottom - top + 1)
    panel_gap = 2
    total_inner = max(30, w - 4)
    panel_w = max(18, (total_inner - (2 * panel_gap)) // 3)

    x1 = 2
    x2 = x1 + panel_w + panel_gap
    x3 = x2 + panel_w + panel_gap

    notes_focused = self.focus == "notes"
    tasks_focused = self.focus == "tasks"
    problems_focused = self.focus == "problems"

    self.panel_boxes = {
      "notes": (top, x1, panel_h, panel_w),
      "tasks": (top, x2, panel_h, panel_w),
      "problems": (top, x3, panel_h, panel_w),
    }

    self._draw_box(top, x1, panel_h, panel_w, "Notes (n)", notes_focused)
    self._draw_box(top, x2, panel_h, panel_w, "Tasks (t)", tasks_focused)
    self._draw_box(top, x3, panel_h, panel_w, "Problems (p)", problems_focused)

    self._render_items(top, x1, panel_h, panel_w, self.notes, self.note_index, notes_focused)
    self._render_items(top, x2, panel_h, panel_w, self.tasks, self.task_index, tasks_focused)
    self._render_items(top, x3, panel_h, panel_w, self.problems, self.problem_index, problems_focused)

    status_attr = curses.color_pair(5)
    if "failed" in self.status.lower() or "issue" in self.status.lower():
      status_attr = curses.color_pair(7)
    self._safe_add(h - 3, 2, self.status, status_attr)

    if self.input_mode:
      if self.input_mode == "add_note":
        prompt = "add note> "
      elif self.input_mode == "add_task":
        prompt = "add task> "
      else:
        prompt = "add subproblem> " if self.input_parent_problem_id else "add problem> "
      self._safe_add(h - 2, 2, prompt + self.input_buf, curses.color_pair(1) | curses.A_BOLD)
      curses.curs_set(1)
      self.stdscr.move(h - 2, min(w - 2, 2 + len(prompt) + len(self.input_buf)))
    else:
      curses.curs_set(0)

    if self.show_help:
      self._draw_help()

    if self.show_problem_detail:
      self._draw_problem_detail()

    if self.show_link_picker:
      self._draw_link_picker()

    self.stdscr.refresh()


def main(argv: list[str]) -> int:
  parser = argparse.ArgumentParser(description="TUI for assistant_cli.py")
  data_dir = _default_data_dir()
  data_dir.mkdir(parents=True, exist_ok=True)
  parser.add_argument("--db-notes", default=str(data_dir / "notes.db"))
  parser.add_argument("--db-tasks", default=str(data_dir / "tasks.db"))
  parser.add_argument("--db-problems", default=str(data_dir / "problems.db"))
  parser.add_argument("--tz", default=None)
  args = parser.parse_args(argv)

  if not CLI_PATH.exists():
    print(f"Could not find assistant CLI at: {CLI_PATH}", file=sys.stderr)
    return 2

  def _run(stdscr) -> None:
    app = AssistantTUI(
      stdscr,
      db_notes=args.db_notes,
      db_tasks=args.db_tasks,
      db_problems=args.db_problems,
      tz=args.tz,
    )
    app.run()

  curses.wrapper(_run)
  return 0


if __name__ == "__main__":
  raise SystemExit(main(sys.argv[1:]))
