#!/usr/bin/env python3
"""Assemble the public wiki from the repository documentation.

The site is NOT built straight out of `docs/`. `docs/` also holds working notes,
in-flight refactor plans and operator runbooks that have no business on a public
URL, so publishing the folder wholesale would leak by default. Instead the `nav`
of wiki/mkdocs.yml is the manifest: this script resolves every nav entry, copies
only those files into wiki/build/, lays the translations down beside them, and
refuses to go on if a published page trips the leak guard.

Three jobs, in order:

1. Copy. Each nav path resolves against wiki/content/ first (pages that belong
   to the site itself, like the landing page) then docs/ (the real source).
2. Translate. wiki/translations/<lang>/<path> lands as build/<path>.<lang>.md,
   which is the layout mkdocs-static-i18n reads. A page with no translation is
   simply absent, and the plugin falls back to English.
3. Guard. Stale translations and leaked hostnames/credentials fail --check.

A translation is published only when its front matter says `reviewed: true`:
a complete but unreviewed translation stays out, and the page falls back to
English. A missing page is better than a poor one.

A translation carries `source_sha` in its front matter: the sha256 of the
English file it was translated from. When the English page moves on, the sha
stops matching and the translation is stale - it still ships (a slightly old
page beats no page) but it gets a banner and --check reports it. Without this
the translations rot silently, which is the usual way multilingual docs die.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import re
import shutil
import sys
import unicodedata
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover - the CI image installs it
    sys.exit("pyyaml is required: pip install pyyaml")

WIKI = Path(__file__).resolve().parent
REPO = WIKI.parent
DOCS = REPO / "docs"
CONTENT = WIKI / "content"
TRANSLATIONS = WIKI / "translations"
BUILD = WIKI / "build"

# Kept in step with the mkdocs `languages` block and with the app's own
# catalogues (i18n/<lang>/), so the site speaks exactly what Evey speaks.
LANGUAGES = ["fr", "de", "es", "hi", "zh"]

# Non-markdown files the published pages link to. Globs are relative to docs/
# and, like the nav, they are an allow-list: nothing else in docs/ is copied.
# The OpenAPI specs are the API reference - the README pages around them are
# just an index, so publishing one without the other ships broken links.
ASSET_GLOBS = ["api/**/*.swagger.json"]
# Screenshots: binary, so copied as is rather than read for the leak scan.
IMAGE_GLOBS = ["images/**/*.png", "images/**/*.webp"]

# Brand files, copied from evey-brand/ (outside docs/) to build/assets/brand/.
# The site uses the real mark rather than a copy that would drift from
# evey-brand/, the single source of truth.
BRAND_FILES = {
    "evey-brand/favicon/favicon.svg": "assets/brand/favicon.svg",
    "evey-brand/svg/evey-wordmark-transparent-white.svg": "assets/brand/wordmark.svg",
}

STALE_BANNER = {
    "fr": "Cette traduction est en retard sur la version anglaise. En cas de doute, lisez la page en anglais.",
    "de": "Diese Übersetzung ist gegenüber der englischen Fassung veraltet. Im Zweifel lesen Sie die englische Seite.",
    "es": "Esta traducción está desactualizada respecto a la versión en inglés. En caso de duda, consulte la página en inglés.",
    "hi": "यह अनुवाद अंग्रेज़ी संस्करण से पुराना है। संदेह होने पर अंग्रेज़ी पृष्ठ पढ़ें।",
    "zh": "此翻译落后于英文版本。如有疑问，请阅读英文页面。",
}


DRAFT_BANNER = {
    "fr": "Brouillon non relu : cette page n'est pas encore publiée.",
    "de": "Unüberprüfter Entwurf: Diese Seite ist noch nicht veröffentlicht.",
    "es": "Borrador sin revisar: esta página aún no está publicada.",
    "hi": "बिना समीक्षा का मसौदा: यह पृष्ठ अभी प्रकाशित नहीं है।",
    "zh": "未审阅的草稿：此页面尚未发布。",
}


class YamlIgnoreUnknown(yaml.SafeLoader):
    """mkdocs.yml carries `!!python/name:` tags that SafeLoader rejects.

    We only need the nav, so unknown tags collapse to None instead of blowing up.
    """


YamlIgnoreUnknown.add_multi_constructor("", lambda loader, suffix, node: None)


def nav_paths(nav) -> list[str]:
    """Flatten the mkdocs nav into the list of markdown paths it references."""
    out: list[str] = []
    if isinstance(nav, str):
        out.append(nav)
    elif isinstance(nav, list):
        for item in nav:
            out.extend(nav_paths(item))
    elif isinstance(nav, dict):
        for value in nav.values():
            out.extend(nav_paths(value))
    return out


def source_for(path: str) -> Path | None:
    """wiki/content/ wins over docs/ so the site can override or add a page."""
    for root in (CONTENT, DOCS):
        candidate = root / path
        if candidate.is_file():
            return candidate
    return None


# ── Off-site links ────────────────────────────────────────────────────────────
# The docs are written to be read on GitHub, so they link sideways into the tree:
# `../internal/app/transcoding/playback_plan.go`, `../docker-compose.yml`,
# `CODEBASE_MAP.md`. Those paths mean nothing under a docs_dir, and mkdocs
# --strict rightly refuses them. Rewriting them to blob URLs keeps the author's
# intent (go look at the code) and keeps the source files readable on GitHub -
# the alternative, editing twenty pages by hand, would break the GitHub view to
# fix the site.
BLOB = "https://github.com/Evey-project/evey/blob/master/"
MD_LINK = re.compile(r"\]\(([^)\s]+?)(\s+\"[^\"]*\")?\)")


def rewrite_offsite_links(rel: str, text: str, published: set[str]) -> tuple[str, list[str]]:
    """Point links that leave the published set at the repository on GitHub."""
    here = Path(rel).parent
    unresolved: list[str] = []

    def replace(match: re.Match) -> str:
        target, title = match.group(1), match.group(2) or ""
        if re.match(r"\A(https?:|mailto:|#|/)", target):
            return match.group(0)
        path, _, anchor = target.partition("#")
        if not path:
            return match.group(0)
        resolved = (here / path).as_posix()
        # Normalise ../ segments textually - the target may legitimately be
        # outside the tree we are assembling, so resolve() is the wrong tool.
        parts: list[str] = []
        for segment in resolved.split("/"):
            if segment in ("", "."):
                continue
            if segment == "..":
                if parts:
                    parts.pop()
                else:
                    parts.append("..")
            else:
                parts.append(segment)
        norm = "/".join(parts)
        if norm in published:
            return match.group(0)
        # Outside docs/ entirely: `../x` normalised above already dropped the
        # leading segment, so rebuild the repo-relative path from the raw link.
        raw = (here / path).as_posix()
        repo_rel = raw[3:] if raw.startswith("../") else f"docs/{norm}"
        if not (REPO / repo_rel).exists():
            unresolved.append(f"{rel}: {target}")
            return match.group(0)
        suffix = f"#{anchor}" if anchor else ""
        return f"]({BLOB}{repo_rel}{suffix}{title})"

    return MD_LINK.sub(replace, text), unresolved


# ── Stable anchors across languages ───────────────────────────────────────────
# Translating a heading changes the anchor it generates, which silently kills
# every link pointing at it - including links from pages that are still in
# English, because a partially translated site serves those side by side with
# translated ones. So every translated heading is pinned to the anchor its
# English counterpart generates: a deep link works in all six languages, and a
# reader can switch language without losing their place on the page.
#
# The pinning is positional: the nth heading of a translation is the nth heading
# of the source. That holds only while the translation mirrors the structure, so
# a mismatch is refused rather than guessed at.
HEADING = re.compile(r"^(#{1,6})\s+(.*?)\s*$")
FENCE = re.compile(r"^\s*(```|~~~)")


def headings(text: str) -> list[tuple[int, str]]:
    """Line index and title of each heading, skipping fenced code blocks."""
    out: list[tuple[int, str]] = []
    fenced = False
    for i, line in enumerate(text.splitlines()):
        if FENCE.match(line):
            fenced = not fenced
            continue
        if fenced:
            continue
        m = HEADING.match(line)
        if m:
            out.append((i, m.group(2)))
    return out


def slugify(title: str) -> str:
    """Reproduce pymdownx.slugs.slugify(case=lower), the site's slugifier.

    Punctuation is dropped and each remaining space becomes one dash, WITHOUT
    collapsing: "Security headers & CSP - read this" really does slug to
    "security-headers--csp---read-this", and a naive collapse would produce an
    anchor that does not exist.
    """
    text = unicodedata.normalize("NFC", title).lower()
    text = re.sub(r"\{[^}]*\}\s*$", "", text).strip()  # drop an existing {#id}
    text = re.sub(r"[^\w\s-]", "", text, flags=re.UNICODE)
    return re.sub(r"\s", "-", text)


def pin_anchors(rel: str, source: str, translated: str) -> tuple[str, str | None]:
    """Give each translated heading the anchor of its English counterpart."""
    src_h, tr_h = headings(source), headings(translated)
    if len(src_h) != len(tr_h):
        return translated, (
            f"{rel}: {len(tr_h)} headings against {len(src_h)} in English - "
            "anchors cannot be aligned, so links into this page would break"
        )
    lines = translated.splitlines()
    # slug the translator would have computed -> the anchor we are pinning.
    remap: dict[str, str] = {}
    for (_, src_title), (line_no, tr_title) in zip(src_h, tr_h):
        if tr_title.endswith("}"):  # already carries an explicit id
            continue
        anchor = slugify(src_title)
        if not anchor:
            continue
        hashes = lines[line_no].split(" ", 1)[0]
        lines[line_no] = f"{hashes} {tr_title} {{ #{anchor} }}"
        # Three plausible guesses at what a translator wrote: the true slug, its
        # ASCII form (the default slugifier drops accents), and a collapsed form
        # (people write one dash where punctuation produced two).
        tr_slug = slugify(tr_title)
        variants = (
            tr_slug,
            _deaccent(tr_slug),
            re.sub(r"-{2,}", "-", tr_slug),
            re.sub(r"-{2,}", "-", _deaccent(tr_slug)),
        )
        for variant in variants:
            if variant and variant != anchor:
                remap[variant] = anchor

    out = "\n".join(lines) + ("\n" if translated.endswith("\n") else "")

    # A translator naturally links to the anchor their own heading would have
    # produced. Now that headings are pinned to the English anchor, those links
    # point at nothing - so they are remapped rather than left to rot.
    def relink(match: re.Match) -> str:
        target = match.group(1)
        return f"](#{remap[target]})" if target in remap else match.group(0)

    return re.sub(r"\]\(#([^)]+)\)", relink, out), None


def _deaccent(text: str) -> str:
    """The ASCII form a translator gets from the default slugifier."""
    return "".join(
        c for c in unicodedata.normalize("NFKD", text) if not unicodedata.combining(c)
    )


def sha(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


FRONT_MATTER = re.compile(r"\A---\n(.*?)\n---\n", re.DOTALL)


def split_front_matter(text: str) -> tuple[dict, str]:
    match = FRONT_MATTER.match(text)
    if not match:
        return {}, text
    try:
        meta = yaml.safe_load(match.group(1)) or {}
    except yaml.YAMLError:
        meta = {}
    return (meta if isinstance(meta, dict) else {}), text[match.end():]


def with_edit_source(text: str, repo_path: str) -> str:
    """Records the file a reader should edit. The build tree flattens docs/,
    wiki/content/ and the translations into one docs_dir, so mkdocs' own
    edit_uri points Home at docs/index.md and every translation at a .fr.md
    that does not exist; overrides/partials/actions.html reads this instead."""
    meta, body = split_front_matter(text)
    meta["edit_source"] = repo_path
    header = yaml.safe_dump(meta, allow_unicode=True, sort_keys=False).strip()
    return f"---\n{header}\n---\n{body}"


# ── Sources ───────────────────────────────────────────────────────────────────
# A page may end with an HTML comment listing what its behavioural claims rest
# on (a file and a symbol, a test, an ADR, an i18n key). It is invisible on the
# site and read in review. The only mechanical check: a cited repository path
# that no longer exists is reported, as a warning - the page may need a look
# after a refactor, it is not wrong for that alone.
SOURCES_BLOCK = re.compile(r"<!--\s*sources\b(.*?)-->", re.DOTALL)
SOURCE_PATH = re.compile(r"(?<![\w./-])((?:internal|cmd|frontend|docs|i18n|wiki|docker|scripts)/[\w./-]+)")


def missing_sources(rel: str, text: str) -> list[str]:
    out: list[str] = []
    for block in SOURCES_BLOCK.findall(text):
        for path in SOURCE_PATH.findall(block):
            path = path.rstrip(".,;:")
            if not (REPO / path).exists():
                out.append(f"{rel}: {path}")
    return out


# ── Leak guard ────────────────────────────────────────────────────────────────
# Publishing is one-way: a hostname pushed to a public site is scraped and
# cached within the hour. These patterns are deliberately blunt - a false
# positive costs one line in ALLOWED, a false negative costs an address on the
# internet.
LEAK_PATTERNS: list[tuple[str, str]] = [
    (r"/home/[a-z][a-z0-9_-]*/", "absolute path inside a developer home directory"),
    # An e-mail needs an alphabetic TLD: without it `theintrodb@3.0.0` (an npm
    # version pin) reads as an address. The lookbehind keeps `buf.gen.local.yaml`
    # from parsing as the host `gen.local` in the pattern below.
    (r"(?<![\w.+-])[\w.+-]+@(?!example\.)[a-z][\w-]*\.[a-z]{2,}(?![\w-])", "e-mail address"),
    (r"(?<![\w.-])[a-z0-9][a-z0-9-]*\.(lan|local|internal|home)(?![\w.-])", "internal hostname"),
    (r"\bgh[pousr]_[A-Za-z0-9]{16,}", "GitHub token"),
    (r"\bsk-[A-Za-z0-9]{20,}", "API secret key"),
    (r"\beyJ[A-Za-z0-9_-]{20,}\.", "JWT"),
    (r"\bAKIA[0-9A-Z]{16}\b", "AWS access key"),
    (r"-----BEGIN [A-Z ]*PRIVATE KEY-----", "private key"),
    # Workshop references. These leak no secret, they leak that the page was
    # written for an audience of one: a private memory file, an agent worktree,
    # a staging box the reader does not have. The docs audit of 2026-09-08 found
    # eleven of these and zero real credentials, which is why they are guarded.
    (r"(?<![\w/])memory/[a-z0-9_]+\.md", "reference to a private memory note"),
    (r"\.claude/", "reference to an agent working directory"),
    (r"(?<![\w-])pr[ée]prod(?![\w-])", "reference to a private staging environment"),
    (r"(?<![\w-])rtk\s+(proxy|git|gain)", "reference to private local tooling"),
]

# Literals that match a pattern above but are documentation, not disclosure.
ALLOWED = {
    "git@github.com",  # the SSH clone URL, in every getting-started page
    "security@eveyproject.org",
    "contact@eveyproject.org",
    "noreply@eveyproject.org",
}


def scan_for_leaks(path: Path, text: str) -> list[str]:
    problems: list[str] = []
    for lineno, line in enumerate(text.splitlines(), start=1):
        for pattern, label in LEAK_PATTERNS:
            for hit in re.finditer(pattern, line, flags=re.IGNORECASE):
                found = hit.group(0)
                if found in ALLOWED:
                    continue
                problems.append(f"{path}:{lineno}: {label}: {found}")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="fail on a missing page, a stale translation or a leak (CI gate)",
    )
    parser.add_argument(
        "--drafts",
        action="store_true",
        help="review preview: also publish translations marked `reviewed: false`, "
        "under a draft banner. Never used for the public site.",
    )
    parser.add_argument(
        "--restamp",
        action="store_true",
        help="rewrite every translation's source_sha to the current English page. "
        "Run this ONLY after actually bringing the translations up to date - it "
        "silences the staleness guard, it does not satisfy it.",
    )
    args = parser.parse_args()

    config = yaml.load((WIKI / "mkdocs.yml").read_text(encoding="utf-8"), Loader=YamlIgnoreUnknown)
    paths = nav_paths(config.get("nav"))

    # Build into a private directory and swap it in at the end. Several
    # translators can run this at once, and a plain rmtree of BUILD would pull
    # files out from under a concurrent run - and out from under `mkdocs serve`,
    # which is watching that directory.
    staging = WIKI / f".build-{os.getpid()}"
    if staging.exists():
        shutil.rmtree(staging)
    staging.mkdir(parents=True)

    missing: list[str] = []
    leaks: list[str] = []
    stale: list[str] = []
    restamped: list[str] = []
    unresolved: list[str] = []
    structure: list[str] = []
    translated = {lang: 0 for lang in LANGUAGES}
    held = {lang: 0 for lang in LANGUAGES}
    cited: list[str] = []

    # The set of paths that exist on the site, needed before any link is
    # rewritten: a link is "off-site" only relative to what actually ships.
    published = {rel for rel in paths if source_for(rel) is not None}
    for pattern in ASSET_GLOBS + IMAGE_GLOBS:
        published.update(str(a.relative_to(DOCS)) for a in DOCS.glob(pattern))

    for rel in paths:
        src = source_for(rel)
        if src is None:
            missing.append(rel)
            continue

        english = src.read_text(encoding="utf-8")
        english_source = english
        # The freshness digest is taken on the file as written, before link
        # rewriting: a translator computes it with `sha256sum docs/<page>`, and
        # it must not move because the blob URL prefix changed.
        digest = sha(english)
        leaks.extend(scan_for_leaks(Path(rel), english))
        cited.extend(missing_sources(rel, english))
        english, dangling = rewrite_offsite_links(rel, english, published)
        unresolved.extend(dangling)

        english = with_edit_source(english, str(src.relative_to(REPO)))
        dest = staging / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(english, encoding="utf-8")

        for lang in LANGUAGES:
            tr = TRANSLATIONS / lang / rel
            if not tr.is_file():
                continue
            raw = tr.read_text(encoding="utf-8")
            meta, body = split_front_matter(raw)
            draft = args.drafts and meta.get("reviewed") is False
            if meta.get("reviewed") is not True and not draft:
                held[lang] += 1
                continue
            leaks.extend(scan_for_leaks(Path(f"{lang}/{rel}"), body))
            body, dangling = rewrite_offsite_links(rel, body, published)
            unresolved.extend(dangling)
            body, misaligned = pin_anchors(f"{lang}/{rel}", english_source, body)
            if misaligned:
                structure.append(misaligned)
            if args.restamp and meta.get("source_sha") != digest:
                restamped.append(f"{lang}/{rel}")
                meta["source_sha"] = digest
                header = yaml.safe_dump(meta, allow_unicode=True, sort_keys=False).strip()
                tr.write_text(f"---\n{header}\n---\n{body}", encoding="utf-8")
            elif meta.get("source_sha") != digest:
                stale.append(f"{lang}/{rel}")
                body = f"!!! warning\n    {STALE_BANNER[lang]}\n\n" + body
            if draft:
                body = f"!!! note\n    {DRAFT_BANNER[lang]}\n\n" + body
            translated[lang] += 1
            # Page settings (hide: navigation...) ride in the same front matter
            # as source_sha and reviewed; they go back on top, above any banner.
            page_meta = {k: v for k, v in meta.items() if k not in ("source_sha", "reviewed")}
            page_meta["edit_source"] = str(tr.relative_to(REPO))
            if page_meta:
                header = yaml.safe_dump(page_meta, allow_unicode=True, sort_keys=False).strip()
                body = f"---\n{header}\n---\n\n" + body
            out = staging / rel.replace(".md", f".{lang}.md")
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_text(body, encoding="utf-8")

    # Linked assets (OpenAPI specs) ride along, scanned like any published page:
    # a spec carries example hosts and tokens as readily as prose does.
    assets = 0
    for pattern in ASSET_GLOBS:
        for src in sorted(DOCS.glob(pattern)):
            rel = src.relative_to(DOCS)
            body = src.read_text(encoding="utf-8")
            leaks.extend(scan_for_leaks(rel, body))
            dest = staging / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dest)
            assets += 1

    for pattern in IMAGE_GLOBS:
        for src in sorted(DOCS.glob(pattern)):
            dest = staging / src.relative_to(DOCS)
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dest)
            assets += 1

    for src_rel, dest_rel in BRAND_FILES.items():
        src = REPO / src_rel
        if not src.is_file():
            missing.append(src_rel)
            continue
        dest = staging / dest_rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dest)
        assets += 1

    # Site-owned static files (stylesheets, images).
    stylesheets = CONTENT / "stylesheets"
    if stylesheets.is_dir():
        shutil.copytree(stylesheets, staging / "stylesheets")

    # Swap: replace the old tree with the new one, then drop the old. Not fully
    # atomic (a directory rename cannot be), but the window is milliseconds
    # rather than the whole build.
    previous = WIKI / f".build-old-{os.getpid()}"
    if BUILD.exists():
        BUILD.rename(previous)
    staging.rename(BUILD)
    if previous.exists():
        shutil.rmtree(previous, ignore_errors=True)

    total = len(paths)
    print(f"pages: {total - len(missing)}/{total} copied into {BUILD.relative_to(REPO)}")
    print(f"assets: {assets} copied")
    for lang in LANGUAGES:
        pct = (translated[lang] * 100 // total) if total else 0
        print(f"  {lang}: {translated[lang]}/{total} reviewed and published ({pct}%), {held[lang]} held back unreviewed")

    if restamped:
        print("\nRESTAMPED - source_sha refreshed, on your word that the content follows:")
        for rel in restamped:
            print(f"  {rel}")

    failed = bool(structure)
    if missing:
        failed = True
        print("\nMISSING - listed in the nav, found in neither wiki/content/ nor docs/:")
        for rel in missing:
            print(f"  {rel}")
    if stale:
        print("\nSTALE - the English page changed since this translation was written:")
        for rel in stale:
            print(f"  {rel}")
        if args.check:
            failed = True
    if structure:
        print("\nSTRUCTURE - translation does not mirror its source:")
        for problem in structure:
            print(f"  {problem}")
    if unresolved:
        print("\nDANGLING - link target found neither on the site nor in the repository:")
        for problem in sorted(set(unresolved)):
            print(f"  {problem}")
        if args.check:
            failed = True
    if cited:
        print("\nSOURCES - a path cited in a page's sources block no longer exists (warning):")
        for problem in cited:
            print(f"  {problem}")
    if leaks:
        failed = True
        print("\nLEAK GUARD - these would be published as-is:")
        for problem in leaks:
            print(f"  {problem}")

    if failed and args.check:
        print("\n❌ wiki assembly refused")
        return 1
    print("\n✅ wiki assembled" if not failed else "\n⚠️  assembled with warnings")
    return 0


if __name__ == "__main__":
    sys.exit(main())
