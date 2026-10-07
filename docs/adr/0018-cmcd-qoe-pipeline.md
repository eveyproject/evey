# ADR-0018: CMCD-based QoE telemetry with a non-blocking drop-on-full sink

- Status: Accepted

## Context

Evey's player and transcoder need a feedback signal: is the client starving, buffering, or comfortably ahead? Without it the encoder cannot pace its bursts (pause when the client is ahead, prefetch when the buffer is fat), the per-title ladder cannot learn, and the admin has no QoE dashboard - only closed-source commercial "insights" dashboards do this.

Three approaches were considered:

1. **Custom telemetry schema.** A bespoke `{bufferMs, droppedFrames, ...}` JSON ping. Rejected: re-invents a standard, and the player would carry Evey-specific instrumentation no other tool understands.
2. **An external metrics pipeline** (statsd → Prometheus → Grafana). Operationally heavy for a single-process self-hosted server (most installs are a Raspberry Pi / weak NAS), and the encoder feedback loop needs the signal *in-process, synchronously-readable*, not round-tripped through a TSDB.
3. **CMCD (CTA-5004) ingested in-process.** The player already speaks CMCD - it is the industry-standard Common Media Client Data header/query format. Ingesting it natively means zero bespoke client instrumentation and a signal the encoder can read live.

A second tension: the ingest path sits next to the **HLS segment hot path**. Telemetry must NEVER add latency or backpressure to media serving - a slow or contended QoE write must not stall a segment response.

## Decision

QoE is a dedicated bounded context (`internal/app/qoe`) fed by CMCD parsed in a shared package (`internal/pkg/cmcd`), with an **in-process, non-blocking, drop-on-full** sink.

**CMCD over a custom schema.** Clients send CMCD (CTA-5004) on segment requests. `internal/pkg/cmcd` parses it (capped at a fixed key budget to bound a hostile client). The decision is: speak the standard the player already emits rather than a custom format - no Evey-specific client code, and the data is interpretable by any CMCD-aware tool.

**A channel and a worker, never an external queue.** Ingest hands the parsed sample to a buffered channel drained by a worker goroutine that writes the QoE store asynchronously. The HLS hot path does a non-blocking channel send and returns immediately. An in-process sink (not Kafka/NATS) keeps the resource footprint flat and lets the encoder feedback loop read the live signal without a network hop.

**Dropping when full, deliberately.** If the sink channel is full (a burst, or a slow store), the sample is **dropped**, not blocked-on. Telemetry is best-effort by definition; losing a QoE ping is invisible to the user, whereas blocking a segment response is a stall. This trade-off is the whole point of the design and must not be "fixed" by making the send blocking.

**The feedback loop respects a manual quality choice.** The encoder reads the QoE signal to pace itself (SIGCONT on a starving client, prefetch on a fat buffer, refined clientHighWater) without ever overriding a user's manual quality selection - the loop adjusts *pacing*, not the chosen rung.

## Consequences

- **Positive:** zero bespoke client instrumentation; the encoder gets a live in-process starvation signal; the HLS hot path is never blocked by telemetry; the `/admin/qoe` dashboard (24h/7j) is a real differentiator vs closed-source Insights.
- **Cost:** under sustained burst the sink drops samples - the dashboard is a representative sample, not a guaranteed-complete log. This is intentional. Any future change that makes the ingest send blocking, or routes it through a synchronous store write on the hot path, reintroduces the exact latency risk this ADR exists to prevent.
- **Anyone adding a telemetry field** must keep the sink non-blocking and the parse bounded.
