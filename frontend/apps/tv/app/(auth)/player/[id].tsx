import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  createPlayerEventReporter,
  itemLabel,
  playerAudioStreams,
  playerSubtitleStreams,
} from "@evey/api";
import { View, Text, Pressable, ActivityIndicator, StyleSheet, useWindowDimensions, useTVEventHandler, BackHandler, findNodeHandle } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { VideoView } from "expo-video";
import { useSourcedVideoPlayer } from "@evey/client/native/useSourcedVideoPlayer";
import { setPlayhead, setSeekPreview } from "@evey/client";
import { playerBufferOptions } from "@evey/client/native/playerBufferOptions";
import { getDeviceProfile } from "@evey/media-caps";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetMediaItem,
  parseTrackPick,
} from "@evey/api";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { dashUrlFor, devDashEnabled } from "../../../src/lib/devDash";
import { useAudioCapsBeacon } from "../../../src/hooks/useAudioCapsBeacon";
import { useDecoderTelemetry } from "../../../src/hooks/useDecoderTelemetry";
import { playbackReloadRetry, reloadableStatus, reloadAtPlayhead } from "@evey/client/native/playbackReload";
import { useQoEBeacon } from "../../../src/hooks/useQoEBeacon";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters, findCurrentChapterIndex } from "../../../src/hooks/useChapters";
import { resumeNudger } from "../../../src/lib/resumeNudge";
import { useSleepTimer } from "../../../src/hooks/useSleepTimer";
import { seekDeltaForKey } from "../../../src/hooks/playerSeek";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import { playerExit } from "../../../src/hooks/playerExit";
import { noteViewerActivity, resolveStartPositionMs, servedVariant, shedI18nKey, useAutoMarkWatched, useFileAudioSelection, usePlayerProfilePrefs, usePlayerTrackSelection, usePlayingVideoTrack, useRemoteSeek, nextPlaybackSpeed, useProfileLock, type AudioTrackPlayer } from "@evey/client";
import { playerTitle } from "../../../src/lib/playerTitle";
import { useCastConnectBridge } from "../../../src/hooks/useCastConnectBridge";
import { useInterruptedPlaybackMarker } from "../../../src/hooks/useInterruptedPlaybackMarker";
import { useAuthStore } from "@evey/store";
import { QualityPicker } from "../../../src/components/QualityPicker";
import { AudioTrackPicker, type AudioStream } from "../../../src/components/AudioTrackPicker";
import { SubtitlePicker, type SubtitleStream } from "../../../src/components/SubtitlePicker";
import { TrackOptionsMenu } from "../../../src/components/TrackOptionsMenu";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { SleepTimerPicker } from "../../../src/components/SleepTimerPicker";
import { VolumePicker } from "../../../src/components/VolumePicker";
import { StreamInfoPanel } from "../../../src/components/StreamInfoPanel";
import { SkipIntroButton } from "../../../src/components/SkipIntroButton";
import { SubtitleOverlay } from "../../../src/components/SubtitleOverlay";
import { SubtitleStylePicker } from "../../../src/components/SubtitleStylePicker";
import {
  DEFAULT_SUBTITLE_STYLE,
  getSubtitleStylePref,
  saveSubtitleStylePref,
  type SubtitleStyle,
} from "../../../src/lib/subtitleStyle";
import { NextEpisodeOverlay } from "../../../src/components/NextEpisodeOverlay";
import { PlayerBottomPanel } from "../../../src/components/PlayerBottomPanel";
import { PlayerScrim } from "../../../src/components/PlayerScrim";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";
import { PlayerStatusScreen, PlayerLoadingScreen, PlayerSleepScreen } from "../../../src/components/PlayerStatusScreen";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { useSyncplay } from "../../../src/hooks/useSyncplay";
import { SyncplaySessionBadge } from "../../../src/components/SyncplaySessionBadge";

const REMOTE_KEYS = new Set(["up", "down", "left", "right", "select", "longSelect", "playPause", "play", "pause", "fastForward", "rewind", "menu", "back", "longLeft", "longRight"]);

const CONTROLS_TIMEOUT = 4000;
// Longer while the controls hold the remote: the viewer is aiming at a button,
// not glancing at the time.
const CONTROLS_FOCUSED_TIMEOUT = 8000;
// A native "loading" status fires on every micro-stall (segment boundary, ABR
// switch), same as the raw `waiting` event web debounces - see
// HLSPlayer.tsx's setIsBufferingDebounced. Flashing a spinner over those
// reads as "the app is struggling" when nothing is wrong.
const BUFFERING_OVERLAY_DELAY_MS = 1200;

