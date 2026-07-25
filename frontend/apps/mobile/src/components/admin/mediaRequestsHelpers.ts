// Pure helpers extracted from the media-requests admin screen so the status
// label/formatting logic can be unit-tested without mounting react-native
// components or the generated @evey/api hooks.

export function capitalizeStatus(s?: string): string {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export type StatusFilter = "all" | "pending" | "approved" | "rejected" | "later";

export const STATUS_FILTERS: StatusFilter[] = ["all", "pending", "approved", "rejected", "later"];

// Maps a request status to the i18n key used by both the web and mobile
// queue rows (requests.status*), keeping the two clients in sync.
export function requestStatusI18nKey(status?: string): string {
  return `requests.status${capitalizeStatus(status)}`;
}
