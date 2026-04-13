# ADR-0003: Plugin-scoped JWT tokens

- Status: Accepted
- Date: 2026-02-20

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
