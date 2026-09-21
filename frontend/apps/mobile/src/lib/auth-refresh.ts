// Shared implementation lives in @evey/client (packages/client/src/native/auth-refresh.ts).
// This module keeps the path the screens and their tests import.
export { isAccessExpired, ensureAccessToken, refreshAccessTokenIfNeeded, forceRefresh } from "@evey/client/native/auth-refresh";
export type { AccessTokenState } from "@evey/client/native/auth-refresh";
