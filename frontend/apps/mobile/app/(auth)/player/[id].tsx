import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Pressable,
  StatusBar,
  StyleSheet,
  useWindowDimensions,
  BackHandler,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView, isPictureInPictureSupported } from "expo-video";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as ScreenOrientation from "expo-screen-orientation";
import * as NavigationBar from "expo-navigation-bar";
import { setAudioBoost, releaseAudioBoost, hasAudioBoost } from "../../../src/lib/audioBoost";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import { resumeNudger } from "../../../src/lib/resumeNudge";
import { playerTrim, resolveVolume } from "../../../src/lib/volumePolicy";
import {
  hasDeviceVolumeControl,
  noteObservedDeviceVolume,
  readDeviceVolumeFraction,
  readDeviceVolumeSteps,
  setDeviceVolumeFraction,
} from "../../../src/lib/deviceVolume";
import { getDownloadPath, getDownloadedMedia, hasRenderableAudio } from "../../../src/lib/downloads";
import {
  saveOfflineProgress,
  getOfflineProgress,
  clearOfflineProgress,
  preferLocalProgress,
  type OfflineProgress,
} from "../../../src/lib/offlineProgress";
import { getDeviceAudioCodecs } from "../../../src/lib/deviceCodecs";
import { useTranslation } from "react-i18next";
import { toast, useAuthStore } from "@evey/store";
import { ICON_SIZE } from "@evey/theme";
import { BottomSheet } from "../../../src/components/BottomSheet";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  createPlayerEventReporter,
  playerAudioStreams,
  playerSubtitleStreams,
  parseTrackPick,
  parseVttCues,
  playbackServiceReportProgress,
  type SubtitleCue,
} from "@evey/api";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useAudioCapsBeacon } from "../../../src/hooks/useAudioCapsBeacon";
import { useDecoderTelemetry } from "../../../src/hooks/useDecoderTelemetry";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlayerGestures } from "../../../src/hooks/usePlayerGestures";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { useHlsRungs } from "../../../src/hooks/useHlsRungs";
import { useSleepTimer } from "../../../src/hooks/useSleepTimer";
import { SleepTimerPicker } from "../../../src/components/SleepTimerPicker";
import { pickNativeAudioTrack } from "../../../src/lib/audioTrackMatch";
import { reportedPlaybackState } from "../../../src/lib/castPlaybackState";
import { backPressAction, playerExit } from "../../../src/lib/playerExit";
import { resolveStartPositionMs, servedVariant, useAutoMarkWatched, usePlayerProfilePrefs, usePlayerTrackSelection, usePlayingVideoTrack } from "@evey/client";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { PlayerEpisodesPanel } from "../../../src/components/player/PlayerEpisodesPanel";
import { StreamInfoPanel } from "../../../src/components/StreamInfoPanel";
import { SkipIntroOverlay } from "../../../src/components/SkipIntroOverlay";
import { CastContext } from "../../../src/hooks/useChromecast";
import { useCastSession } from "../../../src/hooks/useCastSession";
import { useSyncPlay } from "../../../src/hooks/useSyncPlay";
import { SyncPlayBar } from "../../../src/components/SyncPlayBar";
import { SyncPlayModal } from "../../../src/components/SyncPlayModal";
import { PlayOnSheet } from "../../../src/components/media/PlayOnSheet";
import { ShedScreen, ErrorScreen, LoadingScreen } from "../../../src/components/player/PlayerStatusScreens";
import { CastControllerScreen } from "../../../src/components/player/CastControllerScreen";
import { GestureIndicators } from "../../../src/components/player/GestureIndicators";
import { NextEpisodeOverlay } from "../../../src/components/player/NextEpisodeOverlay";
import { PlayerSeekBar } from "../../../src/components/player/PlayerSeekBar";
import { PlayerControlsRow } from "../../../src/components/player/PlayerControlsRow";
import { PlayerTopRightCluster } from "../../../src/components/player/PlayerTopRightCluster";
import { withAudioOffsetParam } from "../../../src/components/player/audioSync";
import {
  PlayerSettingsPanel,
  type AudioStream,
  type SubtitleStream,
} from "../../../src/components/player/PlayerSettingsPanel";
import { useControlsAutoHide } from "../../../src/components/player/useControlsAutoHide";
import { useSettledGeometry } from "../../../src/components/player/useSettledGeometry";
import { SubtitleOverlay } from "../../../src/components/player/SubtitleOverlay";
import {
  DEFAULT_SUBTITLE_STYLE,
  getSubtitleStylePref,
  saveSubtitleStylePref,
  type SubtitleStyle,
} from "../../../src/lib/subtitleStyle";
import { pickExternalSubtitle } from "../../../src/lib/externalSubtitle";

import { PluginSlot } from "../../../src/features/plugins/PluginSlot";

// Sheets still carried by a BottomSheet; audio / subtitles / quality / speed
// now live in the settings panel (web parity).
type SheetName = "chapters" | "streamInfo" | "sleepTimer" | "syncPlay" | "episodes" | "playOn";


// An expired or vanished stream token is the one native failure worth
// retrying, and it is common: the token store is in memory, so a server
// restart kills every token a paused player still holds. Prod saw 16 of these
// in one day from the Android app, each one a viewer stuck on an error screen
// with nothing wrong but a dead capability.
//
// ExoPlayer and AVPlayer both surface the HTTP status in the message, which is
// all we have to go on - there is no typed error for it.
function isAuthPlaybackError(message: string | undefined): boolean {
  if (!message) return false;
  const m = message.toLowerCase();
  return m.includes("401") || m.includes("403") || m.includes("unauthorized") || m.includes("forbidden");
}

