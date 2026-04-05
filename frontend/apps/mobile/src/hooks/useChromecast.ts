let gc: any = null;
try { gc = require("react-native-google-cast"); } catch { /* not installed */ }

export const CastContext = gc?.default ?? gc?.CastContext ?? null;
export const CastButtonNative = gc?.CastButton ?? null;
export const useCastStateHook = gc?.useCastState ?? (() => null);
export const useRemoteMediaClientHook = gc?.useRemoteMediaClient ?? (() => null);
export const useMediaStatusHook = gc?.useMediaStatus ?? (() => null);
export const MediaStreamType = gc?.MediaStreamType ?? { BUFFERED: "buffered" };
export const CastStateEnum = gc?.CastState ?? { CONNECTED: "connected" };
