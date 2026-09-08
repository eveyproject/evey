# ADR-0028: The install is one compose file

- Status: Accepted
- Date: 2026-08-29

## Context

Installing Evey used to require inventing two secrets by hand and running
Postgres as a second container. A product audit put install and operations at
7.6 out of 10 and named the reason: the person who wants to drop one container
and open a browser was not served.

## Decision

The image carries what it needs. On first boot the entrypoint generates the
database password and the search key into `/data/keys`, and runs Postgres and
Meilisearch inside the container as the non-root server user. The database is
embedded exactly when `EVEY_DB_HOST` is absent, so every install that already
pointed at its own Postgres keeps working untouched.

The install is therefore one `docker-compose.yml` with no `.env`, and the
variations - a second media folder, an existing reverse proxy, an external
database - are a few lines each on that same file.

Rejected: SQLite, which would have removed the second container at the cost of
a different database in development and production. The embedded Postgres gives
the same result while the code keeps talking to one engine.

## Consequences

- A generated secret is never written into the compose file, so nothing in the
  quick start can be copied wrong.
- The data directory now holds a Postgres cluster, which pins its major version
  to the image. A newer major refuses to open an older cluster rather than
  risking it, and the way across is documented in
  [installation.md](../installation.md).
- An operator who wants the database lifecycle in their own hands still has it:
  setting `EVEY_DB_HOST` turns the embedded one off.
- The repository's own `docker-compose.yml` remains the operator layout, because
  the development stack depends on it. That file is not the install; the install
  is the block in `installation.md`.
