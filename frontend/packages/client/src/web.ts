// @evey/client/web - the shared modules that need a browser: canPlayType and
// MSE probes, the AudioContext output probe, hls.js. The web app and the TV
// web client import this entry; the React Native apps must not, because
// nothing here runs without a DOM. Everything platform-free lives in the
// main entry.
//
// Apps import the MODULE they need (@evey/client/web/<name>) when a test
// doubles one of them; the barrel is for app code.
export * from "./web/codec";
export * from "./web/outputChannels";
export * from "./web/hlsCodecParams";
export * from "./web/usePlaySession";
export * from "./web/useTvPlaySession";
export * from "./web/PluginIframe";
export * from "./web/useDeviceRegistration";
export * from "./web/usePresenceHeartbeat";
