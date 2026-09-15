// A "watch together" guest is not logged in: instead of a JWT it holds a
// media-scoped guest stream token minted by POST /api/v1/syncplay/join-code.
// Mirror of the web features/syncplay/guestSession.ts. RN has no sessionStorage
// and no tabs, so this lives in a module variable: it survives navigation from
// login -> watch within the running app, and is dropped on app restart or when
// the guest leaves (clearGuestSession) - re-enter the code to rejoin.

export interface GuestSession {
  token: string;
  roomId: string;
  mediaId: string;
}

let current: GuestSession | null = null;

export function writeGuestSession(s: GuestSession): void {
  current = s;
}

export function readGuestSession(mediaId: string): GuestSession | null {
  const s = current;
  return s && s.mediaId === mediaId && s.token && s.roomId ? s : null;
}

export function clearGuestSession(): void {
  current = null;
}
