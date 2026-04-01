import { Users, TriangleAlert } from "lucide-react";
import { createLazyFileRoute, useRouter } from "@tanstack/react-router";
import { useCallback, useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem, libraryServiceListMediaItems } from "@evey/api";
import { usePlaySession } from "@/hooks/usePlaySession";
import { useChapters } from "@/hooks/useChapters";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import type { AudioStream, SubtitleStream } from "@/features/playback/HLSPlayer";
import { useSyncPlay } from "@/features/syncplay/useSyncPlay";
import { SyncPlayBar } from "@/features/syncplay/SyncPlayBar";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { useParentalGate, markUnlocked } from "@/hooks/useParentalGate";
import { ParentalPINDialog } from "@/components/ParentalPINDialog";
import { useAuthStore, usePlayerStore, setPlayerCleanup } from "@evey/store";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

export const Route = createLazyFileRoute("/_auth/player/$mediaId")({
  component: PlayerPage,
});

function PlayerPage() {
  const { mediaId } = Route.useParams();
  const router = useRouter();
  const profileId = useAuthStore((s) => s.profileId);

  const { data: gateItem, isLoading: gateLoading } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  const { needsPin } = useParentalGate(gateItem?.officialRating);
  const [pinVerified, setPinVerified] = useState(false);
  const blocked = !gateLoading && needsPin() && !pinVerified;

  if (gateLoading) {
    return (
      <div className="fixed inset-0 bg-black z-overlay flex items-center justify-center">
        <Spinner />
      </div>
    );
  }

  if (blocked) {
    return (
      <div className="fixed inset-0 bg-black z-overlay flex items-center justify-center">
        {profileId && (
          <ParentalPINDialog
            open
            onClose={() => router.history.back()}
            onVerified={() => {
              if (profileId) markUnlocked(profileId);
              setPinVerified(true);
            }}
            profileId={profileId}
          />
        )}
      </div>
    );
  }

  return <PlayerPageInner mediaId={mediaId} />;
}

