import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { eveyFetchRaw } from "@evey/api";
import { useAuthStore, useMusicPlayerStore } from "@evey/store";
import { Music, Play, Pause, SkipBack, SkipForward, X, Volume2, VolumeX, Shuffle } from "lucide-react";

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

function MiniTrackArt({ trackID, coverArtURL }: { trackID: string; coverArtURL?: string }) {
  const [error, setError] = useState(false);
  // Pin token at trackID-change time: subscribing to accessToken would
  // re-fetch the cover art on every 15-min token refresh.
  const src = useMemo(() => {
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${trackID}/art${tp}`;
  }, [trackID]);
  const size = 48;
  if (error || !coverArtURL) {
    return (
      <div
        className="flex items-center justify-center bg-[var(--surface)] rounded-[var(--radius-sm)]"
        style={{ width: size, height: size, minWidth: size }}
      >
        <Music size={size * 0.5} className="text-[var(--text-muted)] opacity-50" />
      </div>
    );
  }
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      width={size}
      height={size}
      className="object-cover rounded-[var(--radius-sm)] bg-[var(--surface)]"
      style={{ width: size, height: size, minWidth: size }}
      onError={() => setError(true)}
    />
  );
}

export function GlobalAudioPlayer() {
  const { t } = useTranslation();
  const playing = useMusicPlayerStore((s) => s.playing);
  const queue = useMusicPlayerStore((s) => s.queue);
  const shuffle = useMusicPlayerStore((s) => s.shuffle);
  const externallyPaused = useMusicPlayerStore((s) => s.externallyPaused);
  const toggleShuffle = useMusicPlayerStore((s) => s.toggleShuffle);
  const next = useMusicPlayerStore((s) => s.next);
  const prev = useMusicPlayerStore((s) => s.prev);
  const close = useMusicPlayerStore((s) => s.close);
  const source = useMusicPlayerStore((s) => s.source);
  const navigate = useNavigate();
  // The audio element lives in the layout (outside the router Outlet), so this
  // navigation swaps the page content without remounting <audio> — playback is
  // never interrupted.
  const goToSource = !source
    ? undefined
    : () => {
        if (source.type === "playlist") {
          void navigate({ to: "/music", search: { tab: "playlists", playlistId: source.id } });
        } else if (source.type === "artist") {
          void navigate({ to: "/music", search: { tab: "tracks", artist: source.id } });
        } else {
          void navigate({ to: "/music", search: { tab: "tracks", album: source.id } });
        }
      };
  // Pin token at track-change time: subscribing to accessToken would rebuild
  // src on every refresh cycle, which aborts the in-flight audio stream.
  const src = useMemo(() => {
    if (!playing) return undefined;
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${playing.ID}/stream${tp}`;
  }, [playing?.ID]);

  const audioRef = useRef<HTMLAudioElement>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  // Ref mirrors keep the latest value accessible from the track-change effect
  // without re-triggering play() on every volume/mute change.
  const volumeRef = useRef(volume);
  volumeRef.current = volume;
  const mutedRef = useRef(muted);
  mutedRef.current = muted;
  // Mirror so the track-change effect can read the latest paused intent
  // without listing externallyPaused as a dep (which would re-fire play()).
  const externallyPausedRef = useRef(externallyPaused);
  externallyPausedRef.current = externallyPaused;

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !playing) return;
    // The <audio> element is keyed by track id, so on a track change it has
    // remounted with currentTime=0 / duration=NaN. Reset the mirrored state
    // immediately so the seek bar doesn't briefly show the previous track's
    // length (letting the user drag past the new track's end) before
    // onLoadedMetadata fires.
    setCurrentTime(0);
    setDuration(0);
    audio.volume = volumeRef.current;
    audio.muted = mutedRef.current;
    // Respect an active external pause (e.g. a trailer modal): don't start
    // playback that the externally-paused effect would immediately stop —
    // that race produced a brief blip of audio.
    if (externallyPausedRef.current) {
      setIsPlaying(false);
      return;
    }
    (async () => {
      try {
        await audio.play();
        setIsPlaying(true);
      } catch (e: unknown) {
        setIsPlaying(false);
        if (e instanceof DOMException && e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
      }
    })();
  }, [playing?.ID]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.muted = muted;
  }, [muted]);

  // React to externally-forced pause (e.g. trailer modal opened).
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (externallyPaused) {
      audio.pause();
    } else if (playing) {
      void audio.play().catch(() => { /* autoplay may be blocked */ });
    }
  }, [externallyPaused, playing?.ID]);

  if (!playing) return null;

  async function togglePlay() {
    const audio = audioRef.current;
    if (!audio) return;
    if (isPlaying) {
      audio.pause();
      setIsPlaying(false);
    } else {
      try {
        await audio.play();
        setIsPlaying(true);
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
      }
    }
  }

  function seek(e: React.ChangeEvent<HTMLInputElement>) {
    const audio = audioRef.current;
    if (!audio) return;
    const val = Number(e.target.value);
    audio.currentTime = val;
    // Read back: the browser clamps to the real duration, so reflect the
    // actual position rather than the raw slider value.
    setCurrentTime(audio.currentTime);
  }

  const currentIndex = queue.findIndex((q) => q.ID === playing.ID);

  return (
    <div className="fixed bottom-0 left-0 right-0 flex items-center gap-2 sm:gap-4 px-3 sm:px-6 py-3 bg-[var(--surface)] border-t border-[var(--border)] z-overlay">
      <audio
        ref={audioRef}
        key={playing.ID}
        src={src}
        onTimeUpdate={() => setCurrentTime(audioRef.current?.currentTime ?? 0)}
        onLoadedMetadata={() => setDuration(audioRef.current?.duration ?? 0)}
        onEnded={() => {
          // A finished track counts as a listen — feeds the playlist generator's
          // taste signal. Fire-and-forget; never block playback advance.
          void eveyFetchRaw({ url: `/api/v1/music/tracks/${playing.ID}/played`, method: "POST" }).catch(() => {});
          next();
        }}
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
      />
      <div className="flex items-center gap-3 min-w-0 w-[120px] sm:w-[200px]">
        <MiniTrackArt trackID={playing.ID} coverArtURL={playing.CoverArtURL} />
        {goToSource ? (
          <button
            type="button"
            onClick={goToSource}
            title={source!.label}
            aria-label={source!.label}
            className="group min-w-0 text-left bg-transparent border-none cursor-pointer p-0"
          >
            <p className="font-medium truncate text-sm text-[var(--text)] group-hover:text-[var(--accent)] transition-colors-fast">{playing.Title || "—"}</p>
            <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "—"}</p>
          </button>
        ) : (
          <div className="min-w-0">
            <p className="font-medium truncate text-sm text-[var(--text)]">{playing.Title || "—"}</p>
            <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "—"}</p>
          </div>
        )}
      </div>
      <div className="flex flex-col items-center gap-1 flex-1">
        <div className="flex items-center gap-4">
          <button
            onClick={toggleShuffle}
            className={`bg-transparent border-none cursor-pointer ${shuffle ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}
            aria-label={t("media.music.shuffle")}
            aria-pressed={shuffle}
            title={t("media.music.shuffle")}
          >
            <Shuffle size={16} aria-hidden="true" />
          </button>
          <button onClick={prev} disabled={!shuffle && currentIndex <= 0} className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer" aria-label={t("common.previous")}>
            <SkipBack size={18} aria-hidden="true" />
          </button>
          <button onClick={togglePlay} className="bg-[var(--accent)] border-none cursor-pointer text-[var(--filter-active-text)] rounded-full w-9 h-9 flex items-center justify-center" aria-label={isPlaying ? t("common.aria.pause") : t("common.aria.play")}>
            {isPlaying ? <Pause size={16} aria-hidden="true" /> : <Play size={16} aria-hidden="true" />}
          </button>
          <button onClick={next} disabled={!shuffle && currentIndex >= queue.length - 1} className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer" aria-label={t("common.next")}>
            <SkipForward size={18} aria-hidden="true" />
          </button>
        </div>
        <div className="flex items-center gap-2 w-full max-w-[200px] sm:max-w-[400px]">
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8 text-right">{formatDuration(currentTime)}</span>
          <input type="range" min={0} max={duration || 0} step={1} value={currentTime} onChange={seek} aria-label={t("media.music.seekPosition")} className="flex-1 accent-[var(--accent)]" />
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8">{duration > 0 ? formatDuration(duration) : "—"}</span>
        </div>
      </div>
      <div className="hidden sm:flex items-center gap-2 min-w-[100px]">
        <button
          onClick={() => setMuted((m) => !m)}
          className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast p-0 flex items-center"
          aria-label={muted ? t("media.music.unmute") : t("media.music.mute")}
          aria-pressed={muted}
          title={muted ? t("media.music.unmute") : t("media.music.mute")}
        >
          {muted || volume === 0 ? <VolumeX size={16} aria-hidden="true" /> : <Volume2 size={16} aria-hidden="true" />}
        </button>
        <input type="range" min={0} max={1} step={0.02} value={volume} onChange={(e) => setVolume(Number(e.target.value))} aria-label={t("media.music.volume")} className="w-[72px] accent-[var(--accent)]" />
      </div>
      <button onClick={close} className="text-[var(--text-muted)] bg-transparent border-none cursor-pointer" aria-label={t("common.aria.close")} title={t("common.aria.close")}>
        <X size={18} aria-hidden="true" />
      </button>
    </div>
  );
}
