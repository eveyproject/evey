# Architecture Decision Records

Structural decisions for Evey, numbered sequentially.

**Amend a fact, supersede a decision.** A third of the entries here carry an
`## Amendment` or `## Update` section, and that is the intended practice: when the
text turned out to describe the code wrongly, the correction is appended and the
body is left as written, so a reader sees both what was believed and
what was found. When the *decision itself* changes, the entry is not edited at
all - a new ADR supersedes it and says so. The rule is about the decision, not
about the prose: correcting a false claim is honesty, rewriting history is not.

**What earns an ADR.** A decision that is expensive to reverse, and whose reason
cannot be recovered from the code or from a test. Everything else belongs
somewhere cheaper: a coding rule goes in [code-conventions.md](../code-conventions.md),
an unimplemented intention goes in [docs/design/](../design/), and a local
justification goes in a comment next to the code it explains. An ADR that
records something a reader could have deduced adds a page to maintain and
nothing to understand.

**Numbers are never reused.** A retired entry leaves a gap rather than letting a
future ADR inherit a number that older commits, comments and reviews still refer
to.

| # | Title | Status |
| --- | --- | --- |
| [0001](0001-hexagonal-architecture.md) | Hexagonal architecture (ports & adapters) | Accepted |
| [0002](0002-transactional-unit-port.md) | Transactional unit port | Accepted |
| [0003](0003-plugin-scoped-tokens.md) | Plugin-scoped JWT tokens | Accepted |
| [0004](0004-proto-deprecation-strategy.md) | Proto deprecation strategy | Accepted |
| [0005](0005-two-layer-authorization.md) | Two-layer authorization (interceptor RBAC + handler resource check) | Accepted |
| [0006](0006-scanner-change-detection.md) | Scanner change-detection contract | Accepted |
| [0009](0009-cqrs-repo-split.md) | CQRS repository split | Accepted |
| [0010](0010-event-bus.md) | In-process event bus with typed domain events | Accepted |
| [0011](0011-intro-detection-providers.md) | External intro providers before the heuristic pipeline | Accepted |
| [0012](0012-plugin-token-deny-by-default.md) | Deny-by-default authorization for plugin tokens | Accepted |
| [0013](0013-userspace-plugin-resource-monitor.md) | Userspace plugin resource monitor (cgroups abandoned) | Accepted |
| [0014](0014-landlock-plugin-sandbox.md) | Landlock plugin sandbox, applied by a launcher shim | Accepted |
| [0015](0015-hardware-acceleration.md) | One image for every GPU, drivers injected at runtime | Accepted |
| [0016](0016-mkv-fastpath-prober.md) | In-process mkvgo fast-path for the MKV scan prober (ffprobe stays the authority) | Accepted |
| [0017](0017-software-hdr-tonemap-delivery-resolution.md) | Software HDR→SDR tone-map at delivery resolution, not source | Accepted |
| [0018](0018-cmcd-qoe-pipeline.md) | CMCD-based QoE telemetry with a non-blocking drop-on-full sink | Accepted |
| [0019](0019-token-epoch-revocation.md) | Token-epoch revocation for stateless JWTs | Accepted |
| [0020](0020-hls-native-audio-rendition.md) | HLS-native audio rendition (EXT-X-MEDIA) for multi-language switching | Accepted |
| [0021](0021-plugins-on-tv.md) | Plugins on TV: WebView portage now, native SDUI renderer later | Accepted |
| [0022](0022-hls-decision-pinning-and-av-resync.md) | HLS decision pinning (vd=/ad=) and A/V resync (source-delay cancel + user offset) | Accepted |
| [0023](0023-refresh-rotation-grace.md) | Refresh-token rotation grace window (sibling sessions for multi-tab race losers) | Accepted |
| [0024](0024-playback-plan-resolver.md) | App-layer PlaybackPlan resolver (one resolution per request) | Accepted |
| [0025](0025-server-driven-cast.md) | Server-driven cast with a custom Web Receiver (app ID D36A449B) | Accepted, amended |
| [0026](0026-remote-access-inferred-from-a-published-url.md) | Remote access inferred from a published URL, unless the operator chose | Accepted |
| [0028](0028-install-is-one-compose-file.md) | The install is one compose file | Accepted |
| [0029](0029-transcoding-routes-are-measured-not-assumed.md) | Transcoding routes are measured, not assumed (route-priced admission, per-route backend, proof-gated fallback) | Accepted |
| [0030](0030-one-plan-one-decision.md) | One plan, one decision (route resolved once and carried by the plan; fingerprint, class, cost key, producer and builder are projections) | Accepted, implemented |
| [0031](0031-demo-mode-is-a-boot-flag-on-the-production-binary.md) | The demo is the production binary behind one boot flag (EVEY_DEMO, deny-by-default policy, guarded list) | Accepted |

## Retired

| # | Was | Where it went | Why |
| --- | --- | --- | --- |
| 0007 | Context lifecycle for fire-and-forget goroutines | [code-conventions.md](../code-conventions.md#fire-and-forget-goroutines) | A coding rule, reversible file by file, enforced by review and by guards. |
| 0008 | Domain aggregate constructor pattern | [code-conventions.md](../code-conventions.md#domain-aggregate-constructors) | Same: a pattern to follow, not a decision to defend. |
| 0027 | Demo mode as a boot flag | [design/demo-mode.md](../design/demo-mode.md) | Proposed, never implemented. It recorded an intention, not a decision the code embodies. |

See also [docs/ARCHITECTURE.md](../ARCHITECTURE.md) for the high-level picture.
