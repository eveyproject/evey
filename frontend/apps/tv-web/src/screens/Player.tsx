import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Hls from "hls.js";
import { formatTime, getBaseURL, useLibraryServiceGetMediaItem } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useMediaSegments, useNextEpisode, useProgressReporter, usePlaybackSettings, clampSeekTarget, holdScrubRate, tapSeekStep, SEEK_COMMIT_DELAY_MS, HOLD_SCRUB_TICK_MS } from "@evey/client";
import { usePlaySession, prefersNativeHLS } from "@evey/client/web";
import { useTvKeys } from "../tv/focus";
import { Spinner } from "../components/ui";
import { goBack, replace } from "../app/router";

const OSD_HIDE_MS = 4000;
// No wasm audio engine here: the server transcodes AC-3/E-AC-3 to AAC.
const TV_DEVICE = { deviceId: "tv-browser", deviceName: "TV Browser", clientName: "Evey TV", wasmAudio: false };

// The television player, driven by the remote alone: OK shows or hides the
// bar and pauses, left/right seek, back leaves. hls.js on MSE, with the
// server's HLS master negotiated by the shared play session; a set whose
// engine plays HLS natively (rare on a TV) gets the <video> element.
export function PlayerScreen({ mediaId, restart }: { mediaId: string; restart: boolean }) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const { streamInfo, sessionId, streamToken, isLoading, isError, progress } = usePlaySession(mediaId, TV_DEVICE);
  const { data: skip } = usePlaybackSettings();
  const { data: segments } = useMediaSegments(mediaId);
  const [osd, setOsd] = useState(true);
  const [paused, setPaused] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [failed, setFailed] = useState(false);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seekedToStart = useRef(false);

  const showOsd = useCallback(() => {
    setOsd(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setOsd(false), OSD_HIDE_MS);
  }, []);

  // Progress to the server every few seconds, and the admin's remote
  // pause/play/stop commands back - the same reporter the phone runs.
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

  const nextEp = useNextEpisode({
    media,
    currentTime: time,
    duration,
    onNavigate: (id) => replace({ name: "player", mediaId: id }),
  });

  // Load the negotiated stream once the session answered.
  useEffect(() => {
    const video = videoRef.current;
    const url = streamInfo?.directPlayUrl;
    if (!video || !url || !streamToken) return;
    const absolute = /^https?:\/\//i.test(url) ? url : `${getBaseURL()}${url}`;
    setFailed(false);
    const startAt = restart ? 0 : Number(progress?.positionMs ?? 0) / 1000;
    const seekToStart = () => {
      if (seekedToStart.current) return;
      seekedToStart.current = true;
      if (startAt > 0) video.currentTime = startAt;
      void video.play().catch(() => setPaused(true));
    };

    if (streamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY" || prefersNativeHLS() || !Hls.isSupported()) {
      video.src = absolute;
      video.addEventListener("loadedmetadata", seekToStart, { once: true });
      return () => {
        video.removeEventListener("loadedmetadata", seekToStart);
        video.removeAttribute("src");
        video.load();
      };
    }

    const hls = new Hls({
      // The sliding stream token is re-read on every segment request so a
      // long film never outlives the token the manifest was opened with.
      xhrSetup: (xhr) => {
        const tok = useAuthStore.getState().accessToken;
        if (tok) xhr.setRequestHeader("Authorization", `Bearer ${tok}`);
      },
      maxBufferLength: 30,
      backBufferLength: 30,
    });
    hlsRef.current = hls;
    hls.on(Hls.Events.MANIFEST_PARSED, (_e, data) => {
      console.info("[player] manifest parsed", data.levels.map((l) => `${l.width}x${l.height}@${l.bitrate}`).join(" "), "audio tracks", data.audioTracks.length);
      seekToStart();
    });
    hls.on(Hls.Events.ERROR, (_e, data) => {
      // Every error, fatal or not, in the console: a television has no
      // devtools and this is what a bug report can carry.
      console.warn("[player] hls", data.type, data.details, data.fatal ? "fatal" : "", data.response?.code ?? "", data.reason ?? "");
      if (!data.fatal) return;
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
      else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
      else setFailed(true);
    });
    hls.loadSource(absolute);
    hls.attachMedia(video);
    return () => {
      hls.destroy();
      hlsRef.current = null;
    };
  }, [streamInfo?.directPlayUrl, streamInfo?.playMethod, streamToken, restart, progress?.positionMs]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const onTime = () => setTime(video.currentTime);
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
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current);
    };
  }, [showOsd]);

  // The seek gesture, with the same rules as the Android TV player (see
  // seekGesture in @evey/client): taps add up into ONE seek that commits once
  // the pressing stops, each further tap taking a bigger step; a held key
  // (the browser repeats it) scrubs a preview at a rate that climbs with the
  // hold and commits on release. The bar and the clock show the preview.
  const [preview, setPreview] = useState<number | null>(null);
  const previewRef = useRef<number | null>(null);
  const timeRef = useRef(0);
  const durationRef = useRef(0);
  timeRef.current = time;
  durationRef.current = duration;
  const tapCountRef = useRef(0);
  const commitTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const holdRef = useRef<{ dir: 1 | -1; startedAt: number; lastAt: number; timer: ReturnType<typeof setInterval> } | null>(null);

  const setPreviewSec = useCallback((sec: number) => {
    previewRef.current = sec;
    setPreview(sec);
  }, []);
  const commitSeek = useCallback(() => {
    const target = previewRef.current;
    previewRef.current = null;
    setPreview(null);
    tapCountRef.current = 0;
    const video = videoRef.current;
    if (video && target !== null) video.currentTime = target;
  }, []);
  const scheduleCommit = useCallback(() => {
    if (commitTimer.current) clearTimeout(commitTimer.current);
    commitTimer.current = setTimeout(() => {
      commitTimer.current = null;
      commitSeek();
    }, SEEK_COMMIT_DELAY_MS);
  }, [commitSeek]);
  const seekBy = useCallback((sec: number) => {
    const base = previewRef.current ?? timeRef.current;
    tapCountRef.current += 1;
    const step = Math.sign(sec) * tapSeekStep(tapCountRef.current, Math.abs(sec));
    setPreviewSec(clampSeekTarget(base, step, durationRef.current));
    scheduleCommit();
    showOsd();
  }, [setPreviewSec, scheduleCommit, showOsd]);
  const beginHold = useCallback((dir: 1 | -1) => {
    if (holdRef.current) return;
    if (commitTimer.current) { clearTimeout(commitTimer.current); commitTimer.current = null; }
    const now = Date.now();
    const timer = setInterval(() => {
      const hold = holdRef.current;
      if (!hold) return;
      const t = Date.now();
      const dt = (t - hold.lastAt) / 1000;
      hold.lastAt = t;
      const base = previewRef.current ?? timeRef.current;
      setPreviewSec(clampSeekTarget(base, hold.dir * holdScrubRate(t - hold.startedAt) * dt, durationRef.current));
      showOsd();
    }, HOLD_SCRUB_TICK_MS);
    holdRef.current = { dir, startedAt: now, lastAt: now, timer };
  }, [setPreviewSec, showOsd]);
  const endHold = useCallback(() => {
    const hold = holdRef.current;
    if (!hold) return;
    clearInterval(hold.timer);
    holdRef.current = null;
    scheduleCommit();
  }, [scheduleCommit]);
  useEffect(() => () => {
    if (holdRef.current) clearInterval(holdRef.current.timer);
    if (commitTimer.current) clearTimeout(commitTimer.current);
  }, []);

  const togglePlay = useCallback(() => {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
    showOsd();
  }, [showOsd]);

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

  // The remote: the five-way keys are the player's here, not the focus
  // engine's, so the handler listens itself.
  useEffect(() => {
    const forward = skip?.skipForwardSeconds ?? 30;
    const backward = skip?.skipBackwardSeconds ?? 30;
    const dirOf = (e: KeyboardEvent): 1 | -1 | 0 =>
      e.key === "ArrowRight" || e.keyCode === 39 ? 1 : e.key === "ArrowLeft" || e.keyCode === 37 ? -1 : 0;
    const onKeyDown = (e: KeyboardEvent) => {
      const dir = dirOf(e);
      if (dir !== 0) {
        e.preventDefault();
        // The browser repeats a held key: the first repeat starts the scrub,
        // the release ends it. A plain press is one tap.
        if (e.repeat) beginHold(dir);
        else if (!holdRef.current) seekBy(dir * (dir > 0 ? forward : backward));
        return;
      }
      if (e.key === "Enter" || e.keyCode === 13) {
        e.preventDefault();
        if (nextEp.visible && !nextEp.ended) nextEp.playNow();
        else togglePlay();
        return;
      }
      if (e.key === "ArrowUp" || e.key === "ArrowDown" || e.keyCode === 38 || e.keyCode === 40) {
        e.preventDefault();
        showOsd();
      }
    };
    const onKeyUp = (e: KeyboardEvent) => {
      if (dirOf(e) !== 0) endHold();
    };
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
    };
  }, [seekBy, beginHold, endHold, togglePlay, showOsd, skip, nextEp]);

  useTvKeys(useCallback((key, e) => {
    e.stopImmediatePropagation();
    switch (key) {
      case "back": case "stop": goBack(); return;
      case "play": void videoRef.current?.play(); showOsd(); return;
      case "pause": videoRef.current?.pause(); showOsd(); return;
      case "playPause": togglePlay(); return;
      case "forward": seekBy(skip?.skipForwardSeconds ?? 30); return;
      case "rewind": seekBy(-(skip?.skipBackwardSeconds ?? 30)); return;
    }
  }, [togglePlay, seekBy, showOsd, skip]));

  const shown = preview ?? time;
  const pct = duration > 0 ? Math.min(100, (shown / duration) * 100) : 0;

  return (
    <div className="player">
      <video ref={videoRef} playsInline autoPlay />
      {(isLoading && !streamInfo) ? <div className="screen screen-center" style={{ position: "absolute", inset: 0 }}><Spinner /></div> : null}
      {(isError || failed) ? (
        <div className="screen screen-center" style={{ position: "absolute", inset: 0 }}>
          <p className="error">{t("player.error", "This title could not be played.")}</p>
          <p className="muted">{t("common.back", "Back")}</p>
        </div>
      ) : null}
      <div className={`osd ${osd || paused ? "" : "hidden"}`}>
        <div className="title" style={{ fontSize: "2vw" }}>{media?.seriesTitle ? `${media.seriesTitle} · ` : ""}{media?.title}</div>
        <div className="bar"><div style={{ width: `${pct}%` }} /></div>
        <div className="times">
          <span>{paused ? "❚❚ " : "▶ "}{formatTime(shown)}{preview !== null ? ` (${preview >= time ? "+" : "-"}${formatTime(Math.abs(preview - time))})` : ""}</span>
          <span>{nextEp.visible ? `${t("player.nextEpisode", "Next episode")}${nextEp.ended ? ` · ${nextEp.countdown}` : ""}` : formatTime(duration)}</span>
        </div>
      </div>
    </div>
  );
}
