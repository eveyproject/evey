# ADR-0010: In-process event bus with typed domain events

- Status: Accepted
- Date: 2026-04-20

## Context

Cross-domain coordination inside Evey is frequent: a library scan emits `MediaItemAdded`, which the notification service turns into push notifications, the trickplay service into a background ffmpeg job, and the meili indexer into an index write. A naive implementation has every domain import every other domain to call their methods directly - and the library domain ends up depending on notifications, trickplay, search, and everything downstream of its write path.

Two alternatives were considered and rejected:

1. **Direct method calls.** Couples domains tightly; every new consumer forces a new import and constructor wiring into the producer. Violates ADR-0001's hexagonal boundary (domain must not know about its consumers).
2. **External message bus** (NATS, Kafka, Redis streams). Operational overhead unjustified for a single-process self-hosted server. Most installs run on a Raspberry Pi 4; an external broker triples the resource footprint.

We want loose coupling and in-process simplicity. An in-memory pub/sub bus with typed event structs strikes the right balance.

## Decision

All cross-domain coordination goes through `shared.EventBus`. Events are *domain-typed structs*, not string names with opaque payloads.

### Event bus interface

```go
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventName string, handler func(ctx context.Context, event Event))
}

type Event interface {
    EventName() string
}
```

The bus is `internal/adapters/driven/eventbus.InMemoryEventBus` - a simple goroutine-per-subscriber dispatcher. Wired once in `cmd/evey/wire.go`, injected into every service that publishes or subscribes.

### Events are structs in each domain's `events.go`

Each domain owns its events next to its model:

```go
// internal/domain/library/events.go
type MediaItemAdded struct {
    MediaItemID shared.MediaItemID
    LibraryID   shared.LibraryID
    Title       string
    Type        string
    SeriesTitle string
    Genres      []string
    OccurredAt  time.Time
}

func (MediaItemAdded) EventName() string { return "media.item.added" }
```

Publishers construct the struct, subscribers type-assert in their handler:

```go
func (s *NotificationService) handleMediaItemAdded(ctx context.Context, ev shared.Event) {
    e, ok := ev.(library.MediaItemAdded)
    if !ok { return }
    // ...
}
```

A string-payload bus was tried first and abandoned - every subscriber duplicated JSON-unmarshalling, and a renamed field in the producer silently broke consumers.

### Subscribers are isolated - they must recover panics

```go
func (s *CommandService) handlePluginError(ctx context.Context, ev shared.Event) {
    defer func() {
        if rv := recover(); rv != nil {
            slog.Error("handlePluginError panic recovered", slog.Any("panic", rv))
        }
    }()
    // ...
}
```

A deterministic panic on one subscriber would otherwise take down the bus goroutine or force the bus retry loop to burn queue slots at full speed. Every subscriber defers its own recover; the bus does not swallow panics at the dispatcher level because that would mask bugs.

### Subscribers run on a detached context

The ctx passed to a subscriber comes from `shared.EventBus` internals, not from the publisher's request. Long-running handlers (notification dispatch, trickplay generation) can outlive the originating request. See [ADR-0007](0007-fire-and-forget-context.md) for context lifecycle rules that apply here.

### No cross-domain imports of non-event types

A domain MAY import another domain's events (they're part of the public contract). It MUST NOT import another domain's services, models, or repositories. Example: `notification.CommandService` imports `library.MediaItemAdded` - allowed. It must not import `library.LibraryRepository` - forbidden.

The test is mechanical: `go doc` on an event struct should work across domain lines; any other cross-domain symbol lookup is a smell.

## Consequences

