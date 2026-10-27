// Opt-in "remember me": a viewer sent back to this screen (expired session,
// server reinstalled) finds the username filled. Only the username: a password
// in localStorage is readable by any script on the origin, and Quick Connect
// already spares typing it with a remote.
// Kept apart from the refresh token so a session ending never erases it.
const USER_KEY = "evey-tv-saved-username";
// Written by earlier versions; erased on sight, never read.
const LEGACY_PASS_KEY = "evey-tv-saved-password";

export function getSavedUsername(): string | null {
  try {
    localStorage.removeItem(LEGACY_PASS_KEY);
    return localStorage.getItem(USER_KEY) || null;
  } catch {
    return null;
  }
}

export function saveUsername(username: string): void {
  try {
    localStorage.setItem(USER_KEY, username);
  } catch {
    /* storage refused: the box just won't stick */
  }
}

export function clearSavedUsername(): void {
  try {
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(LEGACY_PASS_KEY);
  } catch {
    /* ignore */
  }
}
