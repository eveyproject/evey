# ADR-0007: Context lifecycle for fire-and-forget goroutines

- Status: Accepted
- Date: 2026-04-20

## Context

Several gRPC handlers return an immediate response and continue the real work in a background goroutine: photo library scan, trickplay generation, audit-log kill-session with grace period, notification dispatch, etc. The choice of `context.Context` passed to that goroutine is not obvious - and each wrong choice failed in production in a different way.

Observed failure modes:

1. **Request ctx in a long-running goroutine.** `photo_handler.ScanPhotoDirectory` fell back to the request `ctx` when `serverCtx` was nil. The gRPC framework cancels the request ctx the moment the handler returns, so the scan goroutine got cancelled mid-walk and silently dropped half the library.
2. **Naked `context.Background()` with no deadline.** `admin_audit.KillSession` slept 5 s then called `StopStream(context.Background(), sessionID)`. If the downstream service hung, the goroutine leaked forever.
3. **Goroutine without `defer recover()`.** A panic in the background goroutine took down the whole server; the handler had already returned, so the panic had no caller to propagate to.

The common mistake is thinking of the goroutine as an extension of the request when it should be treated as an independent unit of work with its own lifetime, cancellation, and error-recovery story.

## Decision

Fire-and-forget goroutines must follow all three rules below. No exceptions for "small" or "fast" tasks - all three grew out of production failures, including short-lived ones.

### Rule 1 - detach from the request context

Never pass `ctx` (the gRPC handler context) to a goroutine that outlives the handler. Use one of:

- `serverCtx` injected at wire time - when the operation must be cancelled on server shutdown (scans, periodic workers). Wire via an option like `WithShutdownCtx(ctx)`.
- `context.Background()` + `WithTimeout(N)` - when the operation is short and self-bounded (admin audit grace period, async notification send).
- Never use the request ctx as a fallback. `if serverCtx == nil { serverCtx = ctx }` is a bug; the correct fallback is `context.Background()`.

### Rule 2 - every timeout is bounded

No naked `context.Background()` reaches I/O. Every detached context is wrapped with `WithTimeout`. 30 s is the default for single-row writes, 5 min for ffprobe/ffmpeg/meili operations, 10 min for background enrichment passes. Duration is justified in a comment when it deviates from those defaults.

### Rule 3 - every goroutine has `defer recover()`

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("panic recovered in <operation>",
                slog.Any("panic", r),
                slog.Any("stack", debug.Stack()))
        }
    }()
    // work
}()
```

Recovery is logged with `slog.Error` and the stack, not swallowed. The message prefix names the operation so the source is identifiable in log search. `recover()` must be inside an anonymous `defer` - `defer recover()` alone does nothing.

## Consequences

- **Reviewers have a checklist.** Any new `go func()` in an adapter/handler is checked against the three rules at PR time. Violation is treated as a bug, not a style issue.
- **`serverCtx` is a first-class dependency.** Services that spawn background work take it as a constructor option (`WithShutdownCtx`). Adapters that receive request-scoped work don't need it.
- **No defense-in-depth for `recover()`.** We do not add a top-level `runtime/debug`-based panic handler on top of the per-goroutine recovers. If a goroutine forgets the defer, the process crash is acceptable because it surfaces the bug loudly instead of masking it.
- **Test fakes inherit the discipline.** Fake event buses and fake schedulers also run subscribers in goroutines; the fakes use the same recover pattern so tests don't mask panics that production would surface.

## References

- `internal/adapters/driving/grpcapi/photo_handler.go` - serverCtx fallback → `context.Background()`
- `internal/adapters/driving/grpcapi/admin_audit.go` - `StopStream` now inside `WithTimeout(30s)`
- `internal/adapters/driven/postgres/backup.go` - `pg_dump` copy goroutine with recover + `pw.CloseWithError`
- Commit 2026-04-20: `fix(grpc): context lifecycle on fire-and-forget handler goroutines`
