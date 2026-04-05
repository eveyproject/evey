let gc: any = null;
let nativeAvailable = false;
try {
  gc = require("react-native-google-cast");
  // Verify native module is actually linked (not just JS stub)
  const ctx = gc?.default ?? gc?.CastContext;
  ctx?.getCastState?.();
  nativeAvailable = true;
} catch { /* not installed or native module missing */ }

const safe = nativeAvailable ? gc : null;
export const CastContext = safe?.default ?? safe?.CastContext ?? null;
export const CastButtonNative = safe?.CastButton ?? null;
export const useCastStateHook = safe?.useCastState ?? (() => null);
export const useRemoteMediaClientHook = safe?.useRemoteMediaClient ?? (() => null);
export const useMediaStatusHook = safe?.useMediaStatus ?? (() => null);
export const MediaStreamType = safe?.MediaStreamType ?? { BUFFERED: "buffered" };
export const CastStateEnum = safe?.CastState ?? { CONNECTED: "connected" };
