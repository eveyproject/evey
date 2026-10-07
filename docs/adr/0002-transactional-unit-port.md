# ADR-0002: Transactional unit port

- Status: Accepted

## Context

An audit found `UserService.CreateUser` was non-atomic: it wrote to `users`, then `user_preferences`, then `favorites` through three separate repository calls. A failure between the second and third write left orphan rows. Several other app services had the same pattern.

We need a way for the application layer to compose multi-aggregate writes inside a single transaction without leaking `*sql.Tx` into the domain.

## Decision

Introduce a domain port `shared.TransactionalUnit` with a single method:

```go
type TransactionalUnit interface {
    Do(ctx context.Context, fn func(ctx context.Context) error) error
}
```

Postgres adapter implements it by opening a `sql.Tx` and storing it in `ctx` under a typed key. All postgres repositories call a helper `txFromCtx(ctx, db)` that returns the ambient `*sql.Tx` if present, otherwise the pool.

Application services wrap multi-step writes:

```go
return s.uow.Do(ctx, func(ctx context.Context) error {
    if err := s.users.Insert(ctx, u); err != nil { return err }
    if err := s.prefs.InsertDefault(ctx, u.ID); err != nil { return err }
    return s.favs.InitFor(ctx, u.ID)
})
```

## Consequences

- Repositories stay framework-light (still depend only on `context.Context` and `sql.DB`/`sql.Tx` behind the helper).
- Tests inject a fake `TransactionalUnit` that just calls `fn(ctx)` - no real transaction needed in unit tests.
- Nested `Do` calls reuse the existing transaction (idempotent).
- All existing non-atomic services must migrate. `UserService.CreateUser` is the first migration.

## References

- `internal/domain/shared/transactional.go`
- `internal/adapters/driven/postgres/txunit.go`
- [ADR-0001](0001-hexagonal-architecture.md)
