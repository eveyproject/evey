import { CAPS_NAMESPACE, type CapsTransport } from "@evey/media-caps";

let gc: any = null;
let nativeAvailable = false;
try {
  gc = require("react-native-google-cast");
  // Verify native module is actually linked (not just JS stub)
  const ctx = gc?.default ?? gc?.CastContext;
  ctx?.getCastState?.();
  nativeAvailable = true;
} catch (err) { console.warn("not installed or native module missing", err); }

const safe = nativeAvailable ? gc : null;
export const CastContext = safe?.default ?? safe?.CastContext ?? null;
export const CastButtonNative = safe?.CastButton ?? null;
export const useCastStateHook = safe?.useCastState ?? (() => null);
export const useRemoteMediaClientHook = safe?.useRemoteMediaClient ?? (() => null);
export const useMediaStatusHook = safe?.useMediaStatus ?? (() => null);
// Connected receiver metadata (modelName drives the cast capability profile,
// see @evey/media-caps castUrl). Same defensive fallback as every other hook here: no
// native module, no device info, buildCastStreamUrl falls back to the floor.
export const useCastDeviceHook = safe?.useCastDevice ?? (() => null);
export const CastStateEnum = safe?.CastState ?? { CONNECTED: "connected" };

// Imperative device read for the moment of loadMedia: the useCastDevice hook
// delivers the device on a LATER render, so the auto-load that fires right on
// connection still sees null and the capability profile silently falls back
// to the floor (observed in prod: beacon "model=unknown" while a real TV was
// connected). The session is authoritative and available synchronously after
// connect.
export async function getConnectedDeviceModelName(): Promise<string | null> {
  try {
    const session = await CastContext?.getSessionManager?.()?.getCurrentCastSession?.();
    const device = await session?.getCastDevice?.();
    return device?.modelName ?? null;
  } catch {
    return null;
  }
}

// CapsChannel is the caps handshake's transport over the CURRENT session's
// custom channel (react-native-google-cast CastChannel on CAPS_NAMESPACE).
// close() destroys the channel: nothing the receiver says afterwards - a late
// answer from a session the caller has moved on from - can reach anyone.
export interface CapsChannel {
  transport: CapsTransport;
  close(): Promise<void>;
}

// openCapsChannel binds a transport to the session connected right now, or
// null when there is none (no native module, no session, the channel refused):
// the caller then negotiates from the model name or the floor. Opened per
// ask, never kept across sessions.
export async function openCapsChannel(): Promise<CapsChannel | null> {
  try {
    const session = await CastContext?.getSessionManager?.()?.getCurrentCastSession?.();
    if (!session?.addChannel) return null;
    const channel = await session.addChannel(CAPS_NAMESPACE);
    if (!channel) return null;
    let closed = false;
    return {
      transport: {
        send: (request) => (closed ? Promise.resolve() : channel.sendMessage(request)),
        subscribe: (onMessage) => {
          channel.onMessage((message: unknown) => { if (!closed) onMessage(message); });
          return () => channel.offMessage();
        },
      },
      close: async () => {
        closed = true;
        try { await channel.remove(); } catch { /* session already gone */ }
      },
    };
  } catch {
    return null;
  }
}
