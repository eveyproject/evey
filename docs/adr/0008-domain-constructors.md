# ADR-0008: Domain aggregate constructor pattern (`NewX + Option + Validate`)

- Status: Accepted
- Date: 2026-04-20

## Context

Early in the project, domain aggregates (`Library`, `Plugin`, `Profile`, `PlaySession`, ...) were constructed by direct struct-literal initialisation:

```go
lib := &library.Library{ID: "...", Name: "...", Type: "..."}
```

Three problems surfaced:

1. **Invalid state survives construction.** A handler that forgot the `ID` or `LibraryID` field persisted a row with an empty primary key, discovered hours later when a foreign-key insert failed with a cryptic error.
2. **Validation logic drifts.** `Library.Validate()` existed but was only called at some boundaries - create/update handlers remembered it, scanner goroutines didn't. Each aggregate grew its own partial validation surface.
3. **Default values are invisible.** Wiring `Enabled: true`, `CreatedAt: time.Now()`, `IsDefault` flags was copy-pasted across every caller, each copy prone to drift.

We ran a 15-commit sweep on 2026-04-19 to migrate every aggregate to a consistent factory pattern; this ADR formalises the rules so future aggregates follow suit without another sweep.

## Decision

Every domain aggregate ships with exactly one canonical constructor:

```go
func NewX(id ID, required RequiredParam, opts ...XOption) (*X, error) {
    x := &X{
        ID:              id,
        RequiredField:   required,
        CreatedAt:       time.Now(),
        Enabled:         true,   // sensible defaults
    }
    for _, opt := range opts {
        opt(x)
    }
    if err := x.Validate(); err != nil {
        return nil, err
    }
    return x, nil
}
```

### Rule 1 - identity is positional, everything else is an Option

Parameters the aggregate cannot exist without (ID, owner refs like `LibraryID`, `UserID`) are positional `NewX(id, ownerID, ...)`. Anything else - preferences, flags, optional timestamps, avatar paths, locales - is a `func(*X)` option:

```go
user.WithProfileLocale("fr")
user.WithProfilePreferences(prefs)
user.WithProfileMaxRating("PG-13")
```

Options are discoverable in godoc (`grep "^func With"` lists them all) and IDE autocomplete surfaces them. A missing option is a meaningful default; a missing positional param is a compile error.

### Rule 2 - `Validate()` is called at construction (and, on two repos, at read)

`NewX` calls `Validate()` before returning. That half holds everywhere.

**The read half does not, and this ADR used to claim it did.** It read:
"Repositories loading rows from the DB *also* call `Validate()` defensively".
Measured on 2026-08-01: of 64 `*_repo.go` files under
`internal/adapters/driven/postgres/`, exactly **two** do
(`media_stream_repo.go`, `share_link_repo.go`). Everything else hydrates the
aggregate straight from the row.

The sentence is corrected rather than the code, deliberately and for now:
adding the check everywhere turns a single pre-existing row that violates an
invariant - one a hand-written `UPDATE` or an old migration left behind - into
a **load error**, i.e. a server that refuses data it has been serving happily.
That is a bigger failure mode than the one the guard prevents, and picking it
is an operator's call (fail-closed and surface the bad row, versus log-and-
continue), not something to slip in under a doc-conformance banner.

So the honest statement of the invariant today: **a struct that entered the
system through `NewX` satisfies its invariants; a struct hydrated from the
database is trusted to.** Three consecutive architecture audits reported the
gap against the old wording - it was the documentation that was wrong.

Open decision, tracked in the backlog: implement the read-side check on the
identity/permission-carrying repos only (`user`, `profile`, `plugin`), with an
explicit choice of what a failed load does.

### Rule 3 - required identity fields surface `InvalidArgument` at the gRPC boundary

Every gRPC handler that constructs an aggregate does:

```go
p, err := user.NewProfile(id, userID, name, opts...)
if err != nil {
    return nil, status.Errorf(codes.InvalidArgument, "%v", err)
}
```

No domain errors leak as `Internal`. Invariant violations are always caller mistakes from the gRPC perspective; if they were server bugs, they would panic before reaching the handler.

### Rule 4 - default values belong inside `NewX`, not at each callsite

`NewPlaySession` stamps `StartedAt = time.Now()`; `NewTranscodeJob` defaults `Status = Queued`; `NewBackupManifest` clones the tables slice. No caller reproduces these defaults - one place, one source of truth.

## Applied scope

Migrated aggregates (complete as of 2026-04-19):

- `library.Library`, `library.MediaItem` (with 4 sub-aggregate builders)
- `user.User`, `user.Profile`, `user.APIKey`, `user.InviteLink`
- `plugin.Plugin`, `metadata.MetadataOverride`, `metadata.ArtworkRecord`
- `playback.PlaySession`, `favorite.Favorite`, `webhook.Config`
- `notification.Notification`, `subtitle.SubtitleDownload`
- `backup.BackupManifest`, `music.Track`, `transcoding.TranscodeJob`
- `photo.Photo`

Deferred (explicit exceptions):

- `feed.AdminConfig` / `feed.ProfileConfig` - keyed by `(ProfileID, FeedType)` without a standalone identity, options pattern adds noise for zero benefit.
- Nested sub-aggregates (`library.MediaFile`, `library.MediaCatalog`, ...) - constructed by their parent `NewMediaItem`, don't need their own factories.

## Consequences

- **Tests use the same path as production.** Fixtures go through `NewX` instead of struct literals, so invalid fixtures fail tests rather than producing invalid DB rows.
- **gRPC converters stay simple.** Proto → domain conversion is now two steps: proto decode, then `NewX(...)`. The converter never touches defaults.
- **Evolution cost is linear.** Adding a new required field means changing `NewX`'s signature - a compile error at every callsite, exactly what we want. Adding an optional field means one new `With*` function, zero callsite changes.
- **`Validate()` duplication is accepted.** Each aggregate repeats its own "ID required" / "enum valid" checks. A generic validator would be over-engineering; the domain aggregate is the authority for its own invariants.

## References

- `internal/domain/*/model.go` - the 15+ `NewX` functions
- Commits 2026-04-18/19 (the sweep): `feat(<aggregate>): NewX constructor + ID required in Validate`
- [ADR-0001](0001-hexagonal-architecture.md) - why the domain layer owns these invariants

## Update (2026-07-12)

The "Deferred" exception for `feed.AdminConfig` / `feed.ProfileConfig` no
longer holds: both aggregates adopted the options pattern -
`NewFeedAdminConfig(feedType, opts ...AdminConfigOption)` and
`NewFeedProfileConfig(feedType, profileID, opts ...ProfileConfigOption)`
(`internal/domain/feed/model.go`). The earlier "adds noise for zero benefit"
call was reversed once feed grew enough optional fields to justify it. The rest
of the ADR stands; note that `plugin.Plugin` uses the `NewPluginFromManifest(id,
sourceHash, manifest)` variant (permissions come from the manifest), not a
`With*` options list.
