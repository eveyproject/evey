# ADR-0005: Two-layer authorization (interceptor RBAC + handler resource check)

- Status: Accepted
- Amended by: [ADR-0012](0012-plugin-token-deny-by-default.md) - mint-time scope filtering proved insufficient; enforcement is now deny-by-default at the interceptor
- Date: 2026-04-20

## Context

Evey's gRPC API exposes both server-wide admin operations (library CRUD, user management, system config) and per-user resources (playlists, favorites, watch history, library-scoped media). Mixing both on a single authorization layer produces two recurring failure modes:

- **Forgotten admin gate.** A new admin RPC ships without the `IsAdmin` check and any authenticated user can delete libraries. Easy to miss in review because the handler looks like the user RPCs sitting next to it.
- **Forgotten resource check.** A user RPC reads `media_item_id` from the request and returns the row without verifying the caller has access to that row's library. Classic IDOR.

We also run periodic security audits. Every pass so far has surfaced false-positive IDOR findings on admin-only endpoints - the audit agent sees "no `checkLibraryAccess()` call" in `DeleteLibrary` and flags HIGH, missing that the interceptor one layer up has already rejected every non-admin.

## Decision

Split authorization into two distinct layers with orthogonal responsibilities.

**Layer 1 - Interceptor RBAC: "can you reach this endpoint at all".** A single unary/stream interceptor (`AuthInterceptor.enforceRBAC`) consults `DefaultRBACConfig()` before any handler runs and bucketises every gRPC method:

- **`PublicMethods`** - no authentication required (`Login`, `RefreshToken`, `RegisterWithInvite`, `RequestPasswordReset`, `ResetPassword`, `InitiateQuickConnect`, `CheckQuickConnect`, setup-wizard RPCs).
- **`MethodRoles[m] == RoleAdmin`** - every destructive, system-wide, or cross-user operation. Rejection with `codes.PermissionDenied` if `claims.IsAdmin == false`.
- **Default (absent from both maps) == `RoleUser`** - any authenticated caller.

The interceptor is the only authorization code the `/evey.v1.Admin*Service/*` handlers see. They trust the layer.

**Layer 2 - Handler resource check: "can you touch THIS row".** For user-scoped resources the handler runs an owner / library-access check against `claims.UserID` - either a handler helper like `checkLibraryAccess(ctx, libraryID)`, or a delegation to the app service that raises a typed ownership error (e.g. playlists return `appplaylist.ErrNotOwner` when `claims.UserID` isn't the owner). This layer cannot be promoted to the interceptor because the resource identifier is inside the request proto, not in the method name - the interceptor has no way to resolve `media_item_id → library_id → granted?` without decoding each proto.

Admins skip Layer 2 by design: they're trusted to operate on any library.

## Consequences

- **Adding an admin RPC is one line** in `rbac.go`. Forgetting it is loud: the method defaults to `RoleUser`, the `rbac_test.go` table-check fails when the method is missing from expectations.
- **Adding a user RPC that touches a library/playlist/session requires a handler-level owner check.** No interceptor layer can do it for you.
- **Audit tools shipping without RBAC awareness misfire.** We accept the cost: the rule for future audits is "before flagging IDOR on a mutation RPC, grep `rbac.go` - `RoleAdmin` = not exploitable".
- **Plugin tokens need a third lever** (scope filtering at mint time) because an authenticated plugin consumer can call `IssuePluginToken` (RoleUser by design) but must not receive admin scopes. See [ADR-0003](0003-plugin-scoped-tokens.md).
- **Defense-in-depth is intentionally skipped on admin RPCs.** We do NOT add a redundant `checkLibraryAccess()` inside `DeleteLibrary` "just in case RBAC is bypassed". If the interceptor is bypassed, the whole server is compromised; a second check on one handler doesn't change that.

## References

- `internal/adapters/driving/grpcapi/interceptors/rbac.go` - policy
- `internal/adapters/driving/grpcapi/interceptors/auth.go` - enforcement (`enforceRBAC`)
- `internal/adapters/driving/grpcapi/interceptors/rbac_test.go` - table-check forcing every known method to appear in expectations
- [ADR-0003](0003-plugin-scoped-tokens.md) - why plugin tokens filter scopes at mint instead of trusting the RBAC layer
