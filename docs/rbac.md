# RBAC — Role-Based Access Control

Evey's gRPC surface is gated by a method-to-role map enforced in a single
interceptor before any handler runs. This doc covers what the policy
does, where it lives, how to extend it, and the common mistakes it
tends to cause during audits.

## Where it lives

| File | Purpose |
|---|---|
| `internal/adapters/driving/grpcapi/interceptors/rbac.go` | Policy: public methods + method→role map |
| `internal/adapters/driving/grpcapi/interceptors/auth.go` | Enforcement: `AuthInterceptor.enforceRBAC` runs before every unary/stream handler |
| `cmd/evey/grpc.go` | Wire: `AuthInterceptor.WithRBAC(DefaultRBACConfig()).Unary()` / `.Stream()` |

## The three buckets

Every gRPC method falls into exactly one bucket. The interceptor decides
which path to take based on `grpc.UnaryServerInfo.FullMethod`
(e.g. `/evey.v1.LibraryService/DeleteLibrary`).

1. **`PublicMethods`** — no authentication required. Login, refresh
   token, register-with-invite, password reset, QuickConnect initiate /
   check, `AdminSystemService/SetupWizard`, `AdminSystemService/GetSetupStatus`.
   These RPCs must be reachable by an anonymous client so the user can
   obtain a token (or the first admin can finish setup).

2. **`MethodRoles[...] == RoleAdmin`** — require `claims.IsAdmin == true`.
   Everything destructive, everything that mutates server-wide config,
   everything that touches other users' data, everything that invokes
   TMDB/OMDb at server cost:
   - `Admin*Service/*` — system, task, session, plugin, media admin RPCs
   - Library CRUD: `CreateLibrary`, `UpdateLibrary`, `DeleteLibrary`,
     `ScanLibrary`
   - User management: `CreateUser`, `DeleteUser`, `DisableUser`,
     `ListUsers`, `CreateAPIKey`, `ListAPIKeys`, `RevokeAPIKey`
   - User invites (`CreateInviteLink`)
   - Library access management: `Get/Set/RevokeUserLibraryAccess`
   - Metadata: `RefreshMetadata`, `SearchMetadata`, overrides
   - MKV tooling, transcode cache, webhook management, backups,
     remote access, playback settings, server config
   - Collection CRUD: `CreateCollection`, `UpdateCollection`,
     `DeleteCollection`, `AddCollectionItem`
   - Photo deletion, music delete/scan

3. **Default (absent from both maps) = `RoleUser`** — any authenticated
   user. Reading one's own data, playback, favorites, playlists,
   collections read, push subscription management.

## Why two layers (RBAC + handler checks)

RBAC answers "is this caller allowed to reach this endpoint at all".
It does **not** answer "is this caller allowed to touch **this specific
row**". For user-scoped resources (library access grants, playlist
ownership, watch history, session ownership) the handler itself runs
`checkLibraryAccess()` / owner checks / `requireClaims()`.

Example: `GetMediaItem` is `RoleUser` (any authenticated user can call
it), but the handler then calls `checkLibraryAccess(ctx, item.LibraryID)`
so user A can't read media from library B they weren't granted.

Admin RPCs don't need the second check — they're by design allowed to
see everything.

## Adding a new method

1. Add it to `DefaultRBACConfig()` in `rbac.go`:
   - Public bootstrap? → `PublicMethods[method] = true`
   - Admin-only? → `MethodRoles[method] = RoleAdmin`
   - User-accessible? → **don't add it**; the default is `RoleUser`.
2. Add a test case in `interceptors/rbac_test.go`. The existing table
   verifies every known method resolves to the expected role. When a
   method is missing from the table, the test fails.
3. If the method touches a user-owned resource, add an owner check in
   the handler too (see "Why two layers" above).

## Plugin tokens

Plugins can request short-lived tokens via `UserService/IssuePluginToken`
(deliberately absent from `MethodRoles` — resolves to `RoleUser`, so
iframe plugin consumers can mint them). The service layer then filters
out any `admin:*` / `system:*` scope via `FilterPluginScopes` before
signing the JWT. RBAC can't enforce this distinction at the interceptor
level because scope filtering is about what's *inside* the token, not
who's allowed to call the issue RPC.

## Common audit false-positives

When reviewing a handler for IDOR (e.g. `UpdateLibrary`,
`DeleteCollection`), the usual signal is "no `checkLibraryAccess()`
call — anyone can mutate any library". That's **wrong for admin
endpoints**: the RBAC interceptor has already rejected every non-admin
before the handler runs. Admins are allowed to touch any library by
design.

Rule: before flagging an IDOR on a mutation RPC, grep for the method
in `rbac.go`. If it's mapped to `RoleAdmin`, the finding is
defense-in-depth at best — not exploitable.

## Failure modes & logging

- Unauthenticated call to a protected method → `codes.Unauthenticated`,
  translated via `translator.TranslatedError(ctx, codes.Unauthenticated,
  "errors.auth.missing_authorization")`.
- Authenticated user hitting an admin RPC → `codes.PermissionDenied`,
  `"errors.auth.admin_access_required"`.
- API-key-based auth (`UserService.CreateAPIKey`) follows the same
  RBAC path; the API key's owner's `is_admin` flag decides.
- QuickConnect-acquired tokens carry the authorizing user's roles —
  a TV that authorizes with a non-admin account cannot call admin
  RPCs even if the user is admin on another device.

## Related docs

- [`SECURITY.md`](./SECURITY.md) — plugin sandbox + supply-chain model
  (narrower scope than RBAC)
- [`ADR 0003: Plugin-scoped tokens`](./adr/0003-plugin-scoped-tokens.md)
  — why plugin tokens filter admin scopes instead of relying on RBAC
