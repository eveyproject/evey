// Shared implementation lives in @evey/client (packages/client/src/native/fetcher-setup.ts).
// This module keeps the path the app and its tests import, and the phone's
// policy for a dead session: sign out, forget the tokens and the cached
// profile list, go to the login screen.
import { initFetcher as initSharedFetcher } from "@evey/client/native/fetcher-setup";
import { leaveOnlyWhenSessionGone } from "@evey/client";
import { useAuthStore, useServerListStore, clearProfileCache } from "@evey/store";
import { router } from "expo-router";
import { resolveServerUrl } from "./server-config";
import { clearRefreshToken } from "./refresh-token";
import { eveyClientUserAgent } from "./clientIdentity";

export async function initFetcher() {
  await initSharedFetcher({
    // Same resolution as the boot and every screen: active server first, legacy
    // key as the fallback (see resolveServerUrl for what divergence cost us).
    resolveServerUrl: () => resolveServerUrl(useServerListStore.getState().getActiveServer()?.url),
    userAgent: eveyClientUserAgent(),
    // Only leave when the session is really gone (a refused refresh clears
    // the tokens): a 401 scoped to one call while a live token is held - the
    // refresh just succeeded - is that call's problem, and it signed the
    // viewer out with a refresh token still valid. The television's guard.
    onUnauthorized: leaveOnlyWhenSessionGone(() => {
      useAuthStore.getState().logout();
      clearRefreshToken().catch((err) => console.warn("clear refresh token failed", err));
      // The cached profile list describes ONE account on ONE server. Left behind,
      // it would name the previous household's profiles on the next login.
      void clearProfileCache();
      router.replace("/login");
    }),
  });
}
