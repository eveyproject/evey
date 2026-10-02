import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { eveyFetchRaw, formatTime, getFreshAccessToken } from "@evey/api";
import { pollMusicRemoteCommands } from "@evey/client";
import { toast, useAuthStore, useMusicPlayerStore, type MusicTrack, type MusicSource } from "@evey/store";
import { Music, Play, Pause, SkipBack, SkipForward, X, Volume2, VolumeX, Shuffle, ListMusic } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { randomUUID } from "@/lib/randomId";

function MiniTrackArt({ trackID, coverArtURL, token }: { trackID: string; coverArtURL?: string; token?: string }) {
  const [error, setError] = useState(false);
  // The token the stream was minted with, so the art never outlives it either.
  const src = useMemo(() => {
    const tp = token ? `?token=${encodeURIComponent(token)}` : "";
    // 48px slot, 2x for high-density screens: a display-sized WebP, not the
    // full cover embedded in the file.
    return `/api/v1/music/tracks/${trackID}/art${tp}${tp ? "&" : "?"}w=96`;
  }, [trackID, token]);
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

// QueuePanel mirrors the mobile app's queue sheet (mobile is the spec here: the
// web player had the store but no UI to inspect/edit it). Kept intentionally
// minimal - list, current-track highlight, jump, remove - matching what mobile
// ships rather than adding drag-reorder mobile doesn't have either.
interface QueuePanelProps {
  queue: MusicTrack[];
  playing: MusicTrack;
  onJump: (track: MusicTrack) => void;
  onRemove: (track: MusicTrack) => void;
  onClose: () => void;
}

function QueuePanel({ queue, playing, onJump, onRemove, onClose }: QueuePanelProps) {
  const { t } = useTranslation();
  return (
    <div
      role="dialog"
      aria-label={t("media.music.queue", "Queue")}
      className="absolute bottom-full right-0 mb-2 w-[min(90vw,320px)] max-h-96 overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] shadow-lg"
    >
      <div className="flex items-center justify-between px-3 py-2 border-b border-[var(--border)] sticky top-0 bg-[var(--surface)]">
        <span className="text-sm font-semibold text-[var(--text)]">{t("media.music.queue", "Queue")}</span>
        <button
          onClick={onClose}
          className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast p-0 flex items-center"
          aria-label={t("common.aria.close")}
        >
          <X size={ICON.sm} aria-hidden="true" />
        </button>
      </div>
      {queue.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] text-center py-6 px-3">{t("media.music.queueEmpty", "Queue empty")}</p>
      ) : (
        <ul>
          {queue.map((track) => {
            const isCurrent = track.ID === playing.ID;
            return (
              <li key={track.ID} className="flex items-center border-b border-[var(--border)] last:border-b-0">
                <button
                  onClick={() => onJump(track)}
                  aria-current={isCurrent ? "true" : undefined}
                  className={`flex-1 min-w-0 text-left px-3 py-2 bg-transparent border-none cursor-pointer ${isCurrent ? "bg-[var(--surface-raised)]" : ""}`}
                >
                  <p className={`text-sm truncate ${isCurrent ? "font-semibold text-[var(--accent)]" : "text-[var(--text)]"}`}>{track.Title || "-"}</p>
                  <p className="text-xs truncate text-[var(--text-muted)]">{track.Artist || "-"}</p>
                </button>
                <button
                  onClick={() => onRemove(track)}
                  className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast p-2 flex items-center shrink-0"
                  aria-label={t("media.music.removeFromQueue", "Remove from queue")}
                  title={t("media.music.removeFromQueue", "Remove from queue")}
                >
                  <X size={ICON.sm} aria-hidden="true" />
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// A stable id for this tab's music playback, so the admin "En cours de lecture"
// panel tracks one session per player (not one per track). Module-level, so it
// MUST NOT touch crypto.randomUUID directly: absent on plain http, and this
// module is in AuthLayout's chunk - the throw blanked every page.
const nowPlayingSessionId = `music-${randomUUID()}`;

// nowPlayingDeviceName is the short client label shown on the admin card.
function nowPlayingDeviceName(): string {
  if (typeof navigator === "undefined") return "Navigateur";
  const ua = navigator.userAgent;
  if (/Firefox\//.test(ua)) return "Firefox";
  if (/Edg\//.test(ua)) return "Edge";
  if (/Chrome\//.test(ua)) return "Chrome";
  if (/Safari\//.test(ua)) return "Safari";
  return "Navigateur";
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
  const play = useMusicPlayerStore((s) => s.play);
  const setQueue = useMusicPlayerStore((s) => s.setQueue);
  const source = useMusicPlayerStore((s) => s.source);
  const [showQueue, setShowQueue] = useState(false);
  const navigate = useNavigate();

  // Jumping within the queue must keep the queue and source intact - `play()`
  // resets `source` to null unless it's re-passed, which would break the
  // "back to album" link in the mini-player title after a queue jump.
  const jumpToTrack = (track: MusicTrack, currentSource: MusicSource | null) => {
    if (track.ID === playing?.ID) return;
    play(track, queue, currentSource);
  };

  // No dedicated store action for removal (out of scope here), so this reuses
  // the existing setQueue/play/close primitives. Removing the currently
  // playing track advances to its former neighbour rather than leaving
  // playback pointed at a track no longer in the queue - mirrors mobile's
  // musicQueue.removeFromQueue behaviour.
  const removeFromQueue = (track: MusicTrack, currentSource: MusicSource | null) => {
    const idx = queue.findIndex((t) => t.ID === track.ID);
    const newQueue = queue.filter((t) => t.ID !== track.ID);
    if (playing?.ID === track.ID) {
      const nextTrack = newQueue[idx] ?? newQueue[idx - 1];
      if (nextTrack) {
        play(nextTrack, newQueue, currentSource);
      } else {
        close();
      }
    } else {
      setQueue(newQueue);
    }
  };
  // The audio element lives in the layout (outside the router Outlet), so this
  // navigation swaps the page content without remounting <audio> - playback is
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
  // The token is minted FRESH at every track change, then pinned for that
  // track's lifetime. The store's token is what the last API call left behind,
  // and an album is long enough for it to expire between two tracks with
  // nobody touching the page: the next track's stream then 401s and the music
  // stops (prod logs, 21-09, `auth rejected invalid_token` on
  // /music/tracks/{id}/stream). Pinned, because subscribing to accessToken
  // would rebuild src on every refresh cycle and abort the in-flight stream.
  // Same rule as the television and the phone.
  const trackId = playing?.ID;
  const [pinned, setPinned] = useState<{ trackId: string; token?: string } | null>(null);
  useEffect(() => {
    if (!trackId) { setPinned(null); return; }
    let live = true;
    getFreshAccessToken()
      .then((token) => { if (live) setPinned({ trackId, token }); })
      .catch(() => { if (live) setPinned({ trackId, token: useAuthStore.getState().accessToken ?? undefined }); });
    return () => { live = false; };
  }, [trackId]);
  const token = pinned && pinned.trackId === trackId ? pinned.token : undefined;
  const src = useMemo(() => {
    if (!trackId || !pinned || pinned.trackId !== trackId) return undefined;
    const tp = token ? `?token=${encodeURIComponent(token)}` : "";
    return `/api/v1/music/tracks/${trackId}/stream${tp}`;
  }, [trackId, pinned, token]);

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
  // Guards against double-counting a listen: `ended` can fire more than once for
  // a single playthrough (browser quirks / re-renders), which inflates the
  // play-count taste signal. Reset on each new track's loadedmetadata, set on
  // the first ended - a genuine replay reloads the track and counts again.
  const playRecordedRef = useRef(false);
  // Counts consecutive stream errors. A systemic failure (expired token, server
  // down) makes every track error, and onError -> next() -> onError would loop
  // forever in shuffle (and flood toasts). Reset whenever a track actually plays.
  const errorCountRef = useRef(0);

  useEffect(() => {
    const audio = audioRef.current;
    // `src` gates this effect: the token arrives a microtask after the track
    // changes, and play() on a source-less element only throws.
    if (!audio || !playing || !src) return;
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
    // playback that the externally-paused effect would immediately stop -
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
    // eslint-disable-next-line react-hooks/exhaustive-deps -- playing?.ID already tracks every playing transition (store only replaces the object on a real track change); listing playing would refire on immaterial reference churn
  }, [playing?.ID, src]);

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
    // eslint-disable-next-line react-hooks/exhaustive-deps -- playing?.ID already tracks every playing transition, see the effect above
  }, [externallyPaused, playing?.ID]);

  // Report this playback to the admin "En cours de lecture" panel every ~10s
  // while a track is loaded, and again on every play/pause so the admin card
  // follows within its own refresh. Remote commands (play/pause/stop) arrive
  // on a long poll the moment the admin sends them; the heartbeat still
  // carries one back as a fallback. When playback stops the client stops
  // beating and the server's TTL evicts the session.
  const nowPlayingIdRef = useRef<string | undefined>(playing?.ID);
  nowPlayingIdRef.current = playing?.ID;
  useEffect(() => {
    if (!playing?.ID) return;
    const ctrl = new AbortController();
    function applyCommand(cmd: string) {
      const el = audioRef.current;
      if (cmd === "pause") {
        el?.pause();
        setIsPlaying(false);
      } else if (cmd === "play") {
        void el?.play().catch(() => { /* autoplay may be blocked */ });
      } else if (cmd === "stop") {
        close();
      }
    }
    async function beat() {
      const audio = audioRef.current;
      try {
        const res = await eveyFetchRaw({
          url: "/api/v1/music/nowplaying",
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            sessionId: nowPlayingSessionId,
            trackId: nowPlayingIdRef.current,
            deviceName: nowPlayingDeviceName(),
            positionMs: Math.floor((audio?.currentTime ?? 0) * 1000),
            isPaused: audio ? audio.paused : true,
          }),
        });
        if (ctrl.signal.aborted) return;
        const data = (await res.json().catch(() => null)) as { remoteCommand?: string } | null;
        if (data?.remoteCommand) applyCommand(data.remoteCommand);
      } catch { /* best-effort telemetry, never disrupt playback */ }
    }
    // The poll starts once the first beat has registered the session, so its
    // first request is not a 404 back-off.
    void beat().then(() => pollMusicRemoteCommands(nowPlayingSessionId, ctrl.signal, applyCommand));
    const iv = setInterval(() => void beat(), 10_000);
    const audio = audioRef.current;
    const onPlayState = () => void beat();
    audio?.addEventListener("play", onPlayState);
    audio?.addEventListener("pause", onPlayState);
    return () => {
      ctrl.abort();
      clearInterval(iv);
      audio?.removeEventListener("play", onPlayState);
      audio?.removeEventListener("pause", onPlayState);
    };
  }, [playing?.ID, close]);

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
    <div className="fixed bottom-0 left-0 right-0 flex items-center gap-2 sm:gap-4 px-3 sm:px-6 pt-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] bg-[var(--surface)] border-t border-[var(--border)] z-overlay">
      <audio
        ref={audioRef}
        key={playing.ID}
        src={src}
        onTimeUpdate={() => setCurrentTime(audioRef.current?.currentTime ?? 0)}
        onLoadedMetadata={() => {
          setDuration(audioRef.current?.duration ?? 0);
          playRecordedRef.current = false;
        }}
        onEnded={() => {
          // A finished track counts as a listen - feeds the playlist generator's
          // taste signal. Fire-and-forget; never block playback advance. Guarded
          // so a duplicate `ended` for the same playthrough counts only once.
          if (!playRecordedRef.current) {
            playRecordedRef.current = true;
            void eveyFetchRaw({ url: `/api/v1/music/tracks/${playing.ID}/played`, method: "POST" }).catch(() => {});
          }
          next();
        }}
        onPlay={() => {
          errorCountRef.current = 0;
          setIsPlaying(true);
        }}
        onPause={() => setIsPlaying(false)}
        onError={() => {
          errorCountRef.current += 1;
          // Quietly skip a one-off bad track, but once we've failed across the
          // whole queue, stop and surface a single error instead of looping.
          if (errorCountRef.current >= Math.max(queue.length, 1)) {
            errorCountRef.current = 0;
            setIsPlaying(false);
            toast.error(t("common.error"));
            return;
          }
          next();
        }}
      />
      <div className="flex items-center gap-3 min-w-0 w-[120px] sm:w-[200px]">
        <MiniTrackArt trackID={playing.ID} coverArtURL={playing.CoverArtURL} token={token} />
        {goToSource ? (
          <button
            type="button"
            onClick={goToSource}
            title={source!.label}
            aria-label={source!.label}
            className="group min-w-0 text-left bg-transparent border-none cursor-pointer p-0"
          >
            <p className="font-medium truncate text-sm text-[var(--text)] group-hover:text-[var(--accent)] transition-colors-fast">{playing.Title || "-"}</p>
            <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "-"}</p>
          </button>
        ) : (
          <div className="min-w-0">
            <p className="font-medium truncate text-sm text-[var(--text)]">{playing.Title || "-"}</p>
            <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "-"}</p>
          </div>
        )}
      </div>
      <div className="flex flex-col items-center gap-1 flex-1">
        <div className="flex items-center gap-4">
          <button
            onClick={toggleShuffle}
            className={`bg-transparent border-none cursor-pointer min-h-11 min-w-11 flex items-center justify-center ${shuffle ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}
            aria-label={t("media.music.shuffle")}
            aria-pressed={shuffle}
            title={t("media.music.shuffle")}
          >
            <Shuffle size={ICON.md} aria-hidden="true" />
          </button>
          <button onClick={prev} disabled={!shuffle && currentIndex <= 0} className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer min-h-11 min-w-11 flex items-center justify-center" aria-label={t("common.previous")}>
            <SkipBack size={ICON.lg} aria-hidden="true" />
          </button>
          <button onClick={togglePlay} className="bg-[var(--accent)] border-none cursor-pointer text-[var(--filter-active-text)] rounded-full min-w-11 min-h-11 flex items-center justify-center" aria-label={isPlaying ? t("common.aria.pause") : t("common.aria.play")}>
            {isPlaying ? <Pause size={ICON.md} aria-hidden="true" /> : <Play size={ICON.md} aria-hidden="true" />}
          </button>
          <button onClick={next} disabled={!shuffle && currentIndex >= queue.length - 1} className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer min-h-11 min-w-11 flex items-center justify-center" aria-label={t("common.next")}>
            <SkipForward size={ICON.lg} aria-hidden="true" />
          </button>
        </div>
        <div className="flex items-center gap-2 w-full max-w-[200px] sm:max-w-[400px]">
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8 text-right">{formatTime(currentTime)}</span>
          <input type="range" min={0} max={duration || 0} step={1} value={currentTime} onChange={seek} aria-label={t("media.music.seekPosition")} className="flex-1 accent-[var(--accent)]" />
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8">{duration > 0 ? formatTime(duration) : "-"}</span>
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
          {muted || volume === 0 ? <VolumeX size={ICON.md} aria-hidden="true" /> : <Volume2 size={ICON.md} aria-hidden="true" />}
        </button>
        <input type="range" min={0} max={1} step={0.02} value={volume} onChange={(e) => setVolume(Number(e.target.value))} aria-label={t("media.music.volume")} className="w-[72px] accent-[var(--accent)]" />
      </div>
      <button
        onClick={() => setShowQueue((v) => !v)}
        className={`tap-target flex items-center justify-center bg-transparent border-none cursor-pointer ${showQueue ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}
        aria-label={t("media.music.queue", "Queue")}
        aria-pressed={showQueue}
        aria-expanded={showQueue}
        title={t("media.music.queue", "Queue")}
      >
        <ListMusic size={ICON.lg} aria-hidden="true" />
      </button>
      <button onClick={close} className="tap-target flex items-center justify-center text-[var(--text-muted)] bg-transparent border-none cursor-pointer" aria-label={t("common.aria.close")} title={t("common.aria.close")}>
        <X size={ICON.lg} aria-hidden="true" />
      </button>
      {showQueue && (
        <QueuePanel
          queue={queue}
          playing={playing}
          onJump={(track) => jumpToTrack(track, source)}
          onRemove={(track) => removeFromQueue(track, source)}
          onClose={() => setShowQueue(false)}
        />
      )}
    </div>
  );
}
