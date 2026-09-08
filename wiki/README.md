# wiki.eveyproject.org

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

Four pages under `docs/` are absent from the nav on purpose. They are listed
here so the next reader does not have to guess whether it was an oversight.

| Page | Why |
| --- | --- |
| `design/demo-mode.md` | A proposal. Publishing it would let a reader mistake an intention for a feature the server has. |
| `design/transcode-saga.md` | Same: no code has been written against it. |
| `dockerhub.md` | The Docker Hub description, pushed there by CI. On the site it would duplicate the installation page. |
| `runbooks/README.md` | Replaced by `wiki/content/runbooks/index.md`, which is the site's own section landing page. |

Two pages were retired from this list after being checked rather than assumed:
the release runbook (one private line, generalised) and the codebase map (every
path it cites verified to exist). Measure before excluding - a page removed
costs a reader information, a page kept costs a line in the nav.

## Translations

The site ships in the six languages the apps ship in: English (the source),
French, German, Spanish, Hindi and Chinese. A translation lives at
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
```

`wiki/build/` and `wiki/site/` are generated. Both are ignored by git.