// Android KeyEvent actions as react-native-tvos forwards them. Measured on
// the box (2026-09-20): a tap delivers ONE event, "right" with action 1; a
// held key delivers "longRight" with action 0 once the platform's long-press
// delay has passed, then "longRight" with action 1 on release.
const KEY_ACTION_DOWN = 0;
const KEY_ACTION_UP = 1;
export default function PlayerScreen() {
  const theme = useTheme();
  // `restart=1` means the viewer chose "from the beginning" on the media page.
  // Without it the player always resumes, which is the right default - this is
  // the only way to say otherwise.
  // `start` (seconds) comes from a Cast Connect LOAD: the sender already knew
  // where the viewer was, and that position wins over this profile's watch
  // progress on this television.
  const { id, roomId, restart, audio, subtitle, start } = useLocalSearchParams<{ id: string; roomId?: string; restart?: string; audio?: string; subtitle?: string; start?: string }>();
  // Tracks picked on the media page (absolute stream indices, "off" = no
  // subtitle): they seed the selection below and count as a manual pick.
  const trackPick = parseTrackPick({ audio, subtitle });
  const { t } = useTranslation();
  const mediaId = id ?? "";
  useAudioCapsBeacon(mediaId);
  // Fatal-native-error beacon, identical to the web and mobile players'.
  // Self-throttling (dedupe + per-session budget) so a stream failing in a
  // loop cannot flood the admin log.
  const reportPlayerEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);

  // Two hops up so the top bar can name the SHOW, not just the episode: an
  // episode hangs off its season, and no payload carries the series title (see
  // the backlog note of 2026-08-03). Only fetched while an episode is playing,
  // and only once per playback - a bounded cost the rows could not afford.
  const isEpisodePlaying = media?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const { data: season } = useLibraryServiceGetMediaItem(media?.parentId ?? "", {
    query: { enabled: isEpisodePlaying && !!media?.parentId },
  });
  const { data: series } = useLibraryServiceGetMediaItem(season?.parentId ?? "", {
    query: { enabled: isEpisodePlaying && !!season?.parentId },
  });
  const seasonNumber = season?.indexNumber || media?.seasonIndexNumber || undefined;

  // A series or a season has no file of its own: asking the server to stream
  // one can only 404, and the player would then blame the stream. It happened
  // in the field - the home hero hands the player whatever it is spotlighting,
  // and "Maison du dragon" is a series. Callers are supposed to resolve an
  // episode first; this is the backstop for the ones that do not, and for the
  // race where the media page has not loaded its episode list yet.
  const notPlayable =
    media?.type === "MEDIA_ITEM_TYPE_SERIES" || media?.type === "MEDIA_ITEM_TYPE_SEASON";
  useEffect(() => {
    if (notPlayable) router.replace(`/media/${mediaId}`);
  }, [notPlayable, mediaId]);
  // A film starts, the album stops. The music player is mounted for the whole
  // signed-in app and kept playing under the video - two soundtracks at once,
  // and its bar drawn over the picture - until the viewer closed it by hand.
  useEffect(() => {
    useMusicQueueStore.getState().clear();
  }, [mediaId]);
  const { streamUrl, sessionId, streamInfo, isLoading, isError, isAuthError, shedReason, stopSession, restartWithTracks, progress, restartPositionMs } =
    usePlaySession(mediaId, {
      ...(trackPick.audio === undefined ? {} : { audioStreamIndex: trackPick.audio }),
      // The encoder is pre-warmed where the player will start: over, or at
      // the cast's hand-over position.
      restart: restart === "1",
      ...(Number(start) > 0 ? { startSec: Number(start) } : {}),
    });

  // The running profile's languages and "auto-play next episode" choice, the
  // same reading on every player (usePlayerProfilePrefs). A track picked in
  // this session is never overridden by them (userPickedAudioRef / ...).
  const {
    audioLangs: profileAudioLangs,
    subtitleLangs: profileSubtitleLangs,
    autoplayNext,
    setAutoplayNext,
  } = usePlayerProfilePrefs();
  const handleToggleAutoplayNext = useCallback(() => setAutoplayNext(!autoplayNext), [setAutoplayNext, autoplayNext]);

  const [showControls, setShowControls] = useState(true);
  // True while the action bar owns the D-pad. Mirrors the existing
  // skipVisible / syncplayBadgeFocused pattern: exactly one thing interprets
  // a key press at a time.
  const [barFocused, setBarFocused] = useState(false);
  // The full-screen surface below every overlay. Some native view must hold
  // TV focus at all times or Android stops delivering key events entirely -
  // the screen deliberately has no other focusable while the bar is closed,
  // which left the whole player deaf (remote AND pointer) on real hardware.
  const [surfaceFocused, setSurfaceFocused] = useState(false);
  const surfaceRef = useRef<View>(null);
  const playButtonRef = useRef<View>(null);
  const rewindButtonRef = useRef<View>(null);
  const forwardButtonRef = useRef<View>(null);
  const settingsButtonRef = useRef<View>(null);
  const speedButtonRef = useRef<View>(null);
  const timelineRef = useRef<View>(null);
  const barRefs = useMemo(() => ({
    rewind: rewindButtonRef,
    play: playButtonRef,
    forward: forwardButtonRef,
    speed: speedButtonRef,
    settings: settingsButtonRef,
    timeline: timelineRef,
  }), []);
  // The timeline is a control of its own in the row: while it holds the
  // focus, left/right move the playhead instead of walking the buttons, and
  // OK lands the jump (owner, 2026-09-22: "the timer bar must be selectable
  // and navigable").
  const [timelineFocused, setTimelineFocused] = useState(false);
  // Native tags of the five controls, resolved once they are on screen: the
  // nextFocus* props take a tag, not a ref.
  const [focusTags, setFocusTags] = useState<{ rewind?: number; play?: number; forward?: number; settings?: number; speed?: number; timeline?: number }>({});

  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [showTrackMenu, setShowTrackMenu] = useState(false);
  const [showSubtitleStyle, setShowSubtitleStyle] = useState(false);
  const [subtitleStyle, setSubtitleStyle] = useState<SubtitleStyle>(DEFAULT_SUBTITLE_STYLE);
  const [showAudioPicker, setShowAudioPicker] = useState(false);
  const [showSubtitlePicker, setShowSubtitlePicker] = useState(false);
  const [showChaptersPicker, setShowChaptersPicker] = useState(false);
  const [showSleepTimerPicker, setShowSleepTimerPicker] = useState(false);
  const [showVolumePicker, setShowVolumePicker] = useState(false);
  const [showStreamInfoPanel, setShowStreamInfoPanel] = useState(false);
  const [skipVisible, setSkipVisible] = useState(false);
  // Whether the Skip button itself holds TV focus - distinct from skipVisible,
  // see the remote handler's "select" guard below (B5).
  const [skipFocused, setSkipFocused] = useState(false);
  // Armed by SkipIntroButton while the credits countdown runs: BACK then
  // keeps watching instead of leaving the player (prod 2026-10-15).
  const cancelCountdownRef = useRef<(() => void) | null>(null);
  const [selectedQuality, setSelectedQuality] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const committedTimeRef = useRef(0);
  // Whether anything on screen follows the clock right now.
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const [volumePct, setVolumePct] = useState(100);
  const [muted, setMuted] = useState(false);
  // Debounced "the stream stalled" indicator - see setIsBufferingDebounced
  // below for why it is not just `ev.status === "loading"` on its own.
  const [isBuffering, setIsBuffering] = useState(false);
  // Mirrors for the remote handlers: a key repeat or a hold tick arrives
  // faster than this screen re-renders on the television, and a closure over
  // the state would add to a playhead that is already stale.
  const currentTimeRef = useRef(0);
  const durationRef = useRef(0);
  // A fatal native failure (decoder error, 401 on a segment, broken manifest)
  // surfaces ONLY through the player's statusChange event - isError from
  // usePlaySession covers just the session-start network call. Ignoring it
  // left a black screen with live controls.
  const [nativeError, setNativeError] = useState<string | null>(null);
  // The native player is recreated per media (new streamUrl), but this state
  // wasn't reset with it - a fatal error on one item stayed glued to the screen
  // of the NEXT one (e.g. after a next-episode auto-advance), showing a dead
  // error screen over a stream that hadn't even started yet.
  useEffect(() => { setNativeError(null); }, [mediaId]);
  // When the last reload on a 404 or 410 was made (see playbackReloadRetry).
  const reloadRetryAtRef = useRef<number | null>(null);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Whether the NEXT player instance should start playing. Changing tracks or
  // quality recreates the native player, whose setup used to call play()
  // unconditionally - a paused screen sprang back to life on every track
  // change. Handlers snapshot the playing state right before restarting;
  // defaults to true so first entry (and next-episode remounts) autoplay.
  const resumeAfterRestartRef = useRef(true);

  const audioStreams: AudioStream[] = useMemo(() => playerAudioStreams(streamInfo?.mediaStreams), [streamInfo]);
  const subtitleStreams: SubtitleStream[] = useMemo(() => playerSubtitleStreams(streamInfo?.mediaStreams), [streamInfo]);

  // The audio and subtitle choice, shared with the phone
  // (usePlayerTrackSelection): the title page's pick and a hand pick win over
  // the profile's languages, a new title starts from scratch.
  // burnInSubtitleIndex is what the SERVER may be told (`?subtitleTrack=` is an
  // order to burn, and a burn-in denies the copy rung), clientSubtitleIndex the
  // track SubtitleOverlay draws itself, subtitle-relative.
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



  const { data: chaptersData } = useChapters(mediaId);
  const chapters = chaptersData ?? [];

  // Leaving the screen calls stopSession(), which clears streamUrl. Letting the
  // source go back to null would release the native player one frame before
  // the unmount releases the screen. Holding the last source keeps the identity
  // stable so unmount performs the single real release.
  // (This is hygiene, not the exit-crash fix: that one was an unguarded read in
  // getPositionMs, see the comment there.)
  const lastSourceRef = useRef<{ uri: string; contentType?: "hls" | "dash" } | null>(null);
  // Dev DASH (src/lib/devDash.ts): the same session's manifest.mpd, probed
  // first; any refusal plays the HLS master. Failed once, the title stays on
  // HLS until the screen changes media.
  const [dashSource, setDashSource] = useState<{ url: string; state: "probing" | "on" | "off" } | null>(null);
  const dashFailedRef = useRef(false);
  useEffect(() => { dashFailedRef.current = false; }, [mediaId]);
  useEffect(() => {
    const url = streamUrl && devDashEnabled() && !dashFailedRef.current ? dashUrlFor(streamUrl) : null;
    if (!url) {
      setDashSource(null);
      return;
    }
    let cancelled = false;
    setDashSource({ url, state: "probing" });
    fetch(url)
      .then((res) => {
        if (cancelled) return;
        console.log(`[dev-dash] manifest.mpd ${res.status}: ${res.ok ? "DASH" : "HLS fallback"}`);
        setDashSource({ url, state: res.ok ? "on" : "off" });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        console.log(`[dev-dash] manifest.mpd unreachable (${String(err)}): HLS fallback`);
        setDashSource({ url, state: "off" });
      });
    return () => { cancelled = true; };
  }, [streamUrl]);
  const dashActiveRef = useRef(false);
  const videoSource = useMemo(() => {
    if (!streamUrl) return lastSourceRef.current;
    if (dashSource?.state === "probing") return lastSourceRef.current;
    const isDash = dashSource?.state === "on";
    dashActiveRef.current = isDash;
    const isHls = !isDash && streamUrl.includes("m3u8");
    const next = {
      uri: isDash ? dashSource.url : streamUrl,
      ...(isDash ? { contentType: "dash" as const } : isHls ? { contentType: "hls" as const } : {}),
    };
    lastSourceRef.current = next;
    return next;
  }, [streamUrl, dashSource]);

  const resumePositionSec = useMemo(() => {
    // A quality, audio or subtitle switch continues where the viewer was.
    if (restartPositionMs != null) return restartPositionMs / 1000;
    if (restart === "1") return 0;
    // A cast hands over a position the viewer was actually at, a second ago,
    // on another device. It outranks the progress row this television would
    // otherwise read (which may be stale, or belong to an older viewing).
    const cast = Number(start);
    if (Number.isFinite(cast) && cast > 0) return cast;
    // Played, or within the last seconds: start over (the rule of every client).
    return resolveStartPositionMs(false, progress) / 1000;
  }, [progress, restart, start, restartPositionMs]);

  // No source yet (the stream URL is still being negotiated) means no native
  // player at all: see useSourcedVideoPlayer for why a sourceless one is unsafe.
  const player = useSourcedVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    const buffer = playerBufferOptions(getDeviceProfile());
    if (buffer) p.bufferOptions = { ...p.bufferOptions, ...buffer };
    if (resumePositionSec > 0) {
      p.currentTime = resumePositionSec;
    }
    if (resumeAfterRestartRef.current) p.play();
  });

  // A kids profile reaching the end of its opening hours: the lock screen
  // covers the player, and the film stops instead of playing its buffer out.
  const profileLock = useProfileLock();
  useEffect(() => {
    if (!profileLock.closed || !player) return;
    try {
      if (player.playing) player.pause();
    } catch { /* released */ }
  }, [profileLock.closed, player]);

  // expo-video hands JavaScript a proxy onto a NATIVE object. When the source
  // changes - a quality switch, an audio-track switch, leaving the screen -
  // that native object is released, and any later touch throws
  // "Cannot use shared object that was already released" (or, more cryptically,
  // "cannot be cast to VideoPlayer (received Integer)"). React's own listeners
  // are unsubscribed on cleanup, but callbacks that fire from OUTSIDE the
  // component lifecycle - a SyncPlay websocket message arriving mid-teardown -
  // have no such protection, and one of them took the whole player screen down
  // to a red box.
  //
  // `withPlayer` is the guard: it runs the access, and swallows exactly the
  // released-object failure. Anything else still surfaces.
  const withPlayer = useCallback(<T,>(fn: (p: NonNullable<typeof player>) => T): T | undefined => {
    if (!player) return undefined;
    try {
      return fn(player);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (/already released|cannot be cast to/i.test(msg)) return undefined;
      throw err;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player]);


  // Once playback has actually started, a later "loading" status is a real
  // stall worth telling the viewer about - before that, it is just the normal
  // startup sequence the entry spinner (isLoading) already covers.
  const hasStartedPlaybackRef = useRef(false);
  useEffect(() => { if (isPlaying) hasStartedPlaybackRef.current = true; }, [isPlaying]);
  const bufferingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const setIsBufferingDebounced = useCallback((stalled: boolean) => {
    if (stalled) {
      if (!hasStartedPlaybackRef.current) return;
      if (bufferingTimerRef.current !== null) return; // already counting down
      bufferingTimerRef.current = setTimeout(() => {
        bufferingTimerRef.current = null;
        setIsBuffering(true);
      }, BUFFERING_OVERLAY_DELAY_MS);
      return;
    }
    if (bufferingTimerRef.current !== null) {
      clearTimeout(bufferingTimerRef.current);
      bufferingTimerRef.current = null;
    }
    setIsBuffering(false);
  }, []);
  useEffect(() => () => {
    if (bufferingTimerRef.current !== null) clearTimeout(bufferingTimerRef.current);
  }, []);

  // The variant ExoPlayer decodes now, for the stream info panel.
  const playingTrack = usePlayingVideoTrack(player);
  // The QoE samples of /admin/qoe's device table: ExoPlayer sends no CMCD,
  // the app does. Only a transcode (HLS, or the DASH trial) has segments.
  useQoEBeacon(player, {
    mediaId,
    sessionId,
    format: dashSource?.state === "on" ? "d" : streamUrl?.includes("m3u8") ? "h" : null,
    playing: playingTrack.playing,
    ladderBitrates: playingTrack.ladderBitrates,
  });

  // Filled once nextEp and handleBack exist (below); read by the playToEnd
  // listener, which only subscribes once per player.
  const playedToEndRef = useRef<() => void>(() => {});

  useEffect(() => {
    if (!player) return;
    // The real end of the stream: currentTime can stop short of the catalogue
    // duration, so nextEp.ended alone may never turn true at the end of a film.
    const endSub = player.addListener("playToEnd", () => playedToEndRef.current());
    const timeSub = player.addListener("timeUpdate", (ev) => {
      currentTimeRef.current = ev.currentTime;
      // The fine position goes to the playhead store, read by the subtitle
      // overlay and the timeline alone; this screen's own copy moves once a
      // second, or every tick re-rendered the whole screen.
      setPlayhead(ev.currentTime);
      if (Math.abs(ev.currentTime - committedTimeRef.current) >= 1) {
        committedTimeRef.current = ev.currentTime;
        setCurrentTime(ev.currentTime);
      }
      // A tick can land between the release and the unsubscribe. Same value,
      // no state: a set per tick had React re-running the screen.
      withPlayer((p) => {
        if (p.duration !== durationRef.current) {
          durationRef.current = p.duration;
          setDuration(p.duration);
        }
      });
    });
    const statusSub = player.addListener("statusChange", (ev) => {
      withPlayer((p) => {
        setIsPlaying(p.playing);
        setDuration(p.duration);
      });
      setIsBufferingDebounced(ev.status === "loading");
      if (ev.status === "error" && dashActiveRef.current && !dashFailedRef.current) {
        // Dev DASH: the HLS master takes over the same session.
        console.log(`[dev-dash] DASH source failed (${ev.error?.message ?? ""}): HLS fallback`);
        dashFailedRef.current = true;
        setDashSource((d) => (d ? { ...d, state: "off" } : d));
        return;
      }
      // A 410 (a URL the server no longer serves as held) or a 404 (a
      // resource gone, e.g. evicted during a long pause): reload the same
      // source at the playhead, like the web player does.
      const now = Date.now();
      if (ev.status === "error" && player && playbackReloadRetry(ev.error?.message, reloadRetryAtRef.current, now)) {
        reloadRetryAtRef.current = now;
        const at = currentTimeRef.current;
        reportPlayerEvent({
          type: "native_player_reload",
          code: reloadableStatus(ev.error?.message) ?? 0,
          details: ev.error?.message ?? "",
          fatal: false,
          posSec: Math.round(at),
        });
        reloadAtPlayhead(player, lastSourceRef.current, at).catch(() => {
          setNativeError(ev.error?.message ?? "playback error");
        });
        return;
      }
      if (ev.status === "error") {
        console.warn("[player] native playback error:", ev.error?.message);
        // Beacon BEFORE the state flip, same reporter as web and mobile: a
        // direct-play death is invisible server-side (every byte it served
        // was a 200), console.warn goes nowhere on a living-room TV, and the
        // tester's report is a photo of the error screen.
        let posSec = 0;
        withPlayer((p) => { posSec = Math.round(p.currentTime || 0); });
        reportPlayerEvent({
          type: "native_player_error",
          details: ev.error?.message ?? "",
          fatal: true,
          posSec,
        });
        setNativeError(ev.error?.message ?? "playback error");
      }
    });
    // See lib/resumeNudge.ts: a resume after a long pause seeks in place so
    // the picture leaves with the sound instead of three seconds behind it.
    const nudge = resumeNudger((sec) => withPlayer((p) => p.seekBy(sec)));
    const playSub = player.addListener("playingChange", () => {
      withPlayer((p) => {
        setIsPlaying(p.playing);
        nudge(p.playing);
      });
    });
    return () => {
      endSub.remove();
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player, withPlayer, reportPlayerEvent, setIsBufferingDebounced]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // SyncPlay ("watch together"): TV only ever JOINS a room, landing here via
  // a `?roomId=` param - see app/(auth)/syncplay/index.tsx and
  // src/hooks/useSyncplay.ts for the join flow and the host-authority rule.
  const [syncplayBadgeFocused, setSyncplayBadgeFocused] = useState(false);
  // Every one of these fires from a websocket, i.e. possibly while the player
  // is being torn down - hence withPlayer rather than a plain `player?.`.
  const syncplay = useSyncplay({
    getLocalPositionMs: () => withPlayer((p) => p.currentTime * 1000) ?? 0,
    onPlay: (posMs, shouldSeek) => {
      withPlayer((p) => {
        if (shouldSeek) p.currentTime = posMs / 1000;
        p.play();
      });
    },
    onPause: (posMs, shouldSeek) => {
      withPlayer((p) => {
        if (shouldSeek) p.currentTime = posMs / 1000;
        p.pause();
      });
    },
    onSeek: (posMs) => {
      withPlayer((p) => {
        p.currentTime = posMs / 1000;
      });
    },
  });
  // The hook's object changes with every socket state; connect is a stable
  // useCallback, so the effect depends on it alone.
  const { connect: syncplayConnect } = syncplay;
  const syncplayRoomId = syncplay.state.roomId;
  useEffect(() => {
    if (roomId && !syncplayRoomId) syncplayConnect(roomId);
  }, [roomId, syncplayRoomId, syncplayConnect]);

  // Past 90% the title is watched: the rule every player shares.
  useAutoMarkWatched(mediaId, currentTime, duration);

  // A mark the next launch reads if the system kills this player mid-film
  // (lib/interruptedPlayback.ts): the home then offers to carry on.
  const profileId = useAuthStore((s) => s.profileId);
  useInterruptedPlaybackMarker({
    enabled: !roomId,
    mediaId,
    profileId,
    title: playerTitle(media, series?.title ?? media?.seriesTitle ?? (season ? itemLabel(season, t) : undefined), seasonNumber),
    isPlaying,
    getPositionMs: () => currentTimeRef.current * 1000,
    getPick: () => ({
      ...(userPickedAudioRef.current ? { audio: selectedAudioIndex } : {}),
      ...(userPickedSubtitleRef.current ? { subtitle: selectedSubtitleIndex ?? "off" } : {}),
    }),
  });

  // Controls go away when the room goes quiet - ALWAYS. They used to stay put
  // for as long as the row held the focus, which since BACK became the only
  // way out meant they never left at all: a mask and three buttons parked over
  // the film (owner, 2026-09-19). Every key press restarts this timer, so
  // walking the row keeps them up; a viewer who puts the remote down gets the
  // picture back. The row simply gets longer than a passing wake.
  const barFocusedRef = useRef(false);
  barFocusedRef.current = barFocused;

  // armHideTimer restarts the countdown; `wake` says whether the controls
  // should also be SHOWN. The two are separate on purpose: the hide itself
  // hands the D-pad back to the surface (barFocused goes false), and a version
  // that always showed turned that into a loop - the controls blinked off and
  // straight back on, which is why they never left the picture.
  // The timer is re-armed by every key and by the hooks' activity callbacks:
  // setting the same value again still has React run this whole screen once
  // before it bails out, so the state is only touched when it changes.
  const shownRef = useRef(true);
  const armHideTimer = useCallback((wake: boolean) => {
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    if (wake && !shownRef.current) {
      shownRef.current = true;
      setShowControls(true);
    }
    const delay = barFocusedRef.current ? CONTROLS_FOCUSED_TIMEOUT : CONTROLS_TIMEOUT;
    hideTimerRef.current = setTimeout(() => {
      if (!shownRef.current) return;
      shownRef.current = false;
      setShowControls(false);
      setBarFocused(false);
    }, delay);
  }, []);

  // Controls go away when the room goes quiet - ALWAYS. Every key press
  // restarts the countdown, so walking the row keeps them up; a viewer who
  // puts the remote down gets the picture back. The row simply gets longer
  // than a passing wake.
  const resetHideTimer = useCallback(() => armHideTimer(true), [armHideTimer]);
  // The stores outlive this screen: a new title starts from nothing pending.
  useEffect(() => {
    setSeekPreview(null);
    setPlayhead(0);
    return () => setSeekPreview(null);
  }, []);
  const openSettings = useCallback(() => setShowTrackMenu(true), []);

  // Entering the row shows them and gives them the long delay; leaving only
  // restarts the short one - without re-showing what just went away.
  useEffect(() => {
    armHideTimer(barFocused);
  }, [barFocused, armHideTimer]);

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, [resetHideTimer]);

  const togglePlayPause = useCallback(() => {
    // Through withPlayer like every other native touch: a press can land after
    // the player was released (leaving the screen, switching track) and the
    // raw access throws "cannot be cast to VideoPlayer".
    withPlayer((p) => {
      if (p.playing) {
        p.pause();
        if (syncplay.state.connected) syncplay.sendPause(p.currentTime * 1000);
      } else {
        p.play();
        if (syncplay.state.connected) syncplay.sendPlay(p.currentTime * 1000);
      }
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer, syncplay]);

  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;

  // A seek the viewer builds up with the remote, the same on every television
  // player (useRemoteSeek): presses move a preview the bar and the clock show
  // live, ONE seek lands once the pressing stops, a held key scrubs.
  const {
    bump: bumpSeek,
    beginHold,
    endHold,
    commitNow: commitSeekNow,
    isHolding,
  } = useRemoteSeek({
    positionSec: currentTime,
    getPosition: () => currentTimeRef.current,
    getDuration: () => durationRef.current,
    seekTo: (sec) => withPlayer((p) => { p.currentTime = sec; }),
    onActivity: resetHideTimer,
    // The bar and the seek badge read the preview from the store: held here,
    // every press re-rendered this whole screen.
    onPreviewChange: setSeekPreview,
  });

  // Which seek key is currently down, for the sets that never send a
  // longLeft/longRight and instead repeat the plain key while it is held: a
  // second "down" for the same direction before its "up" is that repeat, and
  // starts the same scrub a long-press event would.
  const seekKeyDownRef = useRef<1 | -1 | null>(null);

  // One reading of a left/right press, shared by the surface and the focused
  // timeline: a tap is a skip that adds up, a hold scrubs. Android sends TWO
  // longLeft/longRight per hold (press and release), with the key action.
  const handleSeekKey = useCallback((type: string, evt: { eventKeyAction?: number | string }) => {
    const dir: 1 | -1 = type === "right" || type === "longRight" ? 1 : -1;
    const long = type === "longLeft" || type === "longRight";
    const action = Number(evt.eventKeyAction);
    const skip = dir > 0 ? skipFwd : skipBwd;
    if (Number.isNaN(action)) {
      // No key action (Apple TV): one event per press.
      const delta = seekDeltaForKey(type, skipBwd, skipFwd);
      if (delta !== 0) bumpSeek(delta);
    } else if (action === KEY_ACTION_DOWN) {
      if (long || seekKeyDownRef.current === dir) beginHold(dir);
      else seekKeyDownRef.current = dir;
    } else if (action === KEY_ACTION_UP) {
      seekKeyDownRef.current = null;
      if (long || isHolding()) endHold(skip);
      else bumpSeek(dir * skip);
    }
  }, [skipFwd, skipBwd, beginHold, endHold, bumpSeek, isHolding]);

  const cycleSpeed = useCallback(() => {
    setPlaybackSpeed((prev) => {
      const next = nextPlaybackSpeed(prev);
      withPlayer((p) => { p.playbackRate = next; });
      return next;
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer]);

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, quality || undefined);
    },
    [player, restartWithTracks, selectedAudioIndex, burnInSubtitleIndex],
  );

  // The file itself opens on its own default track and its URL never changes:
  // the selection is applied to the player (useFileAudioSelection). A change
  // still renegotiates below, the new track may not be one the box decodes.
  useFileAudioSelection(player as unknown as AudioTrackPlayer | null, {
    enabled: streamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY",
    audioStreams,
    selectedAudioIndex,
  });

  const handleAudioSelect = useCallback(
    (index: number) => {
      userPickedAudioRef.current = true;
      setSelectedAudioIndex(index);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(index, burnInSubtitleIndex, selectedQuality || undefined);
    },
    [player, restartWithTracks, burnInSubtitleIndex, selectedQuality, setSelectedAudioIndex, userPickedAudioRef],
  );

  const handleSubtitleSelect = useCallback(
    (index: number | null) => {
      userPickedSubtitleRef.current = true;
      // Text and PGS both render client-side in SubtitleOverlay, so switching
      // between them (or off) needs no manifest change and is instant. Only a
      // track we cannot draw - VobSub, DVB - still costs a restart, because the
      // server has to burn that one into the frames itself.
      const prev = subtitleStreams.find((s) => s.index === selectedSubtitleIndex);
      const next = index === null ? null : subtitleStreams.find((s) => s.index === index);
      if (prev?.burnIn || next?.burnIn) {
        resumeAfterRestartRef.current = player?.playing ?? true;
        restartWithTracks(selectedAudioIndex, next?.burnIn ? index ?? undefined : undefined, selectedQuality || undefined);
      }
      setSelectedSubtitleIndex(index);
    },
    [player, restartWithTracks, selectedAudioIndex, selectedQuality, subtitleStreams, selectedSubtitleIndex, setSelectedSubtitleIndex, userPickedSubtitleRef],
  );

  // What ac3go handed the speakers, reported: a track decoded to silence is
  // otherwise invisible (the picture plays, progress ticks). A box too slow
  // for ac3go gets the same title, same spot, with the audio transcoded.
  useDecoderTelemetry(mediaId, reportPlayerEvent, () => currentTimeRef.current, {
    onTooSlow: () => {
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);
    },
  });

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      // An unclamped segment end past (or too close to) the true duration
      // jumps straight into a black frame or the tail credits scroll instead
      // of the next scene. Only clamp once duration is known - 0 means it
      // hasn't loaded yet, and clamping against that would pin every skip to 0.
      const target = duration > 0 ? Math.min(toSec, Math.max(0, duration - 5)) : toSec;
      withPlayer((p) => { p.currentTime = target; });
    },
    [withPlayer, duration],
  );

  const handleToggleMute = useCallback(() => {
    setMuted((prev) => {
      const next = !prev;
      withPlayer((p) => { p.muted = next; });
      return next;
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer]);

  const handleVolumeSelect = useCallback(
    (pct: number) => {
      setVolumePct(pct);
      setMuted(false);
      withPlayer((p) => {
        p.volume = pct / 100;
        p.muted = false;
      });
      resetHideTimer();
    },
    [withPlayer, resetHideTimer],
  );

  // Same exit contract as the web/mobile player's back button (playerExit):
  // pop the player's own history entry when there is one - safe because episode
  // changes REPLACE - otherwise (deep link, device "loadAndPlay" command)
  // REPLACE toward the media page (an episode exits to its season) so no
  // player entry is ever left behind for a later back press to walk into.
  const handleBack = useCallback(() => {
    stopSession();
    const exit = playerExit(router.canGoBack(), mediaId, media);
    if (exit.action === "back") router.back();
    else router.replace(`/media/${exit.mediaId}` as never);
  }, [stopSession, mediaId, media]);

  const handleNavigateEpisode = useCallback(
    (episodeId: string) => {
      stopSession();
      router.replace(`/(auth)/player/${episodeId}` as never);
    },
    [stopSession],
  );

  // Cast Connect: project THIS player's state onto the MediaSession, and let
  // the television's remote and every connected sender act on it. The player
  // above stays the source of truth - the bridge only reports it, and routes
  // commands back into the same handlers a keypress uses.
  useCastConnectBridge({
    // Same title the top bar shows: the show's name leads for an episode.
    title: playerTitle(media, series?.title ?? media?.seriesTitle ?? (season ? itemLabel(season, t) : undefined), seasonNumber),
    currentTime,
    duration,
    isPlaying,
    speed: playbackSpeed,
    onPlay: () => withPlayer((p) => p.play()),
    onPause: () => withPlayer((p) => p.pause()),
    onStop: handleBack,
    onSeekTo: (sec) => withPlayer((p) => { p.currentTime = Math.max(0, duration > 0 ? Math.min(sec, duration) : sec); }),
  });

  // The next-episode auto-advance (and its "play now" button) jump straight
  // into another player instance, bypassing the media detail screen where
  // the parental PIN gate normally lives. useNextEpisode calls onNavigate
  // both on its own countdown and from playNow, so gating that single
  // callback covers both paths. onNavigate is wired through a ref so the
  // gate (which needs nextEp.nextEpisode's rating, only known once
  // Entry gate: the player is reachable WITHOUT going through the media detail
  // screen - the home hero, a deep link, and the remote "loadAndPlay" device
  // command all land straight here. Gating the screen itself (rather than each
  // caller) is the invariant that holds for any future entry point. The server
  // refuses the stream regardless; this turns a dead error into a PIN prompt.
  const { needsPin: entryNeedsPin, profileId: entryGateProfileId } = useParentalGate(media?.officialRating);
  const [entryUnlocked, setEntryUnlocked] = useState(false);
  const entryGated = !!media && !entryUnlocked && entryNeedsPin();

  useEffect(() => {
    if (entryGated) withPlayer((p) => p.pause());
  }, [entryGated, withPlayer]);

  const handleEntryPinVerified = useCallback(() => {
    setEntryUnlocked(true);
    withPlayer((p) => p.play());
  }, [withPlayer]);

  // useNextEpisode itself has resolved it) can be defined after the hook
  // call without creating a circular dependency.
  const nextEpNavigateRef = useRef<(episodeId: string) => void>(() => {});
  const forwardNextEpNavigate = useCallback((episodeId: string) => {
    nextEpNavigateRef.current(episodeId);
  }, []);

  // Nobody answered "still watching?": the session ends and the screen goes
  // black (PlayerSleepScreen); the first key leaves for the media page.
  const [asleep, setAsleep] = useState(false);
  const wokeRef = useRef(false);
  const fallAsleep = useCallback(() => {
    stopSession();
    setAsleep(true);
  }, [stopSession]);
  const wake = useCallback(() => {
    // OK fires the target's onPress and the screen's "select" event both.
    if (wokeRef.current) return;
    wokeRef.current = true;
    handleBack();
  }, [handleBack]);

  const nextEp = useNextEpisode({
    media,
    currentTime,
    duration,
    onNavigate: forwardNextEpNavigate,
    autoAdvance: autoplayNext,
    onStillWatchingTimeout: fallAsleep,
  });

  const { needsPin: nextEpNeedsPin, profileId: nextEpGateProfileId } = useParentalGate(
    nextEp.nextEpisode?.officialRating,
  );
  const [showNextEpPinModal, setShowNextEpPinModal] = useState(false);
  const [pendingNextEpisodeId, setPendingNextEpisodeId] = useState<string | null>(null);

  const gatedNavigateEpisode = useCallback(
    (episodeId: string) => {
      if (nextEpNeedsPin()) {
        setPendingNextEpisodeId(episodeId);
        setShowNextEpPinModal(true);
        return;
      }
      handleNavigateEpisode(episodeId);
    },
    [nextEpNeedsPin, handleNavigateEpisode],
  );
  nextEpNavigateRef.current = gatedNavigateEpisode;

  const handleNextEpPinVerified = useCallback(() => {
    setShowNextEpPinModal(false);
    if (pendingNextEpisodeId) {
      handleNavigateEpisode(pendingNextEpisodeId);
      setPendingNextEpisodeId(null);
    }
  }, [pendingNextEpisodeId, handleNavigateEpisode]);

  // Manual "next episode" control (item 2): same gate as the auto-advance,
  // triggered on demand instead of by the countdown.
  const handleManualNextEpisode = useCallback(() => {
    if (nextEp.nextEpisode?.id) gatedNavigateEpisode(nextEp.nextEpisode.id);
  }, [nextEp.nextEpisode, gatedNavigateEpisode]);

  // Manual "previous episode" control: never driven by the auto-advance
  // countdown, so it is gated independently on the PREVIOUS episode's own
  // rating rather than reusing the next-episode gate above.
  const { needsPin: prevEpNeedsPin, profileId: prevEpGateProfileId } = useParentalGate(
    nextEp.previousEpisode?.officialRating,
  );
  const [showPrevEpPinModal, setShowPrevEpPinModal] = useState(false);

  const handlePreviousEpisode = useCallback(() => {
    if (!nextEp.previousEpisode?.id) return;
    if (prevEpNeedsPin()) {
      setShowPrevEpPinModal(true);
      return;
    }
    handleNavigateEpisode(nextEp.previousEpisode.id);
  }, [nextEp.previousEpisode, prevEpNeedsPin, handleNavigateEpisode]);

  const handlePrevEpPinVerified = useCallback(() => {
    setShowPrevEpPinModal(false);
    if (nextEp.previousEpisode?.id) handleNavigateEpisode(nextEp.previousEpisode.id);
  }, [nextEp.previousEpisode, handleNavigateEpisode]);

  const sleepTimer = useSleepTimer({ onMinutesFire: () => withPlayer((p) => p.pause()) });
  // Both hooks return a fresh object every render; their cancel callbacks are
  // stable, so effects depend on those instead of the whole object.
  const { cancel: cancelSleepTimer } = sleepTimer;
  const { cancel: cancelNextEp } = nextEp;

  // A film, or an episode with nothing after it: leave for its page once it has
  // played to the end, as the web and browser TV players do, instead of holding
  // the last frame. With a next episode, pin the clock on the end so the up-next
  // countdown starts even when the last tick stopped short of the duration.
  playedToEndRef.current = () => {
    if (!nextEp.nextEpisode) {
      handleBack();
      return;
    }
    currentTimeRef.current = duration;
    committedTimeRef.current = duration;
    setPlayhead(duration);
    setCurrentTime(duration);
  };

  // "End of episode" sleep mode has no countdown of its own - it rides the same
  // "ended" signal the next-episode auto-advance uses. Cancelling the
  // auto-advance too stops it from starting the next item right after pausing.
  useEffect(() => {
    if (nextEp.ended && sleepTimer.state?.kind === "endOfEpisode") {
      withPlayer((p) => p.pause());
      cancelSleepTimer();
      cancelNextEp();
    }
  }, [nextEp.ended, sleepTimer.state?.kind, withPlayer, cancelSleepTimer, cancelNextEp]);

  // The 30s "up next" card only informs - the D-pad still seeks and pauses
  // right through it (item B4). Only the real end-of-episode countdown
  // (nextEp.ended) claims the same exclusive ownership as the other overlays
  // below, the instant before the auto-advance actually fires.
  // Every panel the settings menu opens. They are its children: BACK out of
  // one returns to the menu, never to the film (owner, 2026-09-19).
  const anySubPanelOpen =
    showQualityPicker || showAudioPicker || showSubtitlePicker || showSubtitleStyle ||
    showChaptersPicker || showSleepTimerPicker || showVolumePicker || showStreamInfoPanel;

  const closeSubPanels = useCallback(() => {
    setShowQualityPicker(false);
    setShowAudioPicker(false);
    setShowSubtitlePicker(false);
    setShowChaptersPicker(false);
    setShowSleepTimerPicker(false);
    setShowVolumePicker(false);
    setShowStreamInfoPanel(false);
    setShowSubtitleStyle(false);
  }, []);

  const closeAllOverlays = useCallback(() => {
    closeSubPanels();
    setShowTrackMenu(false);
  }, [closeSubPanels]);

  // The end-of-episode countdown owns the D-pad only while its card is ON
  // SCREEN. `ended` alone is not enough: on the last episode of a season
  // there is no next episode, the card renders nothing, and gating on
  // `ended` left every key - BACK included - swallowed by an overlay nobody
  // could see. The owner's television froze on the credits (2026-09-20).
  const endedOverlayOpen = nextEp.ended && nextEp.visible;
  const anyOverlayOpen =
    showQualityPicker || showTrackMenu || showAudioPicker || showSubtitlePicker ||
    showChaptersPicker || showSleepTimerPicker || showVolumePicker || showStreamInfoPanel ||
    showSubtitleStyle ||
    endedOverlayOpen;

  // D-pad remote handler. The callback param is annotated explicitly: the
  // react-native-tvos alias resolves per-app, and the hoisted classic RN
  // types (shared workspace) leave HWEvent unresolvable in some installs -
  // a structural annotation keeps tsc deterministic everywhere.
  //
  // Full remote map (every plain direction plus its long-press variant is
  // claimed, so anything else lives behind the longSelect options menu):
  //   select/playPause  -> toggle play/pause
  //   longSelect        -> options menu (audio/subtitles/chapters/volume/sleep/info)
  //   left/right        -> skip back/forward
  //   longLeft/longRight-> accelerated seek (native TV "hold" events)
  //   up / longUp       -> cycle speed / previous episode
  //   down / longDown   -> quality picker / next episode
  //   menu/back         -> exit (playerExit contract)
  // The subscription must be STABLE, the logic must be FRESH.
  //
  // react-native's useTVEventHandler keys its effect on the handler's identity
  // (`useEffect(..., [handleEvent])`), so an inline arrow tears the native
  // listener down and re-adds it on every render - and this screen re-renders
  // about twice a second while playing (timeUpdateEventInterval = 0.5). A D-pad
  // press landing in that remove/re-add gap is silently dropped, which reads as
  // "the remote didn't respond". Same family as the syncplay churn fixed in
  // 3df3e8afb, but this one is the subscription itself.
  //
  // The body below is left EXACTLY as it was and simply re-assigned each render:
  // that keeps every value it closes over current without having to enumerate
  // them as dependencies, which is where a stale-closure bug would creep in on
  // a screen this stateful.
  const remoteHandlerRef = useRef<(evt: { eventType: string } | undefined) => void>(() => {});
  remoteHandlerRef.current = (evt: { eventType: string } | undefined) => {
    if (!evt) return;
    const type = evt.eventType;
    if (asleep) {
      if (REMOTE_KEYS.has(type)) wake();
      return;
    }
    // A key on the remote is the viewer saying they are still here (the
    // "still watching?" count, stillWatching.ts). Focus and blur are not: a
    // card that takes focus as it appears emits them with nobody there.
    if (REMOTE_KEYS.has(type)) noteViewerActivity();

    // While a picker/menu is showing, or the end-of-episode auto-advance
    // countdown is actually running, its own FocusableCard rows own the
    // D-pad via the native TV focus engine - if we kept interpreting the same
    // key here (e.g. "select") it would double-fire (native onPress AND
    // togglePlayPause()).
    if (anyOverlayOpen) return;
    // The Skip button holds TV focus while visible, but "select" must only be
    // swallowed while it is the one that ACTUALLY has it - a `skipVisible`-only
    // check swallowed OK even when focus had moved elsewhere, muting the key
    // for no reason (the double-fire this guards against needs the button's
    // own onPress to be the one about to fire).
    if (skipFocused && type === "select") return;
    if (syncplayBadgeFocused && type === "select") return;
    // OK on the focused surface already fires its native onPress - handling
    // "select" here too would double-toggle play/pause.
    if (surfaceFocused && type === "select") return;
    // The credits countdown is running: BACK is the refusal, not the exit.
    if (cancelCountdownRef.current && (type === "menu" || type === "back")) {
      cancelCountdownRef.current();
      return;
    }
    // The 30s "up next" card is on screen but hasn't claimed the D-pad (see
    // anyOverlayOpen above) - back/menu dismisses IT instead of exiting the
    // player, since the viewer is a keypress from losing their place otherwise.
    if (nextEp.visible && (type === "menu" || type === "back")) {
      nextEp.cancel();
      return;
    }
    // The episode is over and there is nothing to offer next: BACK, MENU or OK
    // leave the player right away instead of first waking controls over a
    // frozen last frame.
    if (nextEp.ended && !nextEp.visible && (type === "menu" || type === "back" || type === "select")) {
      handleBack();
      return;
    }
    // While the action bar holds focus its own buttons own the D-pad: left and
    // right walk the row, OK activates. Interpreting the same keys here would
    // seek behind the viewer's back while they are picking an audio track.
    if (barFocused) {
      // BACK leaves the row - and only BACK. "up" used to leave it too, which
      // meant walking from the settings icon back up to the play button
      // folded the whole overlay away mid-gesture (measured on the box,
      // 2026-09-19): the row stopped being focusable under the viewer's
      // thumb. Inside the row the native focus engine owns every direction.
      if (type === "menu" || type === "back") {
        setBarFocused(false);
        resetHideTimer();
        return;
      }
      // On the timeline, left/right are the playhead's: the control keeps
      // the native focus on itself (its neighbours point back at it) and the
      // same gesture as on the surface moves the preview.
      if (timelineFocused && (type === "left" || type === "right" || type === "longLeft" || type === "longRight")) {
        handleSeekKey(type, evt as { eventKeyAction?: number | string });
        return;
      }
      // Any other key: the viewer is driving the row, keep it alive.
      resetHideTimer();
      return;
    }

    if (type === "select" || type === "playPause") {
      togglePlayPause();
    } else if (type === "left" || type === "right" || type === "longLeft" || type === "longRight") {
      handleSeekKey(type, evt as { eventKeyAction?: number | string });
    } else if (type === "down") {
      // The one way into the options, and the bar says so on screen. Every
      // other binding this screen used to carry (long-OK for the menu, up for
      // speed, long-up/long-down for episodes) was invisible: the owner could
      // not work out how to drive the player at all. They live in the bar now.
      // One press, hidden controls or not: a first DOWN that only woke them
      // left the owner on a bar they could see and not reach (2026-10-24).
      if (!showControls) resetHideTimer();
      setBarFocused(true);
    } else if (type === "menu" || type === "back") {
      if (showControls) handleBack();
      else resetHideTimer();
    } else {
      // Any other key just wakes the controls rather than doing something the
      // viewer cannot predict.
      resetHideTimer();
    }
  };
  useTVEventHandler(useCallback((evt: { eventType: string } | undefined) => remoteHandlerRef.current?.(evt), []));

  useEffect(() => {
    if (!showControls) return;
    const t = setTimeout(() => {
      const next = {
        rewind: findNodeHandle(rewindButtonRef.current) ?? undefined,
        play: findNodeHandle(playButtonRef.current) ?? undefined,
        forward: findNodeHandle(forwardButtonRef.current) ?? undefined,
        settings: findNodeHandle(settingsButtonRef.current) ?? undefined,
        speed: findNodeHandle(speedButtonRef.current) ?? undefined,
        timeline: findNodeHandle(timelineRef.current) ?? undefined,
      };
      setFocusTags((prev) =>
        prev.rewind === next.rewind && prev.play === next.play &&
        prev.forward === next.forward && prev.settings === next.settings &&
        prev.speed === next.speed && prev.timeline === next.timeline ? prev : next,
      );
    }, 60);
    return () => clearTimeout(t);
  }, [showControls]);

  // The row is mounted all along and only becomes focusable when it opens, so
  // nothing moves the D-pad onto it by itself.
  useEffect(() => {
    if (!barFocused) {
      setTimelineFocused(false);
      return;
    }
    const t = setTimeout(
      () => (playButtonRef.current as unknown as { requestTVFocus?: () => void })?.requestTVFocus?.(),
      50,
    );
    return () => clearTimeout(t);
  }, [barFocused]);

  useEffect(() => {
    let alive = true;
    void getSubtitleStylePref().then((v) => {
      if (alive && v) setSubtitleStyle(v);
    });
    return () => { alive = false; };
  }, []);

  const handleSubtitleStyleChange = useCallback((next: SubtitleStyle) => {
    setSubtitleStyle(next);
    void saveSubtitleStylePref(next);
  }, []);

  // BACK belongs to whatever is on top. A submenu is an RN Modal, so Android
  // hands it onRequestClose - and then delivers the SAME press to the screen
  // behind, which popped the player and dumped the viewer back on the media
  // page (owner, 2026-09-19). Claiming it here consumes it: the top layer
  // closes and the film keeps playing.
  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      if (asleep) {
        wake();
        return true;
      }
      if (cancelCountdownRef.current) {
        cancelCountdownRef.current();
        return true;
      }
      // One step back up the menu, not out of it: a panel returns to the
      // settings menu that opened it, and only the menu itself returns to the
      // film.
      if (anySubPanelOpen) {
        closeSubPanels();
        setShowTrackMenu(true);
        return true;
      }
      if (endedOverlayOpen) {
        cancelNextEp(); // closeAllOverlays cannot dismiss this one
        return true;
      }
      if (anyOverlayOpen) {
        closeAllOverlays();
        return true;
      }
      if (barFocused) {
        setBarFocused(false);
        resetHideTimer();
        return true;
      }
      return false; // nothing open: the player's own exit contract runs
    });
    return () => sub.remove();
  }, [asleep, wake, anySubPanelOpen, anyOverlayOpen, endedOverlayOpen, cancelNextEp, barFocused, closeSubPanels, closeAllOverlays, resetHideTimer]);

  // Imperative focus reclaim: whenever no overlay/bar should own the D-pad,
  // force native focus back onto the surface - if no native view is focused
  // Android stops delivering key events and the player goes deaf. Missing
  // skipVisible/the PIN modals here (B6) meant the surface never reclaimed
  // focus once the Skip button (or a PIN prompt) disappeared, leaving the
  // remote dead until some OTHER overlay happened to toggle anyOverlayOpen.
  useEffect(() => {
    if (barFocused || anyOverlayOpen || skipVisible || entryGated || showNextEpPinModal || showPrevEpPinModal) return;
    const t = setTimeout(
      () => (surfaceRef.current as unknown as { requestTVFocus?: () => void })?.requestTVFocus?.(),
      100,
    );
    return () => clearTimeout(t);
  }, [barFocused, anyOverlayOpen, skipVisible, entryGated, showNextEpPinModal, showPrevEpPinModal]);

  // Called from useProgressReporter's CLEANUP, which runs while the screen is
  // being popped and the native player may already be released. Reading it raw
  // threw ERR_USING_RELEASED_SHARED_OBJECT from an effect cleanup, and React
  // reported that against this component - the exit turned into an error
  // screen (device, 2026-08-03). Its neighbour below already learned this.
  const getPositionMs = useCallback(() => {
    return (withPlayer((p) => p.currentTime) ?? 0) * 1000;
  }, [withPlayer]);

  const getIsPaused = useCallback(() => {
    try {
      return !(player?.playing ?? false);
    } catch {
      return true;
    }
  }, [player]);

  // volumePct/muted already track the player's own state (see
  // handleVolumeSelect/handleToggleMute) - reporting them was previously
  // skipped entirely, so the "now playing" surface (and a remote command
  // reacting to it) never knew this TV had turned its volume down or muted.
  const getVolume = useCallback(() => volumePct / 100, [volumePct]);
  const getIsMuted = useCallback(() => muted, [muted]);

  const remoteCallbacks = useMemo(
    () => ({
      onRemotePause: () => {
        try {
          player?.pause();
        } catch (err) {
          console.warn("remote pause failed:", err);
        }
      },
      onRemotePlay: () => {
        try {
          player?.play();
        } catch (err) {
          console.warn("remote play failed:", err);
        }
      },
      onRemoteStop: () => {
        handleBack();
      },
    }),
    [player, handleBack],
  );

  useProgressReporter(sessionId, getPositionMs, getIsPaused, getVolume, getIsMuted, remoteCallbacks);

  // Server-side load shed (HTTP 503): distinct from isError below - nothing is
  // broken, the server just isn't ready to serve THIS request yet, for one of
  // two unrelated reasons (see usePlaySession's ShedReason doc). Calm clock
  // icon (not a red error) and copy that names neither "transcoding" nor any
  // other internal term.
  if (asleep) return <PlayerSleepScreen onWake={wake} />;

  if (shedReason) {
    return (
      <PlayerStatusScreen
        icon="time-outline"
        title={t(shedI18nKey(shedReason, "title"))}
        body={t(shedI18nKey(shedReason, "body"))}
        actionLabel={t("player.retry")}
        onAction={() => {
          // Retrying after a shed means the user wants playback back - always autoplay.
          resumeAfterRestartRef.current = true;
          restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);
        }}
        onBack={handleBack}
      />
    );
  }

  // An expired session is not a broken stream. Retrying it can only fail
  // again, so the screen offers the one action that helps: sign in.
  if (isAuthError) {
    return (
      <PlayerStatusScreen
        icon="log-in-outline"
        title={t("player.sessionExpired.title", "Your session has expired")}
        body={t("player.sessionExpired.body", "Sign in again to keep watching.")}
        actionLabel={t("auth.login.signIn")}
        onAction={() => router.replace("/login")}
        onBack={handleBack}
      />
    );
  }

  if (isError || nativeError) {
    return (
      <PlayerStatusScreen
        title={t("player.failedToLoadStream", "Failed to load stream")}
        actionLabel={t("player.retry")}
        onAction={() => {
          // A fatal native error otherwise stays glued to the screen even if
          // the retry succeeds - see the nativeError reset effect above for
          // the twin bug (a stale error surviving a media CHANGE).
          setNativeError(null);
          resumeAfterRestartRef.current = true;
          restartWithTracks(selectedAudioIndex, burnInSubtitleIndex, selectedQuality || undefined);
        }}
        onBack={handleBack}
      />
    );
  }

  if (isLoading || !streamUrl) return <PlayerLoadingScreen />;
  // A stream URL means a source, hence a player; this only tells the type
  // system so, and covers the commit in which the source just arrived.
  if (!player) return <PlayerLoadingScreen />;

  // The preview position (set the instant a seek key fires) takes over the
  // display until the real playhead catches up - see playerSeek.ts.

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <VideoView
        player={player}
        style={[styles.video, { width: screenWidth, height: screenHeight }]}
        nativeControls={false}
        contentFit="contain"
      />

      {/* Client-side subtitle rendering, which the TV had none of: expo-video
          exposes track SELECTION only, so a text track sent to the server used
          to answer a sidecar nobody fetched and simply never appeared, and a
          bitmap one was burnt into the video at the cost of the copy rung.
          Sits under the controls (drawn before them in the tree). */}
      <SubtitleOverlay
        mediaId={mediaId}
        subtitleIndex={clientSubtitleIndex}
      />

      {isBuffering && (
        <View style={styles.bufferingOverlay} pointerEvents="none">
          <ActivityIndicator size="large" color={theme.colors.accent} />
        </View>
      )}

      {/* Focus holder and pointer surface, kept under every overlay. */}
      <Pressable
        ref={surfaceRef}
        style={StyleSheet.absoluteFill}
        focusable={true}
        // The Skip button also asks for preferred focus while visible (item
        // B5) - both requesting it on the same mount is the focus fight that
        // let the surface silently steal OK away from the button.
        hasTVPreferredFocus={!barFocused && !anyOverlayOpen && !skipVisible}
        onFocus={() => setSurfaceFocused(true)}
        onBlur={() => setSurfaceFocused(false)}
        onPress={() => {
          if (!showControls) {
            resetHideTimer();
            return;
          }
          togglePlayPause();
        }}
        accessible={true}
        accessibilityLabel={t("player.controls.surface", "Player surface")}
      />

      <QualityPicker
        visible={showQualityPicker}
        currentQuality={selectedQuality}
        onSelect={handleQualitySelect}
        onClose={() => setShowQualityPicker(false)}
      />

      <TrackOptionsMenu
        visible={showTrackMenu}
        onClose={() => setShowTrackMenu(false)}
        // Audio and subtitles live in here now: the control bar that used to
        // carry their own buttons is gone, replaced by the settings icon on
        // the timeline (owner, 2026-09-19).
        hasAudioOptions={audioStreams.length > 1}
        onOpenAudio={() => {
          setShowTrackMenu(false);
          setShowAudioPicker(true);
        }}
        hasSubtitleOptions={subtitleStreams.length > 0}
        onOpenSubtitles={() => {
          setShowTrackMenu(false);
          setShowSubtitlePicker(true);
        }}
        onOpenSubtitleStyle={() => {
          setShowTrackMenu(false);
          setShowSubtitleStyle(true);
        }}
        onOpenQuality={() => {
          setShowTrackMenu(false);
          setShowQualityPicker(true);
        }}
        qualityLabel={selectedQuality > 0 ? `${selectedQuality}p` : t("player.settings.auto")}
        onCycleSpeed={cycleSpeed}
        speedLabel={`${playbackSpeed}x`}
        onPreviousEpisode={nextEp.previousEpisode ? () => {
          setShowTrackMenu(false);
          handlePreviousEpisode();
        } : undefined}
        onNextEpisode={nextEp.nextEpisode ? () => {
          setShowTrackMenu(false);
          handleManualNextEpisode();
        } : undefined}
        autoplayNext={autoplayNext}
        onToggleAutoplayNext={handleToggleAutoplayNext}
        hasChapters={chapters.length > 0}
        currentChapterTitle={chapters[findCurrentChapterIndex(chapters, currentTime * 1000)]?.title}
        onOpenChapters={() => {
          setShowTrackMenu(false);
          setShowChaptersPicker(true);
        }}
        onOpenVolume={() => {
          setShowTrackMenu(false);
          setShowVolumePicker(true);
        }}
        onOpenSleepTimer={() => {
          setShowTrackMenu(false);
          setShowSleepTimerPicker(true);
        }}
        hasStreamInfo={!!streamInfo}
        onOpenStreamInfo={() => {
          setShowTrackMenu(false);
          setShowStreamInfoPanel(true);
        }}
      />

      <AudioTrackPicker
        visible={showAudioPicker}
        streams={audioStreams}
        currentIndex={selectedAudioIndex}
        onSelect={handleAudioSelect}
        onClose={() => setShowAudioPicker(false)}
      />

      <SubtitlePicker
        visible={showSubtitlePicker}
        streams={subtitleStreams}
        currentIndex={selectedSubtitleIndex}
        onSelect={handleSubtitleSelect}
        onClose={() => setShowSubtitlePicker(false)}
      />

      <SubtitleStylePicker
        visible={showSubtitleStyle}
        style={subtitleStyle}
        onChange={handleSubtitleStyleChange}
        onClose={() => setShowSubtitleStyle(false)}
      />

      <ChaptersPicker
        visible={showChaptersPicker}
        onClose={() => setShowChaptersPicker(false)}
        chapters={chapters}
        currentTimeMs={currentTime * 1000}
        onSeek={(sec) => withPlayer((p) => { p.currentTime = sec; })}
      />

      <VolumePicker
        visible={showVolumePicker}
        onClose={() => setShowVolumePicker(false)}
        volumePct={volumePct}
        muted={muted}
        onSelect={handleVolumeSelect}
        onToggleMute={handleToggleMute}
      />

      <SleepTimerPicker
        visible={showSleepTimerPicker}
        onClose={() => setShowSleepTimerPicker(false)}
        state={sleepTimer.state}
        onPickMinutes={sleepTimer.setMinutes}
        onPickEndOfEpisode={sleepTimer.setEndOfEpisode}
        onCancel={sleepTimer.cancel}
      />

      <StreamInfoPanel
        visible={showStreamInfoPanel}
        onClose={() => setShowStreamInfoPanel(false)}
        streamInfo={streamInfo ?? null}
        audioStreamIndex={selectedAudioIndex}
        served={servedVariant(streamInfo, playingTrack.playing, playingTrack.ladderBitrates)}
      />

      <SkipIntroButton
        mediaId={mediaId}
        filePath={media?.filePath}
        currentTime={currentTime}
        onSkip={handleSkipIntro}
        onVisibleChange={setSkipVisible}
        onFocusChange={setSkipFocused}
        liftForNextEpisode={nextEp.visible}
        onNextEpisode={nextEp.nextEpisode ? nextEp.playNow : undefined}
        autoAdvance={autoplayNext}
        cancelCountdownRef={cancelCountdownRef}
      />

      <NextEpisodeOverlay
        visible={nextEp.visible}
        ended={nextEp.ended}
        countdown={nextEp.countdown}
        stillWatching={nextEp.stillWatching}
        nextEpisode={nextEp.nextEpisode}
        onPlayNow={nextEp.playNow}
        onCancel={nextEp.cancel}
      />

      {entryGateProfileId ? (
        <ParentalPINModal
          visible={entryGated}
          onClose={() => router.back()}
          onVerified={handleEntryPinVerified}
          profileId={entryGateProfileId}
        />
      ) : null}

      {nextEpGateProfileId ? (
        <ParentalPINModal
          visible={showNextEpPinModal}
          onClose={() => { setShowNextEpPinModal(false); setPendingNextEpisodeId(null); }}
          onVerified={handleNextEpPinVerified}
          profileId={nextEpGateProfileId}
        />
      ) : null}

      {prevEpGateProfileId ? (
        <ParentalPINModal
          visible={showPrevEpPinModal}
          onClose={() => setShowPrevEpPinModal(false)}
          onVerified={handlePrevEpPinVerified}
          profileId={prevEpGateProfileId}
        />
      ) : null}

      {showControls && (
        <View style={styles.overlay} pointerEvents="box-none">
          {/* Top bar: what is playing, and the badges that are state rather
              than controls. */}
          <View style={styles.topBar}>
            <PlayerScrim edge="top" />
            <Text style={styles.mediaTitle} numberOfLines={1}>
              {playerTitle(media, series?.title ?? media?.seriesTitle ?? (season ? itemLabel(season, t) : undefined), seasonNumber)}
            </Text>
            <View style={styles.badgeRow}>
              <SyncplaySessionBadge
                connected={syncplay.state.connected}
                onFocusChange={setSyncplayBadgeFocused}
                onPress={() => {
                  if (syncplay.state.connected) void syncplay.leaveRoom();
                  else router.push("/(auth)/syncplay");
                }}
              />
              {selectedQuality > 0 && (
                <View style={styles.speedBadge}>
                  <Text style={styles.speedText}>{selectedQuality}p</Text>
                </View>
              )}
            </View>
          </View>

          <PlayerBottomPanel
            mediaId={mediaId}
            screenWidth={screenWidth}
            duration={duration}
            isPlaying={isPlaying}
            playbackSpeed={playbackSpeed}
            skipFwd={skipFwd}
            skipBwd={skipBwd}
            chapters={chapters}
            barFocused={barFocused}
            focusTags={focusTags}
            refs={barRefs}
            timelineFocused={timelineFocused}
            onTimelineFocusChange={setTimelineFocused}
            onSeekBy={bumpSeek}
            onTogglePlayPause={togglePlayPause}
            onCycleSpeed={cycleSpeed}
            onOpenSettings={openSettings}
            onCommitSeek={commitSeekNow}
            onActivity={resetHideTimer}
          />
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  video: {},
  bufferingOverlay: {
    ...StyleSheet.absoluteFillObject,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.15)",
  },
  // No veil over the middle of the frame. A flat 40% black across the whole
  // picture is what made the controls feel like a mask sitting on the film
  // (owner, 2026-09-19); the title and the timeline carry their own fades
  // (PlayerScrim) instead, and the image between them stays the image.
  overlay: {
    ...StyleSheet.absoluteFillObject,
    justifyContent: "space-between",
  },
  topBar: {
    paddingTop: 32,
    paddingBottom: 72,
    paddingHorizontal: 40,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  mediaTitle: {
    color: "#fff",
    fontSize: FONT_SIZE["2xl"],
    fontWeight: "700",
    flex: 1,
  },
  badgeRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    marginLeft: 16,
  },
  speedBadge: {
    backgroundColor: "rgba(255,255,255,0.15)",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.25)",
    paddingHorizontal: 10,
    paddingVertical: 3,
    borderRadius: 6,
  },
  speedText: {
    color: "#fff",
    fontSize: FONT_SIZE.sm,
    fontWeight: "700",
  },
});
