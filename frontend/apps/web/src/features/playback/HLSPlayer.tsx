import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuthStore, toast } from "@evey/store";
import { isClientDrawableSubtitle } from "@evey/api";
import { Users } from "lucide-react";
import Hls from "hls.js";
import type { V1StreamInfo } from "@evey/api";
import { PGSSubtitleOverlay } from "./PGSSubtitleOverlay";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
import { nominalQualityTier } from "./playerUtils";
import { ProgressReporter } from "./ProgressReporter";
import { AutoMarkWatched } from "./AutoMarkWatched";
import { ICON } from "@/constants/iconSizes";
import { createRenderStallWatch, formatRenderWindow, formatCadence, bufferedAheadSec, decideRenderCap, type RenderCapState } from "./renderStall";
import { decideNetworkStep, formatNetworkStep, newNetworkCapState, recordStall, recordThroughput, shouldHandoffToTranscode, type NetworkCapState } from "./networkCap";
import { registerRenderStats, playbackSnapshot } from "./playbackSnapshot";
import { NextEpisodeOverlay } from "./NextEpisodeOverlay";
import { SkipIntroButton } from "./SkipIntroButton";
import { KeyboardShortcutsOverlay } from "./KeyboardShortcutsOverlay";
import { GestureFeedback, AdminMessageBanner, BufferingIndicator, FatalErrorOverlay, ShedOverlay } from "./PlayerOverlays";
import type { Chapter } from "@/hooks/useChapters";
import { usePlaybackSettings } from "@/hooks/usePlaybackSettings";
import { useSleepTimer } from "@/hooks/useSleepTimer";
import {
  getPreferredAudioLang,
  setPreferredAudioLang,
  getPreferredSubtitleLang,
  setPreferredSubtitleLang,
  getSeriesTrackPrefs,
  setSeriesTrackPrefs,
  getSubtitleStyle,
  setSubtitleStyle,
  type SubtitleStyle,
} from "@/hooks/usePlaybackPrefs";
import { eveyFetch, createPlayerEventReporter } from "@evey/api";
import { buildTranscodeHlsUrl, castVideoPreload, getSelectedBurnInTrack, pickCastTapPath, pickPinnedLevel, pickPlayOnFallback, pickTranscodeAudioSwitchPath, planCastLoad, planRendererLoad, resolveDefaultAudio, resolveDefaultSubtitle } from "./hooks/hlsPlayerUtils";
import { castProfileFor, castStreamInfoParams } from "@evey/media-caps/cast";
import { userIdFromAccessToken } from "@evey/media-caps/launch";
import { playbackServiceGetStreamInfo } from "@evey/api";
import type { CastTarget } from "@/hooks/useCasting";
import { useDirectPlayback } from "./hooks/useDirectPlayback";
import { useTranscodePlayback, type ShedReason } from "./hooks/useTranscodePlayback";
import { useWasmAudioPlayback } from "./wasmaudio/useWasmAudioPlayback";
import { CONTROLS_FADE_MS, fadeClasses, useFadeParked } from "./hooks/useFadeParked";
import { canWasmDecodeAC3, isWasmDecodableAudioCodec } from "@/utils/codec";
import { useSubtitleManager } from "./hooks/useSubtitleManager";
import { nudgeOnResume } from "./hooks/resumeNudge";
import { usePlayerKeyboard } from "./hooks/usePlayerKeyboard";
import { useTouchGestures } from "./hooks/useTouchGestures";
import { useOverlayTime } from "./hooks/useOverlayTime";
import { useCasting } from "@/hooks/useCasting";
import { useRemotePlayback } from "@/hooks/useRemotePlayback";
// Admin message on screen: the same 12 s as the other players and the cast receiver.
import { ADMIN_MESSAGE_MS, servedVariant, usePlayerProfilePrefs } from "@evey/client";


export type { AudioStream, SubtitleStream };

// How long playback must be CONTINUOUSLY stalled before the buffering overlay
// appears. Micro-stalls (segment boundaries, fast seeks, rendition switches)
// resolve well under this and must not flash a spinner over a playing video;
// genuine stalls (cold transcode start on a weak host, network loss) exceed it
// and still get user feedback.
const BUFFERING_SPINNER_DELAY_MS = 1500;

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
  mediaType?: string;
  streamToken: string;
  onStreamTokenRefresh: () => Promise<string>;
  // Mints a long-lived token for Chromecast playback (the device can't
  // re-validate the short sliding token mid-stream). Optional - without
  // it the cast falls back to the regular session token.
  onCastTokenRequest?: () => Promise<string>;
  startPositionMs?: number;
  knownDurationMs?: number;
  filePath?: string;
  onEnded?: () => void;
  audioStreams?: AudioStream[];
  subtitleStreams?: SubtitleStream[];
  // Tracks picked on the media page before playback (absolute stream
  // indices, "off" = no subtitle). They win over every automatic resolution
  // and count as a manual pick, so the profile preference never undoes them.
  initialAudioIndex?: number;
  initialSubtitle?: number | "off";
  title?: string;
  onClose?: () => void;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  chapters?: Chapter[];
  seriesId?: string;
  onVideoRef?: (el: HTMLVideoElement | null) => void;
  // "Watch together" is rendered in the top-right cluster next to the cast
  // button (which owns the cast state). The route passes the trigger + whether
  // to show it (hidden once a sync room is active).
  onWatchTogether?: () => void;
  showWatchTogether?: boolean;
}


