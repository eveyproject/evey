#!/usr/bin/env python3
"""Regenerate frontend/apps/web/src/test/en-translations.json from i18n/en/*.yaml.

The web unit tests fetch the live English translations from the dev server
and fall back to this file when no server answers - which is every CI run.
A stale file means CI tests different strings than a developer's machine.
The flattening mirrors internal/pkg/i18n.flatten: file stem as prefix,
nested maps recursed, string values only. Keys are parsed as plain strings
(YAML 1.1 would read the country code NO as a boolean).

Usage: python3 scripts/i18n-fixture.py   (from the repo root)
The Go guard internal/pkg/i18n TestWebFixtureMatchesEnglishYAML fails until
this has been run after a change to i18n/en.
"""
import glob
import json
import os
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(ROOT, "i18n", "en")
DST = os.path.join(ROOT, "frontend", "apps", "web", "src", "test", "en-translations.json")
# The mobile app ships an embedded English catalogue as its floor: until the
# server's answer lands, every t() falls back to it. Hand-maintained, that floor
# held 74 keys out of 2878, so a cold start painted raw keys - the header search
# read "media.search.placeholder" on the home screen (emulator, 2026-09-05).
# Generated from the same source as the web fixture so the two cannot drift.
MOBILE_DST = os.path.join(ROOT, "frontend", "apps", "mobile", "src", "i18n", "fallback.en.json")


def flatten(prefix, mapping, out):
    for key, value in mapping.items():
        full = f"{prefix}.{key}" if prefix else str(key)
        if isinstance(value, str):
            out[full] = value
        elif isinstance(value, dict):
            flatten(full, value, out)


def main():
    out = {}
    for path in sorted(glob.glob(os.path.join(SRC, "*.yaml"))):
        stem = os.path.basename(path)[:-5]
        with open(path, encoding="utf-8") as fh:
            data = yaml.load(fh, Loader=yaml.BaseLoader) or {}
        flatten(stem, data, out)
    ordered = dict(sorted(out.items()))
    for dst in (DST, MOBILE_DST):
        with open(dst, "w", encoding="utf-8") as fh:
            json.dump(ordered, fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        print(f"{len(out)} keys -> {os.path.relpath(dst, ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
