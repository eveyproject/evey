// The wiring itself is ../lib/fetcherSetup (shared with the browser TV
// client); this module routes the 401 retry through the device's own refresh
// flow, the one bound to its secure store.
import { initFetcher as initSharedFetcher, type FetcherSetupOptions as SharedOptions } from "../lib/fetcherSetup";
import { forceRefresh } from "./auth-refresh";

export type FetcherSetupOptions = Omit<SharedOptions, "forceRefresh" | "userAgent"> & { userAgent: string };

export async function initFetcher(options: FetcherSetupOptions) {
  await initSharedFetcher({ ...options, forceRefresh });
}
