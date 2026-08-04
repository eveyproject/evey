// The bare package, not ../i18n: that module also pulls in expo-localization
// at import time (used only for first-launch language detection), which this
// net does not need and which is unmocked outside app screens. i18next's
// default export is a singleton - the app's own bootstrap already configures
// it before any mutation can fire.
import i18n from "i18next";

/**
 * What to say when a write failed and the screen that started it said
 * nothing. Mirrors web src/utils/errorClassification.ts: httpStatus is only
 * set once a response actually came back from the server (see
 * packages/api/src/fetcher.ts), so its absence means the request never
 * reached it - offline, not a generic failure.
 */
function isOffline(error: unknown): boolean {
  if (typeof error !== "object" || error === null) return true;
  return typeof (error as { httpStatus?: unknown }).httpStatus !== "number";
}

export function mutationErrorMessage(error: unknown): string {
  return isOffline(error) ? i18n.t("common.offline.description") : i18n.t("common.error");
}
