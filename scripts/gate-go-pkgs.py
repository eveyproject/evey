#!/usr/bin/env python3
"""Go packages the fast gate tests: the ones the outgoing changes can affect
directly, not the whole module.

`go test` on every package reruns, under -race, every package that IMPORTS a
changed one: a one-line change in app/user re-tests driving/http for minutes.
The fast gate tests the packages the change lands in instead, and leaves the
reverse dependencies to the pull request's CI, which runs the full suite on the
same commits. What still breaks at compile time in a dependent (a signature, a
test file) is caught by `go build ./...` and `go vet ./...`, which stay
whole-module.

A changed file selects:
    a .go file                    its package
    any other file in a package   that package (testdata, embedded assets)
    a file under a data tree      every package whose tests name that tree
                                  (i18n/, examples/, proto/ ... read by tests)
    go.mod, go.sum                everything (a dependency bump touches all)

Prints one package directory per line ("./internal/app/user"), or "./..." for
everything, or nothing when no Go-relevant file changed. Stdlib only.

    gate-go-pkgs.py [--base REV]     changes since REV (default: fork point with dev)
    gate-go-pkgs.py --selftest
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

GO_TREES = ("cmd/", "internal/", "tests/")
# Never selected: generated code; the Postgres adapter, which the gate runs
# separately against a real database (repo-itest-gate) and CI runs without -race;
# and the architecture guards, which read the whole tree and run on every gate
# without -race (ci-arch): 7 s there, 170 s under -race for nothing it can find.
EXCLUDED = ("gen/", "internal/adapters/driven/postgres", "internal/contractguard")
ALL = "./..."


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def base_rev() -> str:
    """Fork point with dev, where every work branch starts; master, then HEAD~1."""
    for ref in ("origin/dev", "origin/master"):
        try:
            return git("merge-base", ref, "HEAD").strip()
        except subprocess.CalledProcessError:
            continue
    return git("rev-parse", "HEAD~1").strip()


def changed_files(base: str) -> list[str]:
    """Tracked changes since base, committed or not, plus untracked files."""
    tracked = git("diff", "--name-only", base).splitlines()
    untracked = git("ls-files", "--others", "--exclude-standard").splitlines()
    return sorted({f for f in tracked + untracked if f})


def package_dir(path: str, is_pkg_dir) -> str | None:
    """Nearest directory at or above path's own that is a Go package, inside a
    Go tree. A testdata file belongs to the package holding the testdata dir."""
    d = os.path.dirname(path)
    while d and any((d + "/").startswith(t) for t in GO_TREES):
        if is_pkg_dir(d):
            return d
        d = os.path.dirname(d)
    return None


def select(files: list[str], is_pkg_dir, test_files_naming) -> list[str]:
    """The package dirs to test, or [ALL]. Pure: the filesystem comes in as
    is_pkg_dir(dir) and test_files_naming(tree) so the selftest can fake it."""
    pkgs: set[str] = set()
    for f in files:
        if f in ("go.mod", "go.sum"):
            return [ALL]
        if any((f + "/").startswith(t) for t in GO_TREES):
            d = package_dir(f, is_pkg_dir)
            if d:
                pkgs.add(d)
            continue
        tree = f.split("/", 1)[0]
        if "/" in f and not tree.startswith("."):
            pkgs.update(test_files_naming(tree))
    return sorted(p for p in pkgs if not any(p.startswith(x) for x in EXCLUDED))


def real_is_pkg_dir(d: str) -> bool:
    try:
        return any(name.endswith(".go") for name in os.listdir(d))
    except OSError:
        return False


def real_test_files_naming(tree: str) -> set[str]:
    """Package dirs with a _test.go file reaching the tree by path: a relative
    "../../<tree>/..." or a filepath.Join(..., "<tree>", ...). A bare "<tree>"
    string is not enough - a JSON field named "frontend" reads no file."""
    t = re.escape(tree)
    pattern = re.compile(r'\.\./' + t + r'["/]|Join\(.*"' + t + r'["/]')
    found: set[str] = set()
    for root in GO_TREES:
        for p in Path(root).rglob("*_test.go"):
            try:
                if pattern.search(p.read_text(errors="replace")):
                    found.add(str(p.parent))
            except OSError:
                continue
    return found


def selftest() -> int:
    pkg_dirs = {"internal/app/user", "internal/pkg/i18n", "internal/adapters/driven/pluginrunner",
                "internal/adapters/driven/postgres", "cmd/evey"}
    readers = {"i18n": {"internal/pkg/i18n"}, "examples": {"internal/adapters/driven/pluginrunner"}}
    cases = [
        (["internal/app/user/access_service.go"], ["internal/app/user"]),
        (["internal/app/user/testdata/x/fixture.json"], ["internal/app/user"]),
        (["i18n/fr/admin.yaml"], ["internal/pkg/i18n"]),
        (["examples/plugins/weather/plugin.yaml", "cmd/evey/main.go"],
         ["cmd/evey", "internal/adapters/driven/pluginrunner"]),
        (["go.sum", "internal/app/user/a.go"], [ALL]),
        (["internal/adapters/driven/postgres/user_repo.go"], []),
        (["README.md", "docs/SECURITY.md", ".github/workflows/ci.yml"], []),
        (["internal/contractguard/route_auth_test.go"], []),
        (["frontend/apps/web/src/main.tsx"], []),
    ]
    failed = 0
    for files, want in cases:
        got = select(files, lambda d: d in pkg_dirs, lambda t: readers.get(t, set()))
        if got != want:
            print(f"selftest FAIL: {files} -> {got}, want {want}", file=sys.stderr)
            failed += 1
    if failed:
        return 1
    print(f"→ gate-go-pkgs selftest: {len(cases)} cases.")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--base")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()
    if args.selftest:
        return selftest()
    os.chdir(git("rev-parse", "--show-toplevel").strip())
    pkgs = select(changed_files(args.base or base_rev()), real_is_pkg_dir, real_test_files_naming)
    for p in pkgs:
        print(p if p == ALL else "./" + p)
    return 0


if __name__ == "__main__":
    sys.exit(main())
