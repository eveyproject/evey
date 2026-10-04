# eveyproject.org

The public documentation site, built from `docs/` with MkDocs Material.

## How a page gets published

The `nav` in `mkdocs.yml` is the manifest. `assemble.py` reads it, resolves each
entry against `wiki/content/` then `docs/`, and copies only those files into
`wiki/build/`, which is the real `docs_dir`. Nothing else in `docs/` reaches the
site, so a working note dropped next to a published page cannot leak by
accident. To publish a page, add it to the nav. To unpublish one, remove it.

`wiki/content/` holds pages that belong to the site rather than to the
repository, starting with the landing page. A file there shadows the same path
in `docs/`.

### What is deliberately not published

These pages under `docs/` are absent from the nav on purpose. They are listed
here so the next reader does not have to guess whether it was an oversight.

| Page | Why |
| --- | --- |
| `design/demo-mode.md` | A proposal. Publishing it would let a reader mistake an intention for a feature the server has. |
| `design/transcode-saga.md` | Same: no code has been written against it. |
| `dockerhub.md` | The Docker Hub description, pushed there by CI. On the site it would duplicate the installation page. |
| `runbooks/README.md` | Replaced on the site by `operations/troubleshooting.md`. |
| `runbooks/release.md` | How maintainers cut a release: for contributors, linked from the contributing page, not an operator task. |

Two pages were retired from this list after being checked rather than assumed:
the release runbook (one private line, generalised) and the codebase map (every
path it cites verified to exist). Measure before excluding - a page removed
costs a reader information, a page kept costs a line in the nav.

## One page per topic

The site is Evey's reference documentation, organised by the question a reader
has: getting a server running, administering it, understanding playback,
operating it, extending it, and how it is built. Each topic has one canonical
page; other pages link to it rather than explaining it again, so two pages
cannot drift apart. A setting gets documentation only when its consequences are
not obvious from its label and help text in the interface.

## Sources

A page may end with an invisible block naming what its behavioural claims rest
on: a file and a symbol, a test, an ADR, an i18n key. No line numbers, they
move with every refactor. Tests are the best source when one exists, because
they fail when the behaviour changes.

```html
<!-- sources
decision: internal/app/transcoding/decision.go - evaluateAudio
example: internal/app/transcoding/route_golden_test.go
-->
```

`assemble.py` warns when a cited repository path no longer exists. It checks
nothing else.

## Translations

English is the source. A translation is published only when its front matter
says `reviewed: true`: a complete but unreviewed translation stays out, and the
page falls back to English. A language is built (`build: true` in
`mkdocs.yml`) once its first reviewed page exists. Terms that name something in
the interface use the interface's own wording for that language (`i18n/<lang>/`). A translation lives at
`wiki/translations/<lang>/<same path as the English page>` and is laid down as
`<page>.<lang>.md` at build time. A page with no translation falls back to
English rather than 404ing, so partial coverage is a usable state.

A translation whose structure no longer matches its source (a reorganised
page, not a reworded one) cannot have its anchors pinned and would be refused
by `--check`. Rename it `<page>.md.paused` so the site falls back to English
for that page until the translation is redone. The `.paused` suffix keeps the
old text in the tree for the translator without publishing it.

Each translation carries the sha256 of the English file it was translated from:

```yaml
---
source_sha: 3448d11aea4bf0ffdbf449f4e492060664f8d57108610d92f3a286f7b23eb264
reviewed: true
---
```

Get it with `sha256sum docs/<page>` (or `sha256sum wiki/content/<page>`). When
the English page moves on, the digest stops matching: the translation still
ships, with a banner telling readers it is behind, and `--check` reports it.
Translated documentation rots silently otherwise, which is how multilingual
docs usually die.

## Guards

`assemble.py --check` fails the build on:

- a nav entry whose file is missing;
- a link that resolves to neither a published page nor a file in the repository;
- a translation whose `source_sha` no longer matches;
- anything the leak guard matches - a developer home path, a private hostname,
  a token, a private key, or a reference to a private note, a staging box or
  local tooling.

`mkdocs build --strict` then fails on a broken internal link. Links that
deliberately point outside `docs/` (at the source tree, at `docker-compose.yml`)
are rewritten to blob URLs on github.com, so the pages stay readable both on
GitHub and on the site.

## Building it

```sh
make wiki-check      # assemble + guards, no site
make wiki            # full strict build into wiki/site (Docker)
make wiki-serve      # local preview on http://localhost:8000
make wiki-publish    # build HEAD, commit it into ../evey-project.github.io
```

`wiki/build/` and `wiki/site/` are generated. Both are ignored by git.

The site is served by GitHub Pages from `Evey-project/evey-project.github.io`.
`make wiki-publish` builds HEAD in a detached worktree and commits the result
into the local clone of that repository; pushing that clone puts it live.
