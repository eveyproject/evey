import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Hls from "hls.js";
import {
  formatAudioTrackLabel,
  formatTime,
  trackDisplayName,
  useLibraryServiceGetMediaItem,
  pickPreferredStreamIndex,
  pickDefaultSubtitle,
  isBurnInSubtitle,
  burnInSubtitleIndex as pickBurnInSubtitle,
  clientSubtitleTrack,
  type V1MediaStream,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  useMediaSegments, useNextEpisode, useProgressReporter, usePlaybackSettings, useChapters, useActiveProfile,
  clampSeekTarget, holdScrubRate, tapSeekStep, SEEK_COMMIT_DELAY_MS, HOLD_SCRUB_TICK_MS,
} from "@evey/client";
import { useTvPlaySession, prefersNativeHLS, type ShedReason } from "@evey/client/web";
import { useTvKeys } from "../tv/focus";
import { Spinner, Button } from "../components/ui";
import { FocusRoot } from "../tv/focus";
import { goBack, replace } from "../app/router";
import { SubtitleOverlay } from "../player/SubtitleOverlay";
import { TrickplayTile } from "../player/TrickplayTile";
import { SettingsPanel, chapterLabel, type PanelSection } from "../player/SettingsPanel";
import { useMusicQueueStore } from "../music/queue";

const OSD_HIDE_MS = 4000;
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];
const QUALITIES = [0, 2160, 1080, 720, 480];
// No wasm audio engine here: the server transcodes AC-3/E-AC-3 to AAC.
const TV_DEVICE = { deviceId: "tv-browser", deviceName: "TV Browser", clientName: "Evey TV", wasmAudio: false };

const SHED_KEYS: Record<Exclude<ShedReason, null>, { title: string; body: string }> = {
  too_many_streams: { title: "player.shed.tooManyStreams.title", body: "player.shed.tooManyStreams.body" },
  file_not_ready: { title: "player.shed.fileNotReady.title", body: "player.shed.fileNotReady.body" },
  calibrating: { title: "player.shed.calibrating.title", body: "player.shed.calibrating.body" },
  file_needs_remux: { title: "player.shed.fileNeedsRemux.title", body: "player.shed.fileNeedsRemux.body" },
};

function csv(v: string | undefined): string[] {
  return (v ?? "").split(",").map((s) => s.trim()).filter(Boolean);
}

