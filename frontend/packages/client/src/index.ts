// @evey/client - the hooks and logic the three clients (web, mobile, TV) run
// unchanged. Nothing here imports react-native, expo or the DOM: a module
// that needs the platform takes it as an argument (a logger, an unlock store,
// a flag) and the app's re-export wires it. Each app keeps a one-line module
// at its old path so screens and their tests import what they always did.
// Modules that reach a native probe are in ./native (@evey/client/native).
export * from "./log";

export * from "./hooks/useSleepTimer";
export * from "./hooks/useDebounce";
export * from "./hooks/useMediaSegments";
export * from "./hooks/useAdminSegments";
export * from "./hooks/usePlaybackSettings";
export * from "./hooks/useChapters";
export * from "./hooks/usePopular";
export * from "./hooks/useFavorite";
export * from "./hooks/useParentalGate";
export * from "./hooks/useWatchedMap";
export * from "./hooks/useActiveProfile";
export * from "./hooks/useNextEpisode";
export * from "./hooks/useAuthenticatedImage";
export * from "./hooks/useProgressReporter";

export * from "./lib/errorClassification";
export * from "./lib/mutationErrorMessage";
export * from "./lib/mutationErrorNet";
export * from "./lib/certifications";
export * from "./lib/resumeNudge";
export * from "./lib/network-scan";
export * from "./lib/pluginBridge";

export * from "./api/collections";
export * from "./api/playlists";