function PlayerPageInner({ mediaId }: { mediaId: string }) {
  const { t } = useTranslation();
  const { room: roomParam } = Route.useSearch();
  const router = useRouter();
  const [showShareDialog, setShowShareDialog] = useState(false);
  const [shareRoomId, setShareRoomId] = useState<string | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { streamInfo, sessionId, streamToken, fetchStreamToken, isLoading, isError, progress, stopSession } = usePlaySession(mediaId);
  const [loadingTimedOut, setLoadingTimedOut] = useState(false);
  useEffect(() => {
    if (!isLoading) { setLoadingTimedOut(false); return; }
    const timer = setTimeout(() => setLoadingTimedOut(true), 30_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  // Sync store with session state — only persist mini player if video is actively playing
  useEffect(() => {
    usePlayerStore.getState().play(mediaId);
    return () => {
      const state = usePlayerStore.getState();
      if (state.mode === "full" && state.isActive) {
        const video = state.videoElement as HTMLVideoElement | null;
        if (video && !video.paused && !video.ended && video.readyState > 2) {
          state.setMode("mini");
        } else {
          state.stop();
        }
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaId]);

  useEffect(() => {
    if (sessionId) usePlayerStore.getState().setSessionId(sessionId);
  }, [sessionId]);

  useEffect(() => {
    usePlayerStore.getState().setStopSessionFn(stopSession);
    return () => usePlayerStore.getState().setStopSessionFn(null);
  }, [stopSession]);

  const handleStreamTokenRefresh = useCallback(() => fetchStreamToken(mediaId), [fetchStreamToken, mediaId]);

  const [syncState, syncControls] = useSyncPlay({
    onPlay: (positionMs) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = positionMs / 1000;
      v.play().catch(() => {});
    },
    onPause: (positionMs) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = positionMs / 1000;
      v.pause();
    },
    onSeek: (positionMs) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = positionMs / 1000;
    },
  });

  useEffect(() => {
    if (roomParam && !syncState.roomId) {
      syncControls.joinRoom(roomParam).catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional: only join when URL roomParam changes, not when syncState/syncControls update
  }, [roomParam]);
  const { data: mediaItem } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  useEffect(() => {
    if (mediaItem?.title) usePlayerStore.getState().play(mediaId, mediaItem.title);
  }, [mediaItem?.title, mediaId]);

  const handleClose = useCallback(() => {
    usePlayerStore.getState().stop();
    router.history.back();
  }, [router]);

  const audioStreams: AudioStream[] =
    streamInfo?.mediaStreams
      ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
      .map((s, i) => ({
        index: s.index ?? 0,
        trackIndex: i,
        language: s.language,
        title: s.title,
        isDefault: s.isDefault,
      })) ?? [];

  const subtitleStreams: SubtitleStream[] =
    streamInfo?.mediaStreams
      ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
      .map((s, i) => {
        const codec = s.codec?.toLowerCase() ?? "";
        const burnIn = IMAGE_SUBTITLE_CODECS.has(codec);
        return {
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          isExternal: s.isExternal,
          externalPath: s.externalPath,
          codec: s.codec,
          burnIn,
        };
      }) ?? [];

  const { data: chapters } = useChapters(mediaId);
  const startPositionMs = progress?.positionMs ? Number(progress.positionMs) : 0;

  const isEpisode = mediaItem?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const seasonId = isEpisode ? (mediaItem?.parentId ?? "") : "";

  const { data: season } = useQuery({
    queryKey: [`/api/v1/media/${seasonId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(seasonId, signal),
    enabled: !!seasonId,
  });
  const seriesId = season?.parentId ?? "";

  const { data: seasonsData } = useQuery({
    queryKey: ["/api/v1/media", { parentId: seriesId }],
    queryFn: ({ signal }) => libraryServiceListMediaItems({ parentId: seriesId }, signal),
    enabled: !!seriesId,
  });

  const [selectedSeasonId, setSelectedSeasonId] = useState(seasonId);
  useEffect(() => { if (seasonId) setSelectedSeasonId(seasonId); }, [seasonId]);

  const { data: episodesData } = useQuery({
    queryKey: ["/api/v1/media", { parentId: selectedSeasonId }],
    queryFn: ({ signal }) => libraryServiceListMediaItems({ parentId: selectedSeasonId }, signal),
    enabled: !!selectedSeasonId,
  });

  const seasons = [...(seasonsData?.items ?? [])]
    .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0))
    .map((s) => ({ id: s.id!, title: s.title ?? `Season ${s.indexNumber}`, indexNumber: s.indexNumber }));

  const episodes = [...(episodesData?.items ?? [])]
    .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0))
    .map((ep) => ({ id: ep.id!, title: ep.title ?? `Episode ${ep.indexNumber}`, indexNumber: ep.indexNumber }));

  function handleEpisodeChange(episodeId: string) {
    usePlayerStore.getState().stop();
    void router.navigate({ to: "/player/$mediaId", params: { mediaId: episodeId } });
  }

  const handleEnded = useCallback(() => {
    handleClose();
  }, [handleClose]);

  async function handleWatchTogether() {
    try {
      const roomId = await syncControls.createRoom(mediaId);
      setShareRoomId(roomId);
      setShowShareDialog(true);
    } catch {
      // ignore
    }
  }

  const shareUrl = shareRoomId
    ? `${window.location.origin}/player/${mediaId}?room=${shareRoomId}`
    : "";

  return (
    <div
      className="fixed inset-0 bg-black z-overlay flex flex-col"
    >
      <div className="flex-1 relative flex items-center justify-center">
        {!syncState.roomId && !isLoading && !isError && streamInfo && (
          <button
            aria-label={t("player.watchTogether")}
            onClick={() => void handleWatchTogether()}
            className="absolute top-4 right-4 z-overlay-raised flex items-center gap-2 px-3 py-2 rounded-full text-white text-sm font-medium transition-opacity-fast hover:opacity-80"
            style={{ backgroundColor: "var(--overlay-bg)", border: "1px solid var(--overlay-btn-bg)" }}
          >
            <Users size={16} />
            {t("player.watchTogether")}
          </button>
        )}
        {isError ? (
          <div className="flex flex-col items-center gap-4 text-white">
            <TriangleAlert size={48} style={{ color: "var(--color-danger)" }} strokeWidth={1.5} />
            <p className="text-lg font-medium">{t("player.failedToStart")}</p>
            <div className="flex gap-3">
              <Button variant="overlay-accent" onClick={() => window.location.reload()}>
                {t("player.retry")}
              </Button>
              <Button variant="overlay" onClick={handleClose}>
                {t("player.goBack")}
              </Button>
            </div>
          </div>
        ) : isLoading || !streamInfo || !sessionId || !streamToken ? (
          <div className="flex flex-col items-center gap-4">
            {loadingTimedOut ? (
              <>
                <TriangleAlert size={48} style={{ color: "var(--color-danger)" }} strokeWidth={1.5} />
                <p className="text-lg font-medium text-white">{t("player.failedToStart")}</p>
                <div className="flex gap-3">
                  <Button variant="overlay-accent" onClick={() => window.location.reload()}>{t("player.retry")}</Button>
                  <Button variant="overlay" onClick={handleClose}>{t("player.goBack")}</Button>
                </div>
              </>
            ) : (
              <>
                <Spinner />
                <Button variant="overlay" className="mt-4" onClick={handleClose} data-testid="player-loading-back">{t("player.goBack")}</Button>
              </>
            )}
          </div>
        ) : (
          <HLSPlayer
            key={mediaId}
            streamInfo={streamInfo}
            sessionId={sessionId}
            streamToken={streamToken}
            onStreamTokenRefresh={handleStreamTokenRefresh}
            mediaId={mediaId}
            mediaType={mediaItem?.type}
            startPositionMs={startPositionMs}
            knownDurationMs={mediaItem?.durationMs ? Number(mediaItem.durationMs) : undefined}
            filePath={mediaItem?.filePath}
            audioStreams={audioStreams}
            subtitleStreams={subtitleStreams}
            title={mediaItem?.title}
            onClose={handleClose}
            onEnded={handleEnded}
            seasons={isEpisode ? seasons : undefined}
            currentSeasonId={selectedSeasonId}
            episodes={isEpisode ? episodes : undefined}
            currentEpisodeId={mediaId}
            onSeasonChange={setSelectedSeasonId}
            onEpisodeChange={handleEpisodeChange}
            chapters={chapters}
            seriesId={isEpisode ? seriesId : undefined}
            onVideoRef={(el) => {
              videoRef.current = el;
              usePlayerStore.getState().setVideoElement(el);
              if (el) {
                setPlayerCleanup(() => { el.pause(); el.removeAttribute("src"); el.load(); });
              } else {
                setPlayerCleanup(null);
              }
            }}
          />
        )}
        {showShareDialog && shareRoomId && (
          <div
            className="absolute inset-0 z-overlay-top flex items-center justify-center"
            style={{ background: "var(--overlay-bg-strong)" }}
            onClick={() => setShowShareDialog(false)}
          >
            <div
              className="rounded-xl p-6 text-white flex flex-col gap-4 max-w-sm w-full mx-4"
              style={{ background: "var(--bg-gradient-mid)", border: "1px solid var(--overlay-border-subtle)" }}
              onClick={(e) => e.stopPropagation()}
            >
              <h2 className="text-lg font-semibold">{t("player.watchTogether")}</h2>
              <p className="text-sm text-white/70">{t("player.shareDesc")}</p>
              <div
                className="rounded px-3 py-2 text-sm font-mono break-all select-all"
                style={{ background: "var(--overlay-border-subtle)" }}
              >
                {shareUrl}
              </div>
              <div className="flex gap-3">
                <Button variant="overlay" className="flex-1" onClick={() => void navigator.clipboard.writeText(shareUrl)}>
                  {t("player.copyLink")}
                </Button>
                <Button variant="overlay-accent" className="flex-1" onClick={() => setShowShareDialog(false)}>
                  {t("player.close")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
      {syncState.roomId && (
        <SyncPlayBar
          roomId={syncState.roomId}
          roomCode={syncState.roomCode}
          isHost={syncState.isHost}
          members={syncState.members}
          connected={syncState.connected}
          onLeave={() => void syncControls.leaveRoom()}
        />
      )}
      <PluginSlot name="player.overlay" context={{ mediaId }} />
    </div>
  );
}
