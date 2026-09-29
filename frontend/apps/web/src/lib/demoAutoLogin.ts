// The demo signs its visitors in by itself. Its credentials are public and
// its database is restored every hour, which wipes every refresh token: a
// visitor whose session dies mid-visit would otherwise land on a login form
// asking for a password everyone knows. So on the demo the login page signs
// in as `demo` on its own - unless THIS visitor signed out on purpose, which
// holds the form open for a day so they can try another account or the login
// screen itself.
const MANUAL_LOGOUT_KEY = "evey-demo-manual-logout-at";
export const MANUAL_LOGOUT_HOLD_MS = 24 * 60 * 60 * 1000;

export const DEMO_USERNAME = "demo";
export const DEMO_PASSWORD = "demo";

// markDemoManualLogout records a deliberate sign-out.
export function markDemoManualLogout(now: number = Date.now()): void {
  try {
    localStorage.setItem(MANUAL_LOGOUT_KEY, String(now));
  } catch {
    // no storage: the next visit auto-signs in again, which is the safe default
  }
}

// demoAutoLoginAllowed: no deliberate sign-out in the last day.
export function demoAutoLoginAllowed(now: number = Date.now()): boolean {
  try {
    const raw = localStorage.getItem(MANUAL_LOGOUT_KEY);
    if (!raw) return true;
    const at = Number(raw);
    return !(Number.isFinite(at) && now - at < MANUAL_LOGOUT_HOLD_MS);
  } catch {
    return true;
  }
}
