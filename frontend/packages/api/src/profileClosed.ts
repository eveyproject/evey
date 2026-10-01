// The "profile closed" signal: the server refused a request because a kids
// profile is outside its opening hours. Kept free of imports so the fetcher
// can raise it without a cycle.

export interface ProfileClosedInfo {
  /** RFC 3339, when the profile opens again; absent when not within a week. */
  nextOpen?: string;
  /** The profile the server found closed; a late answer may name one the device has left. */
  profileId?: string;
}

type Listener = (info: ProfileClosedInfo) => void;
const listeners = new Set<Listener>();

/** Subscribe to "the server refused a request because the profile is closed". */
export function onProfileClosed(fn: Listener): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function notifyProfileClosed(info: ProfileClosedInfo) {
  for (const fn of listeners) {
    try {
      fn(info);
    } catch {
      // A listener must not break the request that revealed the closing.
    }
  }
}

/** The refusal body: the gateway's status JSON with an ErrorInfo detail. */
export function profileClosedFromBody(body: unknown): ProfileClosedInfo | null {
  const details = (body as { details?: unknown } | null)?.details;
  if (!Array.isArray(details)) return null;
  for (const d of details) {
    const det = d as { reason?: unknown; metadata?: Record<string, string> } | null;
    if (det?.reason === "PROFILE_CLOSED") {
      const info: ProfileClosedInfo = {};
      if (det.metadata?.next_open) info.nextOpen = det.metadata.next_open;
      if (det.metadata?.profile_id) info.profileId = det.metadata.profile_id;
      return info;
    }
  }
  return null;
}

export function isProfileClosedError(err: unknown): err is Error & { errorCode: "profile_closed"; nextOpen?: string } {
  return (err as { errorCode?: unknown } | null)?.errorCode === "profile_closed";
}

