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
export * from "./hooks/usePlayingVideoTrack";
export * from "./hooks/useChapters";
export * from "./hooks/usePopular";
export * from "./hooks/useFavorite";
export * from "./hooks/usePlayOnDevices";
export * from "./hooks/useBackgroundTasks";
export * from "./hooks/useUpdateStatus";
export * from "./hooks/useBackgroundJobsForm";
export * from "./hooks/useCpuBudgetForm";
export * from "./hooks/useParentalGate";
export * from "./hooks/useWatchedMap";
export * from "./hooks/useActiveProfile";
export * from "./hooks/useNextEpisode";
export * from "./hooks/useUpNextCountdown";
export * from "./hooks/useChapterSkippables";
export * from "./hooks/useAuthenticatedImage";
export * from "./hooks/useProgressReporter";
export * from "./hooks/useStreamToken";
export * from "./hooks/usePluginThemes";

export * from "./lib/authRefreshCore";
export * from "./lib/fetcherSetup";
export * from "./lib/errorClassification";
export * from "./lib/shed";
export * from "./lib/servedVariant";
export * from "./lib/mutationErrorMessage";
export * from "./lib/mutationErrorNet";
export * from "./lib/certifications";
export * from "./lib/resumeNudge";
export * from "./lib/seekGesture";
export * from "./lib/network-scan";
export * from "./lib/pluginBridge";
export * from "./lib/serverIdentity";
export * from "./lib/artwork";
export * from "./lib/pluginTheme";
export * from "./lib/trickplayCues";
export * from "./lib/homeLibraryRows";
export * from "./lib/homeSections";
export * from "./lib/feedCustomize";
export * from "./lib/heroTitle";
export * from "./lib/homeShortcut";
export * from "./lib/genres";
export * from "./lib/librarySelection";
export * from "./lib/progressPercent";
export * from "./lib/playbackPrefs";
export * from "./lib/playTarget";
export * from "./lib/trailerUrl";
export * from "./lib/personRole";
export * from "./lib/mediaTypeLabel";
export * from "./lib/tvLayout";
export * from "./lib/streamBadges";
export * from "./lib/streamInfoModel";
export * from "./lib/syncplayReconcile";
export * from "./lib/profilePicker";
export * from "./hooks/useSyncplay";
export * from "./hooks/useSyncPlaySocket";
export * from "./lib/playlistType";
export * from "./lib/playerTitle";
export * from "./lib/playerExit";
export * from "./lib/resumePosition";
export * from "./lib/clientUserAgent";
export * from "./lib/subtitleParse";
export * from "./lib/reorder";
export * from "./lib/mediaFile";
export * from "./lib/collectionSubtitle";
export * from "./lib/loadTest";
export * from "./lib/reachability";
export * from "./lib/repairLabels";
export * from "./lib/skipSegments";
export * from "./lib/parentalPin";
export * from "./hooks/useSubtitleTrack";
export * from "./hooks/useTrickplayCues";
export * from "./lib/subtitleStyle";
export * from "./lib/searchResultsView";
export * from "./lib/uiScale";
export * from "./lib/deviceRegistrationCore";
export * from "./lib/photoGrid";
export * from "./lib/photoRoutes";
export * from "./lib/photoSlideshow";
export * from "./lib/musicRoutes";
export * from "./lib/logMedia";
export * from "./lib/webhookEvents";

export * from "./plugins/slotEntries";
export * from "./plugins/hostProtocol";
export * from "./plugins/pluginRpc";
export * from "./plugins/bridgeScript";

export * from "./api/collections";
export * from "./api/playlists";
