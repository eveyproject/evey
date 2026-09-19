#!/usr/bin/env python3
"""Gate receipts: "the gate is green" becomes a claim anyone can re-check.

    scripts/gate-receipt.py run -- make pre-push    run it, record what happened
    scripts/gate-receipt.py status                  green for THIS tree? exit 0/1
    scripts/gate-receipt.py show                    the raw receipt

A receipt pins the exact command, its exit code, the HEAD it ran on and a
fingerprint of the working tree. `status` re-derives the last two from the repo
right now and refuses to say green when either moved: a gate that passed on a
different tree is a memory, not evidence.

Receipts live OUTSIDE the repo (~/.claude/projects/<slug>/gate/) - they are
working context, not a product artifact, and they have no business in a
`git status`, a PR or a docker build.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

SCHEMA = 1


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def repo_root() -> Path:
    return Path(git("rev-parse", "--show-toplevel").strip())


def receipts_dir(root: Path) -> Path:
    """Same slug convention as the rest of ~/.claude/projects."""
    slug = re.sub(r"[^A-Za-z0-9]", "-", str(root.resolve()))
    path = Path.home() / ".claude" / "projects" / slug / "gate"
    path.mkdir(parents=True, exist_ok=True)
    return path


def tree_fingerprint() -> tuple[str, str]:
    """(state, digest) of everything HEAD does not already pin.

    Tracked edits by content, untracked files by name - enough that any change
    an editor makes between the run and the claim breaks the match.
    """
    status = git("status", "--porcelain=v1")
    digest = hashlib.sha256((status + git("diff", "HEAD")).encode()).hexdigest()[:16]
    return ("clean" if not status.strip() else "dirty"), digest


def head_info() -> tuple[str, str, str]:
    return (
        git("rev-parse", "HEAD").strip(),
        git("log", "-1", "--format=%s").strip(),
        git("rev-parse", "--abbrev-ref", "HEAD").strip(),
    )


def cmd_run(args: argparse.Namespace) -> int:
    if not args.command:
        sys.exit("gate-receipt: nothing to run (usage: run -- make pre-push)")

    root = receipts_dir(repo_root())
    head, subject, branch = head_info()
    state, digest = tree_fingerprint()
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    log_path = root / f"{stamp}-{args.label}.log"

    started = time.time()
    print(f"→ gate-receipt: {' '.join(args.command)} on {head[:9]} ({state} tree, {digest})")
    with open(log_path, "w", encoding="utf-8") as log:
        proc = subprocess.Popen(
            args.command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1
        )
        assert proc.stdout is not None
        for line in proc.stdout:
            sys.stdout.write(line)
            log.write(line)
        exit_code = proc.wait()
    duration = round(time.time() - started, 1)

    # The tree may have moved WHILE the gate ran (a shared checkout, a build
    # writing into it). A receipt that pinned only the "before" state would
    # certify a tree the gate never saw.
    state_after, digest_after = tree_fingerprint()
    head_after = git("rev-parse", "HEAD").strip()

    receipt = {
        "schema": SCHEMA,
        "label": args.label,
        "command": args.command,
        "exit_code": exit_code,
        "verdict": "green" if exit_code == 0 else "red",
        "head": head,
        "head_subject": subject,
        "branch": branch,
        "tree_state": state,
        "tree_fingerprint": digest,
        "tree_moved_during_run": (head_after != head or digest_after != digest),
        "started_at": datetime.fromtimestamp(started, timezone.utc).isoformat(timespec="seconds"),
        "duration_s": duration,
        "host": socket.gethostname(),
        "log": str(log_path),
    }
    (root / f"{stamp}-{args.label}.json").write_text(json.dumps(receipt, indent=2) + "\n")
    (root / f"latest-{args.label}.json").write_text(json.dumps(receipt, indent=2) + "\n")

    verdict = "✅ green" if exit_code == 0 else "❌ red"
    print(f"\n{verdict} - exit {exit_code} in {duration}s on {head[:9]} ({state} tree)")
    if receipt["tree_moved_during_run"]:
        print("⚠️  the working tree moved while the gate ran - this receipt certifies nothing.")
    print(f"   receipt: {root}/{stamp}-{args.label}.json")
    return exit_code


def cmd_status(args: argparse.Namespace) -> int:
    path = receipts_dir(repo_root()) / f"latest-{args.label}.json"
    if not path.exists():
        print(f"❔ no {args.label} receipt yet - run `make gate` before claiming anything is green.")
        return 1
    receipt = json.loads(path.read_text())
    head, _, _ = head_info()
    state, digest = tree_fingerprint()

    reasons = []
    if receipt["exit_code"] != 0:
        reasons.append(f"the recorded run exited {receipt['exit_code']}")
    if receipt["head"] != head:
        reasons.append(f"it ran on {receipt['head'][:9]}, HEAD is now {head[:9]}")
    if receipt["tree_fingerprint"] != digest:
        reasons.append(f"the working tree changed since ({receipt['tree_state']}/{receipt['tree_fingerprint']} then, {state}/{digest} now)")
    if receipt.get("tree_moved_during_run"):
        reasons.append("the tree moved while that gate ran")

    print(f"   command : {' '.join(receipt['command'])}")
    print(f"   exit    : {receipt['exit_code']}")
    print(f"   head    : {receipt['head']} ({receipt['head_subject'][:60]})")
    print(f"   tree    : {receipt['tree_state']} / {receipt['tree_fingerprint']}")
    print(f"   when    : {receipt['started_at']} ({receipt['duration_s']}s, {receipt['host']})")
    print(f"   log     : {receipt['log']}")
    if reasons:
        print(f"\n❌ NOT green for the current state: {'; '.join(reasons)}.")
        return 1
    print("\n✅ green, and for exactly this HEAD and this working tree.")
    return 0


def cmd_show(args: argparse.Namespace) -> int:
    path = receipts_dir(repo_root()) / f"latest-{args.label}.json"
    if not path.exists():
        print(f"no {args.label} receipt yet")
        return 1
    print(path.read_text(), end="")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--label", default=os.environ.get("GATE_LABEL", "pre-push"))
    sub = parser.add_subparsers(dest="mode", required=True)
    run = sub.add_parser("run")
    run.add_argument("command", nargs=argparse.REMAINDER)
    run.set_defaults(func=cmd_run)
    sub.add_parser("status").set_defaults(func=cmd_status)
    sub.add_parser("show").set_defaults(func=cmd_show)

    args = parser.parse_args()
    if args.mode == "run" and args.command and args.command[0] == "--":
        args.command = args.command[1:]
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
