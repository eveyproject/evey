// The Cast Connect contract: what a sender tells an Android TV receiver, and
// when.
//
// Two payloads, deliberately separate, because they are read at two different
// moments by two different deciders:
//
//   1. CREDENTIALS (setLaunchCredentialsData on the sender) travel with the
//      LAUNCH, before any media. The Evey TV app's LaunchRequestChecker reads
//      them to answer one question - "should I take this cast at all?" - and
//      a "no" is what makes the platform fall back to our Web Receiver. The
//      sharp case is the one that bit us on 2026-09-19: the television logged
//      into the DEV server while the phone was on prod. Handing that TV a
//      media id from another instance would open the wrong film, or nothing.
//      So the origin rides here, not in the media payload: by the time a LOAD
//      arrives the choice of receiver has already been made.
//
//   2. MEDIA (customData.evey on the LoadRequest) travels with the LOAD. It
//      names WHAT to play by media id, never by URL: the TV app negotiates
//      its own stream with its own capabilities, the way it does when you
//      press play on its remote. The LoadRequest's contentId keeps carrying
//      the URL negotiated for the Web Receiver, so one sender feeds both
//      receivers and neither needs to know about the other.
//
// Both are parsed defensively: they arrive from another process (and, on the
// TV side, from a sender we did not write - a stray Cast client can send
// anything at all).

/** credentialsType for setLaunchCredentialsData. Namespaced, not "json". */
export const CAST_CREDENTIALS_TYPE = "evey";

/** The key our media payload lives under inside the LoadRequest customData. */
export const CAST_CUSTOM_DATA_KEY = "evey";

export interface CastLaunchCredentials {
  v: 1;
  /** Origin of the Evey instance the sender is signed into, no trailing slash. */
  serverOrigin: string;
  /** Account and profile, so the TV can refuse a cast aimed at someone else. */
  userId?: string;
  profileId?: string;
}

export interface CastMediaPayload {
  v: 1;
  mediaId: string;
  /** Absolute position on the media timeline. 0 = from the start. */
  positionSec: number;
  /** Absolute stream indices, as the players use them. */
  audioTrack?: number;
  /** A subtitle index, or "off" for an explicit none. Absent = let the TV decide. */
  subtitleTrack?: number | "off";
  /** Repeated here so a LOAD can be rejected if it somehow crossed instances. */
  serverOrigin: string;
}

// normalizeOrigin strips the trailing slash and lowercases the host, so two
// spellings of the same instance ("https://Evey.test/" and "https://evey.test")
// compare equal. Anything unparseable comes back trimmed, never null: the
// comparison is what decides, and an empty string simply never matches.
export function normalizeOrigin(raw: string | null | undefined): string {
  const s = (raw ?? "").trim();
  if (!s) return "";
  try {
    const u = new URL(s);
    return `${u.protocol}//${u.host.toLowerCase()}`;
  } catch {
    return s.replace(/\/+$/, "");
  }
}

export function buildLaunchCredentials(o: {
  serverOrigin: string;
  userId?: string | null;
  profileId?: string | null;
}): string {
  const creds: CastLaunchCredentials = { v: 1, serverOrigin: normalizeOrigin(o.serverOrigin) };
  if (o.userId) creds.userId = o.userId;
  if (o.profileId) creds.profileId = o.profileId;
  return JSON.stringify(creds);
}

export function parseLaunchCredentials(raw: unknown): CastLaunchCredentials | null {
  const obj = asObject(raw);
  if (!obj || obj.v !== 1) return null;
  const serverOrigin = normalizeOrigin(str(obj.serverOrigin));
  if (!serverOrigin) return null;
  const out: CastLaunchCredentials = { v: 1, serverOrigin };
  const userId = str(obj.userId);
  const profileId = str(obj.profileId);
  if (userId) out.userId = userId;
  if (profileId) out.profileId = profileId;
  return out;
}

