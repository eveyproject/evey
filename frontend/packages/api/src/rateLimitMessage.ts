// Turning a 429 into something a human can act on.
//
// "Too many requests" with no delay is unactionable: the user tries again,
// spends the little credit that refilled, and stays locked out believing the
// app is broken (report 2026-07-29). The server sends the real wait in the
// Retry-After header AND in the body; this renders it as a duration.
//
// Shared by web, mobile and TV: a rate-limited login must read the same on
// every client.

export interface RateLimited {
  retryAfterSeconds: number;
}

// parseRateLimit returns the wait when the response is a 429, else null.
// Header first (standard), body second (proxies strip headers), and a 60s
// floor when neither says anything - never "unknown".
export function parseRateLimit(
  status: number,
  header: string | null | undefined,
  body: unknown,
): RateLimited | null {
  if (status !== 429) return null;
  const fromHeader = Number.parseInt(String(header ?? ""), 10);
  if (Number.isFinite(fromHeader) && fromHeader > 0) return { retryAfterSeconds: fromHeader };
  const b = body as { retryAfterSeconds?: unknown } | null;
  const fromBody = typeof b?.retryAfterSeconds === "number" ? b.retryAfterSeconds : NaN;
  if (Number.isFinite(fromBody) && fromBody > 0) return { retryAfterSeconds: Math.ceil(fromBody) };
  return { retryAfterSeconds: 60 };
}

// formatWait renders seconds as the coarsest honest unit, rounding UP so the
// user never comes back a moment too early.
export function formatWait(
  seconds: number,
  t: (key: string, fallback: string, params?: Record<string, number>) => string,
): string {
  if (seconds < 60) {
    const s = Math.max(1, Math.ceil(seconds));
    return t("auth.rateLimit.seconds", "{{count}} s", { count: s });
  }
  const minutes = Math.ceil(seconds / 60);
  return t("auth.rateLimit.minutes", "{{count}} min", { count: minutes });
}
