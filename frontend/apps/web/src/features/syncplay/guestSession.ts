// A "watch together" guest is not logged in: instead of a JWT it holds a
// media-scoped guest stream token minted by POST /api/v1/syncplay/join-code.
// We stash it in sessionStorage (not localStorage: it must not outlive the tab)
// so the public /watch/$mediaId route can pick it up. Reload → re-enter the code.
export interface GuestSession {
  token: string;
  roomId: string;
  mediaId: string;
}

const KEY = "evey-guest-session";

export function writeGuestSession(s: GuestSession): void {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(s));
  } catch {
    /* storage disabled — the watch route will just redirect to login */
  }
}

export function readGuestSession(mediaId: string): GuestSession | null {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as GuestSession;
    return s.mediaId === mediaId && s.token && s.roomId ? s : null;
  } catch {
    return null;
  }
}

export function clearGuestSession(): void {
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    /* ignore */
  }
}
