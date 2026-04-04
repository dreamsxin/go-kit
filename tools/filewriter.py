#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
filewriter.py — AI-assisted file writer
========================================
Solves the problem of writing large or multi-line files on Windows where
shell heredocs are unavailable and IDE tools may truncate content.

MODES
-----
write   Write content to a file (overwrite).
append  Append content to an existing file.
patch   Apply one or more search-and-replace patches to a file.
read    Print file content (with optional line range).
check   Verify a file contains / does not contain expected strings.

USAGE
-----
# Write a file from a Python string literal in a helper script:
    python tools/filewriter.py write path/to/file.go --content-file tmp_content.py

# Write inline (small content):
    python tools/filewriter.py write path/to/file.txt --text "hello world"

# Append a block:
    python tools/filewriter.py append path/to/file.go --text "// added line"

# Patch (replace first occurrence of OLD with NEW):
    python tools/filewriter.py patch path/to/file.go \
        --old "foo := 1" --new "foo := 2"

# Multiple patches from a JSON file:
    python tools/filewriter.py patch path/to/file.go --patch-file patches.json

# Read lines 10-20:
    python tools/filewriter.py read path/to/file.go --start 10 --end 20

# Check content:
    python tools/filewriter.py check path/to/file.go \
        --contains "package main" --not-contains "TODO"

CONTENT FILE FORMAT (for --content-file)
-----------------------------------------
A plain .py file that assigns a variable named `content`:

    content = r\'\'\'
    package main
    ...
    \'\'\'

PATCH FILE FORMAT (for --patch-file)
--------------------------------------
A JSON array of {"old": "...", "new": "..."} objects:

    [
      {"old": "foo := 1", "new": "foo := 2"},
      {"old": "bar()", "new": "baz()"}
    ]
