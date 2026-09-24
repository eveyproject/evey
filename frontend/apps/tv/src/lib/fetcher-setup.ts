// Shared implementation lives in @evey/client (packages/client/src/native/fetcher-setup.ts).
// This module keeps the path the app and its tests import, and the
// television's policy for a dead session.
import { initFetcher as initSharedFetcher } from "@evey/client/native/fetcher-setup";
import { useAuthStore, useServerListStore } from "@evey/store";
import { router } from "expo-router";
import { resolveServerUrl } from "./server-config";
import { eveyClientUserAgent } from "./clientIdentity";

export async function initFetcher() {
  await initSharedFetcher({
    // Same resolution as the boot and the phone: active server first, legacy
    // key as the fallback. Reading only the legacy key could point the API at
    // another host than the rest of the app (the phone paid for it 2026-07-29).
    resolveServerUrl: () => resolveServerUrl(useServerListStore.getState().getActiveServer()?.url),
    userAgent: eveyClientUserAgent(),
    // Only route to login when the session is actually gone (doRefresh clears
    // the tokens on refusal); a 401 scoped to one call while a token is still
    // held must not eject a viewer who has no keyboard to log back in with.
    onUnauthorized: () => {
      if (useAuthStore.getState().accessToken) return;
      router.replace("/login");
    },
  });
}
