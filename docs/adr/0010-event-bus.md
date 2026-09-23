# ADR-0010: In-process event bus with typed domain events

- Status: Accepted
- Date: 2026-04-20

## Context

Contexts have to react to each other: adding a media item enriches metadata,
generates trickplay, detects an intro, invalidates a feed cache and notifies
users. Calling those services directly would make `library` depend on five
packages it has no business knowing about, and the hexagonal rule forbids it.

## Decision

Cross-context coordination goes through `shared.EventBus`, and events are typed
structs owned by the domain that emits them, not string names with opaque
payloads. The interface is two methods, `Publish` and `Subscribe`.

The implementation is in-process, one goroutine per subscriber, wired once in
the composition root. A slow consumer therefore never blocks a publisher, and
the bus opens no transaction of its own - that is the handler's job.

A struct rather than a map means a subscriber that mistypes a field fails to
compile, and the fields an event carries are discoverable next to the model that
emits it.

## Consequences

- Publishers do not know their subscribers, so adding a reaction to an existing
  event touches only the reacting package.
- Delivery is best-effort and in-memory: a subscriber that panics is recovered
  and logged, and events do not survive a restart. Anything that must not be
  lost is written in the same transaction as the state it describes, not
  published and hoped for.
- Not everything that could be an event is one. Search indexing is a direct port
  call from the scan service, because the index must be consistent with the row
  that was just written rather than eventually consistent with it.

## Amendment (2026-09-23): every published event is a webhook contract

The webhook service subscribes to `*` and serialises the event struct as the
payload, so an event with no in-process subscriber is still delivered to any
webhook configured for `*` or for its name. "No consumer" is therefore never
enough to delete an event: removing or renaming one, or changing its fields, is
an observable change for webhook receivers.

An audit on this date found all 46 declared events published. Three are
redundant on the music side and were kept deliberately, for that reason:
`music.track.scanned` and `music.track.added` are published together, as are
`music.track.deleted` and `music.track.removed`, and `music.album.created` is
published for every scanned track that has an album, not when an album first
appears. Collapsing them is a breaking change to schedule as such, not a cleanup.