"""

import argparse
import json
import os
import sys
from pathlib import Path

# ── 强制 stdout/stderr 使用 UTF-8，解决 Windows GBK 乱码 ─────────────────────
if sys.stdout.encoding and sys.stdout.encoding.lower() != "utf-8":
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")


# ── colour helpers ────────────────────────────────────────────────────────────

RESET  = "\033[0m"
GREEN  = "\033[32m"
RED    = "\033[31m"
YELLOW = "\033[33m"
CYAN   = "\033[36m"

def _ok(msg):   print(f"{GREEN}[OK]{RESET}   {msg}")
def _fail(msg): print(f"{RED}[FAIL]{RESET} {msg}", file=sys.stderr)
def _info(msg): print(f"{CYAN}[..]{RESET}   {msg}")
def _warn(msg): print(f"{YELLOW}[!!]{RESET}   {msg}")

# ── content resolution ────────────────────────────────────────────────────────

def resolve_content(args) -> str:
    """Return the string content from --text or --content-file."""
    if args.text is not None:
        return args.text
    if args.content_file:
        cf = Path(args.content_file)
        if not cf.exists():
            _fail(f"content-file not found: {cf}")
            sys.exit(1)
        ns: dict = {}
        exec(cf.read_text(encoding="utf-8"), ns)  # noqa: S102
        if "content" not in ns:
            _fail(f"content-file must define a variable named 'content': {cf}")
            sys.exit(1)
        return ns["content"]
    _fail("provide --text or --content-file")
    sys.exit(1)

# ── commands ──────────────────────────────────────────────────────────────────

def cmd_write(args):
    content = resolve_content(args)
    path = Path(args.path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    lines = content.count("\n")
    _ok(f"wrote {len(content)} chars / {lines} lines → {path}")


def cmd_append(args):
    content = resolve_content(args)
    path = Path(args.path)
    if not path.exists():
        _fail(f"file does not exist (use 'write' to create): {path}")
        sys.exit(1)
    existing = path.read_text(encoding="utf-8")
    if existing and not existing.endswith("\n"):
        content = "\n" + content
    with path.open("a", encoding="utf-8") as f:
        f.write(content)
    _ok(f"appended {len(content)} chars → {path}")


def cmd_patch(args):
    path = Path(args.path)
    if not path.exists():
        _fail(f"file not found: {path}")
        sys.exit(1)

    # Build list of (old, new) pairs
    patches: list[tuple[str, str]] = []

    if args.patch_file:
        pf = Path(args.patch_file)
        if not pf.exists():
            _fail(f"patch-file not found: {pf}")
            sys.exit(1)
        data = json.loads(pf.read_text(encoding="utf-8"))
        for item in data:
            patches.append((item["old"], item["new"]))
    elif args.old is not None and args.new is not None:
        patches.append((args.old, args.new))
    else:
        _fail("patch requires --old/--new or --patch-file")
        sys.exit(1)

    content = path.read_text(encoding="utf-8")
    changed = 0
    for old, new in patches:
        if old not in content:
            _warn(f"patch target not found (skipped): {old[:60]!r}")
            continue
        content = content.replace(old, new, 1)
        changed += 1
        _ok(f"patched: {old[:50]!r} → {new[:50]!r}")

    if changed:
        path.write_text(content, encoding="utf-8")
        _ok(f"saved {changed} patch(es) → {path}")
    else:
        _warn("no patches applied")


def cmd_read(args):
    path = Path(args.path)
    if not path.exists():
        _fail(f"file not found: {path}")
        sys.exit(1)
    lines = path.read_text(encoding="utf-8").splitlines()
    start = (args.start or 1) - 1          # convert to 0-indexed
    end   = args.end if args.end else len(lines)
    subset = lines[start:end]
    for i, line in enumerate(subset, start=start + 1):
        print(f"{i:5d}  {line}")
    _info(f"showed lines {start+1}–{start+len(subset)} of {len(lines)} total")


def cmd_check(args):
    path = Path(args.path)
    if not path.exists():
        _fail(f"file not found: {path}")
        sys.exit(1)
    content = path.read_text(encoding="utf-8")
    errors = []

    for s in (args.contains or []):
        if s in content:
            _ok(f"contains: {s!r}")
        else:
            _fail(f"missing:  {s!r}")
            errors.append(s)

    for s in (args.not_contains or []):
        if s not in content:
            _ok(f"absent:   {s!r}")
        else:
            _fail(f"present (should be absent): {s!r}")
            errors.append(s)

    if errors:
        sys.exit(1)

# ── CLI ───────────────────────────────────────────────────────────────────────

def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="filewriter",
        description="AI-assisted file writer — safely write/patch files on Windows",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    sub = p.add_subparsers(dest="command", required=True)

    # ── write ──
    pw = sub.add_parser("write", help="Write (overwrite) a file")
    pw.add_argument("path", help="Target file path")
    pw.add_argument("--text", default=None, help="Inline content string")
    pw.add_argument("--content-file", default=None,
                    help="Python file defining a 'content' variable")

    # ── append ──
    pa = sub.add_parser("append", help="Append to an existing file")
    pa.add_argument("path", help="Target file path")
    pa.add_argument("--text", default=None, help="Inline content string")
    pa.add_argument("--content-file", default=None,
                    help="Python file defining a 'content' variable")

    # ── patch ──
    pp = sub.add_parser("patch", help="Search-and-replace patch a file")
    pp.add_argument("path", help="Target file path")
    pp.add_argument("--old", default=None, help="Text to replace")
    pp.add_argument("--new", default=None, help="Replacement text")
    pp.add_argument("--patch-file", default=None,
                    help="JSON file with [{old, new}, ...] patches")

    # ── read ──
    pr = sub.add_parser("read", help="Print file content (with optional line range)")
    pr.add_argument("path", help="File path")
    pr.add_argument("--start", type=int, default=None, help="Start line (1-indexed)")
    pr.add_argument("--end",   type=int, default=None, help="End line (inclusive)")

    # ── check ──
    pc = sub.add_parser("check", help="Verify file contains/lacks strings")
    pc.add_argument("path", help="File path")
    pc.add_argument("--contains",     nargs="*", default=[], metavar="STR",
                    help="Strings that must be present")
    pc.add_argument("--not-contains", nargs="*", default=[], metavar="STR",
                    help="Strings that must be absent")

    return p


def main():
    parser = build_parser()
    args = parser.parse_args()

    dispatch = {
        "write":  cmd_write,
        "append": cmd_append,
        "patch":  cmd_patch,
        "read":   cmd_read,
        "check":  cmd_check,
    }
    dispatch[args.command](args)


if __name__ == "__main__":
    main()
