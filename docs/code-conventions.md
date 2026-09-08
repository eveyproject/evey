# Code conventions

Rules that apply while writing Go in this repository. They are not architecture
decisions: each one is reversible file by file, and most are enforced by a test
rather than by discipline. That is why they live here and not in
[docs/adr/](adr/README.md), whose entries record choices that are expensive to
undo.

Where a guard exists, the guard is the authority. This page explains what it
enforces and why it was added; the test is what actually stops a regression.

## Fire-and-forget goroutines

Several handlers return an immediate response and continue the real work in the
background: photo scans, trickplay generation, the kill-session grace period,
notification dispatch. Each of the three rules below exists because its absence
failed in production.

A background goroutine is an independent unit of work, not an extension of the
request. Treat its lifetime, its cancellation and its error recovery as its own.

**1. Detach from the request context.** Never hand the gRPC handler context to a
goroutine that outlives the handler: the framework cancels it the moment the
handler returns. A photo scan once inherited the request context through a
`serverCtx == nil` fallback and was cancelled mid-walk, silently dropping half a
library. Use `serverCtx`, injected at wire time, when the work must stop at
server shutdown; use `context.Background()` with a timeout when the work is
short and self-bounded. The request context is never a valid fallback.

**2. Bound every timeout.** No naked `context.Background()` reaches IO. A
kill-session goroutine that slept five seconds and then called downstream with
an unbounded context leaked forever when that call hung. Defaults: thirty
seconds for single-row writes, five minutes for ffprobe, ffmpeg and search
operations, ten minutes for background enrichment. A different duration carries
a comment saying why.

**3. Recover in every goroutine.** A panic in a detached goroutine has no caller
left to propagate to, so it takes the server down.

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

Log the recovery with its stack rather than swallowing it, and name the
operation in the message so log search finds it. `recover()` only works inside a
deferred closure: `defer recover()` on its own does nothing.

We deliberately do not add a process-wide panic handler on top of these. A
goroutine that forgets the defer should crash the process loudly rather than
have the bug masked.

Test fakes follow the same pattern - fake buses and schedulers also run
subscribers in goroutines, and a fake that swallowed panics would hide what
production surfaces.

## Domain aggregate constructors

Every domain aggregate ships exactly one canonical constructor. Before this
pattern, struct literals let invalid state through: a missing identifier
persisted a row with an empty primary key, discovered hours later as a foreign
key error.

```go
func NewX(id ID, required RequiredParam, opts ...XOption) (*X, error) {
    x := &X{ID: id, RequiredField: required, CreatedAt: time.Now(), Enabled: true}
    for _, opt := range opts {
        opt(x)
    }
    if err := x.Validate(); err != nil {
        return nil, err
    }
    return x, nil
}
```

**Identity is positional, everything else is an option.** What the aggregate
cannot exist without - its id, its owner references - is a positional parameter,
so omitting it is a compile error. Everything else is a `func(*X)` option, so
omitting it is a meaningful default. Options are discoverable: `grep "^func
With"` lists them all.

**Defaults live inside the constructor**, never at the call site. One place, one
source of truth, no copy that drifts.

**Invariant violations surface as `InvalidArgument`** at the gRPC boundary, never
as `Internal`. From the handler's point of view an invalid aggregate is always a
caller mistake; a genuine server bug would have panicked before reaching it.

**What the constructor guarantees, and what it does not.** A struct that entered
the system through `NewX` satisfies its invariants. A struct hydrated from the
database is *trusted* to: of the repositories under the Postgres adapter, only
two revalidate on read. This is a known gap, not an oversight - adding the check
everywhere turns one pre-existing row that violates an invariant, left by a
hand-written update or an old migration, into a load error, which is a worse
failure than the one it prevents. Whether to fail closed on such a row is an
operator's decision, and it is tracked as an open item rather than slipped in
under a documentation banner.

## Guards

The rules above, and the architecture rules in
[ARCHITECTURE.md](ARCHITECTURE.md), are backed by static tests in
`internal/contractguard/` that fail the build when the code drifts. Read that
package before assuming a convention here is only advisory.
