import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  formatAudioTrackLabel,
  trackDisplayName,
  useLibraryServiceGetMediaItem,
  pickPreferredStreamIndex,
  pickDefaultSubtitle,
  playerAudioStreams,
  playerSubtitleStreams,
  burnInSubtitleIndex as pickBurnInSubtitle,
  clientSubtitleTrack,
  createPlayerEventReporter,
  type V1MediaStream,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  useMediaSegments, useNextEpisode, useProgressReporter, usePlaybackSettings, useChapters, usePlayerProfilePrefs, useAutoMarkWatched, useRemoteSeek, nextPlaybackSpeed,
  useSleepTimer,
  useSyncplay,
  useParentalGate,
  playerTitle,
  clampSeekTarget,
  SLEEP_TIMER_PRESETS_MIN, isSleepPresetActive,
  resolveStartPositionMs,
  shedI18nKey,
  playingFromHlsLevel,
  servedVariant,
  type PlayingVariant,
} from "@evey/client";
import {
  DEFAULT_SUBTITLE_STYLE,
  SUBTITLE_BG_COLORS,
  SUBTITLE_TEXT_COLORS,
  getSubtitleStylePref,
  prefersNativeHLS,
  saveSubtitleStylePref,
  useScreenWakeLock,
  useTvPlaySession,
  type SubtitleStyle,
} from "@evey/client/web";
import { SkipSegmentButton } from "../player/SkipSegmentButton";
import { StreamInfoPanel } from "../player/StreamInfoPanel";
import type Hls from "hls.js";
import { useTvKeys, FocusRoot } from "../tv/focus";
import { Spinner, Button } from "../components/ui";
import { goBack, replace } from "../app/router";
import { SubtitleOverlay } from "../player/SubtitleOverlay";
import { PlayerOsd, BAR_BUTTONS, type BarFocus } from "../player/PlayerOsd";
import { NextEpisodeCard } from "../player/NextEpisodeCard";
import { SettingsPanel, chapterLabel, type PanelSection } from "../player/SettingsPanel";
import { useMusicQueueStore } from "../music/queue";
import { decideHlsFatal } from "../lib/hlsRecovery";
import { activeServerUrl } from "../lib/session";
import { ParentalPINModal } from "../components/ParentalPINModal";

const OSD_HIDE_MS = 4000;
// With the remote on a control the viewer is choosing: the bar waits longer.
const OSD_FOCUSED_HIDE_MS = 8000;
const QUALITIES = [0, 2160, 1080, 720, 480];
// No wasm audio engine here: the server transcodes AC-3/E-AC-3 to AAC.
const TV_DEVICE = { deviceId: "tv-browser", deviceName: "TV Browser", clientName: "Evey TV", wasmAudio: false };

