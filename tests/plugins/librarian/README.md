# Librarian - library housekeeping

Finds what a big collection accumulates silently:

- **Probable duplicates** - the same title present several times. Same year =
  probable duplicate copy (wasted disk); different years = remake or edition,
  flagged softer. The detection half of what the Jellyfin community runs
  Merge Versions / Media Cleaner for - it never touches a file.
- **Saga holes** - numbered series with missing episodes: you keep Rocky II
  and III but Rocky is gone. Understands arabic and roman numbering and
  part/chapter/volume wrappers; an unnumbered title counts as episode 1 of
  its saga. A lone "Blade II" is a choice, not a hole.

Two mounts, exercising both hero-class slots:

- `home.hero` - the global tally across every library, with the first
  findings as clickable lines (deep-link to the media page).
- `library.sidebar` - the findings of the library being browsed
  (`context.libraryId`), compact.

**A clean shelf earns silence**: nothing to report renders nothing and takes
no space on either surface.

## Design notes

- **Permissions: `public` only** - titles, years and types through the host
  bridge's scoped media RPCs. No network, no prefs, no backend state, and
  detection-only by principle: suggesting a deletion is a human's job.
- The backend (`main.js`) is the minimal gRPC lifecycle stub; all behaviour
  lives in the front modules.
- The front modules are dependency-free IIFEs written directly in `dist/`.
  The detection logic's source of truth is `dist/scan.js`; both modules
  inline the block verbatim (the sandbox loads a single self-contained
  script) and `tests/scan.test.mjs` fails if a copy drifts.

## Build the installable zip

    ./build-zip.sh

produces `../librarian.zip`, uploadable through the admin panel.