// The television player, driven by the remote alone: OK shows the bar and
// pauses, left/right seek (taps add up, a hold scrubs with a picture),
// down opens the settings (audio, subtitles, quality, speed, chapters),
// back leaves. hls.js on MSE with the master the shared session negotiated;
// a track or quality change restarts the session, as on the Android app.
export function PlayerScreen({ mediaId, restart, audio, subtitle }: {
  mediaId: string;
  restart: boolean;
  /** The tracks the title page chose before playing, as on the television. */
  audio?: number;
  subtitle?: number | "off";
}) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const { profile } = useActiveProfile();
  const { streamUrl, sessionId, streamInfo, isLoading, isError, shedReason, progress, restartWithTracks } =
    useTvPlaySession(mediaId, TV_DEVICE, {
      ...(audio !== undefined ? { audioStreamIndex: audio } : {}),
      ...(typeof subtitle === "number" ? { subtitleStreamIndex: subtitle } : {}),
    });
  const { data: skip } = usePlaybackSettings();
  const { data: segments } = useMediaSegments(mediaId);
  const { data: chapters } = useChapters(mediaId);
  const [osd, setOsd] = useState(true);
  const [panel, setPanel] = useState(false);
  const [paused, setPaused] = useState(false);
  const [time, setTime] = useState(0);
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
    hideTimer.current = setTimeout(() => setOsd(false), OSD_HIDE_MS);
  }, []);

  // --- tracks ---------------------------------------------------------------
  const audioStreams = useMemo(
    () => (streamInfo?.mediaStreams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
      .map((s) => ({ index: s.index ?? 0, language: s.language, title: s.title, codec: s.codec, channels: s.channels, isDefault: s.isDefault })),
    [streamInfo],
  );
  const subtitleStreams = useMemo(
    () => (streamInfo?.mediaStreams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
      .map((s, i) => ({ index: s.index ?? 0, trackIndex: i, language: s.language, title: s.title, codec: s.codec, isDefault: s.isDefault, isForced: s.isForced, burnIn: isBurnInSubtitle(s.codec) })),
    [streamInfo],
  );
  const [selectedAudio, setSelectedAudio] = useState<number | null>(audio ?? null);
  const [selectedSubtitle, setSelectedSubtitle] = useState<number | null>(subtitle === "off" ? null : subtitle ?? null);
  const userPickedAudio = useRef(audio !== undefined);
  const userPickedSubtitle = useRef(subtitle !== undefined);
  const profileAudioLangs = useMemo(() => csv(profile?.preferences?.["audio_languages"]), [profile]);
  const profileSubLangs = useMemo(() => csv(profile?.preferences?.["subtitle_languages"]), [profile]);

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

  const nextEp = useNextEpisode({
    media,
    currentTime: time,
    duration,
    onNavigate: (id) => replace({ name: "player", mediaId: id }),
  });

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !streamUrl) return;
    setFailed(false);
    const startAt = resumeAtRef.current ?? (restart ? 0 : Number(progress?.positionMs ?? 0) / 1000);
    resumeAtRef.current = null;
    const seekToStart = () => {
      if (seekedToStart.current) return;
      seekedToStart.current = true;
      if (startAt > 0) video.currentTime = startAt;
      video.playbackRate = speed;
      void video.play().catch(() => setPaused(true));
    };

    if (streamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY" || prefersNativeHLS() || !Hls.isSupported()) {
      video.src = streamUrl;
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
      // A 410 is the server saying the master expired (a sealed fingerprint):
      // negotiate again where we are rather than hammering the dead url.
      if (data.response?.code === 410) { restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality); return; }
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
      else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
      else setFailed(true);
    });
    hls.loadSource(streamUrl);
    hls.attachMedia(video);
    return () => {
      hls.destroy();
      hlsRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the stream loads once per url; tracks/quality restart through the session
  }, [streamUrl, streamInfo?.playMethod, restart]);

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
    return () => { if (hideTimer.current) clearTimeout(hideTimer.current); };
  }, [showOsd]);

  // --- seek gesture (same rules as the Android TV player) -------------------
  const [preview, setPreview] = useState<number | null>(null);
  const previewRef = useRef<number | null>(null);
  const timeRef = useRef(0);
  const durationRef = useRef(0);
  timeRef.current = time;
  durationRef.current = duration;
  const tapCountRef = useRef(0);
  const commitTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const holdRef = useRef<{ dir: 1 | -1; startedAt: number; lastAt: number; timer: ReturnType<typeof setInterval> } | null>(null);

  const setPreviewSec = useCallback((sec: number) => { previewRef.current = sec; setPreview(sec); }, []);
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
    commitTimer.current = setTimeout(() => { commitTimer.current = null; commitSeek(); }, SEEK_COMMIT_DELAY_MS);
  }, [commitSeek]);
  const seekBy = useCallback((sec: number) => {
    const base = previewRef.current ?? timeRef.current;
    tapCountRef.current += 1;
    const step = Math.sign(sec) * tapSeekStep(tapCountRef.current, Math.abs(sec));
    setPreviewSec(clampSeekTarget(base, step, durationRef.current));
    scheduleCommit();
    showOsd();
  }, [setPreviewSec, scheduleCommit, showOsd]);
  const seekTo = useCallback((sec: number) => {
    const video = videoRef.current;
    if (video) video.currentTime = clampSeekTarget(0, sec, durationRef.current);
    showOsd();
  }, [showOsd]);
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

  // --- remote ---------------------------------------------------------------
  useEffect(() => {
    if (panel || shedReason) return;
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
      if (e.key === "ArrowDown" || e.keyCode === 40) {
        e.preventDefault();
        // The one way into the options, and the bar says so on screen.
        if (osd) setPanel(true);
        else showOsd();
        return;
      }
      if (e.key === "ArrowUp" || e.keyCode === 38) {
        e.preventDefault();
        showOsd();
      }
    };
    const onKeyUp = (e: KeyboardEvent) => { if (dirOf(e) !== 0) endHold(); };
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
    };
  }, [seekBy, beginHold, endHold, togglePlay, showOsd, skip, nextEp, osd, panel, shedReason]);

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
  const audioLabel = (s: V1MediaStream, fallback: string) => formatAudioTrackLabel(s, fallback);
  const subtitleLabel = (s: V1MediaStream, fallback: string) =>
    `${trackDisplayName(s, fallback)}${s.isForced ? ` - ${t("media.streams.forced")}` : ""}`;
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
      choices: SPEEDS.map((s) => ({ key: String(s), label: `${s}x`, selected: s === speed })),
      onPick: (k) => {
        const s = Number(k);
        setSpeed(s);
        if (videoRef.current) videoRef.current.playbackRate = s;
      },
    });
    if (chapters && chapters.length > 0) {
      const currentIdx = chapters.findIndex((c, i) => time * 1000 >= c.startTimeMs && (i === chapters.length - 1 || time * 1000 < chapters[i + 1].startTimeMs));
      out.push({
        key: "chapters",
        label: t("player.controls.chapters", "Chapters"),
        value: currentIdx >= 0 ? chapters[currentIdx].title : "",
        choices: chapters.map((c, i) => ({ key: String(i), label: chapterLabel(c.title, c.startTimeMs), selected: i === currentIdx })),
        onPick: (k) => seekTo(chapters[Number(k)].startTimeMs / 1000),
      });
    }
    return out;
  }, [audioStreams, subtitleStreams, selectedAudio, selectedSubtitle, burnIn, quality, speed, chapters, time, t, restartKeepingPlace, seekTo]);

  // --- render ---------------------------------------------------------------
  const shown = preview ?? time;
  const pct = duration > 0 ? Math.min(100, (shown / duration) * 100) : 0;
  const barVisible = osd || paused || panel;
  const endsAt = duration > 0 ? new Date(Date.now() + ((duration - shown) * 1000) / (speed || 1)).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : null;

  if (shedReason) {
    const keys = SHED_KEYS[shedReason];
    return (
      <div className="player">
        <FocusRoot autoFocus className="screen screen-center">
          <h1 className="title">{t(keys.title)}</h1>
          <p className="subtitle">{t(keys.body)}</p>
          <div className="actions">
            <Button variant="primary" autoFocus onPress={() => restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality)}>{t("player.retry", "Retry")}</Button>
            <Button onPress={goBack}>{t("common.back", "Back")}</Button>
          </div>
        </FocusRoot>
      </div>
    );
  }

  return (
    <div className="player">
      <video ref={videoRef} playsInline autoPlay />
      <SubtitleOverlay mediaId={mediaId} subtitleIndex={clientSubtitle} currentTimeSec={time} />
      {(isLoading && !streamUrl) ? <div className="screen screen-center" style={{ position: "absolute", inset: 0 }}><Spinner /></div> : null}
      {(isError || failed) ? (
        <FocusRoot autoFocus className="screen screen-center" >
          <p className="error">{t("player.playbackFailed", "This title could not be played.")}</p>
          <div className="actions">
            <Button variant="primary" autoFocus onPress={() => { setFailed(false); restartKeepingPlace(selectedAudio, burnIn ?? undefined, quality); }}>{t("player.retry", "Retry")}</Button>
            <Button onPress={goBack}>{t("common.back", "Back")}</Button>
          </div>
        </FocusRoot>
      ) : null}
      {chapters && chapters.length > 0 && barVisible ? (
        <div className="osd-chapter">{chapters.find((c, i) => shown * 1000 >= c.startTimeMs && (i === chapters.length - 1 || shown * 1000 < chapters[i + 1].startTimeMs))?.title ?? ""}</div>
      ) : null}
      <div className={`osd ${barVisible ? "" : "hidden"}`}>
        <div className="osd-top">
          <div className="title" style={{ fontSize: "2vw" }}>{media?.seriesTitle ? `${media.seriesTitle} · ` : ""}{media?.title}</div>
          <div className="osd-meta">
            {streamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY" ? t("player.quality.original", "Original") : quality ? `${quality}p` : t("player.quality.auto", "Auto")}
            {speed !== 1 ? ` · ${speed}x` : ""}
            {endsAt ? ` · ${t("player.controls.endsAt", { time: endsAt, defaultValue: `Ends at ${endsAt}` })}` : ""}
          </div>
        </div>
        <div className="bar-wrap">
          <div className="trickplay-anchor" style={{ left: `${pct}%` }}>
            <TrickplayTile mediaId={mediaId} previewTimeSec={preview} warm={barVisible} />
          </div>
          <div className="bar">
            <div style={{ width: `${pct}%` }} />
            {duration > 0 && (chapters ?? []).map((c, i) => {
              const p = (c.startTimeMs / 1000 / duration) * 100;
              return p > 0 && p < 100 ? <i key={i} className="notch" style={{ left: `${p}%` }} /> : null;
            })}
          </div>
        </div>
        <div className="times">
          <span>{paused ? "❚❚ " : "▶ "}{formatTime(shown)}{preview !== null ? ` (${preview >= time ? "+" : "-"}${formatTime(Math.abs(preview - time))})` : ""}</span>
          <span className="muted">{t("player.osd.downHint", "▼ Settings")}</span>
          <span>{nextEp.visible ? `${t("player.controls.nextEpisode", "Next episode")}${nextEp.ended ? ` · ${nextEp.countdown}` : ""}` : formatTime(duration)}</span>
        </div>
      </div>
      {panel ? <SettingsPanel sections={sections} onClose={() => { setPanel(false); showOsd(); }} /> : null}
    </div>
  );
}
