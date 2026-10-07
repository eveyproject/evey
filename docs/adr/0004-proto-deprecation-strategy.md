# ADR-0004: Proto deprecation strategy

- Status: Accepted

## Context

`proto/evey/v1/` is consumed by web, mobile (RN), and TV clients. Clients ship independently of the server, so breaking a wire contract silently breaks old installations in the field. We had no written convention for handling field/RPC removal, and `deprecated = true` was never used.

## Decision

Use protobuf's built-in `deprecated = true` option as the single mechanism for signalling "this will go away", with the following policy:

1. **Never remove a field or RPC in-place.** Mark it `deprecated = true` and add a replacement. Old clients continue to work; new clients adopt the replacement.
2. **Never reuse a field number.** Once a tag is assigned, it is burned forever. If a field is removed, reserve the tag: `reserved 7; reserved "old_name";`.
3. **Deprecate for at least two server releases** before physically removing. That gives clients a real window to migrate.
4. **No `v2/` package** until an API-wide redesign. Per-field/per-RPC deprecation is preferred - a full version bump is a major event.
5. Document the replacement in the proto comment above the deprecated member:
   ```proto
   // Deprecated: use `max_bitrate_cellular` (added in v0.42). Will be removed
   // after v0.45.
   int32 legacy_max_bitrate = 7 [deprecated = true];
   ```
6. Generated Go/TS code emits a deprecation warning from the `deprecated = true` option, so IDEs and linters surface usages automatically. No extra tooling needed.

## Consequences

- Breaking changes become visible in PR review (the `[deprecated = true]` diff).
- Removal PRs are trivial diffs - just delete the field and add the `reserved` entry - because the deprecation window already ran.
- The proto file grows over time with reserved entries; acceptable because it is the authoritative changelog of the wire contract.
- Clients that ignore deprecation warnings still work when the deprecated member is removed, as long as they only read known fields. Unknown fields are ignored by protobuf by design.

## References

- [Protobuf field deprecation](https://protobuf.dev/programming-guides/proto3/#options)
- [Google API versioning guide](https://cloud.google.com/apis/design/versioning)
- Go generator emits `// Deprecated: ...` doc comments which `go vet`'s `deprecation` check honours.
