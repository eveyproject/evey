# ADR-0003: Plugin-scoped JWT tokens

- Status: Accepted
- Amended by: [ADR-0012](0012-plugin-token-deny-by-default.md) (enforcement)

## Context

Plugins run inside sandboxed iframes and call the Evey API on the user's behalf. Handing them the full user session token would give every plugin full account privileges, including admin and write access to libraries, users, and system settings. That is unacceptable for third-party or community plugins.

## Decision

Introduce short-lived plugin-scoped JWTs:

- Algorithm: **RS256** (asymmetric, public key exposed for verification).
- TTL: **5 minutes**. Plugins refresh via the host.
- Claims: `{ sub, plugin_id, scopes, exp, iat, jti }`.
- Issued by `app/plugin.TokenService.Mint(ctx, userID, pluginID, requestedScopes)` after intersecting requested scopes with the plugin manifest's declared scopes and the user's permissions.
- Verified by `pkg/auth.PluginJWTVerifier` in `HTTPAuthMiddleware` when the request originates from the plugin host.

### Scope whitelist

Only read-oriented, narrowly scoped capabilities are allowed: `library:read`, `media:read`, `metadata:read`, `playback:read`, `playback:control:self`, `favorite:read`, `playlist:read`, `notification:read`.

### Forbidden scopes (hard-rejected at mint time)

`admin:*`, `auth:*`, `user:write`, `library:write`, `system:*`.

Any token carrying a forbidden scope is rejected both at mint and at verification.

## Consequences

- A compromised plugin cannot escalate to admin, modify libraries, or touch auth.
- Short TTL limits replay windows; `jti` enables revocation if needed.
- Plugin manifests must declare scopes up-front; UI surfaces them during install.
- Adding new scopes requires an explicit whitelist update and review.

## References

- `internal/pkg/auth/plugin_jwt.go`
- `internal/app/plugin/token_service.go`
- [ADR-0001](0001-hexagonal-architecture.md)

## Amendment

Implementation drifted from this record; the decision (short-lived, scope-filtered, RS256, 5-min TTL) stands, but the specifics below are corrected:

- **Scope notation is dot-based**, not colon-based: `media.read`, `library.write`, `notification.sender`, `api.library`, … The forbidden set is the prefixes `admin.` / `auth.` / `system.` plus the exact scopes `user.write` and `library.write` (`pkg/auth.forbiddenPrefix` + the forbidden-exact set in `plugin_jwt.go`).
- The live allow-list is `pkg/auth.AllowedPluginScopes` (`plugin_jwt.go`), which is broader than this ADR's original read-only list (it includes e.g. `media.write`, `api.*`, `notification.sender`, and the later `public` / `prefs` capabilities).
- Method names: minting is `TokenService.IssueToken` (not `Mint`); verification is `JWTGenerator.ParsePluginToken` (there is no `PluginJWTVerifier`).
- Enforcement is no longer "filter at mint and trust thereafter": it is **deny-by-default at the interceptor** - see [ADR-0012](0012-plugin-token-deny-by-default.md).