// The television player, driven by the remote alone: OK shows the bar and
// pauses, left/right seek (taps add up, a hold scrubs with a picture),
// down opens the settings (audio, subtitles, quality, speed, chapters),
// back leaves. hls.js on MSE with the master the shared session negotiated;
// a track or quality change restarts the session, as on the Android app.
function PlayerView({ mediaId, restart, audio, subtitle, roomId }: {
  mediaId: string;
  restart: boolean;
  /** A watch-together room joined from the code screen. */
  roomId?: string;
  /** The tracks the title page chose before playing, as on the television. */
  audio?: number;
  subtitle?: number | "off";
}) {
  const { t, i18n } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [playingVariant, setPlayingVariant] = useState<PlayingVariant | null>(null);
  const [ladderBitrates, setLadderBitrates] = useState<number[]>([]);
  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const reportPlayerEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  const { audioLangs: profileAudioLangs, subtitleLangs: profileSubLangs, autoplayNext: autoAdvance, setAutoplayNext } = usePlayerProfilePrefs();
  const { streamUrl, sessionId, streamInfo, isLoading, isError, shedReason, progress, restartWithTracks } =
    useTvPlaySession(mediaId, TV_DEVICE, {
      ...(audio !== undefined ? { audioStreamIndex: audio } : {}),
      ...(typeof subtitle === "number" ? { subtitleStreamIndex: subtitle } : {}),
      // The encoder is pre-warmed where the player will start.
      restart,
    });
  const { data: skip } = usePlaybackSettings();
  const { data: segments } = useMediaSegments(mediaId);
  const { data: chapters } = useChapters(mediaId);
  const [osd, setOsdState] = useState(true);
  const osdRef = useRef(true);
  const setOsd = useCallback((v: boolean) => { osdRef.current = v; setOsdState(v); }, []);
  const [streamInfoOpen, setStreamInfoOpen] = useState(false);
  const [volume, setVolume] = useState(1);
  // The subtitle appearance the viewer chose, kept in this browser as the
  // phone and the Android television keep theirs.
  const [subtitleStyle, setSubtitleStyle] = useState<SubtitleStyle>(() => getSubtitleStylePref() ?? DEFAULT_SUBTITLE_STYLE);
  const applySubtitleStyle = useCallback((next: SubtitleStyle) => {
    setSubtitleStyle(next);
    saveSubtitleStylePref(next);
  }, []);
  const [panel, setPanel] = useState(false);
  const [barFocus, setBarFocusState] = useState<BarFocus | null>(null);
  const barFocusRef = useRef<BarFocus | null>(null);
  const [paused, setPaused] = useState(false);
  const [time, setTime] = useState(0);
  const committedTimeRef = useRef(0);
  // Whether anything on screen follows the clock right now: the bar, a
  // subtitle being drawn, the seek preview, or a pause.
  const needsFineTimeRef = useRef(true);
  const [duration, setDuration] = useState(0);
  const [failed, setFailed] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [quality, setQuality] = useState(0);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seekedToStart = useRef(false);
  // Where the picture was when a restart tore the stream down: the new
  // stream resumes there, not at the film's start.
  const resumeAtRef = useRef<number | null>(null);

  // A film silences the album, as on the phone and the Android TV.
  useEffect(() => { useMusicQueueStore.getState().clear(); }, [mediaId]);

  const showOsd = useCallback(() => {
    setOsd(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      setOsd(false);
      barFocusRef.current = null;
      setBarFocusState(null);
    }, barFocusRef.current ? OSD_FOCUSED_HIDE_MS : OSD_HIDE_MS);
  }, [setOsd]);
  const setBarFocus = useCallback((next: BarFocus | null) => {
    barFocusRef.current = next;
    setBarFocusState(next);
  }, []);

  // --- tracks ---------------------------------------------------------------
  const audioStreams = useMemo(() => playerAudioStreams(streamInfo?.mediaStreams), [streamInfo]);
  const subtitleStreams = useMemo(() => playerSubtitleStreams(streamInfo?.mediaStreams), [streamInfo]);
  const [selectedAudio, setSelectedAudio] = useState<number | null>(audio ?? null);
  const [selectedSubtitle, setSelectedSubtitle] = useState<number | null>(subtitle === "off" ? null : subtitle ?? null);
  const userPickedAudio = useRef(audio !== undefined);
  const userPickedSubtitle = useRef(subtitle !== undefined);

  // Profile preference first, then the file's default, then the first track.
  useEffect(() => {
    if (userPickedAudio.current || audioStreams.length === 0) return;
    const preferred = pickPreferredStreamIndex(audioStreams, profileAudioLangs);
    setSelectedAudio(preferred ?? audioStreams.find((s) => s.isDefault)?.index ?? audioStreams[0].index);
  }, [audioStreams, profileAudioLangs]);
  // The subtitle auto-pick never restarts the stream by itself: a text or
  // PGS track is drawn here, and a default bitmap track the viewer did not
  // ask for must not turn a copy into a burn-in transcode.
  useEffect(() => {
    if (userPickedSubtitle.current || subtitleStreams.length === 0) return;
    setSelectedSubtitle(pickDefaultSubtitle(subtitleStreams, profileSubLangs));
  }, [subtitleStreams, profileSubLangs]);

  const burnIn = useMemo(() => pickBurnInSubtitle(selectedSubtitle, subtitleStreams), [selectedSubtitle, subtitleStreams]);
  const clientSubtitle = useMemo(() => clientSubtitleTrack(selectedSubtitle, subtitleStreams), [selectedSubtitle, subtitleStreams]);

  const restartKeepingPlace = useCallback((audio: number | null, sub: number | undefined, q: number) => {
    resumeAtRef.current = videoRef.current?.currentTime ?? null;
    seekedToStart.current = false;
    restartWithTracks(audio ?? undefined, sub, q || undefined);
  }, [restartWithTracks]);

  // --- stream ---------------------------------------------------------------
  useProgressReporter(
    sessionId,
    () => (videoRef.current?.currentTime ?? 0) * 1000,
    () => videoRef.current?.paused ?? true,
    () => videoRef.current?.volume ?? 1,
    () => videoRef.current?.muted ?? false,
    {
      onRemotePause: () => videoRef.current?.pause(),
      onRemotePlay: () => void videoRef.current?.play(),
      onRemoteStop: () => goBack(),
    },
  );

  // The profile's own choice, as on the television, toggled from the menu.
  const toggleAutoplay = useCallback(() => { void setAutoplayNext(!autoAdvance); }, [setAutoplayNext, autoAdvance]);
  // Past 90% the title is watched: the rule every player shares.
  useAutoMarkWatched(mediaId, time, duration);
  const nextEp = useNextEpisode({
    media,
    currentTime: time,
    duration,
    autoAdvance,
    onNavigate: (id) => replace({ name: "player", mediaId: id }),
  });

  // The sleep timer the television has: a number of minutes, or the end of
  // this episode. Both stop the picture where it is.
  const sleep = useSleepTimer({ onMinutesFire: () => videoRef.current?.pause() });
  useEffect(() => {
    if (sleep.state?.kind !== "endOfEpisode") return;
    if (duration > 0 && time >= duration - 1) videoRef.current?.pause();
  }, [sleep.state, time, duration]);

  // A television sleeps on an idle remote and takes the sound with it: the
  // lock lives in @evey/client, shared with the music bar.
  useScreenWakeLock(!paused);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !streamUrl) return;
    setFailed(false);
    // A played title, or one stopped in its last seconds, starts over.
    const startAt = resumeAtRef.current ?? resolveStartPositionMs(!!restart, progress) / 1000;
    resumeAtRef.current = null;
    const seekToStart = () => {
      if (seekedToStart.current) return;
      seekedToStart.current = true;
      if (startAt > 0) video.currentTime = startAt;
      video.playbackRate = speed;
      void video.play().catch(() => setPaused(true));
    };

    // The element's own MediaError. Beaconed like every other client's: a
    // television has no console anyone reads. Without hls.js nothing else
    // watches the stream, so it is also the end of playback.
    const onVideoError = (fatal: boolean) => () => {
      const err = video.error;
      if (!err) return;
      reportPlayerEvent({
        type: "video_element_error",
        details: err.message || "",
        fatal: true,
        code: err.code,
        posSec: video.currentTime || 0,
      });
      if (fatal) setFailed(true);
    };
    const nativeError = onVideoError(true);
    // The variant decoded right now, for the stream info panel: hls.js names
    // it, the element's own HLS only shows its size.
    setPlayingVariant(null);
    setLadderBitrates([]);
    const onResize = () => {
      if (video.videoHeight > 0) setPlayingVariant({ width: video.videoWidth, height: video.videoHeight });
    };

    // hls.js is half a megabyte of JavaScript a television has to parse. It
    // is only ever needed once a stream is actually being attached, so it is
    // fetched here rather than with the application: the home, the library
    // and every other screen never pay for it.
    if (streamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY" || prefersNativeHLS()) {
      video.src = streamUrl;
      video.addEventListener("loadedmetadata", seekToStart, { once: true });
      video.addEventListener("error", nativeError);
      video.addEventListener("resize", onResize);
      return () => {
        video.removeEventListener("loadedmetadata", seekToStart);
        video.removeEventListener("error", nativeError);
        video.removeEventListener("resize", onResize);
        video.removeAttribute("src");
        video.load();
      };
    }

    let torn = false;
    // With hls.js the element error is a symptom it may recover from.
    let elementError = nativeError;
    void (async () => {
      const { default: Hls } = await import("hls.js");
      if (torn || !videoRef.current) return;
      if (!Hls.isSupported()) {
        video.src = streamUrl;
        video.addEventListener("loadedmetadata", seekToStart, { once: true });
        video.addEventListener("error", elementError);
        video.addEventListener("resize", onResize);
        return;
      }
      elementError = onVideoError(false);
      video.addEventListener("error", elementError);
      const hls = new Hls({
        // The sliding stream token is re-read on every segment request so a
        // long film never outlives the token the manifest was opened with.
        xhrSetup: (xhr) => {
          const tok = useAuthStore.getState().accessToken;
          if (tok) xhr.setRequestHeader("Authorization", `Bearer ${tok}`);
          // Which app this is: the QoE table files these requests under
          // web-tv-<platform>, where the User-Agent alone would say "tv"
          // (the native app, on a webOS set) or a desktop Chrome. Rides the
          // same preflight as the Authorization header above.
          xhr.setRequestHeader("X-Evey-Client", "tv-web");
        },
        maxBufferLength: 30,
        backBufferLength: 30,
        // CMCD on every segment request, like the web player: without it a
        // television browser never appeared in /admin/qoe's device table.
        // Its User-Agent (Tizen, webOS) files the rows under "tv".
        cmcd: {},
      });
      hlsRef.current = hls;
      // Recovery is bounded per stream (see decideHlsFatal).
      const attempts = { reload: 0, recoverMedia: 0 };
      hls.on(Hls.Events.MANIFEST_PARSED, (_e, data) => {
        console.info("[player] manifest parsed", data.levels.map((l) => `${l.width}x${l.height}@${l.bitrate}`).join(" "), "audio tracks", data.audioTracks.length);
        setLadderBitrates(data.levels.map((l) => l.bitrate));
        seekToStart();
      });
      hls.on(Hls.Events.LEVEL_SWITCHED, (_e, data) => setPlayingVariant(playingFromHlsLevel(hls.levels[data.level])));
      hls.on(Hls.Events.ERROR, (_e, data) => {
        // Every error, fatal or not, in the console: a television has no
        // devtools and this is what a bug report can carry.
        console.warn("[player] hls", data.type, data.details, data.fatal ? "fatal" : "", data.response?.code ?? "", data.reason ?? "");
        // The same beacon as the web player: every media error (the first
        // symptom of a garbled stream) and every fatal one.
        if (data.fatal || data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          const sn = data.frag?.sn;
          reportPlayerEvent({
            type: data.fatal ? "hls_fatal" : "hls_media_error",
            details: data.details,
            fatal: data.fatal,
            code: data.response?.code ?? 0,
            variant: data.frag?.level ?? 0,
            segment: typeof sn === "number" ? sn : 0,
            posSec: video.currentTime || 0,
          });
        }
        if (!data.fatal) return;
        const action = decideHlsFatal({ type: data.type, details: data.details, responseCode: data.response?.code }, attempts);
        if (action === "renegotiate") restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality);
        else if (action === "reload") { attempts.reload++; hls.startLoad(); }
        else if (action === "recoverMedia") { attempts.recoverMedia++; hls.recoverMediaError(); }
        else setFailed(true);
      });
      hls.loadSource(streamUrl);
      hls.attachMedia(video);
    })();
    return () => {
      torn = true;
      video.removeEventListener("loadedmetadata", seekToStart);
      video.removeEventListener("error", elementError);
      video.removeEventListener("resize", onResize);
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the stream loads once per url; tracks/quality restart through the session
  }, [streamUrl, streamInfo?.playMethod, restart]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    // The clock ticks four or five times a second. While the bar is hidden
    // and nothing is being drawn over the picture, nobody is reading it: a
    // whole re-render of the player for each tick is work a weak television
    // pays for every second of a film. Committed finely only when something
    // on screen follows the time, once a second otherwise.
    const onTime = () => {
      const t = video.currentTime;
      const followsTime = needsFineTimeRef.current;
      if (followsTime || Math.abs(t - committedTimeRef.current) >= 1) {
        committedTimeRef.current = t;
        setTime(t);
      }
    };
    const onDur = () => setDuration(Number.isFinite(video.duration) ? video.duration : 0);
    const onPlay = () => setPaused(false);
    const onPause = () => setPaused(true);
    const onEnded = () => { if (!nextEp.nextEpisode) goBack(); };
    video.addEventListener("timeupdate", onTime);
    video.addEventListener("durationchange", onDur);
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("ended", onEnded);
    return () => {
      video.removeEventListener("timeupdate", onTime);
      video.removeEventListener("durationchange", onDur);
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("ended", onEnded);
    };
  }, [nextEp.nextEpisode]);

  useEffect(() => {
    showOsd();
    return () => { if (hideTimer.current) clearTimeout(hideTimer.current); };
  }, [showOsd]);

  // --- seek gesture: the one every television player runs (useRemoteSeek) ---
  const timeRef = useRef(0);
  const durationRef = useRef(0);
  timeRef.current = time;
  durationRef.current = duration;
  const {
    previewSec: preview,
    bump: seekBy,
    beginHold,
    endHold,
    commitNow,
    isHolding,
  } = useRemoteSeek({
    positionSec: time,
    getPosition: () => timeRef.current,
    getDuration: () => durationRef.current,
    seekTo: (sec) => { if (videoRef.current) videoRef.current.currentTime = sec; },
    onActivity: showOsd,
  });
  const seekTo = useCallback((sec: number) => {
    const video = videoRef.current;
    if (video) video.currentTime = clampSeekTarget(0, sec, durationRef.current);
    showOsd();
  }, [showOsd]);

  // Watch together: the room drives play, pause and seek; this player's own
  // play and pause are offered to the room (the server keeps host authority).
  const syncplay = useSyncplay({
    serverUrl: activeServerUrl() ?? "",
    getLocalPositionMs: () => (videoRef.current?.currentTime ?? 0) * 1000,
    onPlay: (ms, shouldSeek) => {
      const v = videoRef.current;
      if (!v) return;
      if (shouldSeek) v.currentTime = ms / 1000;
      void v.play();
    },
    onPause: (ms, shouldSeek) => {
      const v = videoRef.current;
      if (!v) return;
      if (shouldSeek) v.currentTime = ms / 1000;
      v.pause();
    },
    onSeek: (ms) => { if (videoRef.current) videoRef.current.currentTime = ms / 1000; },
  });
  useEffect(() => {
    if (roomId && !syncplay.state.roomId) syncplay.connect(roomId);
  }, [roomId, syncplay]);

  const togglePlay = useCallback(() => {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) {
      void video.play();
      if (syncplay.state.connected) syncplay.sendPlay(video.currentTime * 1000);
    } else {
      video.pause();
      if (syncplay.state.connected) syncplay.sendPause(video.currentTime * 1000);
    }
    showOsd();
  }, [showOsd, syncplay]);

  // The intro, when the server knows where it is and the admin asked for it
  // to be skipped: jump once past it.
  const skippedIntro = useRef(false);
  useEffect(() => {
    const intro = segments?.segments.find((s) => s.type === "intro");
    if (!segments?.introAutoSkip || !intro || skippedIntro.current) return;
    const ms = time * 1000;
    if (ms >= intro.startMs && ms < intro.endMs - 1000) {
      skippedIntro.current = true;
      if (videoRef.current) videoRef.current.currentTime = intro.endMs / 1000;
    }
  }, [time, segments]);

  // --- remote ---------------------------------------------------------------
  // The Android television's model: at rest OK plays/pauses, left/right seek,
  // DOWN enters the bar, BACK shows the bar then leaves. In the bar left/right
  // walk the buttons, DOWN reaches the timeline (left/right seek), BACK exits it.
  const pressBar = useRef<(key: BarFocus) => void>(() => {});
  const cycleSpeedRef = useRef<() => void>(() => {});
  useEffect(() => {
    if (panel || shedReason || streamInfoOpen) return;
    const forward = skip?.skipForwardSeconds ?? 30;
    const backward = skip?.skipBackwardSeconds ?? 30;
    const dirOf = (e: KeyboardEvent): 1 | -1 | 0 =>
      e.key === "ArrowRight" || e.keyCode === 39 ? 1 : e.key === "ArrowLeft" || e.keyCode === 37 ? -1 : 0;
    const isEnter = (e: KeyboardEvent) => e.key === "Enter" || e.keyCode === 13;
    const isDown = (e: KeyboardEvent) => e.key === "ArrowDown" || e.keyCode === 40;
    const isUp = (e: KeyboardEvent) => e.key === "ArrowUp" || e.keyCode === 38;
    const isBack = (e: KeyboardEvent) => e.key === "Escape" || e.key === "GoBack" || e.key === "BrowserBack" || e.keyCode === 27 || e.keyCode === 461 || e.keyCode === 10009 || e.keyCode === 8;
    const seekKey = (e: KeyboardEvent, dir: 1 | -1) => {
      // The browser repeats a held key: the first repeat starts the scrub,
      // the release ends it. A plain press is one tap.
      if (e.repeat) beginHold(dir);
      else if (!isHolding()) seekBy(dir * (dir > 0 ? forward : backward));
    };
    const onKeyDown = (e: KeyboardEvent) => {
      const overlayFocused = !!document.querySelector(".skip-btn.focused, .nextep-btn.focused");
      const dir = dirOf(e);
      const bar = barFocusRef.current;
      const handled = () => { e.preventDefault(); e.stopImmediatePropagation(); };

      if (nextEp.visible && isBack(e)) { handled(); nextEp.cancel(); return; }
      if (overlayFocused && !bar && (isEnter(e) || isUp(e) || isDown(e))) return;

      if (bar) {
        handled();
        if (isBack(e)) { setBarFocus(null); showOsd(); return; }
        if (bar === "timeline") {
          if (dir !== 0) { seekKey(e, dir); return; }
          if (isUp(e)) { setBarFocus("play"); showOsd(); return; }
          if (isEnter(e)) {
            // A pending jump lands now; with nothing pending it is play/pause.
            if (!commitNow()) togglePlay();
          }
          showOsd();
          return;
        }
        const i = BAR_BUTTONS.indexOf(bar);
        if (dir !== 0) setBarFocus(BAR_BUTTONS[Math.min(BAR_BUTTONS.length - 1, Math.max(0, i + dir))]);
        else if (isDown(e)) setBarFocus("timeline");
        else if (isEnter(e)) pressBar.current(bar);
        showOsd();
        return;
      }

      if (dir !== 0) { handled(); seekKey(e, dir); return; }
      if (isEnter(e)) { handled(); togglePlay(); return; }
      if (isDown(e)) {
        handled();
        if (osdRef.current) setBarFocus("play");
        showOsd();
        return;
      }
      if (isUp(e)) { handled(); showOsd(); return; }
      if (isBack(e)) {
        handled();
        if (osdRef.current) goBack();
        else showOsd();
      }
    };
    const onKeyUp = (e: KeyboardEvent) => { if (dirOf(e) !== 0) endHold(); };
    window.addEventListener("keydown", onKeyDown, true);
    window.addEventListener("keyup", onKeyUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown, true);
      window.removeEventListener("keyup", onKeyUp);
    };
  }, [seekBy, beginHold, endHold, commitNow, isHolding, togglePlay, showOsd, setBarFocus, skip, nextEp, panel, shedReason, streamInfoOpen]);

  useTvKeys(useCallback((key, e) => {
    if (panel) return;
    e.stopImmediatePropagation();
    switch (key) {
      case "back": case "stop": goBack(); return;
      case "play": void videoRef.current?.play(); showOsd(); return;
      case "pause": videoRef.current?.pause(); showOsd(); return;
      case "playPause": togglePlay(); return;
      case "forward": seekBy(skip?.skipForwardSeconds ?? 30); return;
      case "rewind": seekBy(-(skip?.skipBackwardSeconds ?? 30)); return;
    }
  }, [togglePlay, seekBy, showOsd, skip, panel]));

  // --- settings panel --------------------------------------------------------
  // The labels the phone, the browser and the Android television print, from
  // the shared helpers: one track must not read differently per client.
  const audioLabel = useCallback((s: V1MediaStream, fallback: string) => formatAudioTrackLabel(s, fallback, i18n.language), [i18n.language]);
  const subtitleLabel = useCallback(
    (s: V1MediaStream, fallback: string) => `${trackDisplayName(s, fallback, i18n.language)}${s.isForced ? ` - ${t("media.streams.forced")}` : ""}`,
    [t, i18n.language],
  );
  const sections: PanelSection[] = useMemo(() => {
    const out: PanelSection[] = [];
    if (audioStreams.length > 1) {
      const current = audioStreams.find((s) => s.index === selectedAudio);
      out.push({
        key: "audio",
        label: t("player.settings.audio", "Audio"),
        value: current ? audioLabel(current, "") : "",
        choices: audioStreams.map((s, i) => ({ key: String(s.index), label: audioLabel(s, t("media.track", { number: s.index ?? i + 1 })), selected: s.index === selectedAudio })),
        onPick: (k) => {
          const idx = Number(k);
          if (idx === selectedAudio) return;
          userPickedAudio.current = true;
          setSelectedAudio(idx);
          restartKeepingPlace(idx, burnIn ?? undefined, quality);
        },
      });
    }
    if (subtitleStreams.length > 0) {
      const current = subtitleStreams.find((s) => s.index === selectedSubtitle);
      out.push({
        key: "subtitles",
        label: t("player.settings.subtitles", "Subtitles"),
        value: current ? subtitleLabel(current, "") : t("player.settings.off", "Off"),
        choices: [
          { key: "off", label: t("player.settings.off", "Off"), selected: selectedSubtitle === null },
          ...subtitleStreams.map((s, i) => ({ key: String(s.index), label: subtitleLabel(s, t("media.track", { number: s.index ?? i + 1 })), selected: s.index === selectedSubtitle })),
        ],
        onPick: (k) => {
          const next = k === "off" ? null : Number(k);
          if (next === selectedSubtitle) return;
          userPickedSubtitle.current = true;
          const prev = subtitleStreams.find((s) => s.index === selectedSubtitle);
          const target = subtitleStreams.find((s) => s.index === next);
          setSelectedSubtitle(next);
          // Text and PGS are drawn here: switching them touches nothing. A
          // burn-in is the server's, going to or from one restarts the stream.
          if (prev?.burnIn || target?.burnIn) restartKeepingPlace(selectedAudio, target?.burnIn ? target.index : undefined, quality);
        },
      });
    }
    if (clientSubtitle !== null) {
      out.push({
        key: "subtitleStyle",
        label: t("player.settings.subtitleStyle"),
        value: `${subtitleStyle.fontSize}%`,
        choices: [
          ...[75, 100, 125, 150, 200].map((size) => ({ key: `size:${size}`, label: `${size}%`, selected: subtitleStyle.fontSize === size })),
          ...SUBTITLE_TEXT_COLORS.map((c) => ({ key: `color:${c.value}`, label: t(c.labelKey), selected: subtitleStyle.color === c.value })),
          ...SUBTITLE_BG_COLORS.map((c) => ({
            key: `bg:${c.value}:${c.opacity ?? 75}`,
            label: t(c.labelKey),
            selected: subtitleStyle.bgColor === c.value && subtitleStyle.bgOpacity === (c.opacity ?? 75),
          })),
          { key: "position:bottom", label: t("player.subtitleStyle.bottom"), selected: subtitleStyle.position === "bottom" },
          { key: "position:top", label: t("player.subtitleStyle.top"), selected: subtitleStyle.position === "top" },
        ],
        onPick: (k) => {
          const [kind, value, extra] = k.split(":");
          if (kind === "size") applySubtitleStyle({ ...subtitleStyle, fontSize: Number(value) });
          else if (kind === "color") applySubtitleStyle({ ...subtitleStyle, color: value });
          else if (kind === "bg") applySubtitleStyle({ ...subtitleStyle, bgColor: value, bgOpacity: Number(extra) });
          else if (kind === "position") applySubtitleStyle({ ...subtitleStyle, position: value === "top" ? "top" : "bottom" });
        },
      });
    }
    out.push({
      key: "quality",
      label: t("player.settings.quality", "Quality"),
      value: quality ? `${quality}p` : t("player.settings.auto", "Auto"),
      choices: QUALITIES.map((q) => ({ key: String(q), label: q ? `${q}p` : t("player.settings.auto", "Auto"), selected: q === quality })),
      onPick: (k) => {
        const q = Number(k);
        if (q === quality) return;
        setQuality(q);
        restartKeepingPlace(selectedAudio, burnIn ?? undefined, q);
      },
    });
    out.push({
      key: "speed",
      label: t("player.settings.speed", "Speed"),
      value: `${speed}x`,
      choices: [],
      onPick: () => {},
      action: () => cycleSpeedRef.current(),
    });
    if (nextEp.previousEpisode) {
      out.push({ key: "previous", label: t("player.controls.previousEpisode"), value: "", choices: [], onPick: () => {}, action: nextEp.playPrevious });
    }
    if (nextEp.nextEpisode) {
      out.push({ key: "next", label: t("player.controls.nextEpisode"), value: "", choices: [], onPick: () => {}, action: nextEp.playNow });
    }
    if (media?.type === "MEDIA_ITEM_TYPE_EPISODE") {
      out.push({
        key: "autoplay",
        label: t("player.settings.autoplayNext"),
        value: autoAdvance ? t("common.enabled") : t("common.disabled"),
        choices: [],
        onPick: () => {},
        action: toggleAutoplay,
      });
    }
    if (chapters && chapters.length > 0) {
      // The current chapter is only read while the panel is open, and this
      // whole list used to be rebuilt on every clock tick for it.
      const currentIdx = panel
        ? chapters.findIndex((c, i) => time * 1000 >= c.startTimeMs && (i === chapters.length - 1 || time * 1000 < chapters[i + 1].startTimeMs))
        : -1;
      out.push({
        key: "chapters",
        label: t("player.controls.chapters", "Chapters"),
        value: currentIdx >= 0 ? chapters[currentIdx].title : "",
        choices: chapters.map((c, i) => ({ key: String(i), label: chapterLabel(c.title, c.startTimeMs), selected: i === currentIdx })),
        onPick: (k) => seekTo(chapters[Number(k)].startTimeMs / 1000),
      });
    }
    out.push({
      key: "volume",
      label: t("player.controls.volume"),
      value: `${Math.round(volume * 100)}%`,
      choices: [0, 25, 50, 75, 100].map((v) => ({ key: String(v), label: `${v}%`, selected: Math.round(volume * 100) === v })),
      onPick: (k) => {
        const next = Number(k) / 100;
        setVolume(next);
        if (videoRef.current) videoRef.current.volume = next;
      },
    });
    out.push({
      key: "sleep",
      label: t("player.settings.sleepTimer"),
      value: sleep.state?.kind === "minutes" && sleep.state.minutesLeft != null
        ? t("player.sleepTimer.minutes", { count: sleep.state.minutesLeft, defaultValue: `${sleep.state.minutesLeft} min` })
        : sleep.state?.kind === "endOfEpisode"
          ? t("player.sleepTimer.endOfEpisode")
          : "",
      choices: [
        { key: "off", label: t("player.sleepTimer.cancel"), selected: !sleep.state },
        ...SLEEP_TIMER_PRESETS_MIN.map((m) => ({
          key: String(m),
          label: t("player.sleepTimer.minutes", { count: m, defaultValue: `${m} min` }),
          selected: isSleepPresetActive(sleep.state, m),
        })),
        { key: "episode", label: t("player.sleepTimer.endOfEpisode"), selected: sleep.state?.kind === "endOfEpisode" },
      ],
      onPick: (k) => {
        if (k === "off") sleep.cancel();
        else if (k === "episode") sleep.setEndOfEpisode();
        else sleep.setMinutes(Number(k));
      },
    });
    if (streamInfo) {
      out.push({
        key: "streamInfo",
        label: t("player.streamInfo.title"),
        value: "",
        choices: [],
        onPick: () => {},
        action: () => { setPanel(false); setStreamInfoOpen(true); },
      });
    }
    return out;
  }, [audioStreams, subtitleStreams, selectedAudio, selectedSubtitle, burnIn, quality, speed, chapters, panel, time, t, restartKeepingPlace, seekTo, sleep, clientSubtitle, subtitleStyle, applySubtitleStyle, volume, nextEp, media?.type, autoAdvance, toggleAutoplay, streamInfo, subtitleLabel, audioLabel]);

  // --- bar -------------------------------------------------------------------
  const cycleSpeed = useCallback(() => {
    const next = nextPlaybackSpeed(speed);
    setSpeed(next);
    if (videoRef.current) videoRef.current.playbackRate = next;
    showOsd();
  }, [speed, showOsd]);
  cycleSpeedRef.current = cycleSpeed;
  const skipBack = skip?.skipBackwardSeconds ?? 30;
  const skipFwd = skip?.skipForwardSeconds ?? 30;
  pressBar.current = (key: BarFocus) => {
    if (key === "rewind") seekBy(-skipBack);
    else if (key === "forward") seekBy(skipFwd);
    else if (key === "play") togglePlay();
    else if (key === "speed") cycleSpeed();
    else if (key === "settings") { setBarFocus(null); setPanel(true); }
  };

  // --- render ---------------------------------------------------------------
  const shown = preview ?? time;
  needsFineTimeRef.current = osd || paused || panel || preview !== null || clientSubtitle !== null;
  const barVisible = osd || paused || panel || barFocus !== null;
  // Only while the bar is up: formatting a local time is not free, and it is
  // written nowhere else.
  const endsAt = barVisible && duration > 0
    ? new Date(Date.now() + ((duration - shown) * 1000) / (speed || 1)).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : null;
  const pendingDelta = preview !== null ? preview - time : 0;

  if (shedReason) {
    return (
      <div className="player">
        <FocusRoot autoFocus className="screen screen-center">
          <h1 className="title">{t(shedI18nKey(shedReason, "title"))}</h1>
          <p className="subtitle">{t(shedI18nKey(shedReason, "body"))}</p>
          <div className="actions">
            <Button variant="primary" autoFocus onPress={() => restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality)}>{t("player.retry", "Retry")}</Button>
            <Button onPress={goBack}>{t("common.back", "Back")}</Button>
          </div>
        </FocusRoot>
      </div>
    );
  }

  return (
    <div className="player" onClick={(e) => { if (e.target === e.currentTarget || (e.target as HTMLElement).tagName === "VIDEO") { if (!osdRef.current) showOsd(); else togglePlay(); } }}>
      <video ref={videoRef} playsInline autoPlay />
      <SubtitleOverlay mediaId={mediaId} subtitleIndex={clientSubtitle} currentTimeSec={time} style={subtitleStyle} />
      {(isLoading && !streamUrl) ? <div className="player-buffering"><Spinner /></div> : null}
      {(isError || failed) ? (
        <FocusRoot autoFocus className="screen screen-center player-overlay">
          <p className="error">{t("player.playbackFailed", "This title could not be played.")}</p>
          <div className="actions">
            <Button variant="primary" autoFocus onPress={() => { setFailed(false); restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality); }}>{t("player.retry", "Retry")}</Button>
            <Button onPress={goBack}>{t("common.back", "Back")}</Button>
          </div>
        </FocusRoot>
      ) : null}
      {barVisible ? (
        <PlayerOsd
          mediaId={mediaId}
          title={media ? playerTitle(media, media.seriesTitle) : ""}
          roomId={roomId}
          syncConnected={syncplay.state.connected}
          quality={quality}
          shown={shown}
          duration={duration}
          preview={preview}
          pendingDelta={pendingDelta}
          endsAt={endsAt}
          paused={paused}
          speed={speed}
          skipBack={skipBack}
          skipFwd={skipFwd}
          chapters={chapters}
          barFocus={barFocus}
          onPress={(key) => { setBarFocus(key); pressBar.current(key); }}
        />
      ) : null}
      {!panel && !streamInfoOpen ? (
        <SkipSegmentButton
          mediaId={mediaId}
          filePath={media?.filePath}
          currentTime={time}
          raised={nextEp.visible}
          onSkip={(sec) => seekTo(sec)}
          onNextEpisode={nextEp.nextEpisode ? nextEp.playNow : undefined}
        />
      ) : null}
      {nextEp.visible ? <NextEpisodeCard nextEp={nextEp} /> : null}
      {panel ? <SettingsPanel sections={sections} onClose={() => { setPanel(false); showOsd(); }} /> : null}
      {streamInfoOpen ? (
        <StreamInfoPanel streamInfo={streamInfo} served={servedVariant(streamInfo, playingVariant, ladderBitrates)} audioStreamIndex={selectedAudio} onClose={() => { setStreamInfoOpen(false); showOsd(); }} />
      ) : null}
    </div>
  );
}


type PlayerProps = Parameters<typeof PlayerView>[0];

/** The player behind the parental gate: a title reached without its page (Play On,
 * next episode, a link) still asks for the PIN before any stream opens. */
export function PlayerScreen(props: PlayerProps) {
  const { data: media, isLoading } = useLibraryServiceGetMediaItem(props.mediaId);
  const { needsPin, profileId, settingsLoading } = useParentalGate(media?.officialRating);
  const [verified, setVerified] = useState<string | null>(null);
  if (isLoading || settingsLoading) return <div className="player"><div className="player-buffering"><Spinner /></div></div>;
  if (verified !== props.mediaId && profileId && needsPin()) {
    return (
      <div className="player">
        <ParentalPINModal profileId={profileId} onClose={goBack} onVerified={() => setVerified(props.mediaId)} />
      </div>
    );
  }
  return <PlayerView key={props.mediaId} {...props} />;
}