- **New consumers are additive.** Adding a new subscriber (email notifications, analytics collector, webhook dispatcher) requires zero changes to the producer.
- **Event renaming is a breaking change.** The `EventName()` string is the stable contract; struct fields are tested by the consumer's type assertion. Never silently rename either; mark old events deprecated and publish the new name in parallel if needed.
- **No replay, no persistence.** Crashes lose in-flight events. For durable workflows (backup jobs, scheduled enrichment) we use the database (scheduled_tasks, job queues), not the bus. The bus is for in-session fan-out.
- **Back-pressure absent.** Subscribers that are slower than the producer will eventually fall behind. We accept this because bursts are rare and the self-hosted deployment tolerates catching up minutes later. If a subscriber needs steady-state throughput guarantees, it should queue to the database instead.

## References

- `internal/domain/shared/event.go` - the `EventBus` + `Event` interfaces
- `internal/pkg/eventbus/eventbus.go` - the implementation (`eventbus.New`, wired in `cmd/evey/wire.go`)
- `internal/domain/*/events.go` - domain event structs
- `internal/app/notification/command_service.go` - subscriber with recover + detached ctx
- [ADR-0001](0001-hexagonal-architecture.md) - domain boundary rules
- [ADR-0007](0007-fire-and-forget-context.md) - subscriber context lifecycle

## Amendment (2026-05-23)

Reference paths corrected: the interfaces live in `internal/domain/shared/event.go` (not `event_bus.go`), and the implementation moved out of `internal/adapters/driven/eventbus/` to `internal/pkg/eventbus/eventbus.go`. The design (typed events, detached ctx, no persistence) is unchanged.

## Update (2026-07-12)

The dispatcher was rewritten since the original Decision; several statements
above no longer describe the code. What actually runs (`internal/pkg/eventbus/eventbus.go`):

- **Not "goroutine-per-subscriber".** It's a **bounded worker pool** (default
  `runtime.NumCPU()*2`, `New` → `NewWithPoolSize`) draining a single **bounded
  queue** of `dispatchJob`s. Publish enqueues; the pool delivers.
- **The bus DOES recover panics centrally.** `callHandler` wraps each subscriber
  in `defer recover()`, and `dispatch` retries a panicking handler up to
  `MaxRetries` (default 3) with backoff. A subscriber's own `defer recover()` is
  therefore optional now, not required - the "the bus does not swallow panics at
  the dispatcher level" and "every subscriber defers its own recover" statements
  (and the 2026-05-23 amendment's "per-subscriber recover ... unchanged") are
  superseded.
- **The `EventBus` interface returns a subscription id and supports
  unsubscribe.** The real signatures are `Subscribe(eventName string, handler
  func(ctx, Event)) string` and `Unsubscribe(id string)` - the snippet in the
  Decision omits both.
- **Back-pressure is drop-on-full, not "catch up minutes later".** When the
  queue is full the event is **dropped** and `eventsDroppedTotal` is
  incremented; there is no buffering that lets a slow subscriber catch up later.
  Durable workflows must still go through the database, as the Consequences say.

The rest of the ADR (typed struct events, `EventName()` as the stable contract,
detached ctx, no persistence, no cross-domain non-event imports) still holds.

## Clarification: what "no cross-domain imports" covers

This ADR forbids one domain **calling** another - the coupling the bus exists to
remove. It does not forbid a domain **naming a type** owned by another when the
dependency is inherent to what it models.

One such import exists and is deliberate: `domain/transcoding` imports
`domain/library` for `library.MediaStream` (`ports.go`, used by
`HLSProcessOpts.Streams`). Transcoding decisions are made *from* stream metadata
- codec, resolution, HDR, channel layout - so the dependency is real, not
accidental; the struct carries 20+ fields and is referenced across ~24 files, so
duplicating it into `shared` would trade one honest import for a type that must
be kept in sync forever. There is no call, no coordination, and no cycle:
`library` does not import `transcoding`.

Recorded here because three consecutive architecture audits flagged it as an
undocumented ADR violation. It is documented now: an inherent type dependency,
in one direction, with no behavioural coupling. A *call* from transcoding into
library would still be a violation, and still belongs on the bus.
