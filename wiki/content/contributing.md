# Contributing

Evey is developed in the open, and the documents written for the people who
work on its code live with the code, on GitHub, rather than on this site. This
page points at them.

## Where to start

| You want to | Read |
| --- | --- |
| Build and run Evey from source | [DEV.md](../DEV.md) |
| Know the rules of a contribution: branches, commits, tests, the licence agreement | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Find your way in the code in half an hour | [Codebase map](CODEBASE_MAP.md) |
| Understand the layering and the bounded contexts | [Architecture](ARCHITECTURE.md) |
| Follow the conventions the code already uses | [Code conventions](code-conventions.md) |
| Work on the interface | [Frontend design system](frontend-design-system.md) |
| Know why a structural choice was made | [Architecture decisions](adr/README.md) |

## Why a decision record, and where

Every structural choice - the hexagonal layout, how plugin tokens are
scoped, why the install is a single compose file, how one playback plan
decides for every client - has an architecture decision record: the context,
the options that were weighed, what was chosen and what it costs. They are
kept in the repository, next to the code they explain, and are the place to
start before proposing to change one of those choices.

## Before you open a pull request

- `make pre-push` is the single gate. It mirrors the CI: a green run there
  means a green pipeline.
- Tests are expected for new behaviour and for every bug fix.
- External contributions need a one-time acceptance of the
  [contributor licence agreement](../CLA.md): Evey is AGPL-3.0-or-later and
  is also offered under a commercial licence. You keep the copyright on your
  work.

Security issues are not reported in public: write to
`security@eveyproject.org` (see [SECURITY.md](../SECURITY.md)).
