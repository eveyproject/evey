#!/usr/bin/env python3
"""Clean-checkout guard: what HEAD declares, HEAD must also contain.

Reads HEAD through git, never the working tree - a committed file pointing at an
uncommitted one resolves here (it is on disk, untracked) and breaks on a fresh
clone, in the docker build and in CI. Covers the frontend workspace and the
Makefile's scripts; go.mod needs none of this, it is always tracked.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import PurePosixPath

try:
    import yaml
except ModuleNotFoundError:
    sys.exit("clean-checkout guard: PyYAML is missing (pip install pyyaml) - refusing to skip.")

WORKSPACE_ROOT = "frontend"
LOCKFILE = f"{WORKSPACE_ROOT}/pnpm-lock.yaml"
WORKSPACE_YAML = f"{WORKSPACE_ROOT}/pnpm-workspace.yaml"
DEP_FIELDS = ("dependencies", "devDependencies", "optionalDependencies")
ENTRY_STRING_FIELDS = ("main", "module", "types", "typings")

FIX_LOCK = f"    -> run `pnpm install` in {WORKSPACE_ROOT}/ and commit the lockfile"


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def read_at_head(path: str) -> str:
    return git("show", f"HEAD:{path}")


def exports_targets(node) -> list[str]:
    """Relative paths in an `exports` tree. Wildcards name a set, not a file: skipped."""
    if isinstance(node, str):
        return [node] if node.startswith("./") and "*" not in node else []
    if isinstance(node, dict):
        return [t for v in node.values() for t in exports_targets(v)]
    if isinstance(node, list):
        return [t for v in node for t in exports_targets(v)]
    return []


def declared_entry_points(pkg: dict) -> list[str]:
    # A bare specifier ("expo-router/entry") resolves through node_modules, not here.
    out = [v for f in ENTRY_STRING_FIELDS if isinstance(v := pkg.get(f), str) and v.startswith("./")]
    binary = pkg.get("bin")
    if isinstance(binary, str):
        out += [binary] if binary.startswith("./") else []
    elif isinstance(binary, dict):
        out += [v for v in binary.values() if isinstance(v, str) and v.startswith("./")]
    return out + exports_targets(pkg.get("exports"))


def member_package_files(tracked: set[str], globs: list[str]) -> list[str]:
    out = [p for p in [f"{WORKSPACE_ROOT}/package.json"] if p in tracked]
    for path in sorted(tracked):
        if not path.startswith(f"{WORKSPACE_ROOT}/") or not path.endswith("/package.json"):
            continue
        rel = PurePosixPath(path).parent.relative_to(WORKSPACE_ROOT).as_posix()
        if any(PurePosixPath(rel).match(g) for g in globs):
            out.append(path)
    return out


def check_lockfile(tracked: set[str], parsed: dict[str, dict]) -> list[str]:
    if LOCKFILE not in tracked:
        return [f"{LOCKFILE} is not committed"]
    lock = yaml.safe_load(read_at_head(LOCKFILE)) or {}
    importers = lock.get("importers") or {}
    # A root `overrides:` entry replaces the specifier pnpm records for every
    # importer, so the lockfile legitimately differs from the package.json there.
    # Scoped keys ("foo@1>bar") name a dependency path: not compared.
    overrides = {
        name: spec
        for name, spec in (lock.get("overrides") or {}).items()
        if isinstance(spec, str) and ">" not in name and "@" not in name.lstrip("@")
    }
    errors: list[str] = []
    for path, pkg in parsed.items():
        rel = PurePosixPath(path).parent.relative_to(WORKSPACE_ROOT).as_posix()
        declared = {d: s for f in DEP_FIELDS for d, s in (pkg.get(f) or {}).items()}
        entry = importers.get(rel)
        if entry is None:
            # pnpm omits a member with no dependencies at all.
            if declared:
                errors.append(f"{LOCKFILE}: no entry for {rel}, which declares {len(declared)} dependencies\n{FIX_LOCK}")
            continue
        locked = {d: (m or {}).get("specifier") for f in DEP_FIELDS for d, m in (entry.get(f) or {}).items()}
        for dep, spec in declared.items():
            if dep not in locked:
                errors.append(f"{LOCKFILE}: {rel} declares {dep}@{spec}, the lockfile does not know it\n{FIX_LOCK}")
            elif locked[dep] != overrides.get(dep, spec):
                errors.append(f"{LOCKFILE}: {rel} declares {dep}@{spec}, the lockfile pins {locked[dep]}\n{FIX_LOCK}")
        for dep in locked.keys() - declared.keys():
            errors.append(f"{LOCKFILE}: {rel} carries a stale {dep} the package.json no longer declares\n{FIX_LOCK}")
    return errors


def check_makefile_scripts(tracked: set[str]) -> list[str]:
    """scripts/* is gitignored bar explicit re-includes, so a target can call a
    script nobody committed - `make` then dies on a fresh clone."""
    if "Makefile" not in tracked:
        return ["Makefile is not committed"]
    referenced = set(re.findall(r"scripts/[\w./-]+\.(?:sh|py|go)", read_at_head("Makefile")))
    return [
        f"Makefile: calls {path}, which is not committed\n"
        f"    -> add `!{path}` to .gitignore, then commit it"
        for path in sorted(referenced - tracked)
    ]


def main() -> int:
    tracked = set(git("ls-tree", "-r", "--name-only", "HEAD").splitlines())
    if WORKSPACE_YAML not in tracked:
        sys.exit(f"clean-checkout guard: {WORKSPACE_YAML} is not committed")
    globs = list((yaml.safe_load(read_at_head(WORKSPACE_YAML)) or {}).get("packages") or [])
    members = member_package_files(tracked, globs)
    # A guard that finds nothing to check must FAIL, not pass (same doctrine as
    # transcode-itest-gate).
    if len(members) < 2:
        sys.exit(f"clean-checkout guard: found {len(members)} workspace package(s) - broken guard, not a pass.")

    parsed = {path: json.loads(read_at_head(path)) for path in members}
    provided = {pkg["name"] for pkg in parsed.values() if pkg.get("name")}

    errors: list[str] = []
    for path, pkg in parsed.items():
        pkg_dir = PurePosixPath(path).parent
        for field in DEP_FIELDS:
            for dep, spec in (pkg.get(field) or {}).items():
                if isinstance(spec, str) and spec.startswith("workspace:") and dep not in provided:
                    errors.append(
                        f"{path}: depends on {dep} ({spec}) but no committed package provides it\n"
                        f"    -> commit that package (its package.json at least), or drop the dependency"
                    )
        for target in declared_entry_points(pkg):
            resolved = (pkg_dir / target).as_posix()
            if resolved not in tracked:
                errors.append(
                    f"{path}: declares {target!r} but {resolved} is not committed\n"
                    f"    -> commit that file, or remove the entry point"
                )
    errors += check_lockfile(tracked, parsed)
    errors += check_makefile_scripts(tracked)

    if errors:
        print("❌ clean-checkout guard: HEAD declares things HEAD does not contain.")
        print("   A fresh clone, the docker build and CI would all fail here.\n")
        for err in errors:
            print(f"  - {err}")
        print(f"\n  {len(errors)} problem(s). Commit what is missing, then re-run.")
        return 1

    print(f"→ clean-checkout guard: {len(parsed)} workspace packages, every dependency, entry point and lockfile row committed.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
