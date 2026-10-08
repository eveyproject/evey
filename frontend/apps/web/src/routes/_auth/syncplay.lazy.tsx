import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useMusicServiceListTracks,
  musicServiceGetTrack,
  syncPlayServiceGetRoom,
  normalizeRoomCode,
  ROOM_CODE_LENGTH,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Pause, Music2, Users, LogOut, SkipForward, SkipBack, ListMusic } from "lucide-react";
import { useSyncPlay } from "@/features/syncplay/useSyncPlay";
import { RoomBuilder, type QueueItem } from "@/features/syncplay/RoomBuilder";

export const Route = createLazyFileRoute("/_auth/syncplay")({
  component: SyncPlayPage,
});

interface TrackLite {
  id: string;
  title: string;
  artist: string;
  duration: number;
  coverArtUrl: string;
}

function useMusicLibraryId(): string | undefined {
  const { data } = useLibraryServiceListLibraries();
  return useMemo(
    () => data?.libraries?.find((l) => l.type === "LIBRARY_TYPE_MUSIC")?.id,
    [data],
  );
}

function useTracksList(libraryId: string | undefined, search: string) {
  return useMusicServiceListTracks(
    { libraryId, limit: 100, search: search || undefined },
    {
      query: {
        enabled: !!libraryId,
        select: (d): TrackLite[] =>
          (d.tracks ?? []).map((t) => ({
            id: t.id ?? "",
            title: t.title ?? "",
            artist: t.artist ?? "",
            duration: t.duration ?? 0,
            coverArtUrl: t.coverArtUrl ?? "",
          })),
      },
    },
  );
}

