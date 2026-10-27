# Contributing

Where to start when you want to work on Evey's code, and what a pull request
is expected to pass. The documents written for contributors live with the
code, on GitHub, rather than on this site; this page points at them.

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
| Cut a release | [Release runbook](runbooks/release.md) |

## Why a decision record, and where

Every structural choice - the hexagonal layout, how plugin tokens are
scoped, why the install is a single compose file, how one playback plan
decides for every client - has an architecture decision record: the context,
the options that were weighed, what was chosen and what it costs. They are
kept in the repository, next to the code they explain, and are the place to
start before proposing to change one of those choices.

## Before you open a pull request

1. Run `make pre-push`: it is the gate, and runs the CI's checks locally in
   this order: vet, vulnerability scan, build, unit tests with `-race`,
   frontend (typecheck, lint, unit tests), security scan, lint,
   [architecture guards](ARCHITECTURE.md#architecture-guards), commit
   hygiene, secret scan, proto breaking-change check, a build from a clean
   checkout, the repository and transcoding integration gates, and the TV
   audit.
   A push to `dev` or to a work branch runs `make pre-push-dev` instead: the
   same gate without the long suites that the pull request's CI runs again
   on the same commits (vulnerability scan, frontend tests and build, the
   repository suite, the TV audit); a push to `master` runs the full gate.
2. Add tests for new behaviour and for every bug fix.
3. For an external contribution, accept the
   [contributor licence agreement](../CLA.md) once: Evey is AGPL-3.0-or-later
   and may also be offered under a commercial licence. You keep the copyright on
   your work.

!!! warning "The gate does not build the Docker image or the Android apps"
    After touching the `Dockerfile` or Kotlin code, build them yourself
    (`docker build --target runtime`, the Gradle release task) before
    pushing.

Security issues are not reported in public: write to
`security@eveyproject.org` (see [SECURITY.md](../SECURITY.md)).

<!-- sources
gate: Makefile - pre-push, pre-push-dev, ci-fast-vet, ci-fast-frontend-static, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
