# ADR-0012: Deny-by-default authorization for plugin tokens

- Status: Accepted
- Extends: [ADR-0003](0003-plugin-scoped-tokens.md) - refines [ADR-0005](0005-two-layer-authorization.md)

## Context

[ADR-0003](0003-plugin-scoped-tokens.md) introduced short-lived, scope-filtered plugin JWTs. [ADR-0005](0005-two-layer-authorization.md) closed by asserting that plugin tokens need "a third lever - scope filtering at mint time" so that a `RoleUser` caller of `IssuePluginToken` can't receive admin scopes.

That turned out to be **necessary but not sufficient**, and the gap was systemic:

- The interceptor recognises a plugin token (`PluginClaims{PluginID, UserID, Scopes}`) and routes it off the normal user-auth path. The RBAC layer from ADR-0005 only enforces `claims.IsAdmin` on `MethodRoles[m] == RoleAdmin`. A plugin token has no `IsAdmin`, so it was neither rejected by the admin gate nor checked against any per-method scope policy.
- Mint-time scope filtering constrains *which scopes a token carries*, not *which methods a token may invoke*. Most RPCs never consult scopes at all - so a token minted with a perfectly benign scope could still reach admin/system/write methods, because nothing on the request path looked at the method-vs-scope relationship.
- `IssuePluginToken` is `RoleUser` by design (any logged-in user installs/runs plugins). So **any authenticated user could mint a plugin token and call admin-only RPCs.** Mint-time filtering didn't help: the methods that mattered didn't gate on scopes.

## Decision

Add a dedicated, **deny-by-default** authorization path for plugin-token requests in the interceptor: `AuthInterceptor.authorizePluginToken(ctx, claims, method)`. A plugin token reaches a method only if it survives all of:

1. **Admin hard-floor.** If `method` is in `MethodRoles` as `RoleAdmin`, reject outright - regardless of scopes carried. No plugin token, with any scope set, ever reaches an admin/system/destructive RPC. This is non-bypassable and not grantable.
2. **Method allowlist.** `method` must appear in `rbac.PluginMethodScopes[method]`, which maps the method to the single scope it requires. Absent from the allowlist ⇒ denied. The default for an unlisted method is *deny*, the opposite of the RBAC layer's `RoleUser` default - forgetting to list a method fails safe.
3. **Scope check.** The token must actually carry the scope the allowlist demands.

**Grantable capabilities (consent at install).** Narrow, read- or self-oriented. Surfaced to the admin at install; enforced at the interceptor:

- **`public`** → `GetPublicMediaItem` / `ListPublicMediaItems` only. A safe projection (`PublicMediaItem`) with **no `file_path`**, reusing the existing access/parental gates.
- **`prefs`** → `PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`) only. Strictly the calling plugin's own key/value rows for the calling user; both `plugin_id` and `user_id` come from the token, never the request.

**Hard-locked (never grantable, never bypassable).** Scopes use **dot notation** (`media.read`, `library.write`, …), not colons. Any scope under the prefixes `admin.`, `auth.`, `system.` (prefix-matched in `pkg/auth.forbiddenPrefix`), plus the exact scopes `user.write` and `library.write`, is hard-locked: requested in a manifest ⇒ install rejected; present in a token ⇒ rejected at mint, and the admin method floor (step 1) rejects admin RPCs independently of any scope.

**Bridge model - the token never enters the sandbox.** The iframe (and the mobile WebView) never hold a plugin token:

- The parent (`PluginIframe` / mobile `PluginSlot`) mints the token via `IssuePluginToken` and **proxies** `prefs`/`public` RPCs on the plugin's behalf.
- The sandbox issues `postMessage` RPCs (correlation-id matched); the parent attaches the token and forwards.
- The sandbox CSP sets `connect-src` to the plugin's declared `external_fetch` hosts only - it cannot reach the Evey API directly even if it reconstructed a request.

## Consequences

- **A plugin token can never escalate.** Mint-time filtering (ADR-0003) is retained as defense-in-depth, but enforcement no longer depends on it: the interceptor denies by default and floors admin methods independently of scopes.
- **Exposing a new method to plugins is one explicit line** in `PluginMethodScopes`. Forgetting it denies access - the safe failure mode (contrast ADR-0005, where forgetting an *admin* entry exposes it; here the polarity is inverted on purpose).
- **The model is verified by red-team plugins**, not just review: `tests/plugins/redteam-bridge` (granted `prefs`+`public`) and `tests/plugins/sandbox-escape/escape-datastore` (no permissions) probe CSP blocks, direct-API denial, `file_path` non-leak, cross-plugin/cross-user data isolation, and admin-method denial.
- This is a self-hosted trust-but-verify boundary for community/third-party plugin code, not a hostile multi-tenant guarantee.

## References

- `internal/adapters/driving/grpcapi/interceptors/auth.go` - `authorizePluginToken` (enforcement)
- `internal/adapters/driving/grpcapi/interceptors/rbac.go` - `PluginMethodScopes` allowlist + admin `MethodRoles` floor
- `internal/adapters/driving/grpcapi/plugin_prefs_handler.go` - `PluginUserDataService` (plugin+user from token)
- `internal/adapters/driving/grpcapi/library_handler.go` - `GetPublicMediaItem` / `ListPublicMediaItems` (no `file_path`)
- `internal/pkg/auth/plugin_jwt.go` - `AllowedPluginScopes` (`public`, `prefs`)
- `internal/adapters/driven/pluginrunner/capabilities.go` - `KnownPermissions`
- `frontend/apps/web/src/features/plugins/{front-bridge.ts,PluginIframe.tsx}` - token-proxy bridge
- [ADR-0003](0003-plugin-scoped-tokens.md), [ADR-0005](0005-two-layer-authorization.md)