function SyncPlayPage() {
  const { t } = useTranslation();
  const libraryId = useMusicLibraryId();
  // Kept (empty search) only to enrich the in-room "now playing" track metadata
  // from its media id; the room builder fetches its own album/track/playlist data.
  const { data: tracks } = useTracksList(libraryId, "");
  const [code, setCode] = useState("");
  const [currentTrack, setCurrentTrack] = useState<TrackLite | null>(null);

  // Metadata seeded from the RoomBuilder queue at room-creation time. The
  // page's own track list is capped at the top 100 (useTracksList), so a queued
  // track outside that window would otherwise render as a bare id; this map
  // covers every track the host actually picked.
  const trackMeta = useRef<Map<string, TrackLite>>(new Map());

  // resolveTrack enriches a media id into a TrackLite: the host's picked-queue
  // map first, then the top-100 list, then a bare-id stub (cover art is fetched
  // by id regardless, so an empty coverArtUrl still renders).
  const resolveTrack = (id: string): TrackLite => {
    const seeded = trackMeta.current.get(id);
    if (seeded) return seeded;
    const found = (tracks ?? []).find((tr) => tr.id === id);
    return found ?? { id, title: "", artist: "", duration: 0, coverArtUrl: "" };
  };
  const [joining, setJoining] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const programmaticRef = useRef(false);
  const echoTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Suppress the echo of a remotely-driven change: applying a peer's play /
  // pause / seek fires the same media events a local action would, and the host
  // would rebroadcast them.
  //
  // The window is re-armed, not stacked. Each call used to start its own
  // untracked timer, so two commands less than 100ms apart - pause+seek, which
  // the protocol naturally sends together - had the FIRST timer clear the flag
  // while the second's `seeked` was still in flight (a seeked event is async,
  // and more so when the buffer has to refill). The host then broadcast a seek
  // nobody asked for.
  const suppressEcho = useCallback(() => {
    programmaticRef.current = true;
    if (echoTimerRef.current) clearTimeout(echoTimerRef.current);
    echoTimerRef.current = setTimeout(() => {
      programmaticRef.current = false;
      echoTimerRef.current = null;
    }, 100);
  }, []);

  useEffect(() => () => {
    if (echoTimerRef.current) clearTimeout(echoTimerRef.current);
  }, []);

  const [state, controls] = useSyncPlay({
    onPlay: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      suppressEcho();
      audio.currentTime = posMs / 1000;
      void audio.play().catch(() => undefined);
    },
    onPause: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      suppressEcho();
      audio.pause();
      audio.currentTime = posMs / 1000;
    },
    onSeek: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      suppressEcho();
      audio.currentTime = posMs / 1000;
    },
    onTrackChange: (_idx, mediaId) => {
      // Rotate to new track: resolve metadata from the seeded queue map, then
      // the top-100 list, then a stub.
      setCurrentTrack(resolveTrack(mediaId));
    },
  });

  // Keep currentTrack in sync with the queue's current track (useful after createRoomFromPlaylist or join)
  useEffect(() => {
    const queueId = state.queue[state.trackIndex];
    if (!queueId) return;
    if (currentTrack?.id === queueId) return;
    setCurrentTrack(resolveTrack(queueId));
    // resolveTrack reads trackMeta (ref) + tracks; tracks in deps covers the
    // top-100 arriving after a join.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.queue, state.trackIndex, tracks, currentTrack?.id]);

  // A JOINER's queue can reference tracks outside its own top-100 list and that
  // the host didn't seed (e.g. an album/playlist room of >100 tracks), leaving
  // currentTrack as a bare-id stub (empty title). Fetch those by id and cache
  // them so the now-playing panel shows real metadata instead of a raw id.
  useEffect(() => {
    if (!currentTrack || currentTrack.title || !currentTrack.id) return;
    const id = currentTrack.id;
    if (trackMeta.current.has(id)) return;
    let cancelled = false;
    musicServiceGetTrack(id)
      .then((res) => {
        const tr = res.track;
        if (cancelled || !tr) return;
        const lite: TrackLite = {
          id: tr.id ?? id,
          title: tr.title ?? "",
          artist: tr.artist ?? "",
          duration: tr.duration ?? 0,
          coverArtUrl: tr.coverArtUrl ?? "",
        };
        trackMeta.current.set(id, lite);
        setCurrentTrack((cur) => (cur?.id === id ? lite : cur));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [currentTrack]);

  const inRoom = !!state.roomId;
  // Pin token at track-change time: subscribing would rebuild streamUrl on
  // every refresh cycle and abort the in-flight audio stream.
  const streamUrl = useMemo(() => {
    if (!currentTrack) return "";
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${currentTrack.id}/stream${tp}`;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only currentTrack.id is read, already tracked
  }, [currentTrack?.id]);
  const artUrl = useMemo(() => {
    if (!currentTrack) return "";
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${currentTrack.id}/art${tp}${tp ? "&" : "?"}w=320`;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only currentTrack.id is read, already tracked
  }, [currentTrack?.id]);

  async function handleCreateQueue(items: QueueItem[]) {
    if (items.length === 0) return;
    setCreating(true);
    setError(null);
    // Seed the metadata map BEFORE the room resolves so the first
    // onTrackChange / queue effect can label tracks the top-100 list misses.
    for (const it of items) {
      if (it.id) {
        trackMeta.current.set(it.id, {
          id: it.id,
          title: it.title,
          artist: it.artist,
          duration: 0,
          coverArtUrl: "",
        });
      }
    }
    try {
      // currentTrack is enriched once the queue's first id resolves (effect + onTrackChange).
      await controls.createRoomFromQueue(items.map((it) => it.id));
    } catch (e) {
      setError((e as Error).message || "error");
      setCurrentTrack(null);
    } finally {
      setCreating(false);
    }
  }

  async function handleJoin() {
    const c = code.trim();
    if (!c) return;
    setJoining(true);
    setError(null);
    try {
      const info = await controls.joinByCode(c);
      if (info.mediaType !== "music") {
        setError(t("media.syncplay.notMusicRoom"));
        await controls.leaveRoom();
        return;
      }
      // Fetch the room to get name etc.; we already have mediaId - build a TrackLite stub
      setCurrentTrack({ id: info.mediaId, title: "", artist: "", duration: 0, coverArtUrl: "" });
      // Enrich via GetRoom if needed later; music metadata would require separate call
      try {
        const room = await syncPlayServiceGetRoom(info.roomId);
        if (room) {
          // no title in room, keep stub
        }
      } catch {
        /* ignore */
      }
    } catch (e) {
      setError((e as Error).message || "error");
    } finally {
      setJoining(false);
    }
  }

  async function handleLeave() {
    await controls.leaveRoom();
    setCurrentTrack(null);
    setCode("");
  }

  // Host-side: broadcast audio play/pause/seek
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !inRoom || !state.isHost) return;
    const onPlay = () => {
      if (programmaticRef.current) return;
      controls.sendPlay(Math.floor(audio.currentTime * 1000));
    };
    const onPause = () => {
      if (programmaticRef.current) return;
      controls.sendPause(Math.floor(audio.currentTime * 1000));
    };
    const onSeeked = () => {
      if (programmaticRef.current) return;
      controls.sendSeek(Math.floor(audio.currentTime * 1000));
    };
    audio.addEventListener("play", onPlay);
    audio.addEventListener("pause", onPause);
    audio.addEventListener("seeked", onSeeked);
    return () => {
      audio.removeEventListener("play", onPlay);
      audio.removeEventListener("pause", onPause);
      audio.removeEventListener("seeked", onSeeked);
    };
  }, [inRoom, state.isHost, controls]);

  if (inRoom) {
    return (
      <div className="max-w-3xl mx-auto py-8 px-4 flex flex-col gap-6">
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("media.syncplay.hubTitle")}</h1>
        <Card className="p-6 flex flex-col gap-4">
          <div className="flex items-center gap-4">
            {artUrl ? (
              <img src={artUrl} alt="" className="w-20 h-20 rounded object-cover bg-[var(--surface)]" onError={(e) => ((e.currentTarget.style.display = "none"))} />
            ) : (
              <div className="w-20 h-20 rounded bg-[var(--surface)] flex items-center justify-center">
                <Music2 size={32} className="text-[var(--text-muted)]" />
              </div>
            )}
            <div className="flex-1 min-w-0">
              <div className="text-lg font-semibold truncate text-[var(--text)]">
                {currentTrack?.title || t("media.syncplay.sharedTrack")}
              </div>
              <div className="text-sm text-[var(--text-muted)] truncate">
                {currentTrack?.artist}
              </div>
              {state.isHost && state.roomCode && (
                <div className="mt-2 flex items-center gap-2">
                  <span className="text-xs text-[var(--text-muted)]">{t("media.syncplay.codeLabel")}</span>
                  <span className="font-mono text-2xl tracking-widest font-bold text-[var(--accent)]">{state.roomCode}</span>
                </div>
              )}
            </div>
          </div>

          {currentTrack?.id && (
            <audio
              ref={audioRef}
              src={streamUrl}
              controls={state.isHost}
              autoPlay={false}
              className="w-full"
              onEnded={() => {
                // Auto-advance the shared queue when a track finishes. Only the
                // host drives it; the server echoes the "next" to every member
                // (host included), so all clients load the next track in sync.
                // No-op at the end of the queue (the hub bounds-checks the index).
                if (state.isHost) controls.sendNext();
              }}
            />
          )}
          {!state.isHost && (
            <p className="text-xs text-[var(--text-muted)]">{t("media.syncplay.followingHost")}</p>
          )}

          {state.queue.length > 0 && state.isHost && (
            <div className="flex items-center justify-center gap-2">
              <Button
                variant="secondary"
                disabled={state.trackIndex <= 0}
                onClick={() => controls.sendPrev()}
              >
                <SkipBack size={16} />
              </Button>
              <span className="text-xs text-[var(--text-muted)]">
                {state.trackIndex + 1} / {state.queue.length}
              </span>
              <Button
                variant="secondary"
                disabled={state.trackIndex >= state.queue.length - 1}
                onClick={() => controls.sendNext()}
              >
                <SkipForward size={16} />
              </Button>
            </div>
          )}

          {state.queue.length > 0 && (
            <div className="flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-2 max-h-48 overflow-y-auto">
              <div className="flex items-center gap-2 text-xs text-[var(--text-muted)] mb-1">
                <ListMusic size={14} />
                <span>{t("media.syncplay.queue")}</span>
              </div>
              {state.queue.map((trackId, idx) => {
                const meta = resolveTrack(trackId);
                return (
                  <button
                    key={`${trackId}-${idx}`}
                    type="button"
                    disabled={!state.isHost}
                    onClick={() => state.isHost && controls.sendSetIndex(idx)}
                    className={`flex items-center gap-2 text-left px-2 py-1 rounded-[var(--radius-sm)] text-sm ${
                      idx === state.trackIndex
                        ? "bg-[var(--surface-raised)] text-[var(--accent)] font-semibold"
                        : "text-[var(--text)] " + (state.isHost ? "hover:bg-[var(--surface-raised)]" : "opacity-70")
                    }`}
                  >
                    <span className="text-xs text-[var(--text-muted)] w-6 text-right">{idx + 1}</span>
                    <Music2 size={12} />
                    {meta.title ? (
                      <span className="flex-1 min-w-0 truncate">
                        {meta.title}
                        {meta.artist && <span className="text-[var(--text-muted)]"> - {meta.artist}</span>}
                      </span>
                    ) : (
                      <span className="flex-1 truncate font-mono text-xs">{trackId}</span>
                    )}
                  </button>
                );
              })}
            </div>
          )}

          <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
            <Users size={16} />
            <span>{state.members.length + 1} {t("media.syncplay.members")}</span>
            <span className={`ml-auto w-2 h-2 rounded-full ${state.connected ? "bg-[var(--color-success)]" : "bg-[var(--color-danger)]"}`} />
            <span className="text-xs">{state.connected ? t("media.syncplay.connected") : t("media.syncplay.connecting")}</span>
          </div>

          <Button variant="secondary" onClick={() => void handleLeave()}>
            <LogOut size={16} className="mr-2" />
            {t("media.syncplay.leave")}
          </Button>
        </Card>
      </div>
    );
  }

  return (
    <div className="max-w-5xl mx-auto py-8 px-4 flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("media.syncplay.hubTitle")}</h1>
      {error && (
        <div className="text-sm text-[var(--color-danger)]">{error}</div>
      )}

      <Card className="p-6 flex flex-col gap-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.syncplay.create")}</h2>
        <RoomBuilder libraryId={libraryId} creating={creating} onCreate={(items) => void handleCreateQueue(items)} />
      </Card>

      <Card className="p-6 flex flex-col gap-4 w-full max-w-md">
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.syncplay.join")}</h2>
        <Input
          label={t("media.syncplay.codeLabel")}
          value={code}
          onChange={(e) => setCode(normalizeRoomCode(e.target.value).slice(0, ROOM_CODE_LENGTH))}
          placeholder="ABC123"
          autoCapitalize="characters"
          autoComplete="off"
          className="font-mono tracking-widest text-center text-lg"
        />
        <Button variant="primary" loading={joining} disabled={code.trim().length < 4} onClick={() => void handleJoin()}>
          <Pause size={16} className="mr-2" />
          {t("media.syncplay.joinRoom")}
        </Button>
      </Card>
    </div>
  );
}