export default function PlayerScreen() {
  // `restart=1` means the viewer chose "from the beginning" on the media page.
// Resuming is the right default; this is the only way to say otherwise.
const { id, restart, audio, subtitle } = useLocalSearchParams<{ id: string; restart?: string; audio?: string; subtitle?: string }>();
  // Tracks picked on the media page (absolute stream indices, "off" = no
  // subtitle): they seed the selection below and count as a manual pick.
  const trackPick = parseTrackPick({ audio, subtitle });
  const { t } = useTranslation();
  const { width: screenW, height: screenH } = useWindowDimensions();
  const rawInsets = useSafeAreaInsets();
  // Fullscreen rotates the window and moves the safe-area insets through two
  // separate native events: publish them together (and hide the controls while
  // a resize is in flight) so the bar is placed in ONE step instead of two.
  const { geometry, resizing } = useSettledGeometry({ width: screenW, height: screenH, insets: rawInsets });
  const insets = geometry.insets;
  const mediaId = id ?? "";
  useAudioCapsBeacon(mediaId);

  // Offline playback: if this media was downloaded, prefer the file already
  // on disk over the network stream. NOTE this only short-circuits the video
  // SOURCE - usePlaySession below still opens a play session against the
  // server for progress reporting/watched-state, so this doesn't give a fully
  // network-free experience (no network still means that session request
  // hangs/fails in the background); it does mean the video itself plays from
  // disk and isn't blocked on the network being reachable.
  const [localFilePath, setLocalFilePath] = useState<string | null>(null);
  // Audio codecs recorded in the download index entry (null for entries that
  // predate the field) - one of the two inputs to the localAudioOk decision.
  const [downloadedAudioCodecs, setDownloadedAudioCodecs] = useState<string[] | null>(null);
  // Playhead recorded by a previous OFFLINE session of this title, if any -
  // the only place it exists when no play session could be opened.
  const [offlineProgress, setOfflineProgress] = useState<OfflineProgress | null>(null);

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  // Series and seasons have no underlying file, so the streaming endpoints 404.
  // Resolve to a playable child (season → episode → episode) so the home hero
  // and any "Play" button on a series detail Just Work. Two redirects max:
  //   /player/<seriesId>  → /player/<season1Id>
  //   /player/<season1Id> → /player/<episode1Id>
  const isSeriesOrSeason =
    media?.type === "MEDIA_ITEM_TYPE_SERIES" || media?.type === "MEDIA_ITEM_TYPE_SEASON";
  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: mediaId, "pagination.pageSize": 500 } as never,
    { query: { enabled: !!mediaId && isSeriesOrSeason } } as never,
  );
  useEffect(() => {
    if (!isSeriesOrSeason) return;
    const items = childrenData?.items;
    if (!items || items.length === 0) return;
    const sorted = [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
    const target = sorted[0];
    if (target?.id && target.id !== mediaId) {
      router.replace(`/player/${target.id}` as never);
    }
  }, [isSeriesOrSeason, childrenData?.items, mediaId]);

  const isEpisode = media?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const { data: parentItem } = useLibraryServiceGetMediaItem(
    media?.parentId ?? "",
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );
  const episodeLabel = useMemo(() => {
    if (!isEpisode) return null;
    const s = parentItem?.indexNumber ?? 0;
    const e = media?.indexNumber ?? 0;
    if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
    if (e > 0) return `E${String(e).padStart(2, "0")}`;
    return null;
  }, [isEpisode, parentItem?.indexNumber, media?.indexNumber]);
  const playerTitle = episodeLabel ? `${episodeLabel} - ${media?.title ?? ""}` : (media?.title ?? "");

  // Sibling episodes of the same season, for the previous/next episode
  // buttons of the transport bar (web player parity).
  const { data: siblingsData } = useLibraryServiceListMediaItems(
    { parentId: media?.parentId ?? "", "pagination.pageSize": 500 } as never,
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );
  const { prevEpisode, nextEpisode } = useMemo(() => {
    const items = siblingsData?.items;
    if (!isEpisode || !items || items.length === 0) return { prevEpisode: null, nextEpisode: null };
    const sorted = [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
    const idx = sorted.findIndex((e) => e.id === mediaId);
    return {
      prevEpisode: idx > 0 ? sorted[idx - 1] : null,
      nextEpisode: idx >= 0 && idx < sorted.length - 1 ? sorted[idx + 1] : null,
    };
  }, [siblingsData?.items, isEpisode, mediaId]);
  // Full current-season episode list, used both for the transport bar's
  // prev/next buttons above (always bound to the current season, web parity)
  // and to gate the Episodes button below.
  const currentSeasonEpisodes = useMemo(() => {
    const items = siblingsData?.items;
    if (!isEpisode || !items) return [];
    return items
      .filter((ep): ep is typeof ep & { id: string } => !!ep.id)
      .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  }, [siblingsData?.items, isEpisode]);

  // Series' other seasons, for the Episodes panel's season selector (web
  // parity's EpisodesPanel). Lazily fetched: most sessions never open the
  // panel, so there's no reason to always pull every season of the series.
  const [episodesPanelOpened, setEpisodesPanelOpened] = useState(false);
  const seriesId = parentItem?.parentId ?? "";
  const { data: seasonsData } = useLibraryServiceListMediaItems(
    { parentId: seriesId, "pagination.pageSize": 500 } as never,
    { query: { enabled: !!seriesId && isEpisode && episodesPanelOpened } } as never,
  );
  const seasons = useMemo(() => {
    const items = seasonsData?.items;
    if (!items) return [];
    return items
      .filter((s): s is typeof s & { id: string } => !!s.id)
      .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  }, [seasonsData?.items]);

  // Season picked in the panel's selector - defaults to (and resets whenever
  // the player mounts on a new episode to) the current episode's season.
  const [selectedSeasonId, setSelectedSeasonId] = useState(media?.parentId ?? "");
  useEffect(() => {
    if (media?.parentId) setSelectedSeasonId(media.parentId);
  }, [media?.parentId]);

  // Episodes of whichever OTHER season the user picked in the selector -
  // siblingsData/currentSeasonEpisodes above already covers the current one,
  // so this only fires once the user actually switches season.
  const { data: otherSeasonEpisodesData } = useLibraryServiceListMediaItems(
    { parentId: selectedSeasonId, "pagination.pageSize": 500 } as never,
    { query: { enabled: !!selectedSeasonId && selectedSeasonId !== media?.parentId } } as never,
  );
  const panelEpisodes = useMemo(() => {
    if (!isEpisode) return [];
    if (selectedSeasonId === media?.parentId) return currentSeasonEpisodes;
    const items = otherSeasonEpisodesData?.items;
    if (!items) return [];
    return items
      .filter((ep): ep is typeof ep & { id: string } => !!ep.id)
      .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  }, [isEpisode, selectedSeasonId, media?.parentId, currentSeasonEpisodes, otherSeasonEpisodesData?.items]);
  // Don't open a play session for a series/season - the redirect above will
  // remount us on a real episode in a tick. Passing "" makes usePlaySession
  // hold its initial loading state instead of hitting /transcode and 404'ing.
  const sessionMediaId = isSeriesOrSeason ? "" : mediaId;
  const { streamUrl, streamToken, sessionId, streamInfo, serverUrl, isLoading, isError, shedReason, stopSession, restartWithTracks, progress, restartPositionMs } =
    usePlaySession(sessionMediaId, {
      ...(trackPick.audio === undefined ? {} : { audioStreamIndex: trackPick.audio }),
      // The encoder is pre-warmed where the player will start.
      restart: restart === "1",
    });
  // A film starts, the album stops - the mini player is mounted for the whole
  // signed-in app and kept playing under the video otherwise.
  useEffect(() => {
    useMusicQueueStore.getState().clear();
  }, [sessionMediaId]);


  const { data: chapters = [] } = useChapters(mediaId);
  // What the transcode actually offers, read from the master playlist. Empty
  // on direct play - there is no ladder to choose from there.
  const availableQualities = useHlsRungs(streamUrl, streamToken);
  const videoViewRef = useRef<VideoView>(null);
  const pipSupported = useMemo(() => isPictureInPictureSupported(), []);
  const mountedRef = useRef(true);
  // Fatal-native-error beacon, identical to the web player's. Self-throttling
  // (dedupe + per-session budget) so a stream failing in a loop cannot flood
  // the admin log.
  const reportPlayerEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  // A new player opens at 100%: whatever gain a previous one left is dropped.
  // Leaving boosted, the phone's media volume goes back to where the boost
  // found it: pinned at maximum, other apps and notifications played at full.
  const deviceBeforeBoostRef = useRef<number | null>(null);
  useEffect(() => {
    void releaseAudioBoost();
    return () => {
      mountedRef.current = false;
      void releaseAudioBoost();
      if (volumeBaseRef.current > 1 && deviceBeforeBoostRef.current !== null) void setDeviceVolumeFraction(deviceBeforeBoostRef.current);
    };
  }, []);

  const [activeSheet, setActiveSheet] = useState<null | SheetName>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // Read by the gestures' PanResponder (built once): a state captured in
  // its closure would be frozen, the ref always stays up to date.
  const settingsOpenRef = useRef(false);
  useEffect(() => { settingsOpenRef.current = settingsOpen; }, [settingsOpen]);
  // One recovery per media: a dead stream token is worth one silent retry, a
  // second identical failure is a real one and must be shown.
  const authRetriedRef = useRef(false);
  // The status listener is installed before restartWithTracks exists in its
  // closure, so it reaches it through a ref that is kept current below.
  const restartWithTracksRef = useRef<(() => void) | null>(null);
  // Audio tracks the NATIVE player exposes from the HLS manifest - the source
  // of truth for a no-reload client-side switch (see handleAudioSelect).
  const [nativeAudioTracks, setNativeAudioTracks] = useState<{ id?: string; language?: string; label?: string }[]>([]);
  const [selectedQuality, setSelectedQuality] = useState(0);

  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  // Audio resync (lip-sync). Mirrors the web player: audioOffsetMs is the
  // raw value the stepper shows, appliedAudioOffsetMs is what's baked into
  // the stream URL - debounced so a burst of taps coalesces into one restart
  // of the native player (see the debounce effect below, next to `player`).
  const [audioOffsetMs, setAudioOffsetMs] = useState(0);
  const [appliedAudioOffsetMs, setAppliedAudioOffsetMs] = useState(0);
  // Only transcoded playback can re-encode the audio with a shifted input -
  // direct play serves the original file bytes untouched.
  const audioOffsetAvailable = !!streamInfo && streamInfo.playMethod !== "PLAY_METHOD_DIRECT_PLAY";

  // Client-rendered text subtitles (SubtitleOverlay): style is a device-wide
  // preference (loaded once below, see the SecureStore-backed effect), the
  // resync offset is per-title like audioOffsetMs (reset on mediaId change).
  const [subtitleStyle, setSubtitleStyle] = useState<SubtitleStyle>(DEFAULT_SUBTITLE_STYLE);
  const [subtitleOffsetMs, setSubtitleOffsetMs] = useState(0);
  useEffect(() => {
    let cancelled = false;
    getSubtitleStylePref().then((saved) => { if (!cancelled && saved) setSubtitleStyle(saved); });
    return () => { cancelled = true; };
  }, []);
  const handleSubtitleStyleChange = useCallback((patch: Partial<SubtitleStyle>) => {
    setSubtitleStyle((prev) => {
      const next = { ...prev, ...patch };
      void saveSubtitleStylePref(next);
      return next;
    });
  }, []);

  // Client-picked external subtitle file (upload): purely client-side, same
  // as the web player - converted to VTT and parsed once at pick time, then
  // rendered by SubtitleOverlay like any other cue source. Reset on mediaId
  // change alongside the other per-title subtitle state (see the mediaId
  // reset effect below).
  const [externalSubtitle, setExternalSubtitle] = useState<{ name: string; cues: SubtitleCue[] } | null>(null);

  const audioStreams: AudioStream[] = useMemo(() => playerAudioStreams(streamInfo?.mediaStreams), [streamInfo]);
  const subtitleStreams: SubtitleStream[] = useMemo(() => playerSubtitleStreams(streamInfo?.mediaStreams), [streamInfo]);

  // The running profile, its languages and its "auto-play next episode"
  // choice: the same reading on every player (usePlayerProfilePrefs). A track
  // picked in this session is never overridden by them (userPickedAudioRef /
  // userPickedSubtitleRef).
  const activeProfileId = useAuthStore((s) => s.profileId);
  const {
    profile: activeProfile,
    audioLangs: profileAudioLangs,
    subtitleLangs: profileSubtitleLangs,
    autoplayNext,
    setAutoplayNext: handleAutoplayNextChange,
  } = usePlayerProfilePrefs();

  // The audio and subtitle choice, shared with the television
  // (usePlayerTrackSelection): the title page's pick and a hand pick win over
  // the profile's languages, a new title starts from scratch.
  // burnInSubtitleIndex is what the SERVER may be told - never the raw
  // selection; restartForSubtitleChange stays explicit, it acts on the track
  // the viewer is moving TO, before it lands in state.
  const {
    selectedAudioIndex,
    setSelectedAudioIndex,
    selectedSubtitleIndex,
    setSelectedSubtitleIndex,
    userPickedAudioRef,
    userPickedSubtitleRef,
    burnInSubtitleIndex,
    clientSubtitleIndex,
  } = usePlayerTrackSelection({
    mediaId,
    pick: trackPick,
    audioStreams,
    subtitleStreams,
    audioLangs: profileAudioLangs,
    subtitleLangs: profileSubtitleLangs,
    restartWithTracks,
  });

  // Keep the recovery path pointing at the CURRENT selection, so a retry after
  // a dead token comes back on the same audio, subtitle and quality the viewer
  // had chosen rather than resetting them.
  restartWithTracksRef.current = () =>
    restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);


  // Resolving the on-disk path is an AUTHORIZATION decision, not a file lookup:
  // downstream, a truthy localFilePath deliberately bypasses the server's shed
  // and error branches so a downloaded title plays with no network. That bypass
  // is only sound if the path itself was gated - otherwise a kids profile
  // replays whatever an adult downloaded earlier. Denied, expired or revoked
  // resolves to null, which falls back to the normal online path.
  useEffect(() => {
    let cancelled = false;
    // Reset per-title state before resolving: this screen is REUSED from one
    // episode to the next, and leaving the previous episode's path in place
    // means videoSource can point at the previous file while mediaId already
    // names the new one.
    setLocalFilePath(null);
    setDownloadedAudioCodecs(null);
    setOfflineProgress(null);
    if (!mediaId) return;
    getDownloadPath(mediaId, {
      activeProfile: {
        id: activeProfileId ?? undefined,
        isKids: activeProfile?.isKids,
        maxAge: activeProfile?.maxAge,
        region: activeProfile?.region,
        // Offline the profile list never arrives, and an unknown parental cap
        // must not resolve to "unrestricted" - see offlineAccess.ts.
        restrictionKnown: !!activeProfile,
      },
    })
      .then(async (p) => {
        if (cancelled) return;
        if (!p) { setLocalFilePath(null); return; }
        // Resolve the local playhead BEFORE publishing the path: videoSource
        // reacts to localFilePath, and the player's setup callback reads
        // resumePositionSec once, at creation. Setting the path first would
        // create the player with no resume point and never revisit it.
        const [entry, saved] = await Promise.all([
          getDownloadedMedia(mediaId).catch(() => null),
          getOfflineProgress(mediaId),
        ]);
        if (cancelled) return;
        setDownloadedAudioCodecs(entry?.audioCodecs ?? null);
        setOfflineProgress(saved);
        setLocalFilePath(p);
      })
      .catch(() => { if (!cancelled) setLocalFilePath(null); });
    return () => { cancelled = true; };
  }, [mediaId, activeProfileId, activeProfile]);

  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const mutedRef = useRef(false);
  mutedRef.current = muted;
  const [brightness, setBrightness] = useState(1);
  // A fatal native failure (decoder error, 401 on a segment, broken manifest)
  // surfaces ONLY through the player's statusChange event - isError from
  // usePlaySession covers just the session-start network call. Ignoring it
  // left a black screen with live controls.
  const [nativeError, setNativeError] = useState<string | null>(null);
  // Clear a stuck native error when the media changes (auto next-episode or a
  // new navigation reuses this screen). Without this, ONE playback error latched
  // the ErrorScreen for every subsequent episode of the session, with no retry -
  // same reset discipline as autoMarkedRef / startedRef below.
  useEffect(() => {
    setNativeError(null);
    // A panel/sheet left open makes no sense on the next media
    // (auto-next-episode or a new navigation reuse this screen).
    setActiveSheet(null);
    setSettingsOpen(false);
    // A resync dialed in for the previous title means nothing on the next one.
    setAudioOffsetMs(0);
    setAppliedAudioOffsetMs(0);
    setSubtitleOffsetMs(0);
    // An externally-picked subtitle file means nothing on the next title.
    setExternalSubtitle(null);
    // (A hand-picked track is forgotten by usePlayerTrackSelection.)
  }, [mediaId]);
  // Whether the NEXT player instance should start playing. Changing tracks or
  // quality recreates the native player, whose setup used to call play()
  // unconditionally - a paused screen sprang back to life on every track
  // change. Handlers snapshot the playing state right before restarting;
  // defaults to true so first entry (and next-episode remounts) autoplay.
  const resumeAfterRestartRef = useRef(true);
  // Mirrors currentTime without going through React state - read by the
  // audio-offset debounce below so it can snapshot the playhead without
  // re-firing that effect on every timeUpdate tick (currentTime changes ~2x/s).
  const currentTimeRef = useRef(0);
  // What ac3go handed the speakers, reported: a track decoded to silence is
  // otherwise invisible (the picture plays, progress ticks).
  useDecoderTelemetry(mediaId, reportPlayerEvent, () => currentTimeRef.current);
  // Set right before an audio-offset restart to the pre-restart playhead, so
  // the next player instance's setup callback resumes there instead of at the
  // stale server-reported watch progress (see the `player` useVideoPlayer call).
  const audioOffsetResumeRef = useRef<number | null>(null);
  // While the user scrubs the timeline, freeze the live timeUpdate: otherwise it
  // overwrites the scrubbed position ~4×/s and the thumb jumps between the finger
  // and the real playback position (the "wobbly" seek).
  const seekingRef = useRef(false);
  const volumeBaseRef = useRef(1);
  const [syncCreating, setSyncCreating] = useState(false);
  const [syncJoining, setSyncJoining] = useState(false);
  const sleepTimer = useSleepTimer({
    onMinutesFire: () => {
      try { player?.pause(); } catch { /* released */ }
      toast.info(t("player.sleepTimer.fired", "Sleep timer ended"));
    },
  });
  const syncPlayRef = useRef<{ player: typeof player } | null>(null);

  const syncPlay = useSyncPlay({
    onPlay: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; syncPlayRef.current.player.play(); } },
    onPause: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; syncPlayRef.current.player.pause(); } },
    onSeek: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; } },
  });

  // Whether the local copy can be HEARD here: the download is the raw remux,
  // and a device without e.g. an AC-3 decoder plays it silent. Codecs come
  // from the index entry (captured at download time), or - for entries that
  // predate that field - from the live session's stream info, which is only
  // there when a stream exists to prefer anyway. No info at all (legacy entry,
  // offline) keeps the historical playable assumption.
  const localAudioOk = useMemo(() => {
    const fromSession = (streamInfo?.mediaStreams ?? [])
      .filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.codec)
      .map((s) => s.codec as string);
    const codecs = downloadedAudioCodecs ?? (fromSession.length > 0 ? fromSession : undefined);
    return hasRenderableAudio({ audioCodecs: codecs }, getDeviceAudioCodecs());
  }, [downloadedAudioCodecs, streamInfo]);

  // How long a downloaded copy waits for the stream that would have sound
  // before giving up and playing silent. The play-session request runs on the
  // api client's generic 60s timeout and is never retried, so a server that
  // ACCEPTS the connection and then says nothing (captive portal, sleeping NAS
  // on a reachable LAN) bought a full minute of loading screen before the
  // local file played anyway. A minute of nothing is worse than eight seconds
  // then silent video, and the viewer can still leave.
  const LOCAL_FALLBACK_WAIT_MS = 8000;
  const [localFallbackDue, setLocalFallbackDue] = useState(false);
  useEffect(() => {
    setLocalFallbackDue(false);
    if (!localFilePath || localAudioOk || streamUrl || isError) return;
    const id = setTimeout(() => setLocalFallbackDue(true), LOCAL_FALLBACK_WAIT_MS);
    return () => clearTimeout(id);
  }, [localFilePath, localAudioOk, streamUrl, isError]);

  // Downloaded copy preferred only when it can be HEARD here. When the audio
  // is not renderable, a reachable server stream wins - the transcode is the
  // only version with sound - and the local file stays the fallback for
  // offline (isError), shed, or a stream that never arrives, where silent
  // video beats no playback at all.
  const useLocalSource = !!localFilePath && (localAudioOk || isError || !!shedReason || localFallbackDue);

  const videoSource = useMemo(() => {
    // Downloaded copy on disk takes priority over the network stream - no
    // reason to re-download bytes the device already has, and it plays even
    // if the play-session request above is stuck/failing offline.
    if (useLocalSource && localFilePath) {
      return { uri: localFilePath.startsWith("file://") ? localFilePath : `file://${localFilePath}` };
    }
    if (!streamUrl) return null;
    const isHls = streamUrl.includes("m3u8");
    // Audio resync: baked into the manifest URL like the web's
    // buildTranscodeHlsUrl (see audioSync.ts) - the server re-encodes the
    // audio with a shifted input on the very next fetch of this source, no
    // separate restart-session call needed.
    const uri = audioOffsetAvailable ? withAudioOffsetParam(streamUrl, appliedAudioOffsetMs) : streamUrl;
    return {
      uri,
      ...(isHls ? { contentType: "hls" as const } : {}),
      // Also send the opaque stream token as a Bearer header: the server accepts
      // cookie/Bearer/query, so if the native player forwards the source headers
      // to its segment requests they authenticate without the token in the URL.
      // The ?token= stays in the URL as the reliable fallback (some engines drop
      // headers on the manifest's relative segment/redirect fetches).
      ...(streamToken ? { headers: { Authorization: `Bearer ${streamToken}` } } : {}),
    };
  }, [streamUrl, streamToken, localFilePath, useLocalSource, audioOffsetAvailable, appliedAudioOffsetMs]);

  const resumePositionSec = useMemo(() => {
    // A quality, audio or subtitle switch continues where the viewer was.
    if (restartPositionMs != null) return restartPositionMs / 1000;
    if (restart === "1") return 0;
    // A locally recorded playhead beats the server's when it is FRESHER (see
    // preferLocalProgress). Everything watched offline lives only here, and
    // the server's copy is whatever it last heard - possibly from before the
    // flight.
    if (preferLocalProgress(offlineProgress, progress?.updatedAt) && offlineProgress) {
      return offlineProgress.positionMs / 1000;
    }
    // Played, or within the last seconds: start over (the rule of every client).
    return resolveStartPositionMs(false, progress) / 1000;
  }, [progress, restart, offlineProgress, restartPositionMs]);

  const player = useVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    // An audio-offset restart takes priority: it always has a live playhead to
    // resume at, whereas resumePositionSec can be a stale server-reported
    // progress (see audioOffsetResumeRef's declaration above).
    const offsetResumeSec = audioOffsetResumeRef.current;
    if (offsetResumeSec !== null) {
      p.currentTime = offsetResumeSec;
      audioOffsetResumeRef.current = null;
    } else if (resumePositionSec > 0) {
      p.currentTime = resumePositionSec;
    }
    if (resumeAfterRestartRef.current) p.play();
  });

  useEffect(() => { syncPlayRef.current = { player }; }, [player]);

  // Debounced audio-offset restart: coalesces a burst of stepper taps into
  // ONE reload of the native player, matching the web's 1200ms debounce
  // (HLSPlayer.tsx). Reads player/currentTime via refs, not effect deps -
  // including them would restart this timer on every timeUpdate tick (~2/s)
  // and the debounce would never fire.
  useEffect(() => {
    if (!audioOffsetAvailable) return;
    if (audioOffsetMs === appliedAudioOffsetMs) return;
    const id = setTimeout(() => {
      resumeAfterRestartRef.current = player?.playing ?? true;
      audioOffsetResumeRef.current = currentTimeRef.current;
      setAppliedAudioOffsetMs(audioOffsetMs);
    }, 1200);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [audioOffsetMs, appliedAudioOffsetMs, audioOffsetAvailable]);

  // The variant ExoPlayer/AVPlayer decodes now, for the stream info panel.
  const playingTrack = usePlayingVideoTrack(player);

  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      if (!mountedRef.current) return;
      if (seekingRef.current) return; // frozen while scrubbing - see seekingRef
      setCurrentTime(ev.currentTime);
      currentTimeRef.current = ev.currentTime;
      try { setDuration(player.duration); } catch (err) { console.warn("released", err); }
    });
    const statusSub = player.addListener("statusChange", (ev) => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); setDuration(player.duration); } catch (err) { console.warn("released", err); }
      // "loading" while playing = network/segment stall: the web shows a
      // small discreet spinner in the bar, we do the same.
      setIsBuffering(ev.status === "loading");
      if (ev.status === "error") {
        console.warn("[player] native playback error:", ev.error?.message);
        // A dead token is repairable: restartWithTracks forgets it and mints a
        // fresh one, which is exactly what the viewer would get by leaving and
        // coming back. Once only - a second identical failure is a real one,
        // and retrying it forever would hide it.
        if (isAuthPlaybackError(ev.error?.message) && !authRetriedRef.current) {
          authRetriedRef.current = true;
          reportPlayerEvent({
            type: "native_player_auth_retry",
            details: ev.error?.message ?? "",
            fatal: false,
            posSec: Math.round(currentTimeRef.current),
          });
          restartWithTracksRef.current?.();
          return;
        }
        // Beacon BEFORE the state flip: this is the only trace the failure
        // leaves anywhere. A direct-play death is invisible server-side (every
        // byte it served was a 200 from ServeFile), console.warn goes nowhere
        // on a tester's phone, and what the viewer reports is "the video bugs"
        // plus a photo of the error screen. Same reporter, same route and the
        // same reason as the web player's - see createPlayerEventReporter.
        reportPlayerEvent({
          type: "native_player_error",
          details: ev.error?.message ?? "",
          fatal: true,
          posSec: Math.round(currentTimeRef.current),
        });
        setNativeError(ev.error?.message ?? "playback error");
      }
    });
    // See lib/resumeNudge.ts: a resume after a long pause seeks in place so
    // the picture leaves with the sound instead of three seconds behind it.
    const nudge = resumeNudger((sec) => { try { player.seekBy(sec); } catch { /* released */ } });
    const playSub = player.addListener("playingChange", () => {
      if (!mountedRef.current) return;
      try {
        setIsPlaying(player.playing);
        nudge(player.playing);
      } catch (err) { console.warn("released", err); }
    });
    // The native player parses the HLS audio group into availableAudioTracks
    // once the manifest loads. Switching a track is then a client-side select
    // (player.audioTrack = ...), instant and with no reload - the right
    // mechanism, decoupled from the manifest DEFAULT flag. Read the current
    // value AND subscribe to changes (it is empty until the manifest resolves).
    const readAudioTracks = () => {
      if (!mountedRef.current) return;
      try { setNativeAudioTracks(player.availableAudioTracks ?? []); } catch { /* released */ }
    };
    readAudioTracks();
    const audioTracksSub = player.addListener("availableAudioTracksChange", readAudioTracks);
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
      audioTracksSub.remove();
    };
  }, [player, reportPlayerEvent]);

  // Hardware step count of the media stream, fetched once: the volume policy
  // needs it synchronously on every drag event to split the level between the
  // device (per hardware step) and the player element (the smooth remainder).
  const deviceStepsRef = useRef<number | null>(null);
  useEffect(() => {
    void readDeviceVolumeSteps().then((steps) => { deviceStepsRef.current = steps; });
  }, []);
  // The level the device is REALLY on (written back by the bridge, or read).
  const deviceNowRef = useRef<number | null>(null);
  const lastVolumeTouchRef = useRef(0);
  const maxVolume = hasAudioBoost() ? 2 : 1;

  // The player element's share of the level (volumePolicy.playerTrim).
  const applyPlayerTrim = useCallback((level: number) => {
    if (!player) return;
    const trim = playerTrim(level, deviceNowRef.current, {
      hasDeviceControl: hasDeviceVolumeControl(),
      deviceSteps: deviceStepsRef.current ?? undefined,
    });
    try { player.volume = trim; } catch (err) { console.warn("released", err); }
  }, [player]);

  // Where the 0-2 bar lands is decided in ONE place (volumePolicy): on Android
  // the 0-100% half drives the DEVICE media volume, so the >100% amplification
  // has a full-scale signal to work with. Elsewhere it stays on the player.
  const applyLevel = useCallback((level: number) => {
    const targets = resolveVolume(level, {
      hasDeviceControl: hasDeviceVolumeControl(),
      deviceSteps: deviceStepsRef.current ?? undefined,
    });
    applyPlayerTrim(level);
    if (targets.deviceFraction !== null) {
      void setDeviceVolumeFraction(targets.deviceFraction).then((applied) => {
        if (applied === null || !mountedRef.current) return;
        deviceNowRef.current = applied;
        applyPlayerTrim(volumeBaseRef.current);
      });
    }
    void setAudioBoost(targets.boostGain);
  }, [applyPlayerTrim]);
  const restoreLocalVolume = useCallback(() => applyLevel(volumeBaseRef.current), [applyLevel]);

  const { castConnected, castClient, castCurrentTime, castDuration, castIsPlaying, castVolume, castIsMuted, castDeviceName, handleCast } = useCastSession({
    player,
    restoreLocalVolume,
    streamUrl,
    mediaId,
    title: media?.title,
    currentTime,
    fallbackDurationSec: media?.durationMs ? Number(media.durationMs) / 1000 : 0,
    streamInfo,
    serverUrl,
  });

  // Open in whatever orientation the user is currently holding the device in.
  // The user toggles fullscreen (== lock to landscape) explicitly via the
  // bottom bar button. Restore the OS default lock when leaving the screen.
  useEffect(() => {
    ScreenOrientation.unlockAsync();
    return () => {
      ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.DEFAULT);
    };
  }, []);

  const [isFullscreen, setIsFullscreen] = useState(false);
  // Measured height of the bottom block (seekbar + row): serves as the
  // pixel anchor for the settings panel, rendered above the bar.
  const [controlsHeight, setControlsHeight] = useState(0);
  // The orientation lock stays OUT of the state updater: React may invoke an
  // updater more than once, and a second lockAsync means a second rotation
  // event - the very churn this screen is trying not to have.
  const toggleFullscreen = useCallback(() => {
    const next = !isFullscreen;
    setIsFullscreen(next);
    ScreenOrientation.lockAsync(
      next ? ScreenOrientation.OrientationLock.LANDSCAPE : ScreenOrientation.OrientationLock.PORTRAIT_UP,
    );
  }, [isFullscreen]);

  // Same exit contract as the web player's back button (playerUtils.playerExit):
  // pop the player's own history entry when there is one - safe because episode
  // changes REPLACE - otherwise (deep link, notification) REPLACE toward the
  // media page (an episode exits to its season) so no player entry is ever
  // left behind for a later back press to walk into.
  const handleBack = useCallback(() => {
    stopSession();
    const exit = playerExit(router.canGoBack(), mediaId, media);
    if (exit.action === "back") router.back();
    else router.replace(`/media/${exit.mediaId}` as never);
  }, [stopSession, mediaId, media]);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      // Close the settings panel first: it is a plain View, so unlike the
      // Modal-based sheets nothing else intercepts the back press for it.
      if (backPressAction(settingsOpenRef.current) === "close-settings") {
        setSettingsOpen(false);
        return true;
      }
      handleBack();
      return true;
    });
    return () => sub.remove();
  }, [handleBack]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // Controls stay up 4.5s after the last interaction, and never auto-hide
  // while paused or while a sheet (audio/subtitle/more/sync/...) is open -
  // see useControlsAutoHide for the timer semantics, unit-tested there.
  const { showControls, resetHideTimer, toggleControls } = useControlsAutoHide({
    isPlaying,
    hasOpenPanel: activeSheet !== null || settingsOpen,
  });

  // A new player instance (next episode, track or quality change, retry) opens
  // at volume 1 and unmuted: it gets the bar's level and mute back.
  useEffect(() => {
    if (!player || castConnected) return;
    applyPlayerTrim(volumeBaseRef.current);
    try { player.muted = mutedRef.current; } catch (err) { console.warn("released", err); }
  }, [player, castConnected, applyPlayerTrim]);

  // Show the level the device is REALLY at, and follow the hardware volume
  // keys, which move the same setting behind the bar's back: re-read every
  // second while the controls are up (they stay up while paused).
  useEffect(() => {
    if (!showControls || !hasDeviceVolumeControl()) return;
    let cancelled = false;
    const sync = () => void readDeviceVolumeFraction().then((fraction) => {
      if (cancelled || fraction === null) return;
      // A drag is still settling: the device may not have caught up yet.
      if (Date.now() - lastVolumeTouchRef.current < 1500) return;
      const prev = deviceNowRef.current;
      deviceNowRef.current = fraction;
      noteObservedDeviceVolume(fraction);
      if (prev !== null && Math.abs(prev - fraction) < 1e-3) return;
      const base = volumeBaseRef.current;
      // At 0 the user silenced Evey, not the phone; at or above 100% the
      // device is meant to be at its maximum.
      if (base === 0 || (base >= 1 && fraction >= 1)) return;
      if (base > 1) void setAudioBoost(1);
      volumeBaseRef.current = fraction;
      setVolume(fraction);
      applyPlayerTrim(fraction);
    });
    sync();
    const timer = setInterval(sync, 1000);
    return () => { cancelled = true; clearInterval(timer); };
  }, [showControls, applyPlayerTrim]);

  const togglePlayPause = useCallback(() => {
    if (!player) return;
    if (player.playing) {
      player.pause();
      if (syncPlay.state.connected) syncPlay.sendPause(player.currentTime * 1000);
    } else {
      player.play();
      if (syncPlay.state.connected) syncPlay.sendPlay(player.currentTime * 1000);
    }
    resetHideTimer();
  }, [player, resetHideTimer, syncPlay]);

  const knownDuration = media?.durationMs ? Number(media.durationMs) / 1000 : 0;
  const effectiveDuration = knownDuration > 0 ? knownDuration : duration;
  // Mirrored into a ref for the gesture hook, whose PanResponder is built once
  // and reads everything through refs (see usePlayerGestures).
  const effectiveDurationRef = useRef(0);
  effectiveDurationRef.current = effectiveDuration;

  // Past 90% the title is watched: the rule every player shares.
  useAutoMarkWatched(mediaId, currentTime, effectiveDuration);

  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;

  const seekPreview = useCallback(
    (pct: number) => {
      const d = knownDuration > 0 ? knownDuration : duration;
      if (d <= 0) return;
      setCurrentTime(pct * d);
      resetHideTimer();
    },
    [duration, knownDuration, resetHideTimer],
  );

  const seekTo = useCallback(
    (pct: number) => {
      const d = knownDuration > 0 ? knownDuration : duration;
      if (d <= 0) return;
      const targetSec = pct * d;
      // Optimistic: show the target immediately so the release doesn't flash the
      // pre-seek position before the first post-seek timeUpdate lands.
      setCurrentTime(targetSec);
      // Native seek on the absolute HLS timeline - works for direct-play AND
      // transcode (the event playlist is 0-based; the server produces in bursts any
      // not-yet-produced segment). Reloading the stream instead replayed it from the start.
      if (player) {
        player.currentTime = targetSec;
      }
      if (syncPlay.state.connected) syncPlay.sendSeek(targetSec * 1000);
      resetHideTimer();
    },
    [player, duration, knownDuration, resetHideTimer, syncPlay],
  );

  // The target of the last +/- tap, while its seek is still landing: a second
  // tap adds to it instead of to the pre-seek position the player still
  // reports (AVPlayer does until the seek completes; timeUpdate lags 0.5 s).
  const pendingSkipRef = useRef<{ target: number; at: number } | null>(null);
  const seekBy = useCallback(
    (seconds: number) => {
      const d = knownDuration > 0 ? knownDuration : duration;
      const pending = pendingSkipRef.current;
      const base = pending && Date.now() - pending.at < 1500 ? pending.target : currentTimeRef.current;
      const target = Math.max(0, d > 0 ? Math.min(base + seconds, d - 1) : base + seconds);
      pendingSkipRef.current = { target, at: Date.now() };
      // Same absolute seek as the seek bar: the relative native seekBy showed
      // the old position until the next tick and skipped SyncPlay.
      if (d > 0) seekTo(target / d);
      else if (player) player.currentTime = target;
      resetHideTimer();
    },
    [player, duration, knownDuration, seekTo, resetHideTimer],
  );

  const handleVolumeChange = useCallback(
    (value: number) => {
      if (!player) return;
      const v = Math.max(0, Math.min(maxVolume, value));
      lastVolumeTouchRef.current = Date.now();
      if (v >= 1 && volumeBaseRef.current < 1) deviceBeforeBoostRef.current = deviceNowRef.current;
      else if (v < 1) deviceBeforeBoostRef.current = null;
      volumeBaseRef.current = v;
      applyLevel(v);
      setVolume(v);
      // Dragging the bar above zero means "I want sound now": same implicit
      // unmute as the web player.
      if (v > 0 && mutedRef.current) {
        try { player.muted = false; } catch (err) { console.warn("released", err); }
        setMuted(false);
      }
    },
    [player, maxVolume, applyLevel],
  );

  // From the screen's state, not player.muted: a new player instance starts
  // unmuted, and the first press muted it again while the icon said muted.
  const toggleMute = useCallback(() => {
    if (!player) return;
    const next = !mutedRef.current;
    try { player.muted = next; } catch (err) { console.warn("released", err); }
    mutedRef.current = next;
    setMuted(next);
  }, [player]);

  const brightnessBaseRef = useRef(1);
  const handleBrightnessChange = useCallback(
    async (value: number) => {
      try {
        const Brightness = require("expo-brightness");
        await Brightness.setBrightnessAsync(value);
        brightnessBaseRef.current = value;
        setBrightness(value);
      } catch (err) { console.warn("expo-brightness not available", err); }
    },
    [],
  );
  // The settings panel's slider must start from the real system brightness,
  // not an arbitrary 100%.
  useEffect(() => {
    try {
      const Brightness = require("expo-brightness");
      Brightness.getBrightnessAsync().then((v: number) => {
        if (!mountedRef.current) return;
        brightnessBaseRef.current = v;
        setBrightness(v);
      });
    } catch (err) { console.warn("expo-brightness not available", err); }
  }, []);

  const getGestureVolume = useCallback(() => volumeBaseRef.current, []);
  const getGestureBrightness = useCallback(() => brightnessBaseRef.current, []);

  // The gesture hook works in absolute seconds; seekTo takes a fraction. The
  // conversion lives here because the runtime is this screen's business.
  const handleScrubCommit = useCallback(
    (targetSeconds: number) => {
      const d = effectiveDurationRef.current;
      if (d <= 0) return;
      // Move the ref forward now, not on the next timeUpdate: chained swipes
      // (how you cross a film) would otherwise each start from the pre-seek
      // position and undo one another.
      currentTimeRef.current = targetSeconds;
      seekTo(Math.max(0, Math.min(1, targetSeconds / d)));
    },
    [seekTo],
  );
  const getScrubPosition = useCallback(() => currentTimeRef.current, []);
  const getScrubDuration = useCallback(() => effectiveDurationRef.current, []);

  const { panResponder, seekFlash, scrubPreview, volumeIndicator, brightnessIndicator, cleanup } = usePlayerGestures({
    onSeekBy: seekBy,
    onScrubCommit: handleScrubCommit,
    getPosition: getScrubPosition,
    getDuration: getScrubDuration,
    onTogglePlayPause: togglePlayPause,
    onToggleControls: toggleControls,
    onVolumeChange: handleVolumeChange,
    maxVolume,
    onBrightnessChange: handleBrightnessChange,
    getVolume: getGestureVolume,
    getBrightness: getGestureBrightness,
    skipForwardSeconds: skipFwd,
    skipBackwardSeconds: skipBwd,
    hasOpenPanel: () => settingsOpenRef.current,
    onCloseOverlays: () => setSettingsOpen(false),
  });

  useEffect(() => cleanup, [cleanup]);

  useEffect(() => {
    NavigationBar.setVisibilityAsync(showControls ? "visible" : "hidden");
  }, [showControls]);

  useEffect(() => {
    NavigationBar.setVisibilityAsync("hidden");
    return () => { NavigationBar.setVisibilityAsync("visible"); };
  }, []);

  const handleNavigateEpisode = useCallback(
    (episodeId: string) => {
      stopSession();
      router.replace(`/(auth)/player/${episodeId}`);
    },
    [stopSession],
  );

  const nextEp = useNextEpisode({
    media,
    currentTime,
    duration: effectiveDuration,
    onNavigate: handleNavigateEpisode,
    autoAdvance: autoplayNext,
  });

  // End-of-episode sleep timer: when the user picked "stop after this episode",
  // intercept videoEnded BEFORE useNextEpisode auto-navigates. Pause the player
  // and clear the timer so the user lands on the next-episode countdown but with
  // the auto-advance cancelled.
  useEffect(() => {
    if (sleepTimer.state?.kind !== "endOfEpisode") return;
    if (!nextEp.ended) return;
    try { player?.pause(); } catch { /* released */ }
    nextEp.cancel();
    sleepTimer.cancel();
    toast.info(t("player.sleepTimer.fired", "Sleep timer ended"));
  }, [sleepTimer, nextEp, player, t]);

  const getPositionMs = useCallback(() => {
    // No mounted guard here, deliberately: the progress reporter's own cleanup
    // runs AFTER this screen's unmount flag flips, so guarding on it made the
    // final report carry position 0 - and the server keeps the FRESHEST write,
    // so leaving the player erased the resume point it had just saved. The
    // try/catch below already covers the released-player case (the reason the
    // guard existed). The TV player reports the same way, with the incident
    // that proved it dated 2026-08-03.
    // While casting, the local player sits paused/muted (see useCastSession's
    // muteLocalPlayer) - reporting ITS position froze progress mid-cast and
    // made resume/"watched" wrong. The receiver's own position is the only
    // one moving during a cast session.
    if (castConnected) return castCurrentTime * 1000;
    try { return (player?.currentTime ?? 0) * 1000; } catch { return 0; }
  }, [player, castConnected, castCurrentTime]);

  // While casting, the receiver owns all three of these, exactly as it owns the
  // position above: the local player is parked paused at volume 0, so reporting
  // it announced a muted, paused session for a television playing out loud.
  const castPlaybackState = useMemo(
    () => ({ connected: castConnected, isPlaying: castIsPlaying, volume: castVolume, isMuted: castIsMuted }),
    [castConnected, castIsPlaying, castVolume, castIsMuted],
  );

  const getIsPaused = useCallback(() => {
    if (!mountedRef.current) return true;
    try {
      const local = { isPaused: !(player?.playing ?? false), volume: player?.volume ?? 1, isMuted: player?.muted ?? false };
      return reportedPlaybackState(local, castPlaybackState).isPaused;
    } catch { return true; }
  }, [player, castPlaybackState]);

  const getVolume = useCallback(() => {
    if (!mountedRef.current) return 1;
    try {
      const local = { isPaused: !(player?.playing ?? false), volume: player?.volume ?? 1, isMuted: player?.muted ?? false };
      return reportedPlaybackState(local, castPlaybackState).volume;
    } catch { return 1; }
  }, [player, castPlaybackState]);

  const getIsMuted = useCallback(() => {
    if (!mountedRef.current) return false;
    try {
      const local = { isPaused: !(player?.playing ?? false), volume: player?.volume ?? 1, isMuted: player?.muted ?? false };
      return reportedPlaybackState(local, castPlaybackState).isMuted;
    } catch { return false; }
  }, [player, castPlaybackState]);

  const remoteCallbacks = useMemo(() => ({
    onRemotePause: () => { try { player?.pause(); } catch (err) { console.warn("released", err); } },
    onRemotePlay: () => { try { player?.play(); } catch (err) { console.warn("released", err); } },
    onRemoteStop: () => handleBack(),
  }), [player, handleBack]);

  // Prewarm the next episode near the end so the hand-off does not wait on a
  // cold transcode start - the difference is very visible on a weak NAS.
  useProgressReporter(sessionId, getPositionMs, getIsPaused, getVolume, getIsMuted, remoteCallbacks, {
    mediaId,
    isEpisode,
    getDurationMs: () => effectiveDuration * 1000,
  }, () => castDeviceName);

  // The playhead of a LOCAL file, kept on disk. useProgressReporter above only
  // runs once a play session exists, and opening one is a request to the
  // server: offline there is no session, so not one tick and not the flush at
  // unmount ever records where the viewer got to. Forty minutes watched on a
  // plane resumed at the pre-takeoff position. This runs off localFilePath, not
  // off the network, so it covers the case by construction.
  const getPositionRef = useRef(getPositionMs);
  getPositionRef.current = getPositionMs;
  const durationRef = useRef(effectiveDuration);
  durationRef.current = effectiveDuration;
  useEffect(() => {
    if (!localFilePath || !mediaId) return;
    const save = () => {
      const ms = getPositionRef.current();
      if (ms > 0) void saveOfflineProgress(mediaId, ms, durationRef.current * 1000);
    };
    // Ten seconds, not the reporter's three: this write costs a file rewrite
    // rather than a request, and losing ten seconds of a resume point is not
    // something anyone notices.
    const id = setInterval(save, 10_000);
    return () => {
      clearInterval(id);
      save();
    };
  }, [localFilePath, mediaId]);

  // Hand the offline playhead back to the server the moment a session exists
  // again, then drop the local copy: it has done its job, and leaving it would
  // let a stale local record outrank a fresher server one later.
  useEffect(() => {
    if (!sessionId || !mediaId) return;
    let cancelled = false;
    void (async () => {
      const local = await getOfflineProgress(mediaId);
      if (cancelled || !local || local.positionMs <= 0) return;
      try {
        await playbackServiceReportProgress({
          sessionId,
          positionMs: String(local.positionMs),
          isPaused: true,
        });
        await clearOfflineProgress(mediaId);
      } catch {
        // Still offline, or the server refused: keep the record for next time.
      }
    })();
    return () => { cancelled = true; };
  }, [sessionId, mediaId]);

  const handleAudioSelect = useCallback(
    (index: number) => {
      userPickedAudioRef.current = true;
      setSelectedAudioIndex(index);
      const stream = audioStreams.find((s) => s.index === index);
      // Prefer an instant, no-reload client-side switch on the native player -
      // the audio group is already in the manifest, so this is decoupled from
      // the DEFAULT flag and from any transcode restart. Fall back to a restart
      // only when the native tracks aren't parsed yet or don't line up.
      const nativeTrack = stream
        ? pickNativeAudioTrack(nativeAudioTracks, stream.trackIndex, stream.language)
        : null;
      if (nativeTrack && player) {
        try {
          (player as unknown as { audioTrack: unknown }).audioTrack = nativeTrack;
          return;
        } catch (err) {
          console.warn("[player] native audioTrack switch failed, restarting:", err);
        }
      }
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(index, burnInSubtitleIndex, selectedQuality || undefined);
    },
    [player, nativeAudioTracks, audioStreams, restartWithTracks, burnInSubtitleIndex, selectedQuality],
  );

  // Restarts the transcode when entering/leaving a burn-in (PGS/VobSub)
  // subtitle - shared by native/server track selection and by switching to
  // an externally-picked file, which always targets nextIndex=null (the
  // server-side subtitle is deselected either way).
  const restartForSubtitleChange = useCallback(
    (nextIndex: number | null) => {
      const prevStream = subtitleStreams.find((s) => s.index === selectedSubtitleIndex);
      const nextStream = nextIndex === null ? null : subtitleStreams.find((s) => s.index === nextIndex);
      if (!prevStream?.burnIn && !nextStream?.burnIn) return;
      resumeAfterRestartRef.current = player?.playing ?? true;
      if (nextStream?.burnIn) {
        restartWithTracks(selectedAudioIndex, nextIndex ?? undefined, selectedQuality || undefined);
      } else {
        restartWithTracks(selectedAudioIndex, undefined, selectedQuality || undefined);
      }
    },
    [player, restartWithTracks, selectedAudioIndex, selectedQuality, subtitleStreams, selectedSubtitleIndex],
  );

  const handleSubtitleSelect = useCallback(
    (index: number | null) => {
      userPickedSubtitleRef.current = true;
      // Picking a native/server track (including Off) always drops any
      // client-picked external subtitle - SubtitleOverlay renders at most
      // one source at a time.
      setExternalSubtitle(null);
      // Text (non-burn-in) subtitles render client-side via SubtitleOverlay -
      // no manifest change needed, so switching between them (or off) is
      // instant with no session restart. Only PGS/VobSub tracks still need a
      // restart: the server has to burn them into the video itself.
      restartForSubtitleChange(index);
      setSelectedSubtitleIndex(index);
    },
    [restartForSubtitleChange],
  );

  // Upload button in PlayerSettingsPanel: picks + converts a local subtitle
  // file client-side (no server upload) and makes it the active subtitle,
  // deselecting any server track the same way handleSubtitleSelect(null) does.
  const handlePickExternalSubtitle = useCallback(async () => {
    const picked = await pickExternalSubtitle();
    if (!picked) return;
    userPickedSubtitleRef.current = true;
    restartForSubtitleChange(null);
    setSelectedSubtitleIndex(null);
    setExternalSubtitle({ name: picked.name, cues: parseVttCues(picked.vtt) });
    setSettingsOpen(false);
  }, [restartForSubtitleChange]);

  // Re-selects an already-loaded external subtitle (e.g. after switching back
  // from a server track) without re-picking the file.
  const handleSelectExternalSubtitle = useCallback(() => {
    if (!externalSubtitle) return;
    userPickedSubtitleRef.current = true;
    restartForSubtitleChange(null);
    setSelectedSubtitleIndex(null);
  }, [externalSubtitle, restartForSubtitleChange]);

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, quality || undefined);
    },
    [player, restartWithTracks, selectedAudioIndex, burnInSubtitleIndex],
  );

  const handleChapterSeek = useCallback(
    (timeSec: number) => {
      // timeSec is absolute. Native seek on the 0-based VOD timeline, transcode
      // included - same path as seekTo/seekBy. This used to restart the whole
      // session on transcode, which replayed from 0 while the UI added an offset
      // on top, so a chapter jump reported roughly twice the real position.
      if (player) {
        player.currentTime = timeSec;
      }
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      if (player) {
        player.currentTime = toSec;
      }
    },
    [player],
  );

  const handlePip = useCallback(async () => {
    if (!videoViewRef.current) return;
    try {
      await videoViewRef.current.startPictureInPicture();
    } catch (err) { console.warn("PiP unavailable", err); }
  }, []);

  const handleSpeedSelect = useCallback((speed: number) => {
    setPlaybackSpeed(speed);
    if (player) player.playbackRate = speed;
    resetHideTimer();
  }, [player, resetHideTimer]);

  // Current chapter + previous/next chapter jumps, same logic as the web:
  // "previous" first returns to the start of the current chapter if we've
  // been in it for more than 3s, otherwise to the previous chapter.
  const currentChapterIndex = useMemo(() => {
    let idx = -1;
    const ms = currentTime * 1000;
    for (let i = 0; i < chapters.length; i++) {
      if (ms >= chapters[i].startTimeMs) idx = i;
    }
    return idx;
  }, [chapters, currentTime]);

  const skipToNextChapter = useCallback(() => {
    const nextIdx = currentChapterIndex >= 0 ? currentChapterIndex + 1 : 0;
    if (nextIdx < chapters.length) handleChapterSeek(chapters[nextIdx].startTimeMs / 1000);
  }, [chapters, currentChapterIndex, handleChapterSeek]);

  const skipToPrevChapter = useCallback(() => {
    const current = currentChapterIndex >= 0 ? chapters[currentChapterIndex] : null;
    if (!current) return;
    const elapsed = currentTime - current.startTimeMs / 1000;
    if (currentChapterIndex > 0 && elapsed <= 3) {
      handleChapterSeek(chapters[currentChapterIndex - 1].startTimeMs / 1000);
    } else {
      handleChapterSeek(current.startTimeMs / 1000);
    }
  }, [chapters, currentChapterIndex, currentTime, handleChapterSeek]);

  // Server-side load shed (HTTP 503): distinct from isError below - nothing is
  // broken, the server just isn't ready to serve THIS request yet, for one of
  // two unrelated reasons (see usePlaySession's ShedReason doc). Calm clock
  // icon (not a red error) and copy that names neither "transcoding" nor any
  // other internal term. A local download plays regardless (see below).
  if (shedReason && !localFilePath) {
    return (
      <ShedScreen
        shedReason={shedReason}
        onRetry={() => {
          // Retrying after a shed means the user wants playback back - always autoplay.
          resumeAfterRestartRef.current = true;
          restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);
        }}
        onBack={handleBack}
      />
    );
  }

  // A local download plays regardless of the play-session network state -
  // don't gate on isError/isLoading/streamUrl when we already have bytes on
  // disk (see the videoSource useMemo above for the same rationale). A native
  // error is different: it means THIS playback died (network or local file),
  // so it always surfaces.
  if (nativeError || (isError && !localFilePath)) {
    return (
      <ErrorScreen
        onBack={handleBack}
        onRetry={() => {
          // Clear the latched native error (see the mediaId-change effect
          // above for why it must be reset explicitly) and restart the
          // session with the same track/quality selection - same "user
          // wants playback back" contract as ShedScreen's onRetry above.
          setNativeError(null);
          resumeAfterRestartRef.current = true;
          restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);
        }}
      />
    );
  }

  // Gate on useLocalSource, not localFilePath: a local copy whose audio the
  // device can't render must WAIT for the stream (the version with sound)
  // instead of starting silent and switching mid-play.
  if (!useLocalSource && (isLoading || !streamUrl)) {
    return <LoadingScreen />;
  }

  const seekProgress = effectiveDuration > 0 ? currentTime / effectiveDuration : 0;

  // Casting mode: show controller instead of local player
  if (castConnected) {
    return (
      <CastControllerScreen
        title={playerTitle}
        deviceName={castDeviceName}
        currentTime={castCurrentTime}
        duration={castDuration}
        isPlaying={castIsPlaying}
        skipFwd={skipFwd}
        skipBwd={skipBwd}
        volume={castVolume ?? undefined}
        muted={castIsMuted ?? undefined}
        onSeekToPosition={(p) => castClient?.seek?.({ position: p })}
        onPlayPause={() => { if (castIsPlaying) castClient?.pause?.(); else castClient?.play?.(); }}
        onSetVolume={(v) => castClient?.setStreamVolume?.(v)}
        onSetMuted={(m) => castClient?.setStreamMuted?.(m)}
        onBack={handleBack}
      />
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />

      {/* Sized by the native layout, not by a JS pixel measure: a
          `width: screenW` stage can only follow a rotation one JS commit AFTER
          the native views have already turned, so the video and every overlay
          spent those frames laid out at the previous orientation's size. */}
      <View style={styles.stage} {...panResponder.panHandlers}>
        <View pointerEvents="none" style={styles.video}>
          <VideoView
            ref={videoViewRef}
            player={player}
            style={styles.video}
            nativeControls={false}
            contentFit="contain"
            allowsPictureInPicture={pipSupported}
          />
        </View>

        <GestureIndicators
          seekFlash={seekFlash}
          scrubPreview={scrubPreview}
          volumeIndicator={volumeIndicator}
          brightnessIndicator={brightnessIndicator}
          skipFwd={skipFwd}
          skipBwd={skipBwd}
        />

        {/* Skip intro */}
        <SkipIntroOverlay
          mediaId={mediaId}
          filePath={media?.filePath}
          currentTime={currentTime}
          onSkip={handleSkipIntro}
          bottomOffset={Math.max(controlsHeight, 96) + 16}
          hidden={nextEp.visible}
          onNextEpisode={nextEp.nextEpisode ? nextEp.playNow : undefined}
          autoAdvance={autoplayNext}
        />

        {nextEp.visible && nextEp.nextEpisode && (
          <NextEpisodeOverlay nextEp={nextEp} bottomOffset={Math.max(controlsHeight, 96) + 16} />
        )}

        {/* Client-side WebVTT rendering: expo-video exposes native subtitle
            TRACK SELECTION only (no cue access, no styling), so a text
            (non-burn-in) subtitle is rendered here instead - see
            handleSubtitleSelect/clientSubtitleIndex above for how the native
            track is kept off to avoid a doubled rendering. Sits below the
            controls (rendered after this in the tree = drawn on top). */}
        <SubtitleOverlay
          mediaId={mediaId}
          subtitleIndex={clientSubtitleIndex}
          currentTimeSec={currentTime}
          offsetMs={subtitleOffsetMs}
          style={subtitleStyle}
          externalCues={externalSubtitle?.cues ?? null}
        />

        {/* Invisible scrim under the settings panel: a tap outside the panel
            closes it (equivalent of the web's handleOverlayClick). The
            player's gestures are suspended while the panel is open. */}
        {settingsOpen && (
          <Pressable
            style={StyleSheet.absoluteFill}
            onPress={() => setSettingsOpen(false)}
            accessible={false}
          />
        )}

        {showControls && (
          // Kept mounted but not painted while the window is resizing: the
          // secondary row's open/closed state (which holds the fullscreen
          // button itself) survives the rotation, and the viewer never sees an
          // intermediate placement.
          <View
            style={[styles.overlay, resizing && styles.overlayHidden]}
            pointerEvents={resizing ? "none" : "box-none"}
            accessibilityElementsHidden={resizing}
            importantForAccessibility={resizing ? "no-hide-descendants" : "auto"}
          >
            <View style={[styles.topBar, { top: 0, paddingTop: Math.max(16, insets.top), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}>
              <Pressable onPress={handleBack} hitSlop={16} accessible={true} accessibilityLabel={t("common.back")} accessibilityRole="button">
                <Ionicons name="chevron-back" size={28} color="#fff" />
              </Pressable>
              <Text style={styles.title} numberOfLines={1}>
                {playerTitle}
              </Text>
            </View>

            {/* Top-right cluster: watch-together, then cast (matches the web
                player layout) - fades with the rest of the controls. */}
            <PlayerTopRightCluster
              syncConnected={syncPlay.state.connected}
              castConnected={castConnected}
              castAvailable={!!CastContext}
              onCast={() => { handleCast(); resetHideTimer(); }}
              onSyncPress={() => {
                if (syncPlay.state.connected) syncPlay.leaveRoom();
                else setActiveSheet("syncPlay");
                resetHideTimer();
              }}
              onPlayOnPress={() => { setActiveSheet("playOn"); resetHideTimer(); }}
              style={{ paddingTop: Math.max(16, insets.top), paddingRight: Math.max(20, insets.right) }}
            />

            <View
              style={[styles.bottomControls, { paddingBottom: Math.max(20, insets.bottom), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}
              onLayout={(e) => setControlsHeight(e.nativeEvent.layout.height)}
            >
              <PlayerSeekBar
                mediaId={mediaId}
                progress={seekProgress}
                currentTime={currentTime}
                effectiveDuration={effectiveDuration}
                chapters={chapters}
                onPreview={seekPreview}
                onCommit={seekTo}
                onScrubbingChange={(s) => { seekingRef.current = s; }}
              />

              {/* Active chapter title under the seek bar (web parity:
                  PlayerControls shows the current chapter's name here). Sighted
                  users otherwise only got it by opening the chapters picker. */}
              {currentChapterIndex >= 0 && chapters[currentChapterIndex]?.title ? (
                <Text style={styles.chapterTitle} numberOfLines={1}>
                  {chapters[currentChapterIndex].title}
                </Text>
              ) : null}

              {/* Transport row, copy of the web bar: episodes/chapters,
                  +/-Ns, time, volume, then "..." toward the secondary row. */}
              <View style={styles.transportRow}>
                <View style={{ flex: 1 }}>
                  <PlayerControlsRow
                    isPlaying={isPlaying}
                    isBuffering={isBuffering}
                    currentTime={currentTime}
                    effectiveDuration={effectiveDuration}
                    skipFwd={skipFwd}
                    skipBwd={skipBwd}
                    isFullscreen={isFullscreen}
                    volume={volume}
                    muted={muted}
                    prevEpisode={prevEpisode?.id ? { id: prevEpisode.id } : null}
                    nextEpisode={nextEpisode?.id ? { id: nextEpisode.id } : null}
                    hasPrevChapter={chapters.length > 0 && currentChapterIndex > 0}
                    hasNextChapter={chapters.length > 0 && currentChapterIndex >= 0 && currentChapterIndex < chapters.length - 1}
                    hasChapters={chapters.length > 0}
                    currentChapterTitle={
                      currentChapterIndex >= 0 ? chapters[currentChapterIndex]?.title : undefined
                    }
                    settingsOpen={settingsOpen}
                    sleepTimerActive={sleepTimer.state !== null}
                    pipSupported={pipSupported}
                    onSeekBy={seekBy}
                    onTogglePlayPause={togglePlayPause}
                    onToggleMute={toggleMute}
                    onVolumeChange={handleVolumeChange}
                    maxVolume={maxVolume}
                    onEpisodeChange={handleNavigateEpisode}
                    onPrevChapter={skipToPrevChapter}
                    onNextChapter={skipToNextChapter}
                    onOpenChapters={() => setActiveSheet("chapters")}
                    onToggleSettings={() => { setSettingsOpen((v) => !v); resetHideTimer(); }}
                    onOpenStreamInfo={() => setActiveSheet("streamInfo")}
                    onOpenSleepTimer={() => setActiveSheet("sleepTimer")}
                    onPip={handlePip}
                    onToggleFullscreen={toggleFullscreen}
                  />
                </View>

                {/* Episodes panel entry point (web parity: PlayerEpisodesPanel,
                    which gates on episodes.length > 0). It used to require > 1,
                    which hid the button on a single-episode season - and with it
                    the panel's SEASON SELECTOR, so the other seasons became
                    unreachable from the player. */}
                {currentSeasonEpisodes.length > 0 && (
                  <Pressable
                    onPress={() => { setEpisodesPanelOpened(true); setActiveSheet("episodes"); resetHideTimer(); }}
                    hitSlop={8}
                    style={styles.episodesBtn}
                    accessible={true}
                    accessibilityLabel={t("player.controls.episodes", "Épisodes")}
                    accessibilityRole="button"
                  >
                    <Ionicons name="list" size={ICON_SIZE.lg} color="#fff" />
                  </Pressable>
                )}
              </View>

            </View>

            {/* Settings panel anchored above the bar, right-aligned - same
                popup as the web player's gear icon. Sibling of the bar and
                positioned in pixels: a percentage bottom broke the
                ScrollView's measured height (panel drawn but the touch
                area shrunk to a few pixels). */}
            {settingsOpen && (
              <PlayerSettingsPanel
                audioStreams={audioStreams}
                currentAudioIndex={selectedAudioIndex}
                onAudioChange={handleAudioSelect}
                subtitleStreams={subtitleStreams}
                currentSubtitleIndex={selectedSubtitleIndex}
                onSubtitleChange={handleSubtitleSelect}
                currentQuality={selectedQuality}
                onQualityChange={handleQualitySelect}
                availableQualities={availableQualities}
                playbackSpeed={playbackSpeed}
                onSpeedChange={handleSpeedSelect}
                brightness={brightness}
                onBrightnessChange={handleBrightnessChange}
                audioOffsetMs={audioOffsetMs}
                onAudioOffsetChange={setAudioOffsetMs}
                audioOffsetAvailable={audioOffsetAvailable}
                subtitleStyle={subtitleStyle}
                onSubtitleStyleChange={handleSubtitleStyleChange}
                subtitleOffsetMs={subtitleOffsetMs}
                onSubtitleOffsetChange={setSubtitleOffsetMs}
                externalSubtitleName={externalSubtitle?.name ?? null}
                externalSubtitleActive={!!externalSubtitle}
                onPickExternalSubtitle={handlePickExternalSubtitle}
                onSelectExternalSubtitle={handleSelectExternalSubtitle}
                autoplayNext={autoplayNext}
                onAutoplayNextChange={handleAutoplayNextChange}
                onClose={() => setSettingsOpen(false)}
                style={{
                  position: "absolute",
                  right: Math.max(20, insets.right),
                  bottom: controlsHeight + 8,
                }}
              />
            )}
          </View>
        )}

        {syncPlay.state.connected && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0 }}>
            <SyncPlayBar
              roomCode={syncPlay.state.roomCode}
              isHost={syncPlay.state.isHost}
              members={syncPlay.state.members}
              connected={syncPlay.state.connected}
              onLeave={syncPlay.leaveRoom}
              queue={syncPlay.state.queue}
              trackIndex={syncPlay.state.trackIndex}
              onNext={syncPlay.sendNext}
              onPrev={syncPlay.sendPrev}
            />
          </View>
        )}
      </View>

      {/* Sheets mount ONLY while open: a hidden RN Modal keeps its children
          mounted, so every panel below used to re-render on each ~2/s
          currentTime tick for the whole playback (and SyncPlayModal kept a
          playlists query alive). All their state lives in this screen's
          hooks, so unmounting a closed sheet loses nothing. */}
      {activeSheet === "chapters" && (
      <ChaptersPicker
        visible={activeSheet === "chapters"}
        onClose={() => setActiveSheet(null)}
        chapters={chapters}
        currentTimeMs={currentTime * 1000}
        onSeek={handleChapterSeek}
      />
      )}

      {activeSheet === "episodes" && (
      <BottomSheet
        visible={activeSheet === "episodes"}
        onClose={() => setActiveSheet(null)}
        title={t("player.controls.episodes", "Épisodes")}
      >
        <PlayerEpisodesPanel
          seasons={seasons}
          currentSeasonId={selectedSeasonId}
          episodes={panelEpisodes}
          currentEpisodeId={mediaId}
          onSeasonChange={setSelectedSeasonId}
          onEpisodeChange={(episodeId) => { handleNavigateEpisode(episodeId); setActiveSheet(null); }}
        />
      </BottomSheet>
      )}

      {activeSheet === "streamInfo" && (
      <StreamInfoPanel
        visible={activeSheet === "streamInfo"}
        onClose={() => setActiveSheet(null)}
        streamInfo={streamInfo}
        audioStreamIndex={selectedAudioIndex}
        served={servedVariant(streamInfo, playingTrack.playing, playingTrack.ladderBitrates)}
      />
      )}

      {activeSheet === "sleepTimer" && (
      <SleepTimerPicker
        visible={activeSheet === "sleepTimer"}
        onClose={() => setActiveSheet(null)}
        state={sleepTimer.state}
        onPickMinutes={(m) => sleepTimer.setMinutes(m)}
        onPickEndOfEpisode={() => sleepTimer.setEndOfEpisode()}
        onCancel={() => sleepTimer.cancel()}
      />
      )}

      {activeSheet === "playOn" && (
      <PlayOnSheet
        visible={activeSheet === "playOn"}
        mediaId={mediaId}
        onClose={() => setActiveSheet(null)}
      />
      )}

      {activeSheet === "syncPlay" && (
      <SyncPlayModal
        visible={activeSheet === "syncPlay"}
        onClose={() => setActiveSheet(null)}
        creating={syncCreating}
        joining={syncJoining}
        onCreateRoom={async () => {
          setSyncCreating(true);
          try {
            await syncPlay.createRoom(mediaId);
            setActiveSheet(null);
          } catch {
            toast.error(t("media.syncplay.createFailed", "Failed to create room"));
          } finally { setSyncCreating(false); }
        }}
        onJoinRoom={async (code) => {
          setSyncJoining(true);
          try {
            await syncPlay.joinByCode(code);
            setActiveSheet(null);
          } catch {
            toast.error(t("media.syncplay.joinFailed", "Failed to join room"));
          } finally { setSyncJoining(false); }
        }}
      />
      )}

      {/* Plugin overlays, same slot as web's player page. Anchored to the
          bottom edge; box-none so an empty slot can never eat a tap meant
          for the controls. */}
      <View pointerEvents="box-none" style={{ position: "absolute", left: 0, right: 0, bottom: 0 }}>
        <PluginSlot name="player.overlay" context={{ mediaId }} />
      </View>

    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  video: {
    width: "100%",
    height: "100%",
  },
  stage: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
  },
  overlayHidden: {
    opacity: 0,
  },
  overlay: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: "rgba(0,0,0,0.4)",
  },
  topBar: {
    position: "absolute",
    left: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    paddingTop: 16,
    paddingHorizontal: 20,
    zIndex: 2,
  },
  title: {
    flex: 1,
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
    textAlign: "center",
  },
  bottomControls: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
    paddingHorizontal: 20,
    zIndex: 2,
  },
  transportRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  chapterTitle: {
    color: "rgba(255,255,255,0.75)",
    fontSize: 12,
    marginTop: 2,
    marginBottom: 2,
  },
  episodesBtn: {
    padding: 4,
    marginLeft: 4,
  },
});
