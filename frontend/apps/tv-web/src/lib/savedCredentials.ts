// Opt-in "remember me", as on the phone: a viewer sent back to this screen
// (expired session, server reinstalled) finds the fields filled instead of
// typing a password with a remote. A browser has no keystore: localStorage is
// the only store a television browser offers, which is why it stays opt-in.
// Kept apart from the refresh token so a session ending never erases it.
const USER_KEY = "evey-tv-saved-username";
const PASS_KEY = "evey-tv-saved-password";

export function getSavedCredentials(): { username: string; password: string } | null {
  try {
    const username = localStorage.getItem(USER_KEY);
    const password = localStorage.getItem(PASS_KEY);
    return username && password ? { username, password } : null;
  } catch {
    return null;
  }
}

export function saveCredentials(username: string, password: string): void {
  try {
    localStorage.setItem(USER_KEY, username);
    localStorage.setItem(PASS_KEY, password);
  } catch {
    /* storage refused: the box just won't stick */
  }
}

export function clearSavedCredentials(): void {
  try {
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(PASS_KEY);
  } catch {
    /* ignore */
  }
}