export function buildCastCustomData(p: {
  mediaId: string;
  positionSec?: number;
  audioTrack?: number | null;
  subtitleTrack?: number | "off" | null;
  serverOrigin: string;
}): Record<string, CastMediaPayload> {
  const payload: CastMediaPayload = {
    v: 1,
    mediaId: p.mediaId,
    // A negative or non-finite resume is a bug upstream; 0 is the honest
    // fallback (the TV then applies its own watch progress).
    positionSec: Number.isFinite(p.positionSec) && (p.positionSec as number) > 0 ? Math.floor(p.positionSec as number) : 0,
    serverOrigin: normalizeOrigin(p.serverOrigin),
  };
  if (typeof p.audioTrack === "number" && Number.isInteger(p.audioTrack) && p.audioTrack >= 0) {
    payload.audioTrack = p.audioTrack;
  }
  if (p.subtitleTrack === "off" || (typeof p.subtitleTrack === "number" && Number.isInteger(p.subtitleTrack) && p.subtitleTrack >= 0)) {
    payload.subtitleTrack = p.subtitleTrack;
  }
  return { [CAST_CUSTOM_DATA_KEY]: payload };
}

// parseCastCustomData accepts the whole customData object and digs out our
// key, so the caller hands it exactly what the Cast SDK gave it.
export function parseCastCustomData(raw: unknown): CastMediaPayload | null {
  const root = asObject(raw);
  if (!root) return null;
  const obj = asObject(root[CAST_CUSTOM_DATA_KEY]) ?? root;
  if (obj.v !== 1) return null;
  const mediaId = str(obj.mediaId);
  if (!mediaId) return null;
  const pos = obj.positionSec;
  const out: CastMediaPayload = {
    v: 1,
    mediaId,
    positionSec: typeof pos === "number" && Number.isFinite(pos) && pos > 0 ? Math.floor(pos) : 0,
    serverOrigin: normalizeOrigin(str(obj.serverOrigin)),
  };
  if (typeof obj.audioTrack === "number" && Number.isInteger(obj.audioTrack) && obj.audioTrack >= 0) {
    out.audioTrack = obj.audioTrack;
  }
  if (obj.subtitleTrack === "off" || (typeof obj.subtitleTrack === "number" && Number.isInteger(obj.subtitleTrack) && obj.subtitleTrack >= 0)) {
    out.subtitleTrack = obj.subtitleTrack as number | "off";
  }
  return out;
}

// launchVerdict is the LaunchRequestChecker's decision, kept here (pure) so
// both sides agree on it and it can be unit-tested away from Android.
//
// `false` is not a failure: it is how a cast is handed back to the Web
// Receiver, which plays it perfectly well. The TV app takes a cast only when
// it can honestly do better than that - signed in, on the same instance, and
// for the same account.
export function shouldAcceptLaunch(o: {
  credentials: CastLaunchCredentials | null;
  tv: { signedIn: boolean; serverOrigin: string; userId?: string | null };
}): boolean {
  if (!o.tv.signedIn) return false;
  // A sender that says nothing about itself (an older Evey, or a third-party
  // Cast client) gets the Web Receiver: this app cannot know what to play.
  if (!o.credentials) return false;
  const tvOrigin = normalizeOrigin(o.tv.serverOrigin);
  if (!tvOrigin || tvOrigin !== o.credentials.serverOrigin) return false;
  // Accounts must match when both sides name one. A TV signed in as someone
  // else must not answer: the film would play under the wrong watch history,
  // and past the wrong parental gate.
  if (o.credentials.userId && o.tv.userId && o.credentials.userId !== o.tv.userId) return false;
  return true;
}

// userIdFromAccessToken reads the account out of a JWT payload without
// verifying it - this is not authorization, it is the value the TV compares
// against its own so a cast is not accepted for someone else's account. An
// unreadable token yields "", which simply never contradicts.
export function userIdFromAccessToken(token: string | null | undefined): string {
  if (!token) return "";
  try {
    const part = token.split(".")[1];
    if (!part) return "";
    const json = atob(part.replace(/-/g, "+").replace(/_/g, "/"));
    const payload = JSON.parse(json) as Record<string, unknown>;
    return str(payload.sub) || str(payload.user_id);
  } catch {
    return "";
  }
}

function asObject(v: unknown): Record<string, unknown> | null {
  if (typeof v === "string") {
    try {
      return asObject(JSON.parse(v));
    } catch {
      return null;
    }
  }
  return typeof v === "object" && v !== null && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
}

function str(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}
