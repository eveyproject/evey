// @evey/client/native - the shared modules that reach react-native, expo or
// a native probe (@evey/media-caps' expo module, expo-audio, expo-secure-store,
// expo-router). Mobile and TV import this entry; the web must not, because
// bundling it would drag react-native into a browser build. Everything
// platform-free lives in the main entry.
//
// Apps import the MODULE they need (@evey/client/native/<name>), not this
// barrel: a barrel evaluates every native module at once, and a test that
// doubles react-native for one hook would trip on another's module-load probe.
export * from "./native/outputChannels";
export * from "./native/audio-session";
export * from "./native/useAudioCapsBeacon";
export * from "./native/useDecoderTelemetry";

export * from "./native/refresh-token";
export * from "./native/sign-out";
export * from "./native/subtitleStyle";
export * from "./native/server-config";
export * from "./native/useServerUrl";
export * from "./native/usePresenceHeartbeat";
export * from "./native/auth-refresh";
export * from "./native/displayHdr";
export * from "./native/deviceCodecs";
export * from "./native/useDeviceRegistration";
export * from "./native/useRefetchOnReturn";
export * from "./native/fetcher-setup";
export * from "./native/PluginWebView";
