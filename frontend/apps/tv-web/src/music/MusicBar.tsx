import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatTime, getBaseURL, getFreshAccessToken } from "@evey/api";
import { coverArtUri } from "@evey/client";
import { useScreenWakeLock } from "@evey/client/web";
import { useMusicQueueStore } from "./queue";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";
import { NextIcon, PreviousIcon } from "../components/icons";

// The bar at the bottom while an album plays, on every screen: what plays,
// its progress, previous / play-pause / next / shuffle / close. One <audio>
// for the queue's lifetime; the stream token is minted fresh at every
// track change - an album outlives a 15-minute token with nobody touching
// the remote, and the next track would 401 otherwise (prod, 2026-09-21).
export function MusicBar() {
  const { t } = useTranslation();
  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const isPlaying = useMusicQueueStore((s) => s.isPlaying);
  const shuffle = useMusicQueueStore((s) => s.shuffle);
  const togglePlay = useMusicQueueStore((s) => s.togglePlay);
  const toggleShuffle = useMusicQueueStore((s) => s.toggleShuffle);
  const next = useMusicQueueStore((s) => s.next);
  const previous = useMusicQueueStore((s) => s.previous);
  const clear = useMusicQueueStore((s) => s.clear);
  const track = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex] : null;
  const trackId = track?.id ?? null;
  const audioRef = useRef<HTMLAudioElement>(null);
  // Music has no picture to hold the screen: without this the television
  // starts its screensaver, sleeps, and the sound goes with it mid-album.
  // Follows what is actually coming out, not the intention: a paused bar
  // releases the screen.
  const [audible, setAudible] = useState(false);
  useScreenWakeLock(audible);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [art, setArt] = useState<string | undefined>(undefined);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (!trackId) {
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
      setArt(undefined);
      return;
    }
    let live = true;
    void getFreshAccessToken().then((token) => {
      if (!live || !token) return;
      audio.src = `${getBaseURL()}/api/v1/music/tracks/${trackId}/stream?token=${encodeURIComponent(token)}`;
      setArt(coverArtUri(getBaseURL(), track?.coverArtUrl ? `${track.coverArtUrl}${track.coverArtUrl.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : undefined, 160));
      if (useMusicQueueStore.getState().isPlaying) void audio.play().catch(() => {});
    });
    return () => { live = false; };
  }, [trackId, track?.coverArtUrl]);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !trackId) return;
    if (isPlaying) void audio.play().catch(() => {});
    else audio.pause();
  }, [isPlaying, trackId]);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const onTime = () => setTime(audio.currentTime);
    const onDur = () => setDuration(Number.isFinite(audio.duration) ? audio.duration : 0);
    const onEnded = () => next();
    audio.addEventListener("timeupdate", onTime);
    audio.addEventListener("durationchange", onDur);
    audio.addEventListener("ended", onEnded);
    return () => {
      audio.removeEventListener("timeupdate", onTime);
      audio.removeEventListener("durationchange", onDur);
      audio.removeEventListener("ended", onEnded);
    };
  }, [next]);

  return (
    <>
      <audio
        ref={audioRef}
        onPlaying={() => setAudible(true)}
        onPause={() => setAudible(false)}
        onEnded={() => setAudible(false)}
      />
      {track ? (
        <FocusRoot focusKey="music-bar" className="music-bar">
          <div className="music-progress"><div style={{ width: `${duration > 0 ? (time / duration) * 100 : 0}%` }} /></div>
          <div className="music-row">
            {art ? <img className="music-art" src={art} alt="" /> : <div className="music-art" />}
            <div className="music-meta">
              <div className="music-title">{track.title || "-"}</div>
              <div className="muted">{[track.artist, track.album].filter(Boolean).join(" · ") || "-"} · {formatTime(time)} / {formatTime(duration || track.duration)}</div>
            </div>
            <div className="actions">
              <Button variant={shuffle ? "primary" : "ghost"} onPress={toggleShuffle}>⇄ {t("media.music.shuffle", "Shuffle")}</Button>
              <Button variant="ghost" label={t("mobile.music.previousTrack")} onPress={previous}><PreviousIcon /></Button>
              <Button variant="primary" onPress={togglePlay}>{isPlaying ? "❚❚" : "▶"}</Button>
              <Button variant="ghost" label={t("mobile.music.nextTrack")} onPress={next}><NextIcon /></Button>
              <Button variant="ghost" onPress={clear}>✕</Button>
            </div>
          </div>
        </FocusRoot>
      ) : null}
    </>
  );
}