export function HLSPlayer({
  streamInfo,
  sessionId,
  mediaId,
  mediaType,
  streamToken,
  onStreamTokenRefresh,
  onCastTokenRequest,
  startPositionMs = 0,
  knownDurationMs,
  filePath,
  onEnded,
  audioStreams: audioStreamsProp = [],
  subtitleStreams = [],
  initialAudioIndex,
  initialSubtitle,
  title,
  onClose,
  seasons,
  currentSeasonId,
  episodes,
  currentEpisodeId,
  onSeasonChange,
  onEpisodeChange,
  chapters = [],
  seriesId,
  onVideoRef,
  onWatchTogether,
  showWatchTogether,
}: HLSPlayerProps) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
  const controlsToggleRef = useRef<(() => void) | null>(null);
  // Tracks whether PlayerControls is currently showing its bars, so the
  // top-right Watch-together / cast cluster fades in lockstep instead of
  // staying pinned on screen the whole time during playback.
  const [controlsVisible, setControlsVisible] = useState(true);
  const controlsParked = useFadeParked(controlsVisible, CONTROLS_FADE_MS);
  // Set by PlayerControls to its reveal action, so hovering the cluster keeps
  // the controls up (and reschedules the hide) just like hovering the bars.
  const revealControlsRef = useRef<(() => void) | null>(null);
  // Set by SkipIntroButton while the credits countdown runs: Escape cancels
  // it instead of leaving the player.
  const cancelCountdownRef = useRef<(() => void) | null>(null);
  // Set by PlayerControls to its Document-PiP-aware toggle, so the "p" keyboard
  // shortcut opens the same rich PiP window as the on-screen button instead of
  // the bare-<video> legacy PiP.
  const pipToggleRef = useRef<(() => void) | null>(null);
  // Gapless multi-episode session ("lecture enchaînée"): per-series opt-in
  // toggle (set from the series page), read once per mount/series change -
  // same imperative localStorage pattern as the audio/subtitle series prefs
  // used below. The server falls back to today's per-episode master whenever
  // it can't honour it, so a stale/wrong read here is harmless.
  const gapless = seriesId ? !!getSeriesTrackPrefs(seriesId)?.gapless : false;
  const positionRef = useRef(startPositionMs / 1000);
  const knownDurationSecRef = useRef(knownDurationMs ? knownDurationMs / 1000 : 0);
  knownDurationSecRef.current = knownDurationMs ? knownDurationMs / 1000 : 0;
  // Only direct_play streams the original file progressively. Everything else -
  // transcode AND direct_stream (a container-only remux served by the HLS copy
  // rung) - runs through the HLS pipeline (useTranscodePlayback), which owns
  // stream-token injection, 401 refresh and CMCD. direct_stream used to be
  // lumped with direct_play and silently played the original (often undecodable)
  // container; routing it through HLS is what makes the remux actually play.
  const [transcodeMode, setTranscodeMode] = useState(
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_PLAY"
  );

  // The running profile, its languages and its "auto-play next episode"
  // choice: the same reading on every player (usePlayerProfilePrefs).
  const activeProfileId = useAuthStore((s) => s.profileId);
  // Identity carried by a Cast Connect launch, so a television signed in
  // elsewhere refuses the cast instead of opening the wrong thing.
  const castProfileId = activeProfileId;
  const castUserId = userIdFromAccessToken(useAuthStore((s) => s.accessToken));
  const {
    profile: activeProfile,
    audioLangs: prefAudioLangs,
    subtitleLangs: profileSubtitleLangs,
    autoplayNext,
    setAutoplayNext: handleAutoplayNextChange,
  } = usePlayerProfilePrefs();
  // The next-episode card and the skip button share the bottom-right corner:
  // the button steps aside while the card is up.
  const [nextCardVisible, setNextCardVisible] = useState(false);

  // Build ordered language lists from profile.preferences, falling back to
  // locale as a last resort (e.g. profile with locale "fr" but no explicit
  // audio_languages still prefers French audio).
  const profileAudioLangs = useMemo((): string[] => {
    const locale = activeProfile?.locale ?? "";
    // Append locale as final fallback only if not already in the list.
    if (locale && !prefAudioLangs.includes(locale)) return [...prefAudioLangs, locale];
    return prefAudioLangs;
  }, [prefAudioLangs, activeProfile?.locale]);

  // Initialize audio/subtitle from streams + whatever prefs are available at
  // first render (profile may not be loaded yet - profileAudioLangs is empty).
  const audioStreams = audioStreamsProp;
  // A track picked on the media page beats every automatic resolution.
  const pickedAudio = initialAudioIndex === undefined ? undefined : audioStreams.find((s) => s.index === initialAudioIndex);
  const defaultAudio = pickedAudio ?? resolveDefaultAudio(
    audioStreams,
    seriesId,
    getSeriesTrackPrefs,
    getPreferredAudioLang,
    profileAudioLangs.length > 0 ? profileAudioLangs : undefined,
  );
  const [currentAudioIndex, setCurrentAudioIndex] = useState(defaultAudio?.index ?? 0);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(defaultAudio?.trackIndex ?? 0);
  const [currentQuality, setCurrentQuality] = useState(0);
  const [autoQuality, setAutoQuality] = useState(true);

  // Resolved audio language - used for forced subtitle matching.
  const defaultAudioLang = defaultAudio?.language ?? defaultAudio?.title;
  const pickedSubtitle = typeof initialSubtitle === "number" ? subtitleStreams.find((s) => s.index === initialSubtitle) : undefined;
  const subtitlePicked = initialSubtitle === "off" || pickedSubtitle !== undefined;
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(() => {
    if (initialSubtitle === "off") return null;
    if (pickedSubtitle) return pickedSubtitle.trackIndex;
    return resolveDefaultSubtitle(
      subtitleStreams,
      seriesId,
      getSeriesTrackPrefs,
      getPreferredSubtitleLang,
      profileSubtitleLangs.length > 0 ? profileSubtitleLangs : undefined,
      defaultAudioLang,
    );
  });

  // Track whether the user has made a manual track selection in this session
  // (a pick on the media page counts). If so, we must not override with
  // profile prefs when they load asynchronously.
  const userPickedAudioRef = useRef(pickedAudio !== undefined);
  const userPickedSubtitleRef = useRef(subtitlePicked);

  // Audio track picked by the CLIENT-side hls.js switch (transcode): it does
  // not go through currentTrackIndex, whose every change destroys/rebuilds the
  // instance and restarts the session. Natural rebuilds (token refresh,
  // subtitle/offset change) read this ref back to build the URL and re-select
  // the rendition - without it they would replay the previous track.
  const audioTrackOverrideRef = useRef<number | null>(null);

  // Height of the wanted rung, readable without a re-render: the hook pins it
  // INSIDE MANIFEST_PARSED, before the first fragment. The sync effect below
  // stays responsible for changes made during playback.
  // Auto normally pins the top rung; a page that cannot paint it lowers this
  // (see the render watch). 0 = no cap.
  const [renderCapHeight, setRenderCapHeight] = useState(0);
  const renderCapStateRef = useRef<RenderCapState>({ streak: 0, lastAt: 0 });
  // The bandwidth follower's rung (0 = none); the lower of the two caps wins.
  const [networkCapHeight, setNetworkCapHeight] = useState(0);
  const networkCapStateRef = useRef<NetworkCapState>(newNetworkCapState());
  // One-shot guard for the explicit transcode handoff: a single-variant master
  // that keeps stalling has no lower rung to ABR onto, so it is swapped once to
  // the transcode ladder (handoffToTranscode). Reset per media below.
  const handedOffRef = useRef(false);
  const autoCapHeight = renderCapHeight > 0 && networkCapHeight > 0 ? Math.min(renderCapHeight, networkCapHeight) : renderCapHeight || networkCapHeight;
  const autoQualityRef = useRef(autoQuality);
  autoQualityRef.current = autoQuality;
  const renderCapHeightRef = useRef(renderCapHeight);
  renderCapHeightRef.current = renderCapHeight;
  const onFragStatsRef = useRef<((s: { bytes: number; transferMs: number; levelIdx: number; durationSec: number }) => void) | null>(null);
  const onBufferStallRef = useRef<(() => void) | null>(null);
  const pinnedQualityRef = useRef(0);
  pinnedQualityRef.current = autoQuality ? autoCapHeight : currentQuality;

  // When the profile finishes loading, apply its audio/subtitle preferences if
  // the user hasn't picked a track manually and the initial resolution used no
  // profile data (i.e. profileAudioLangs was empty at mount time).
  const profileLoadedRef = useRef(false);
  useEffect(() => {
    if (profileLoadedRef.current) return;
    if (profileAudioLangs.length === 0 && profileSubtitleLangs.length === 0) return;
    profileLoadedRef.current = true;

    if (!userPickedAudioRef.current) {
      const best = resolveDefaultAudio(
        audioStreams,
        seriesId,
        getSeriesTrackPrefs,
        getPreferredAudioLang,
        profileAudioLangs.length > 0 ? profileAudioLangs : undefined,
      );
      if (best && best.index !== currentAudioIndex) {
        setCurrentAudioIndex(best.index);
        setCurrentTrackIndex(best.trackIndex);
      }
    }

    if (!userPickedSubtitleRef.current) {
      const currentAudioStream = audioStreams.find((s) => s.index === currentAudioIndex);
      const audioLang = currentAudioStream?.language ?? currentAudioStream?.title;
      const bestSub = resolveDefaultSubtitle(
        subtitleStreams,
        seriesId,
        getSeriesTrackPrefs,
        getPreferredSubtitleLang,
        profileSubtitleLangs.length > 0 ? profileSubtitleLangs : undefined,
        audioLang,
      );
      if (bestSub !== currentSubtitle) {
        setCurrentSubtitle(bestSub);
      }
    }
  // Intentionally omitting currentAudioIndex/currentSubtitle to avoid re-running
  // after the state updates triggered by this same effect.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profileAudioLangs, profileSubtitleLangs]);

  const selectedBurnInTrack = getSelectedBurnInTrack(currentSubtitle, subtitleStreams);
  // The bitmap track the PLAYER draws, as opposed to the one the server burns
  // in. Both are bitmaps; only PGS has a decoder on our side, and offering any
  // other as client-drawn would put a track in the menu that renders nothing.
  const pgsSubtitleTrack =
    currentSubtitle !== null &&
    isClientDrawableSubtitle(subtitleStreams.find((s) => s.trackIndex === currentSubtitle)?.codec)
      ? currentSubtitle
      : null;
  // A cast failure leaves no server-side trace at all (the fling never reaches
  // us), so the cast path reports through the player beacon. Its OWN reporter,
  // deliberately: the transcode one shares a 20-event budget that a stall storm
  // drains in seconds, and the cast attempt is exactly what we must not lose.
  const reportCastEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  const {
    castSdkReady, connected: castConnected, deviceName: castDeviceName,
    castPlayback, startCast, stopCast, castPlayPause, castSeek, castSetVolume, castSetMuted,
  } = useCasting(reportCastEvent);
  // Chrome Android can't run the Cast SDK. Its only web cast path is the
  // Remote Playback API, which flings a source URL to the receiver - but the
  // local player is MSE (hls.js), which has no flingable URL. So casting rides
  // a DEDICATED hidden <video> whose src is the castable HLS/direct URL; the
  // hook watches IT for device availability and prompts on IT. Desktop keeps
  // the Cast SDK (this API never reports availability there).
  const castVideoRef = useRef<HTMLVideoElement | null>(null);
  const [castVideoEl, setCastVideoEl] = useState<HTMLVideoElement | null>(null);
  const remotePlayback = useRemotePlayback(castVideoEl, reportCastEvent);
  // Gate on `supported` (the API exists = Chrome Android), not `available`
  // (watchAvailability): the availability signal on a preload=none element is
  // not guaranteed to fire, and the priority is that the button reliably
  // appears - the OS picker then does device discovery on tap. One tester's
  // original bug was NO button at all; a button that opens an empty native
  // picker is strictly better and matches how most apps behave.
  const castAvailable = castSdkReady || remotePlayback.available || remotePlayback.supported;

  // Which cast path to TRY FIRST - and the user-agent test is not a guess about
  // the hardware, it mirrors the switch Chrome itself gates the Cast SDK on.
  // Measured on ONE phone, 28 seconds apart (prod telemetry, 2026-07-22):
  //   UA "Android 10; K ... Mobile"  -> sdk=true, then session error:unknown
  //   UA "X11; Linux x86_64" (same    -> sdk=true, then CONNECTED
  //      phone, "Desktop site" on)
  // So Chrome for Android reports the SDK as ready in both modes but only
  // completes a session in desktop mode. In mobile mode the SDK is a trap and
  // the Remote Playback fling is the working path; desktop (and desktop-site)
  // keep the SDK, which is proven to connect.
  //
  // Whichever path runs, it runs ALONE: chaining the other one after a failure
  // puts it outside the tap's user activation, where the browser refuses to
  // open a picker at all (InvalidAccessError, also measured).
  const preferFling = remotePlayback.supported && /Android/i.test(navigator.userAgent);

  // Prefetch the long-lived cast token as soon as the mobile cast surface is
  // active: remote.prompt() must run inside the tap's transient user
  // activation (a few seconds in Chrome), and a network round-trip between
  // tap and prompt could outlive it on a slow link. With the token already in
  // hand the tap path reaches the picker without waiting on the network. The
  // token is media-scoped with a long TTL, so one fetch per media is enough.
  // Desktop (SDK) keeps the on-demand fetch - requestSession has no such race.
  const castTokenRef = useRef<string | null>(null);
  // State, not just a ref: the hidden cast element's src is rebuilt when the
  // token lands, so the tap has nothing left to fetch (see castAvailabilitySrc).
  const [castToken, setCastToken] = useState<string | null>(null);
  useEffect(() => {
    castTokenRef.current = null; // new media -> new token
    setCastToken(null);
    if (!remotePlayback.supported || !onCastTokenRequest) return;
    let stale = false;
    onCastTokenRequest()
      .then((t) => { if (!stale) { castTokenRef.current = t; setCastToken(t); } })
      .catch(() => { /* SDK path falls back to fetching on demand */ });
    return () => { stale = true; };
  }, [remotePlayback.supported, onCastTokenRequest, mediaId]);

  // Builds the media a cast session hands the device, once startCast knows
  // WHICH device (its observed caps on the SDK path, its model on the
  // server path, a DLNA renderer): the profile is the receiver's, the server
  // is asked for ITS verdict (GetStreamInfo), and the URL follows that
  // verdict - the browser's own negotiation (streamInfo.directPlayUrl, its
  // codec caps) never reaches a Cast receiver. Nothing known about the
  // device (no caps answer, no model) negotiates on the conservative floor.
  // Resume rides the LoadRequest's currentTime (castResumeSec) on every
  // path: direct play and transcode each hand the receiver a complete
  // absolute timeline, and baking startSec into the URL never worked - no
  // server handler parses it (see planCastLoad).
  const resolveCastUrl = useCallback(async (target: CastTarget): Promise<{ url: string; castResumeSec: number }> => {
    // Swap in a long-lived cast token: the receiver buffers minutes ahead then
    // goes quiet, and the 5-minute sliding token would expire mid-film. The
    // local player dodges this via xhrSetup; the receiver can't. Prefetched
    // above on the mobile path so the tap doesn't wait on the network.
    let castTok = castTokenRef.current;
    if (!castTok && onCastTokenRequest) {
      try {
        castTok = await onCastTokenRequest();
      } catch (err) {
        console.warn("[Cast] cast token request failed, using session token:", err);
        // Falling back to the 5-minute session token means the receiver loses
        // the stream mid-film - a silent console.warn hid that from the logs.
        reportCastEvent({ type: "cast_token_failed", details: String((err as Error)?.name ?? "unknown"), fatal: true });
      }
    }
    const token = castTok ?? streamTokenRef.current;
    const audioTrack = audioTrackOverrideRef.current ?? currentTrackIndex;
    if (target.kind === "dlna") {
      const plan = planRendererLoad(streamInfo.directPlayUrl, token, mediaId, audioTrack, positionRef.current, gapless);
      return { url: plan.url, castResumeSec: plan.resumeSec };
    }
    const profile = castProfileFor(target.kind === "sdk" ? target.caps : null, target.kind === "server" ? target.model : null);
    // The server's verdict for the receiver; a failed call plays it safe
    // (the master) inside planCastLoad.
    const verdict = await playbackServiceGetStreamInfo(mediaId, castStreamInfoParams(profile)).catch(() => null);
    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const plan = planCastLoad(origin, mediaId, token, profile, verdict, audioTrack, positionRef.current, gapless);
    if (import.meta.env.DEV) console.log("[Cast] url:", plan.url, "target:", target.kind, "verdict:", verdict?.playMethod ?? "none");
    return { url: plan.url, castResumeSec: plan.resumeSec };
  }, [streamInfo.directPlayUrl, mediaId, currentTrackIndex, onCastTokenRequest, gapless, reportCastEvent]);

  // Chrome Android: neither browser cast path is reliable here - the Cast SDK
  // claims ready but cannot complete a session, and the Remote Playback fling
  // is refused outright for anything the phone can't fully parse (both
  // measured in prod, 2026-07-22 and 2026-07-24). So the tap goes SERVER-side
  // first, like every self-hosted peer does for non-desktop browsers: the
  // backend discovers the Chromecast (unicast-probing the phone's own subnet,
  // which works from a Docker bridge) and drives it directly - no browser
  // cast API in the loop at all. The Remote Playback fling stays as a last
  // resort for setups where the server genuinely can't reach the device.
  const handleMobileCast = useCallback(async () => {
    const el = castVideoRef.current;
    if (!el) return;
    const server = await startCast(resolveCastUrl, title, { skipSdk: true });
    if (server === "connected" || server === "load-failed" || server === "dismissed") return;
    // NAT-proof fallback for users who don't live on the server's LAN: their
    // own Evey clients (TV app, another tab) hold a WebSocket to the server
    // and take a relayed loadAndPlay - the same channel as the Play On picker.
    try {
      const resp = await eveyFetch<{ devices: { id: string; name: string }[] }>({ url: "/api/v1/devices", method: "GET" });
      const fallback = pickPlayOnFallback(resp?.devices);
      if (fallback.action === "send") {
        await eveyFetch({
          url: `/api/v1/devices/${encodeURIComponent(fallback.device.id)}/command`,
          method: "POST",
          data: { type: "loadAndPlay", mediaId, positionMs: Math.max(0, Math.floor(positionRef.current * 1000)) },
        });
        try { videoRef.current?.pause(); } catch { /* element released */ }
        toast.success(t("player.playOn.sent", { name: fallback.device.name }));
        return;
      }
      if (fallback.action === "picker") {
        // Several Evey devices online - don't guess which TV. The Play On
        // button in the control bar lists them; send the user there.
        toast.info(t("player.cast.usePlayOn"));
        return;
      }
    } catch { /* device offline or relay down - fall through to the fling */ }
    // Fling fallback, still inside the tap's transient activation: the server
    // round-trips are fast and activation survives short awaits - what kills
    // it is consumption, and nothing above consumes it.
    el.muted = false;
    const outcome = await remotePlayback.prompt();
    if (outcome === "connected") {
      // Pause the local player only on a REAL handoff - a dismissed picker
      // must leave the film exactly as it was.
      try { videoRef.current?.pause(); } catch { /* element released */ }
      const resumeAt = positionRef.current > 0 ? Math.floor(positionRef.current) : 0;
      if (resumeAt > 0) { try { el.currentTime = resumeAt; } catch { /* not seekable yet */ } }
      // Drive the remoted element: while connected, play() is forwarded to the
      // device (the SDK path gets autoplay from its LoadRequest; this one has
      // to ask).
      el.play().catch(() => { /* receiver may auto-start on its own */ });
      return;
    }
    if (outcome === "dismissed") return; // the user's choice, stay silent
    // Every path failed. ONE message, and an ACTIONABLE one: for a remote
    // mobile-web user with a bare Chromecast there is no working browser path
    // (Chrome refuses both cast APIs there), so say what DOES work instead of
    // a dead-end "no devices" - the 2026-07-24 lesson twice over.
    toast.error(t(server === "no-devices" ? "player.cast.mobileWebHint" : "player.cast.connectFailed"));
  }, [remotePlayback, startCast, resolveCastUrl, title, t, mediaId]);

  const handleCastToggle = useCallback(async () => {
    if (castConnected) {
      stopCast();
      return;
    }
    // Same handover as the media page's Cast button: an Evey TV app gets the
    // media id and the current position, and plays it with its own player.
    const outcome = await startCast(resolveCastUrl, title, {
      userId: castUserId,
      profileId: castProfileId ?? undefined,
      eveyMedia: {
        mediaId,
        audioTrack: audioTrackOverrideRef.current ?? currentTrackIndex,
        subtitleTrack: currentSubtitle ?? "off",
      },
    });
    if (outcome === "no-devices") {
      toast.error(t("player.cast.noDevices"));
      return;
    }
    if (outcome !== "session-failed") return;
    // No chained fallback to the fling here either: it would run after this
    // failure, outside the tap's user activation, and the browser refuses to
    // open a picker then (InvalidAccessError, observed in production). The
    // platform routing above already picks the path that works here; if it
    // failed, say so honestly instead of opening a door that cannot open.
    toast.error(t("player.cast.connectFailed"));
  }, [castConnected, stopCast, startCast, resolveCastUrl, title, t, castUserId, castProfileId, mediaId, currentTrackIndex, currentSubtitle]);


  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const anyPanelOpenRef = useRef(false);
  const closePanelRef = useRef<(() => void) | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const [externalSubUrl, setExternalSubUrl] = useState<{ url: string; label: string } | null>(null);
  const [subtitleStyle, setSubtitleStyleState] = useState<SubtitleStyle>(() => getSubtitleStyle());

  function updateSubtitleStyle(patch: Partial<SubtitleStyle>) {
    setSubtitleStyleState((prev) => {
      const next = { ...prev, ...patch };
      setSubtitleStyle(next);
      return next;
    });
  }
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const sleepTimer = useSleepTimer({
    onMinutesFire: () => {
      try { videoRef.current?.pause(); } catch { /* element gone */ }
      toast.info(t("player.sleepTimer.fired"));
    },
  });
  // Read-latest mirror so the mount-only "ended" listener below (registered
  // once per video element, not per render) can check the CURRENT sleep timer
  // state without becoming a dependency that would tear down and re-attach
  // the listener on every countdown tick.
  const sleepTimerRef = useRef(sleepTimer);
  sleepTimerRef.current = sleepTimer;
  // Debounced gate in front of setIsBuffering: the raw `waiting` event fires on
  // every micro-stall (segment boundary, fast seek, ABR switch) and flashing
  // the spinner over a video that resumes within a second reads as "the app is
  // struggling" when nothing is wrong. Only surface the overlay when buffering
  // PERSISTS past the threshold; hide immediately on `playing`. Stable identity
  // (useCallback []) is required: both playback hooks exclude the setter from
  // their effect deps on that assumption.
  const bufferingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const setIsBufferingDebounced = useCallback((v: boolean) => {
    if (v) {
      if (bufferingTimerRef.current !== null) return; // already counting down
      bufferingTimerRef.current = setTimeout(() => {
        bufferingTimerRef.current = null;
        setIsBuffering(true);
      }, BUFFERING_SPINNER_DELAY_MS);
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
  const [fatalError, setFatalError] = useState(false);
  // Server-side load shed (HTTP 503): distinct from fatalError - the stream
  // itself isn't broken, the server just refused to start it right now for
  // one of two unrelated reasons. Kept separate so the overlay/copy/icon can
  // differ from the "something is actually wrong" fatal-error case.
  const [shedReason, setShedReason] = useState<ShedReason>(null);
  const wasPausedRef = useRef(false);
  const playbackSpeedRef = useRef(playbackSpeed);
  playbackSpeedRef.current = playbackSpeed;
  const subtitleStreamsRef = useRef(subtitleStreams);
  subtitleStreamsRef.current = subtitleStreams;
  const currentSubtitleRef = useRef(currentSubtitle);
  currentSubtitleRef.current = currentSubtitle;
  const streamTokenRef = useRef(streamToken);
  streamTokenRef.current = streamToken;
  const onStreamTokenRefreshRef = useRef(onStreamTokenRefresh);
  onStreamTokenRefreshRef.current = onStreamTokenRefresh;

  // Give the hidden cast <video> a flingable source URL so Remote Playback can
  // compute device availability (an MSE element never can). A base URL with the
  // session token is enough to advertise the source; the click path rebuilds it
  // with the long-lived cast token + current position via resolveCastUrl.
  const castAvailabilitySrc = useMemo(() => {
    const sessionTok = streamTokenRef.current;
    if (!sessionTok) return undefined;
    // The FULL cast URL, ready to fling: absolute (the receiver fetches it
    // itself, so it must be the address the operator says devices can reach),
    // and carrying the long-lived cast token as soon as the prefetch lands -
    // the 5-minute session token would expire on the receiver mid-film.
    //
    // Everything the tap needs is computed HERE, ahead of time, on purpose:
    // prompt() must run inside the tap's transient user activation, and doing
    // this work in the click handler burned it (Chrome then rejects with
    // NotAllowedError and never opens a picker).
    let url: string;
    if (streamInfo.directPlayUrl) {
      const dp = streamInfo.directPlayUrl;
      url = dp.includes("token=") ? dp : `${dp}${dp.includes("?") ? "&" : "?"}token=${encodeURIComponent(sessionTok)}`;
    } else {
      // The client-side hls.js switch does not go through currentTrackIndex:
      // the ref carries the track actually being listened to when the cast is
      // being prepared.
      url = buildTranscodeHlsUrl(mediaId, sessionTok, audioTrackOverrideRef.current ?? currentTrackIndex, null, 0, 0, gapless);
    }
    if (castToken) {
      const u = new URL(url, window.location.origin);
      u.searchParams.set("token", castToken);
      url = `${u.pathname}?${u.searchParams.toString()}`;
    }
    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    return url.startsWith("http") ? url : `${origin}${url}`;
    // streamToken lands via streamInfo negotiation; key on it so this recomputes once the token is ready.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamInfo, mediaId, currentTrackIndex, gapless, castToken]);

  useEffect(() => {
    // NEVER touch the element while a remote session is live: reassigning src
    // mid-cast (token refresh, audio-track change re-render) would yank the
    // receiver back to zero on a short-lived token.
    if (remotePlayback.connected) return;
    if (castVideoEl && castAvailabilitySrc) castVideoEl.src = castAvailabilitySrc;
  }, [castVideoEl, castAvailabilitySrc, remotePlayback.connected]);

  // While a Remote Playback session is live, the local player must stand down
  // exactly like during an SDK cast: pause it, and re-pause it if anything
  // (media keys, MediaSession, autoplay) wakes it - two simultaneous
  // playbacks of the same film is the bug this guards against. On disconnect,
  // pick the film up locally where the TV left it: the remoted element's
  // currentTime tracked the device.
  const remoteConnected = remotePlayback.connected;
  useEffect(() => {
    const local = videoRef.current;
    if (!remoteConnected || !local) return;
    const repause = () => { try { local.pause(); } catch { /* released */ } };
    repause();
    local.addEventListener("play", repause);
    return () => {
      local.removeEventListener("play", repause);
      const remotePos = castVideoRef.current?.currentTime ?? 0;
      if (remotePos > 0) {
        try { local.currentTime = remotePos; } catch { /* released */ }
      }
    };
  }, [remoteConnected]);
  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;
  const skipFwdRef = useRef(skipFwd);
  skipFwdRef.current = skipFwd;
  const skipBwdRef = useRef(skipBwd);
  skipBwdRef.current = skipBwd;
  const transcodeOffsetRef = useRef(0);

  // --- Hooks ---

  useDirectPlayback({
    videoRef, positionRef, streamInfo, startPositionMs,
    transcodeMode, onEnded,
    setTranscodeMode, setIsBuffering: setIsBufferingDebounced,
    streamTokenRef, onStreamTokenRefreshRef,
  });

  // Live A/V and subtitle resync. Subtitles shift the LOADED cues client-side
  // (instant, no reload); audio needs a re-encode with a shifted input, so a
  // change restarts the transcode at the current position - debounced below so
  // a burst of stepper clicks coalesces into ONE restart. The APPLIED value is
  // what the stream URL carries; the raw value is what the UI shows.
  const [subtitleOffsetMs, setSubtitleOffsetMs] = useState(0);
  const [audioOffsetMs, setAudioOffsetMs] = useState(0);
  const [appliedAudioOffsetMs, setAppliedAudioOffsetMs] = useState(0);
  // wasm-audio fallback latch: set when the client-side AC-3 decode path can't
  // run (wasm load / decode / no server audio URI), which re-negotiates the
  // stream against the normal muxed/AAC master. Reset per media.
  const [wasmAudioDisabled, setWasmAudioDisabled] = useState(false);
  // The relay leg the viewer is HEARING, as the engine reports it: "aac" while
  // the wasm decoder boots, "wasm" once it carries, "aac-latched" when it gave
  // up mid-film and the server's AAC transcode took over. Null until the engine
  // speaks. Distinct from wasmAudioActive below, which is only the topology we
  // ASKED the server for - a latch never touched it, so the stream panel went on
  // announcing "décodé client" over a transcode (prod, 2026-09-20).
  const [wasmAudioLeg, setWasmAudioLeg] = useState<string | null>(null);
  // wasm-to-wasm audio switch: only the audio engine follows this; the video
  // keeps playing (a full restart cost seconds of silence, prod 2026-09-10).
  const [wasmTrackIndex, setWasmTrackIndex] = useState<number | null>(null);
  useEffect(() => {
    setSubtitleOffsetMs(0);
    setAudioOffsetMs(0);
    setAppliedAudioOffsetMs(0);
    setShedReason(null);
    setWasmAudioDisabled(false);
    setWasmAudioLeg(null);
    audioTrackOverrideRef.current = null;
    setWasmTrackIndex(null);
    setRenderCapHeight(0);
    renderCapStateRef.current = { streak: 0, lastAt: 0 };
    setNetworkCapHeight(0);
    networkCapStateRef.current = newNetworkCapState();
    handedOffRef.current = false;
  }, [mediaId]);

  // The wasm-audio path applies when: we're on the HLS/transcode pipeline, the
  // browser can't decode AC-3/E-AC-3 natively but runs WebAssembly, and the
  // SELECTED audio track is AC-3/E-AC-3. The video then plays video-only (the
  // server drops the audio from the master) and WasmAudioEngine decodes the
  // AC-3 rendition client-side, in sync. A failure latches wasmAudioDisabled and
  // the whole thing falls back to the server's AAC transcode.
  const selectedAudioCodec = useMemo(
    () => audioStreams.find((s) => s.trackIndex === currentTrackIndex)?.codec,
    [audioStreams, currentTrackIndex],
  );
  const wasmAudioActive =
    transcodeMode &&
    !wasmAudioDisabled &&
    canWasmDecodeAC3() &&
    isWasmDecodableAudioCodec(selectedAudioCodec);

  const { transcodeStartTime, setTranscodeStartTime, hlsLevels, hlsLevelSizes, autoLevelHeight, autoLevelWidth, playingVariant, ladderBitrates, manifestAudioTracks, handoffToTranscode } = useTranscodePlayback({
    videoRef, hlsRef, positionRef, transcodeOffsetRef,
    wasPausedRef, streamTokenRef, onStreamTokenRefreshRef, audioTrackOverrideRef,
    pinnedQualityRef, onFragStatsRef, onBufferStallRef,
    transcodeMode, mediaId, currentTrackIndex, selectedBurnInTrack,
    startPositionMs, onEnded, setIsBuffering: setIsBufferingDebounced, setFatalError,
    setShedReason,
    // On the wasm branch the correction lives in the audio engine (applied
    // live, no reload): the video leg is fetched with offset 0 so a menu change
    // never restarts the transcode. The native AAC leg keeps the server-side
    // resync (a re-encode with the offset), which needs the reload below.
    audioOffsetMs: wasmAudioActive ? 0 : appliedAudioOffsetMs,
    gapless,
    wasmAudio: wasmAudioActive,
  });

  // What the audio menu offers. Normally the server's stream list; when the
  // caller has none, the renditions the MASTER declares - which is the only
  // thing a guest in a "watch together" room can see, since GetStreamInfo
  // needs a JWT they do not have. Without this a guest could not choose a
  // language at all: the menu simply was not there.
  const menuAudioStreams = useMemo<AudioStream[]>(() => {
    if (audioStreamsProp.length > 0) return audioStreamsProp;
    return manifestAudioTracks.map((t, i) => ({
      index: i,
      trackIndex: i,
      language: t.lang || undefined,
      title: t.name || undefined,
      isDefault: i === 0,
    }));
  }, [audioStreamsProp, manifestAudioTracks]);


  // Client-side AC-3/E-AC-3 decode (ac3go wasm) for browsers without native
  // support. Runs alongside hls.js: the video plays video-only, this engine
  // decodes the out-of-band AC-3 rendition and plays it in sync. A fatal error
  // latches the fallback, which re-requests the muxed/AAC master.
  // Its own reporter, like the cast one: the transcode reporter's 20-event
  // budget is what an hls.js stall storm drains first, and the engine's mode
  // and gate lines are exactly what must survive such a storm.
  const reportWasmAudioEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);

  // The picture slowing down while the sound stays fluid left no trace in any
  // log: the decoder drops nothing (so drop= stays 0/N) and the server sees a
  // client that stopped asking for anything. This watch measures the symptom
  // itself - frames actually presented - next to how late a 250 ms timer runs,
  // which is what a blocked main thread looks like. In Auto it also steps the
  // pinned rung down when the page keeps under-painting a full buffer.
  const reportRenderEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  // Its own budget: a title that hitches all evening must not eat the
  // slowdown and quality reports.
  const reportCadenceEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  onBufferStallRef.current = () => {
    const now = performance.now();
    recordStall(networkCapStateRef.current, now);
    // A single-variant master (copy/hybrid whose transcode fallback rung was
    // omitted as a different ABR group) has no lower rung to step down onto:
    // decideNetworkStep can never rescue it, so repeated stalls just freeze.
    // Swap once to the transcode ladder, which has lower rungs again.
    const hls = hlsRef.current;
    if (!hls || handedOffRef.current || !autoQualityRef.current) return;
    if (shouldHandoffToTranscode(networkCapStateRef.current, hls.levels.length, now)) {
      handedOffRef.current = true;
      reportRenderEvent({
        type: "quality_capped",
        details: `handoff->transcode (single-variant master stalled, levels=${hls.levels.length})`,
        fatal: false,
        posSec: positionRef.current,
      });
      handoffToTranscode();
    }
  };
  onFragStatsRef.current = (s) => {
    const hls = hlsRef.current;
    const video = videoRef.current;
    if (!hls || !video || !autoQualityRef.current) return;
    const now = performance.now();
    recordThroughput(networkCapStateRef.current, { bytes: s.bytes, transferMs: s.transferMs, at: now, durationSec: s.durationSec });
    const levels = hls.levels.map((l) => ({ height: l.height, kbps: l.bitrate / 1000 }));
    const heights = levels.map((l) => l.height);
    const ceiling = pickPinnedLevel({ levelHeights: heights, desiredHeight: renderCapHeightRef.current });
    const step = decideNetworkStep(networkCapStateRef.current, now, levels, hls.currentLevel, ceiling, bufferedAheadSec(video));
    if (!step) return;
    const from = heights[hls.currentLevel] ?? 0;
    setNetworkCapHeight(step.index === heights.length - 1 ? 0 : step.height);
    reportRenderEvent({
      type: step.direction === "down" ? "quality_capped" : "quality_restored",
      details: formatNetworkStep(step, from),
      fatal: false,
      posSec: positionRef.current,
    });
  };
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const watch = createRenderStallWatch(video, {
      onSlowdown: (w) => {
        reportRenderEvent({
          type: "video_slowdown",
          details: formatRenderWindow(w),
          fatal: false,
          posSec: positionRef.current,
          // The full snapshot is the point: a slowdown with an empty buffer is
          // the server being late, one with a full buffer and drop=0 is the
          // page failing to paint what it already has.
          state: playbackSnapshot(video),
        });
        const hls = hlsRef.current;
        if (!hls || !autoQualityRef.current) return;
        const heights = hls.levels.map((l) => l.height);
        const current = heights[hls.currentLevel] ?? 0;
        // The forward-buffer target shrinks with the rung's bitrate (6 s at
        // 70 Mb/s), so "full buffer" is judged against it, not a fixed 8 s.
        const cap = decideRenderCap(renderCapStateRef.current, performance.now(), bufferedAheadSec(video), heights, current, hls.config.maxBufferLength);
        if (cap == null) return;
        setRenderCapHeight(cap);
        reportRenderEvent({
          type: "quality_capped",
          details: `${current}p->${cap}p ${formatRenderWindow(w)}`,
          fatal: false,
          posSec: positionRef.current,
        });
      },
      // The picture hitching at a full frame count: frames held too long or
      // never shown. Nothing else in the logs can see it.
      onCadence: (w, totals) => {
        reportCadenceEvent({
          type: "video_cadence",
          details: `${formatCadence(w, totals)} foc=${document.hasFocus() ? 1 : 0}`,
          fatal: false,
          posSec: positionRef.current,
          state: playbackSnapshot(video),
        });
      },
    });
    const unregister = registerRenderStats(video, () => {
      const w = watch.latest();
      return w ? formatRenderWindow(w) : null;
    });
    return () => {
      unregister();
      watch.stop();
    };
  }, [reportRenderEvent, reportCadenceEvent, videoRef]);
  useWasmAudioPlayback({
    videoRef,
    mediaId,
    currentTrackIndex: wasmTrackIndex ?? currentTrackIndex,
    streamTokenRef,
    onStreamTokenRefresh: onStreamTokenRefreshRef,
    active: wasmAudioActive,
    onFallback: () => setWasmAudioDisabled(true),
    reportEvent: reportWasmAudioEvent,
    onLegChange: setWasmAudioLeg,
    audioOffsetMs,
  });

  useEffect(() => {
    if (audioOffsetMs === appliedAudioOffsetMs) return;
    const id = setTimeout(() => {
      setAppliedAudioOffsetMs(audioOffsetMs);
      // The wasm engine took the new offset live; only the native AAC leg has
      // to restart the transcode at the current position to re-time its audio.
      if (!wasmAudioActive) setTranscodeStartTime(positionRef.current);
    }, 1200);
    return () => clearTimeout(id);
  }, [audioOffsetMs, appliedAudioOffsetMs, setTranscodeStartTime, wasmAudioActive]);

  const { subtitlePreparing } = useSubtitleManager({
    videoRef, hlsRef, currentSubtitle, externalSubUrl, mediaId, streamToken,
    selectedBurnInTrack, transcodeStartTime, currentTrackIndex, subtitleStyle,
    subtitleOffsetMs,
  });

  // Sync playbackRate
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.playbackRate = playbackSpeed;
  }, [playbackSpeed]);

  const [actionFlash, setActionFlash] = useState<"play" | "pause" | null>(null);
  const actionFlashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  function showActionFlash(action: "play" | "pause") {
    if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
    setActionFlash(action);
    actionFlashTimer.current = setTimeout(() => setActionFlash(null), 500);
  }

  // Admin message, drawn inside the player container so it survives
  // fullscreen (a global toast sits outside the fullscreen element and is
  // never painted). Same 12 s the cast receiver gives it.
  const [adminMessage, setAdminMessage] = useState<string | null>(null);
  const adminMessageTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const showAdminMessage = useCallback((message: string) => {
    if (adminMessageTimer.current) clearTimeout(adminMessageTimer.current);
    setAdminMessage(message);
    adminMessageTimer.current = setTimeout(() => setAdminMessage(null), ADMIN_MESSAGE_MS);
  }, []);

  // Retries a shed playback without a full page reload: hls.js's own
  // recommendation for a fatal NETWORK_ERROR is startLoad() (it re-issues
  // whichever request last failed - manifest, level or fragment), so this
  // reuses the live hls.js instance instead of tearing the player down.
  // Native-HLS (Safari, no hls.js instance) falls back to reloading the
  // <video> element, which is the native-path equivalent.
  const handleShedRetry = useCallback(() => {
    setShedReason(null);
    const hls = hlsRef.current;
    if (hls) {
      hls.startLoad();
    } else {
      videoRef.current?.load();
    }
  }, []);

  const handleSeekRequest = useCallback((absoluteTimeSec: number) => {
    const video = videoRef.current;
    if (!video) return;
    // VOD pipeline (transcode + direct): the manifest covers the full
    // media duration. SegmentOrchestrator spawns a rafale at the target
    // segment with `-copyts`, so hls.js can seek via video.currentTime
    // and the served fragment's tfdt lines up with the EXTINF position.
    // No more sliding-window restart branch - that legacy code computed
    // currentTime relative to a transcodeOffset the new pipeline doesn't
    // carry, and produced UI/playhead drift on resume.
    //
    // Record the intended position SYNCHRONOUSLY. positionRef is otherwise only
    // advanced by `timeupdate`, which does not fire until playback reaches the
    // new spot - so for the window between this seek and that first tick,
    // positionRef still holds the PRE-seek position. A deep seek routinely
    // triggers a player rebuild inside that window (quality pin, audio
    // re-select, a 404/token recovery on the not-yet-produced segment), and
    // every rebuild resumes at positionRef.current / transcodeStartTime - so a
    // stale positionRef yanked the playhead straight back to where the user
    // seeked FROM ("I click 2:04, it lands at 1:30"). Writing it here closes
    // that window: the rebuild resumes at the seek target, not the old spot.
    positionRef.current = absoluteTimeSec;
    video.currentTime = absoluteTimeSec;
  }, [positionRef]);

  const seekRef = useRef(handleSeekRequest);
  seekRef.current = handleSeekRequest;

  // Canonical subtitle selection - shared by the settings panel picker and the
  // keyboard "c" shortcut. Crucially, when toggling a burn-in track (PGS/VobSub)
  // in transcode mode it restarts the transcode from the current position so the
  // server re-encodes with the subtitle burned in. The raw setCurrentSubtitle
  // setter alone flips the state without re-encoding, so a burn-in subtitle
  // picked via "c" never actually appeared.
  const handleSubtitleChange = useCallback((trackIndex: number | null) => {
    userPickedSubtitleRef.current = true;
    // Picking a server track (or Off) must dethrone an uploaded external
    // subtitle: useSubtitleManager gives the external file unconditional
    // priority, so without this the menu changed state while the external
    // file kept rendering (mobile got this right first, 2026-07-29). The
    // manager's own effect revokes the blob URL when the value changes.
    setExternalSubUrl(null);
    const prevBurnIn = selectedBurnInTrack;
    const nextSub = trackIndex !== null ? subtitleStreams.find((st) => st.trackIndex === trackIndex) : null;
    const nextBurnIn = nextSub?.burnIn ? trackIndex : null;

    setCurrentSubtitle(trackIndex);

    if (transcodeMode && prevBurnIn !== nextBurnIn) {
      setTranscodeStartTime(positionRef.current);
    }

    if (trackIndex === null) {
      setPreferredSubtitleLang(null);
      if (seriesId) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: null });
      }
    } else {
      const lang = nextSub?.language ?? nextSub?.title ?? null;
      setPreferredSubtitleLang(lang);
      if (seriesId) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: lang });
      }
    }
  }, [selectedBurnInTrack, subtitleStreams, transcodeMode, setCurrentSubtitle, setTranscodeStartTime, seriesId]);

  // Live cast state for the keyboard shortcuts, refreshed render-time (same
  // pattern as lastCastPosRef): while casting, space/k must toggle the TV.
  // A fling (Remote Playback) has no transport to offer, so it registers with
  // playPause undefined: space then does NOTHING, which is the point - it
  // used to fall through and wake the local element behind the TV's back.
  const castKeyboardRef = useRef<{ connected: boolean; isPaused: boolean; playPause?: () => void } | null>(null);
  castKeyboardRef.current = castConnected
    ? { connected: true, isPaused: castPlayback.isPaused, playPause: castPlayPause }
    : remoteConnected
      ? { connected: true, isPaused: false }
      : null;

  usePlayerKeyboard({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef,
    playbackSpeedRef, subtitleStreamsRef, currentSubtitleRef, anyPanelOpenRef,
    closePanelRef, onCloseRef, setPlaybackSpeed, onSubtitleChange: handleSubtitleChange,
    setShowShortcuts, showActionFlash, pipToggleRef, castRef: castKeyboardRef,
    revealControlsRef, cancelCountdownRef,
  });

  const { overlayTime, overlayDuration } = useOverlayTime({
    videoRef, transcodeOffsetRef, knownDurationSecRef,
  });

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    if (!document.fullscreenElement) void el.requestFullscreen();
    else void document.exitFullscreen();
  }, []);

  const handleSingleTap = useCallback(() => {
    if (controlsToggleRef.current) controlsToggleRef.current();
  }, []);

  const { seekFlash, brightness, setBrightnessValue, swipeIndicator, handleTouchStart, handleTouchMove, handleTouchEnd, cleanupTimers } = useTouchGestures({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef, onToggleFullscreen: toggleFullscreen, onSingleTap: handleSingleTap,
  });

  // One answer to "the picture is on another device": a Cast session (SDK or
  // server-brokered) or a Remote Playback fling. Every path that could start
  // the LOCAL element consults it - the re-pause guard further down is the
  // backstop, not the mechanism. A play() that has to be undone is a blip of
  // sound and picture, and a viewer reports that as a bug, rightly.
  const remotePlaybackActive = castConnected || remoteConnected;

  // Last known ABSOLUTE cast position. The receiver plays a complete timeline
  // (file / VOD manifest), so its currentTime IS the media position - no
  // offset. Render-time ref update, same pattern as onCloseRef: castPlayback
  // ticks re-render this component ~1/s while casting, keeping it fresh for
  // the handoff-back.
  const lastCastPosRef = useRef(0);
  if (castConnected && castPlayback.isMediaLoaded) {
    lastCastPosRef.current = castPlayback.currentTime;
  }

  // Auto-pause local player when casting, resume when cast stops - AT THE
  // CAST'S POSITION. Without the seek the local player resumes on the frame
  // it paused at during handoff, and its next progress tick would rewind the
  // user's "continue watching" to a position they watched long past on TV.
  const wasPlayingBeforeCast = useRef(false);
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    if (castConnected) {
      wasPlayingBeforeCast.current = !video.paused;
      if (!video.paused) video.pause();
      // Structural guard, belt over the keyboard routing: while the cast is
      // live, ANY play attempt on the local element (OS media keys,
      // MediaSession, a stray autoplay) is immediately re-paused. Two
      // streams at once - with the local one uncontrollable because the UI
      // is cast-bound - is never a valid state.
      const onPlay = () => video.pause();
      video.addEventListener("play", onPlay);
      return () => video.removeEventListener("play", onPlay);
    }
    if (lastCastPosRef.current > 0) {
      seekRef.current(lastCastPosRef.current);
      lastCastPosRef.current = 0;
    }
    if (wasPlayingBeforeCast.current) {
      video.play().catch((err) => console.warn("video.play failed", err));
      wasPlayingBeforeCast.current = false;
    }
  }, [castConnected]);

  // Sync isPlaying from the video element so the buffering overlay can
  // distinguish "stall while playing" (show only discrete spinner) from
  // "stall while paused/not-yet-started" (show full-screen overlay).
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let pausedAt: number | null = null;
    const onPlay = () => {
      setIsPlaying(true);
      // See resumeNudge.ts: a decoder parked by a long pause restarts on a
      // seek, not on play - without it the clock runs on a frozen picture.
      nudgeOnResume(video, pausedAt);
      pausedAt = null;
    };
    const onPause = () => {
      setIsPlaying(false);
      pausedAt = Date.now();
    };
    const onEnded = () => {
      setIsPlaying(false);
      // "Stop at end of episode" sleep timer: the video already paused itself
      // on "ended". Deliberately NOT cancelling the timer here - suppressing
      // NextEpisodeOverlay's auto-advance is keyed off this same
      // state.kind === "endOfEpisode" (see the JSX below), so clearing it in
      // this same tick would race the overlay's own end-of-video effect and
      // let the auto-advance slip through. The viewer dismisses it explicitly
      // from Settings > Sleep timer > Cancel once they're done.
      if (sleepTimerRef.current.state?.kind === "endOfEpisode") {
        toast.info(t("player.sleepTimer.fired"));
      }
    };
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("ended", onEnded);
    setIsPlaying(!video.paused);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("ended", onEnded);
    };
  // videoRef is a stable ref object; the effect needs to run once on mount.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-fullscreen on landscape rotation (touch devices)
  useEffect(() => {
    if (!("ontouchstart" in window) || typeof window.matchMedia !== "function") return;
    const mql = window.matchMedia("(orientation: landscape)");
    const handler = (e: MediaQueryListEvent) => {
      const container = containerRef.current;
      if (!container) return;
      if (e.matches && !document.fullscreenElement) {
        void container.requestFullscreen().catch((err) => console.warn("requestFullscreen failed", err));
      }
    };
    mql.addEventListener("change", handler);
    return () => mql.removeEventListener("change", handler);
  }, []);

  // Cleanup timers on unmount
  useEffect(() => {
    return () => {
      if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
      if (adminMessageTimer.current) clearTimeout(adminMessageTimer.current);
      cleanupTimers();
    };
  }, [cleanupTimers]);

  // Sync hls.js level with autoQuality / currentQuality state
  useEffect(() => {
    const hls = hlsRef.current;
    if (!hls || !transcodeMode) return;
    // hls.js ABR is intentionally OFF on this transcode path: free ABR loads
    // several rungs at once on a transcode box (each = a concurrent decode), they
    // contend, time out, and it collapses to the 360p floor. Setting currentLevel
    // to a fixed index pins one rung (and disables ABR). "Auto" pins the TOP
    // (calibrated-sustainable) rung - one decode, stable, no switching - and the
    // viewer drops manually via the picker for a title too heavy to hold it.
    // Same helper as the MANIFEST_PARSED pin, so the two paths cannot diverge:
    // this one covers changes made DURING playback (the viewer toggling Auto
    // <-> a height), the hook covers the start.
    const pinned = pickPinnedLevel({
      levelHeights: hls.levels.map((l: { height: number }) => l.height),
      desiredHeight: autoQuality ? autoCapHeight : currentQuality,
    });
    hls.currentLevel = pinned;
    // hlsLevels MUST be a dep: this effect's mount run happens BEFORE hls.js
    // has parsed the manifest (hls.levels is empty → currentLevel=-1, which
    // ENABLES free ABR instead of pinning). Nothing else re-runs the effect
    // afterwards, so on a multi-rung ladder (the hardware path) playback was
    // left on free ABR - just-in-time segment production wrecks its bandwidth
    // estimate (multi-second TTFB on cache misses) and it collapses to the
    // 360p floor: the "Auto downgrades hard on HW transcode" bug. The single-
    // rendition software path masked it (one level = nowhere to fall).
    // hlsLevels is set in MANIFEST_PARSED, so depending on it re-runs the pin
    // exactly when the levels exist.
  }, [autoQuality, currentQuality, autoCapHeight, transcodeMode, hlsLevels]);

  // In Auto, surface the rung ABR actually picked (e.g. "1080p") so the menu
  // shows "Auto (1080p)" rather than a redundant "Auto". Empty until the first
  // level switch (panel then shows a bare "Auto").
  // Label the rung the way the title page labels the file, width included: a
  // 2.40:1 master's "1080p" rung is 1920×798, which is neither "798p" nor 720p.
  const served = useMemo(() => servedVariant(streamInfo, playingVariant, ladderBitrates), [streamInfo, playingVariant, ladderBitrates]);
  const autoQualityLabel = autoQuality && autoLevelHeight ? `${nominalQualityTier(autoLevelHeight, autoLevelWidth)}p` : undefined;

  const nextEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx < 0 || idx >= episodes.length - 1) return null;
    return episodes[idx + 1];
  })();

  const handleNextEpisode = useCallback((id: string) => {
    onEpisodeChange?.(id);
  }, [onEpisodeChange]);

  function handleQualityChange(quality: number) {
    setAutoQuality(false);
    setCurrentQuality(quality);
  }

  function handleAutoQualityToggle(auto: boolean) {
    setAutoQuality(auto);
    // Re-enabling Auto is the viewer asking for the top rung again.
    if (auto) {
      setRenderCapHeight(0);
      setNetworkCapHeight(0);
      networkCapStateRef.current = newNetworkCapState();
    }
  }

  function handleAudioChange(index: number) {
    userPickedAudioRef.current = true;
    const stream = audioStreams.find((s) => s.index === index);
    setCurrentAudioIndex(index);
    if (stream) {
      const lang = stream.language ?? stream.title ?? null;
      setPreferredAudioLang(lang);
      if (seriesId && lang) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, audioLang: lang });
      }
    }

    if (!transcodeMode) {
      if (stream) setCurrentTrackIndex(stream.trackIndex);
      if (hlsRef.current) {
        hlsRef.current.audioTrack = stream?.trackIndex ?? 0;
        return;
      }
      const video = videoRef.current as (HTMLVideoElement & { audioTracks?: { length: number; [i: number]: { id: string; enabled: boolean } } }) | null;
      if (!video) return;
      const tracks = video.audioTracks;
      if (!tracks) return;
      for (let i = 0; i < tracks.length; i++) {
        tracks[i].enabled = tracks[i].id === String(index);
      }
      return;
    }

    // Transcode: the master already exposes every track as a rendition, so
    // switching track is hls.audioTrack - instant, without touching the session
    // (DEFAULT and the switch are decoupled - same mechanism as mobile).
    // currentTrackIndex is NOT updated on this path: any change destroys the
    // instance and restarts the transcode, which used to cut playback.
    const target = stream?.trackIndex ?? 0;
    const wasmForTarget =
      !wasmAudioDisabled && canWasmDecodeAC3() && isWasmDecodableAudioCodec(stream?.codec);
    const path = pickTranscodeAudioSwitchPath({
      hasHls: !!hlsRef.current,
      wasmActiveNow: wasmAudioActive,
      wasmActiveForTarget: wasmForTarget,
      audioRenditionCount: hlsRef.current?.audioTracks.length ?? 0,
      targetTrackIndex: target,
    });
    if (path === "hls-rendition" && hlsRef.current) {
      audioTrackOverrideRef.current = target;
      hlsRef.current.audioTrack = target;
      return;
    }
    if (path === "wasm-engine") {
      audioTrackOverrideRef.current = target;
      setWasmTrackIndex(target);
      return;
    }
    audioTrackOverrideRef.current = null;
    setCurrentTrackIndex(target);
    setTranscodeStartTime(positionRef.current);
  }

  // A live session always wins the tap: SDK sessions stop via handleCastToggle,
  // Remote Playback ones via the hook's stop() (re-prompting mid-session would
  // reload the receiver instead of stopping it). The routing itself lives in
  // pickCastTapPath (pure, test-locked): one tap, one path, no chained
  // fallback. Handed to the unified remote-playback menu's Cast entry - this
  // component still owns which transport a tap resolves to.
  const handleCastTap = {
    "stop-sdk": handleCastToggle,
    "stop-fling": remotePlayback.stop,
    fling: handleMobileCast,
    sdk: handleCastToggle,
  }[pickCastTapPath({ castConnected, remoteConnected, preferFling, castSdkReady })];

  return (
    <div
      ref={containerRef}
      data-player-container
      className="absolute inset-0 w-full h-full bg-black"
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
    >
      {/* The local player suppresses its own native cast button
          (disableRemotePlayback): casting is driven either by the Cast SDK
          (desktop) or by the dedicated hidden cast <video> below (Android). */}
      <video ref={(el) => { videoRef.current = el; if (el) el.disableRemotePlayback = true; onVideoRef?.(el); }} className="w-full h-full" playsInline style={brightness !== 1 ? { filter: `brightness(${brightness})` } : undefined} />
      {/* A bitmap track cannot ride a <track> element, so it is drawn here.
          Only when it is NOT a burn-in: a burn-in is already in the pixels and
          drawing it again would double every line. */}
      <PGSSubtitleOverlay
        mediaId={mediaId}
        trackIndex={selectedBurnInTrack === null ? pgsSubtitleTrack : null}
        streamToken={streamToken}
        videoRef={videoRef}
        offsetMs={subtitleOffsetMs}
      />
      {/* A text track the server is still extracting (202): said where the
          cues would be, instead of a subtitle menu entry that shows nothing
          for minutes. The bitmap overlay above says the same for its tracks. */}
      {subtitlePreparing && (
        <div className="pointer-events-none absolute inset-x-0 bottom-[8%] flex justify-center">
          <span className="rounded bg-black/70 px-3 py-1 text-sm text-white/90" data-testid="subtitle-preparing">
            {t("player.subtitlesPreparing")}
          </span>
        </div>
      )}

      {/* Dedicated cast surface for Chrome Android's Remote Playback API. Kept
          in the DOM (some UAs mishandle prompt() on a never-rendered element)
          but visually gone. It carries a real HLS/direct source URL - the only
          thing that can be flung to a Chromecast - while the main player stays
          on MSE. muted+playsInline so it never makes noise if the UA autoplays. */}
      {/* preload: Chromium never opens the fling picker while this element's
          duration is NaN (see castVideoPreload) - metadata MUST load on the
          platforms where the fling is the tap's path. */}
      <video ref={(el) => { castVideoRef.current = el; setCastVideoEl(el); }} preload={castVideoPreload(preferFling)} muted playsInline aria-hidden="true" tabIndex={-1} style={{ position: "absolute", width: 1, height: 1, opacity: 0, pointerEvents: "none" }} />

      {/* Top-right cluster: Watch together. The Cast trigger used to live
          here too, as a separate button from the bottom bar's Play On picker
          - both did "send this playback elsewhere". They're now one menu,
          in the bottom control bar (see PlayerControls' RemotePlaybackMenu),
          so nothing else needs to render at this position. Fades with the
          control bars (auto-hide) so it doesn't stay pinned on screen during
          playback. */}
      {showWatchTogether && onWatchTogether && (
        <div
          className={`absolute top-4 right-4 z-[11] flex items-center gap-2 [--fade-duration:300ms] ${fadeClasses(controlsVisible, controlsParked)}`}
          onMouseMove={() => revealControlsRef.current?.()}
          // Reveal on focus, exactly as PlayerControls does for the bottom bar.
          // opacity-0 hides these buttons but leaves them in the tab order, and
          // pointer-events-none does not block the keyboard - so without this a
          // keyboard user tabbing after the controls auto-hide lands on "Watch
          // together" with no focus ring and nothing on screen, and Enter
          // still fires it.
          //
          // Revealing rather than removing it from the tab order: hiding it
          // from the keyboard would make it unreachable without a mouse.
          onFocus={() => revealControlsRef.current?.()}
        >
          <button
            aria-label={t("player.watchTogether")}
            onClick={onWatchTogether}
            className="flex items-center gap-2 px-3 py-2 rounded-full text-white text-sm font-medium transition-opacity-fast hover:opacity-80"
            style={{ backgroundColor: "var(--overlay-bg)", border: "1px solid var(--overlay-btn-bg)" }}
          >
            <Users size={ICON.md} aria-hidden="true" />
            {t("player.watchTogether")}
          </button>
        </div>
      )}
      <GestureFeedback seekFlash={seekFlash} swipeIndicator={swipeIndicator} actionFlash={actionFlash} />
      {adminMessage && <AdminMessageBanner message={adminMessage} />}
      {isBuffering && !isPlaying && <BufferingIndicator />}
      {fatalError && (
        <FatalErrorOverlay
          mediaId={mediaId}
          streamToken={streamToken}
          title={title}
          onRetry={() => { setFatalError(false); window.location.reload(); }}
        />
      )}
      {shedReason && !fatalError && <ShedOverlay shedReason={shedReason} onRetry={handleShedRetry} />}
      <ProgressReporter
        sessionId={sessionId}
        videoRef={videoRef}
        transcodeOffsetRef={transcodeOffsetRef}
        mediaId={mediaId}
        mediaType={mediaType}
        onRemoteStop={onClose}
        onRemoteMessage={showAdminMessage}
        cast={castConnected && castPlayback.isMediaLoaded ? {
          active: true,
          positionSec: castPlayback.currentTime,
          isPaused: castPlayback.isPaused,
          volume: castPlayback.volume,
          isMuted: castPlayback.isMuted,
          deviceName: castDeviceName ?? "",
          playPause: castPlayPause,
          stop: stopCast,
        } : undefined}
        remoteActive={remotePlaybackActive}
      />
      <AutoMarkWatched
        mediaId={mediaId}
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        transcodeOffsetRef={transcodeOffsetRef}
      />
      <SkipIntroButton
        mediaId={mediaId}
        filePath={filePath}
        currentTime={overlayTime}
        onSkip={(t) => handleSeekRequest(t)}
        onNextEpisode={nextEpisode ? () => handleNextEpisode(nextEpisode.id) : undefined}
        hidden={nextCardVisible}
        autoAdvance={autoplayNext}
        cancelCountdownRef={cancelCountdownRef}
      />
      {nextEpisode && (
        <NextEpisodeOverlay
          nextEpisode={nextEpisode}
          currentTime={overlayTime}
          duration={overlayDuration}
          onPlayNow={handleNextEpisode}
          onCancel={() => {}}
          suppressAutoAdvance={sleepTimer.state?.kind === "endOfEpisode"}
          autoAdvance={autoplayNext}
          onVisibleChange={setNextCardVisible}
        />
      )}
      <PlayerProvider
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        audioStreams={menuAudioStreams}
        subtitleStreams={subtitleStreams}
        title={title}
        currentAudioIndex={currentAudioIndex}
        currentQuality={currentQuality}
        autoQuality={autoQuality}
        autoQualityLabel={autoQualityLabel}
        currentSubtitleTrackIndex={currentSubtitle}
        playbackSpeed={playbackSpeed}
        seasons={seasons}
        currentSeasonId={currentSeasonId}
        episodes={episodes}
        currentEpisodeId={currentEpisodeId}
        onPlayEpisode={onEpisodeChange}
        chapters={chapters}
        mediaId={mediaId}
        mediaType={mediaType}
        transcodeOffset={transcodeOffsetRef.current}
        skipForwardSeconds={skipFwd}
        skipBackwardSeconds={skipBwd}
        subtitleStyle={subtitleStyle}
        subtitleOffsetMs={subtitleOffsetMs}
        onSubtitleOffsetChange={setSubtitleOffsetMs}
        audioOffsetMs={audioOffsetMs}
        onAudioOffsetChange={setAudioOffsetMs}
        audioOffsetAvailable={transcodeMode}
        streamToken={streamToken}
        brightness={brightness}
        availableQualities={hlsLevelSizes}
        remotePlaybackActive={remotePlaybackActive}
      >
        <PlayerControls
          streamInfo={streamInfo}
          wasmAudioActive={wasmAudioActive}
          wasmAudioLeg={wasmAudioLeg}
          servedVariant={served}
          audioTrackIndex={audioTrackOverrideRef.current ?? currentTrackIndex}
          onClose={onClose}
          onAudioChange={handleAudioChange}
          onQualityChange={handleQualityChange}
          onAutoQualityToggle={handleAutoQualityToggle}
          onSubtitleChange={handleSubtitleChange}
          onSpeedChange={setPlaybackSpeed}
          onSeasonChange={onSeasonChange}
          onEpisodeChange={onEpisodeChange}
          onSeekRequest={handleSeekRequest}
          onSubtitleStyleChange={updateSubtitleStyle}
          onExternalSubtitle={(blobUrl, label) => {
            setCurrentSubtitle(null);
            setExternalSubUrl({ url: blobUrl, label });
          }}
          onPanelOpenChange={(anyOpen) => { anyPanelOpenRef.current = anyOpen; }}
          closePanelRef={closePanelRef}
          controlsToggleRef={controlsToggleRef}
          onVisibilityChange={setControlsVisible}
          revealRef={revealControlsRef}
          pipToggleRef={pipToggleRef}
          onActionFlash={showActionFlash}
          onBrightnessChange={setBrightnessValue}
          castConnected={castConnected}
          castDeviceName={castDeviceName}
          castPlayback={castPlayback}
          onCastPlayPause={castPlayPause}
          onCastSeek={castSeek}
          onCastSetVolume={castSetVolume}
          onCastSetMuted={castSetMuted}
          castAvailable={castAvailable}
          remoteConnected={remoteConnected}
          onCastTrigger={handleCastTap}
          isBuffering={isBuffering}
          isPlaying={isPlaying}
          autoplayNext={autoplayNext}
          onAutoplayNextChange={nextEpisode || (episodes && episodes.length > 0) ? handleAutoplayNextChange : undefined}
          sleepTimerState={sleepTimer.state}
          onSleepTimerMinutes={sleepTimer.setMinutes}
          onSleepTimerEndOfEpisode={sleepTimer.setEndOfEpisode}
          onSleepTimerCancel={sleepTimer.cancel}
        />
      </PlayerProvider>
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
