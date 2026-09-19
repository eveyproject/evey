// JS face of the Cast Connect receiver (Android TV only).
//
// Everything here is optional at runtime: the module is absent on tvOS, in
// Expo Go and in the unit tests, and every call degrades to a no-op rather
// than throwing. A television that cannot be a Cast Connect target is not a
// broken television - it simply keeps receiving casts through Evey's Web
// Receiver, like every other device.
import { parseCastCustomData, type CastMediaPayload } from "@evey/media-caps/launch";

export type CastCommandKind = "play" | "pause" | "stop" | "seek" | "next" | "previous";

export interface CastCommand {
  kind: CastCommandKind;
  /** Only for "seek". */
  positionMs?: number;
}

interface EveyCastTvNative {
  start(): boolean;
  stop(): boolean;
  acknowledgeReady(): boolean;
  setPlaybackState(positionMs: number, durationMs: number, playing: boolean, speed: number): boolean;
  setMetadata(title: string, subtitle: string): boolean;
  clearPlayback(): boolean;
  setSessionState(signedIn: boolean, serverOrigin: string, userId: string, profileId: string): boolean;
  addListener(event: string, cb: (payload: Record<string, unknown>) => void): { remove(): void };
}

function native(): EveyCastTvNative | null {
  try {
    // Required lazily: importing expo-modules-core at module scope breaks the
    // web bundle and the vitest run, neither of which has a native side.

    const { requireOptionalNativeModule } = require("expo-modules-core") as typeof import("expo-modules-core");
    return requireOptionalNativeModule<EveyCastTvNative>("EveyCastTv");
  } catch {
    return null;
  }
}

export function isCastConnectAvailable(): boolean {
  return native() !== null;
}

export function startCastReceiver(): void {
  try { native()?.start(); } catch { /* never block the app's startup */ }
}

export function stopCastReceiver(): void {
  try { native()?.stop(); } catch { /* nothing to stop */ }
}

/** Replays a LOAD that arrived while the JS bundle was still booting. */
export function acknowledgeReady(): void {
  try { native()?.acknowledgeReady(); } catch { /* no receiver here */ }
}

/**
 * Pushes the Evey player's state into the MediaSession. This is a projection
 * of what the player already decided - call it from the player's own state,
 * never compute a separate truth for it.
 */
export function publishPlaybackState(o: { positionSec: number; durationSec: number; playing: boolean; speed?: number }): void {
  try {
    native()?.setPlaybackState(
      Math.max(0, Math.round(o.positionSec * 1000)),
      Math.max(0, Math.round(o.durationSec * 1000)),
      o.playing,
      o.speed ?? 1,
    );
  } catch { /* the session is a nicety, never a failure path */ }
}

export function publishMetadata(title: string, subtitle = ""): void {
  try { native()?.setMetadata(title, subtitle); } catch { /* ignore */ }
}

export function clearPlaybackState(): void {
  try { native()?.clearPlayback(); } catch { /* ignore */ }
}

/**
 * Mirrors the facts the LaunchRequestChecker reads before a cast is accepted.
 * Must be called on every login, logout, server change and profile switch:
 * a stale mirror is how a television ends up accepting a cast for an account
 * or an instance it no longer serves.
 */
export function publishSessionState(o: { signedIn: boolean; serverOrigin?: string | null; userId?: string | null; profileId?: string | null }): void {
  try {
    native()?.setSessionState(o.signedIn, o.serverOrigin ?? "", o.userId ?? "", o.profileId ?? "");
  } catch { /* ignore */ }
}

/** A sender asked this app to play something. */
export function onCastLoad(cb: (payload: CastMediaPayload) => void): () => void {
  const n = native();
  if (!n) return () => {};
  const sub = n.addListener("onCastLoad", (e) => {
    const parsed = parseCastCustomData(e?.raw);
    // A LOAD we cannot read is not ours to honour: it carries no media id, so
    // there is nothing to play. Dropped rather than guessed.
    if (parsed) cb(parsed);
  });
  return () => sub.remove();
}

/** The remote's media keys, and every connected sender's transport controls. */
export function onCastCommand(cb: (cmd: CastCommand) => void): () => void {
  const n = native();
  if (!n) return () => {};
  const sub = n.addListener("onCastCommand", (e) => {
    const kind = e?.kind;
    if (typeof kind !== "string") return;
    const positionMs = typeof e?.positionMs === "number" ? e.positionMs : undefined;
    cb({ kind: kind as CastCommandKind, positionMs });
  });
  return () => sub.remove();
}
