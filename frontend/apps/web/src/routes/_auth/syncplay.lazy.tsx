import { useEffect, useMemo, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useMusicServiceListTracks,
  syncPlayServiceGetRoom,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Play, Pause, Music2, Users, LogOut } from "lucide-react";
import { useSyncPlay } from "@/features/syncplay/useSyncPlay";

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
  const accessToken = useAuthStore((s) => s.accessToken);
  const libraryId = useMusicLibraryId();
  const [search, setSearch] = useState("");
  const { data: tracks } = useTracksList(libraryId, search);
  const [selectedTrackId, setSelectedTrackId] = useState<string>("");
  const [code, setCode] = useState("");
  const [currentTrack, setCurrentTrack] = useState<TrackLite | null>(null);
  const [joining, setJoining] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const programmaticRef = useRef(false);

  const [state, controls] = useSyncPlay({
    onPlay: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      programmaticRef.current = true;
      audio.currentTime = posMs / 1000;
      void audio.play().catch(() => undefined);
      setTimeout(() => (programmaticRef.current = false), 100);
    },
    onPause: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      programmaticRef.current = true;
      audio.pause();
      audio.currentTime = posMs / 1000;
      setTimeout(() => (programmaticRef.current = false), 100);
    },
    onSeek: (posMs) => {
      const audio = audioRef.current;
      if (!audio) return;
      programmaticRef.current = true;
      audio.currentTime = posMs / 1000;
      setTimeout(() => (programmaticRef.current = false), 100);
    },
  });

  const inRoom = !!state.roomId;
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const streamUrl = currentTrack ? `/api/v1/music/tracks/${currentTrack.id}/stream${tokenParam}` : "";
  const artUrl = currentTrack ? `/api/v1/music/tracks/${currentTrack.id}/art${tokenParam}` : "";

  async function handleCreate() {
    if (!selectedTrackId) return;
    setCreating(true);
    setError(null);
    try {
      const track = (tracks ?? []).find((x) => x.id === selectedTrackId) ?? null;
      setCurrentTrack(track);
      await controls.createRoom(selectedTrackId, "music");
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
      // Fetch the room to get name etc.; we already have mediaId — build a TrackLite stub
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
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("media.syncplay.title")}</h1>
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
            />
          )}
          {!state.isHost && (
            <p className="text-xs text-[var(--text-muted)]">{t("media.syncplay.followingHost")}</p>
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
    <div className="max-w-4xl mx-auto py-8 px-4 flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("media.syncplay.title")}</h1>
      {error && (
        <div className="text-sm text-[var(--color-danger)]">{error}</div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card className="p-6 flex flex-col gap-4">
          <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.syncplay.create")}</h2>
          <Input
            label={t("common.search")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("common.search")}
          />
          <div className="max-h-64 overflow-y-auto flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-1">
            {(tracks ?? []).map((tr) => (
              <button
                key={tr.id}
                type="button"
                onClick={() => setSelectedTrackId(tr.id)}
                className={`flex items-center gap-2 text-left px-2 py-1 rounded-[var(--radius-sm)] ${selectedTrackId === tr.id ? "bg-[var(--surface-raised)] text-[var(--accent)]" : "hover:bg-[var(--surface-raised)] text-[var(--text)]"}`}
              >
                <Music2 size={14} />
                <span className="flex-1 truncate text-sm">{tr.title}</span>
                <span className="text-xs text-[var(--text-muted)] truncate max-w-[40%]">{tr.artist}</span>
              </button>
            ))}
            {(!tracks || tracks.length === 0) && (
              <div className="text-xs text-[var(--text-muted)] p-2">{t("media.syncplay.noTracks")}</div>
            )}
          </div>
          <Button variant="primary" loading={creating} disabled={!selectedTrackId} onClick={() => void handleCreate()}>
            <Play size={16} className="mr-2" />
            {t("media.syncplay.createRoom")}
          </Button>
        </Card>

        <Card className="p-6 flex flex-col gap-4">
          <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.syncplay.join")}</h2>
          <Input
            label={t("media.syncplay.codeLabel")}
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, "").slice(0, 6))}
            placeholder="12345"
            inputMode="numeric"
            autoComplete="off"
            className="font-mono tracking-widest text-center text-lg"
          />
          <Button variant="primary" loading={joining} disabled={code.trim().length < 4} onClick={() => void handleJoin()}>
            <Pause size={16} className="mr-2" />
            {t("media.syncplay.joinRoom")}
          </Button>
        </Card>
      </div>
    </div>
  );
}
