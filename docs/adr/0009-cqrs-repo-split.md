# ADR-0009: CQRS repository split

- Status: Accepted

## Context

The original domain repositories were god-interfaces mixing reads and writes:

```go
type LibraryRepository interface {
    Create(ctx, lib) error
    Update(ctx, lib) error
    Delete(ctx, id) error
    GetByID(ctx, id) (*Library, error)
    List(ctx, filter) ([]*Library, error)
}
```

This coupled every read-only caller (list-libraries UI, watcher, search indexer) to the write surface. Two concrete problems:

- **Test fakes bloat.** A read-only test had to implement `Create`/`Update`/`Delete` too, even though the tested code never called them. Mistakes in the write stubs masked real bugs.
- **Hexagonal boundary leak.** Services that advertised "read-only" responsibilities (query services, analytics) could still reach for `Create` through the port. A single stray write in a query service would compile.

Similar problems existed on `MediaItemRepository` (mixed reads/writes) and were already partially split into `MediaItemCommandRepo` + `MediaItemQueryRepo`. We want the same for libraries and for any future repository that grows past five methods.

## Decision

Split every repository port into two interfaces along the CQRS axis - one for writes (`XCommandRepo`), one for reads (`XQueryRepo`) - and migrate callers progressively.

**The three interfaces during migration.**

1. **`XCommandRepo`** - `Create`, `Update`, `Delete`, and any other mutation.
2. **`XQueryRepo`** - `Get*`, `List*`, `Count*`, anything that doesn't mutate.
3. **`XRepository` (legacy, transitional)** - kept as `interface { XCommandRepo; XQueryRepo }` by embedding. Marked `// Deprecated: use XCommandRepo for writes and XQueryRepo for reads.`

The union interface embeds both so it drifts automatically when either CQRS half gets a new method - no handwritten duplication. The postgres adapter type satisfies all three; fakes satisfy all three.

**Migration order per repository.**

1. Introduce `XCommandRepo` + `XQueryRepo` side by side with `XRepository`.
2. Migrate the easy callers first: read-only services take `XQueryRepo`, command services take `XCommandRepo`, watcher-style services take `XQueryRepo`. Each migrated caller ships in its own commit.
3. When `XRepository` has no remaining callers except the postgres type assertion, either collapse the type assertion or delete the interface.

**No big-bang migration.** Rewriting every caller in one commit produced merge conflicts across the pluginrunner, feed, metadata, and scan packages in previous attempts. Per-caller migration is slower on paper but ships faster because each commit is reviewable independently.

## Applied scope

- `library.LibraryRepository` - union of `LibraryCommandRepo` + `LibraryQueryRepo`. Watcher migrated to `LibraryQueryRepo`. Only remaining caller is the postgres type assertion - harmless.
- `library.MediaItemRepository` → `MediaItemCommandRepo` + `MediaItemQueryRepo` split partial. `watcher.go` migrated; `library_repo.go:114` still uses the unified port. Further migration remains in the backlog (large blast radius, needs a dedicated pass).

## Consequences

- **New callers choose explicitly.** There is no "just use the big one" default anymore; a new service either wants to read or to write, and picks accordingly.
- **Fakes can be narrow.** `FakeLibraryQueryRepo` only implements reads; tests of read-only services don't need write stubs.
- **Legacy interface can be retired incrementally.** A repository is "fully migrated" when zero Go files reference `library.LibraryRepository` by name. We don't block new work on reaching that state for every repo at once.
- **Typing costs one extra Go file per domain** (`queries.go` + `commands.go` next to the existing `ports.go`). Acceptable.

## References

- `internal/domain/library/queries.go` - `LibraryQueryRepo`
- `internal/domain/library/commands.go` - `LibraryCommandRepo`
- `internal/app/library/watcher.go` - first narrow-port caller (read-only)

## Amendment

For `library`, migration is **complete**: the transitional `LibraryRepository` union interface has been removed from `ports.go` (the postgres adapter and `FakeLibraryRepository` satisfy `LibraryCommandRepo` + `LibraryQueryRepo` directly). The "Applied scope" notes above describe the mid-migration state; the union no longer exists. `MediaItemRepository` is still a god-interface (its split remains in the backlog) - that part stands.

## Second amendment

`MediaItemRepository` is gone. It was never an embed of the two halves but a
handwritten ten-method interface, so it did not drift *with* them, it drifted
*from* them, and four callers were still on it:

- `IntroDetectionService` and the playlist media checker only read, and take
  `MediaItemQueryRepo` (wired to the query adapter, which delegates those calls
  to the same repository, transaction included).
- The metadata `Enricher` and `CommandService` read an item and write it back.
  They take `MediaItemQueryRepo` for the reads and `metadata.MediaItemUpdater`,
  a one-method port (`Update`) declared by the consumer, for the write - the
  same split `artworkCmd` / `artworkQuery` already applied in that package.

The same pass removed the `app/collection` facade: callers take the
`CommandService` and `QueryService` they need, and the enricher's collection
dependency is split the same way (`CollectionLinker` for `Upsert` /
`LinkMediaItem`, `UnlinkedMovieLister` for the relink sweep's read).

The postgres type now asserts `MediaItemCommandRepo`; the query adapter already
asserted `MediaItemQueryRepo`. `FakeMediaItemRepository` stays as the
single-store fake for read-then-write tests; it models `GetByID`, `List`,
`Count` and `Update`, and panics on the query methods it does not model.

`TestMediaItemRepositoryStaysGone` (contractguard) fails if the interface is
declared again or referenced under any import alias. Unions that are pure
embeds of both halves (`metadata.ArtworkRepository`, `notification.Repository`,
`photo.Repository`) are deliberate and untouched.
