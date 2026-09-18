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
