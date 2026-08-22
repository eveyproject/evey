// Retry policy shared by every react-query query. Lives in its own module so
// the rule can be asserted directly instead of only through main.tsx, which
// boots the whole app on import.

const MAX_RETRIES = 2;

// Statuses a retry can never repair.
// - 401: the refresh flow owns it; a bare retry re-sends the same dead token.
// - 403: a permission does not appear because we asked twice.
// - 404 / 410: retrying makes a deleted item sit through two backoffs before
//   the screen can even say "not found".
// - 429: retrying is exactly what keeps the limiter that just refused us
//   saturated. It tripled the favourites self-DoS (4 631 refusals in one hour
//   for a single browser, 2026-08-15). The UI surfaces Retry-After instead.
const NON_RETRYABLE = new Set([401, 403, 404, 410, 429]);

export function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  const status = (error as { httpStatus?: number } | null)?.httpStatus;
  if (status !== undefined && NON_RETRYABLE.has(status)) return false;
  return failureCount < MAX_RETRIES;
}
