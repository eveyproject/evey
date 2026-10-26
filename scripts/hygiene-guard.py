#!/usr/bin/env python3
"""Hygiene guard: the house rules a regex can decide, checked mechanically.

Three entry points over one rule set, so the same definition serves the gate,
the editor hook and the commit-msg hook:

    (no args)          the commits about to be pushed (BASE..HEAD) + repo-wide
    --paths A B ...    the working-tree content of those files
    --commit-msg FILE  a commit message

Added lines only: the outgoing commits are judged, never the history behind
them, exactly like the gosec/lint ratchets. Rules needing judgement (is this
comment filler? is this abstraction premature?) are NOT here - a gate that
guesses trains people to ignore it.
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from fnmatch import fnmatch
from pathlib import Path

REPO_CLAUDE_DIR = ".claude/"

# Escape hatch for the line that means the character, not the style slip: a
# character class stripping dashes, a doc quoting one. Explicit and greppable.
ALLOW_MARKER = "hygiene-guard:allow"

# This file necessarily contains every pattern it hunts - the rules themselves
# and the selftest fixtures. The selftest is what proves those rules still fire.
SELF = "scripts/hygiene-guard.py"


@dataclass(frozen=True)
class Rule:
    name: str
    pattern: re.Pattern[str]
    message: str
    # Paths exempt from this rule (fnmatch against the repo-relative path).
    skip: tuple[str, ...] = ()
    # False for rules that only make sense inside the repo's own files.
    on_commit_msg: bool = True


RULES: tuple[Rule, ...] = (
    Rule(
        name="em-dash",
        pattern=re.compile("[–—]"),
        message="em/en dash - the house style is a plain '-' everywhere (code, UI, docs, commits)",
        skip=(
            "LICENSE.md",
            "CLA.md",
            "go.sum",
            "*pnpm-lock.yaml",
            "*package-lock.json",
            "*.swagger.json",
            "docs/themes/*",
            "i18n/zh/*",  # The dash is CJK punctuation there, not a typographic slip.
        ),
    ),
    Rule(
        name="ai-attribution",
        pattern=re.compile(
            r"Co-Authored-By:\s*Claude|Generated with \[Claude Code\]|"
            r"noreply@anthropic\.com|claude\.ai/code/session",
            re.IGNORECASE,
        ),
        message="AI attribution trailer - this repo carries none, in code as in commit messages",
    ),
    Rule(
        name="conflict-marker",
        pattern=re.compile(r"^(<{7}|>{7})[ \t]"),
        message="unresolved merge conflict marker",
    ),
    Rule(
        name="dated-line",
        pattern=re.compile(r"\b20\d{2}-[01]\d(-[0-3]\d)?\b"),
        message="a calendar date - the context behind a change belongs in the notes, not in a public tree",
        skip=(
            "docs/*",
            "*.md",
            "*.json",
            "*.sum",
            "*.lock",
            "*pnpm-lock.yaml",
            "*/generated/*",
            "*/migrations/*",
            "i18n/*",
            "*.sql",
            "*.csv",
            "scripts/hygiene-guard.py",
        ),
    ),
    Rule(
        name="elision",
        pattern=re.compile(
            r"\.\.\.\s*(existing|rest of|unchanged|same as)|"
            r"(existing code|rest of (the )?(file|function|code))\s*\.\.\.",
            re.IGNORECASE,
        ),
        message="elision placeholder - a patch that says '... existing code ...' deletes it",
    ),
)


# Private names purged before the repository went public (people, hosts,
# hardware, library titles, a leaked password). Kept as sha256 of the lowercase
# token so the words themselves never re-enter the tree; see the file's header
# to add one. Tokens are letters, digits, dots and dashes, so a hostname or a
# model number is one token.
DENYLIST_FILE = "scripts/hygiene-denylist.sha256"
DENYLIST_RULE = "private-name"
TOKEN = re.compile(r"[a-z0-9][a-z0-9.\-]*[a-z0-9]|[a-z0-9]")


_DENYLIST: frozenset[str] | None = None


def denylist() -> frozenset[str]:
    """Loaded once, next to this script, so it works from any cwd and any worktree."""
    global _DENYLIST
    if _DENYLIST is None:
        try:
            lines = (Path(__file__).resolve().parent.parent / DENYLIST_FILE).read_text(encoding="utf-8").splitlines()
        except OSError:
            lines = []
        _DENYLIST = frozenset(l.strip() for l in lines if l.strip() and not l.startswith("#"))
    return _DENYLIST


def private_name_hit(text: str) -> bool:
    import hashlib

    hashes = denylist()
    for tok in TOKEN.findall(text.lower()):
        for c in {tok, tok.strip(".-")}:
            if c and hashlib.sha256(c.encode()).hexdigest() in hashes:
                return True
    return False


@dataclass(frozen=True)
class Finding:
    rule: str
    location: str
    line: str
    message: str


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def base_rev() -> str:
    """What is already published on THIS branch, so only outgoing commits are judged.

    origin/<branch> comes first on purpose: beta-0.0.1 has no upstream configured,
    and merge-base with origin/master reaches back 1000+ commits of history this
    gate has no business re-judging.
    """
    if base := os.environ.get("HYGIENE_BASE", "").strip():
        return git("rev-parse", "--verify", f"{base}^{{commit}}").strip()
    # A detached worktree has no branch name: "origin/HEAD" would resolve to the
    # default branch and drag the whole divergence back in. The caller that
    # created the worktree knows the real base and passes HYGIENE_BASE.
    branch = git("rev-parse", "--abbrev-ref", "HEAD").strip()
    candidates = (
        ["merge-base", f"origin/{branch}", "HEAD"] if branch != "HEAD" else ["rev-parse", "HEAD~1"],
        ["merge-base", "@{u}", "HEAD"],
        ["merge-base", "origin/master", "HEAD"],
        ["rev-parse", "HEAD~1"],
    )
    for args in candidates:
        try:
            return git(*args).strip()
        except subprocess.CalledProcessError:
            continue
    return git("rev-parse", "HEAD").strip()


def scan_line(path: str, lineno: int | str, text: str, *, commit_msg: bool = False) -> list[Finding]:
    out: list[Finding] = []
    if ALLOW_MARKER in text or path.endswith(SELF):
        return out
    for rule in RULES:
        if commit_msg and not rule.on_commit_msg:
            continue
        if any(fnmatch(path, glob) for glob in rule.skip):
            continue
        if rule.pattern.search(text):
            out.append(Finding(rule.name, f"{path}:{lineno}", text.strip()[:160], rule.message))
    if not path.endswith(DENYLIST_FILE) and private_name_hit(text):
        out.append(
            Finding(
                DENYLIST_RULE,
                f"{path}:{lineno}",
                text.strip()[:160],
                "a word on the private-name denylist (people, hosts, hardware, library titles purged before publication)",
            )
        )
    return out


def scan_diff(base: str, head: str) -> list[Finding]:
    """Added lines of BASE..HEAD, with their real line numbers."""
    diff = git("diff", "--unified=0", "--no-color", "--diff-filter=ACMR", base, head)
    findings: list[Finding] = []
    path, lineno = "", 0
    for raw in diff.splitlines():
        if raw.startswith("+++ b/"):
            path, lineno = raw[6:], 0
        elif raw.startswith("@@"):
            if match := re.search(r"\+(\d+)", raw):
                lineno = int(match.group(1))
        elif raw.startswith("+") and not raw.startswith("+++") and path:
            findings += scan_line(path, lineno, raw[1:])
            lineno += 1
    return findings


def scan_commit_messages(base: str, head: str) -> list[Finding]:
    log = git("log", "--format=%H%x1f%B%x1e", f"{base}..{head}")
    findings: list[Finding] = []
    for entry in (e for e in log.split("\x1e") if e.strip()):
        sha, _, body = entry.strip().partition("\x1f")
        for i, line in enumerate(body.splitlines(), start=1):
            findings += scan_line(f"commit {sha[:9]}", i, line, commit_msg=True)
    return findings


def scan_tracked_paths(head: str) -> list[Finding]:
    """Rules about the shape of the tree, not the content of a line.

    On disk as well as in HEAD: .claude/ is gitignored here, so it can never be
    tracked - and the harness itself recreates it when a permission is granted
    (seen 2026-09-20). A rule that only watched the index would never fire.
    """
    message = "a .claude/ directory has no business in the repo - it belongs under ~/.claude/projects/"
    findings = [
        Finding("repo-claude-dir", path, "", message)
        for path in git("ls-tree", "-r", "--name-only", head).splitlines()
        if path.startswith(REPO_CLAUDE_DIR)
    ]
    root = Path(git("rev-parse", "--show-toplevel").strip())
    if (root / ".claude").exists():
        findings.append(
            Finding("repo-claude-dir", ".claude/ (untracked, on disk)", "", f"{message}\n      -> rm -rf .claude")
        )
    return findings


def scan_files(paths: list[str]) -> list[Finding]:
    findings: list[Finding] = []
    for path in paths:
        try:
            text = open(path, encoding="utf-8").read()
        except (OSError, UnicodeDecodeError):
            continue  # Binary or gone: nothing a line rule can say about it.
        rel = path
        try:
            root = git("rev-parse", "--show-toplevel").strip()
            rel = path[len(root) + 1 :] if path.startswith(root + "/") else path
        except subprocess.CalledProcessError:
            pass
        for i, line in enumerate(text.splitlines(), start=1):
            findings += scan_line(rel, i, line)
    return findings


def report(findings: list[Finding], scope: str) -> int:
    if not findings:
        print(f"→ hygiene guard: {scope} clean ({len(RULES) + 1} rules).")
        return 0
    print(f"❌ hygiene guard: {len(findings)} violation(s) in {scope}.\n")
    for f in findings:
        print(f"  - [{f.rule}] {f.location}\n      {f.line}\n      {f.message}")
    return 1


# (line, expected rule or None) - a guard nobody tests is a guard that rots into
# a no-op, and this one is cheap enough to prove on every run.
SELFTEST: tuple[tuple[str, str | None], ...] = (
    ("plain ascii - dash", None),
    ("typographic — dash", "em-dash"),
    ("en – dash", "em-dash"),
    (f"kept — on purpose {ALLOW_MARKER}", None),
    ("Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>", "ai-attribution"),
    ("Claude-Session: https://claude.ai/code/session_01ABC", "ai-attribution"),
    ("<<<<<<< HEAD", "conflict-marker"),
    (">>>>>>> theirs", "conflict-marker"),
    ("// ... existing code ...", "elision"),
    ("# rest of the file unchanged", None),
    ("    // ... rest of the function", "elision"),
    ("a <<<<<<< inside prose is not a marker", None),
    ("measured 2026-10-26 on a set", "dated-line"),
    ("since 2026-10 the rows page", "dated-line"),
    ("v1.2.3 shipped, port 2026", None),
    ("the canary hygiene-selftest-token must fire", DENYLIST_RULE),
    ("Hygiene-Selftest-Token, whatever the case", DENYLIST_RULE),
    ("a plain sentence with ordinary words", None),
)


def selftest() -> int:
    failures = []
    for text, expected in SELFTEST:
        got = {f.rule for f in scan_line("selftest", 0, text)}
        if (expected is None and got) or (expected is not None and expected not in got):
            failures.append(f"  - {text!r}: expected {expected}, got {sorted(got) or 'nothing'}")
    if failures:
        print(f"❌ hygiene guard selftest: {len(failures)} rule(s) misfire.")
        print("\n".join(failures))
        return 1
    print(f"→ hygiene guard selftest: {len(SELFTEST)} cases, {len(RULES) + 1} rules behaving.")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--paths", nargs="+", help="check these working-tree files instead of the diff")
    parser.add_argument("--commit-msg", help="check this commit message file")
    parser.add_argument("--selftest", action="store_true", help="prove the rules still fire, then exit")
    args = parser.parse_args()

    if args.selftest:
        return selftest()

    if args.paths:
        return report(scan_files(args.paths), f"{len(args.paths)} file(s)")

    if args.commit_msg:
        try:
            body = open(args.commit_msg, encoding="utf-8").read()
        except OSError as err:
            print(f"hygiene guard: cannot read {args.commit_msg}: {err}")
            return 1
        findings = [
            f
            for i, line in enumerate(body.splitlines(), start=1)
            if not line.startswith("#")
            for f in scan_line("commit message", i, line, commit_msg=True)
        ]
        return report(findings, "commit message")

    base, head = base_rev(), git("rev-parse", "HEAD").strip()
    findings = scan_diff(base, head) + scan_commit_messages(base, head) + scan_tracked_paths(head)
    return report(findings, f"{base[:9]}..{head[:9]}")


if __name__ == "__main__":
    sys.exit(main())
