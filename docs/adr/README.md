# Architecture Decision Records

Structural decisions for Evey. New ADRs are numbered sequentially and never rewritten - supersede with a new ADR instead.

| # | Title | Date | Status |
| --- | --- | --- | --- |
| [0001](0001-hexagonal-architecture.md) | Hexagonal architecture (ports & adapters) | 2026-01-15 | Accepted |
| [0002](0002-transactional-unit-port.md) | Transactional unit port | 2026-04-13 | Accepted |
| [0003](0003-plugin-scoped-tokens.md) | Plugin-scoped JWT tokens | 2026-02-20 (amended 2026-05-23) | Accepted |
| [0004](0004-proto-deprecation-strategy.md) | Proto deprecation strategy | 2026-04-16 | Accepted |
| [0005](0005-two-layer-authorization.md) | Two-layer authorization (interceptor RBAC + handler resource check) | 2026-04-20 | Accepted |
| [0006](0006-scanner-change-detection.md) | Scanner change-detection contract | 2026-04-20 (amended 2026-05-23) | Accepted |
| [0007](0007-fire-and-forget-context.md) | Context lifecycle for fire-and-forget goroutines | 2026-04-20 | Accepted |
| [0008](0008-domain-constructors.md) | Domain aggregate constructor pattern (`NewX + Option + Validate`) | 2026-04-20 | Accepted |
| [0009](0009-cqrs-repo-split.md) | CQRS repository split | 2026-04-20 (amended 2026-05-23) | Accepted |
| [0010](0010-event-bus.md) | In-process event bus with typed domain events | 2026-04-20 (amended 2026-05-23) | Accepted |
| [0011](0011-intro-detection-providers.md) | External intro detection providers ahead of the heuristic pipeline | 2026-05-04 (amended 2026-05-23) | Accepted |
| [0012](0012-plugin-token-deny-by-default.md) | Deny-by-default authorization for plugin tokens | 2026-05-23 | Accepted |
| [0013](0013-userspace-plugin-resource-monitor.md) | Userspace plugin resource monitor (cgroups abandoned) | 2026-05-23 | Accepted |
| [0014](0014-landlock-plugin-sandbox.md) | Landlock plugin sandbox via a pre-exec launcher shim | 2026-05-24 | Accepted |
| [0015](0015-hardware-acceleration.md) | HW transcoding: one full-HW image, runtime-injected drivers, admin-selectable mode | 2026-06-01 | Accepted |
| [0016](0016-mkv-fastpath-prober.md) | In-process mkvgo fast-path for the MKV scan prober (ffprobe stays the authority) | 2026-06-03 | Accepted |
| [0017](0017-software-hdr-tonemap-delivery-resolution.md) | Software HDR→SDR tone-map at delivery resolution, not source | 2026-06-04 | Accepted |
| [0018](0018-cmcd-qoe-pipeline.md) | CMCD-based QoE telemetry with a non-blocking drop-on-full sink | 2026-06-14 | Accepted |
| [0019](0019-token-epoch-revocation.md) | Token-epoch revocation for stateless JWTs | 2026-06-14 | Accepted |
| [0020](0020-hls-native-audio-rendition.md) | HLS-native audio rendition (EXT-X-MEDIA) for multi-language switching | 2026-06-14 | Accepted |
| [0021](0021-plugins-on-tv.md) | Plugins on TV: WebView portage now, native SDUI renderer later | 2026-07-09 | Accepted |
| [0022](0022-hls-decision-pinning-and-av-resync.md) | HLS decision pinning (vd=/ad=) and A/V resync (source-delay cancel + user offset) | 2026-07-09 | Accepted |

See also [docs/ARCHITECTURE.md](../ARCHITECTURE.md) for the high-level picture.
