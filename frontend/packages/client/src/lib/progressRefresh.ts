import type { QueryClient, QueryKey } from "@tanstack/react-query";

// The views that show where a viewer stopped: a title's own progress (the
// media page's Resume button) and the resume row.
const PROGRESS_KEY = /^\/api\/v1\/playback\/[^/]+\/progress$/;
const RESUME_KEY = "/api/v1/playback/resume";

export function isProgressViewQuery(key: QueryKey): boolean {
  const root = key[0];
  return typeof root === "string" && (root === RESUME_KEY || PROGRESS_KEY.test(root));
}

// refreshProgressViews re-reads those views once the player's LAST position
// has reached the server. Leaving the player, the media page underneath came
// back and asked for the progress at once - before the final report landed -
// so a first viewing showed Play instead of Resume and started from zero.
export function refreshProgressViews(qc: QueryClient | undefined): void {
  if (!qc) return;
  void qc.invalidateQueries({ predicate: (q) => isProgressViewQuery(q.queryKey) });
}
