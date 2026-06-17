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
import { useAuthStore, usePlayerStore, setPlayerCleanup, useMusicPlayerStore } from "@evey/store";

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

  // Close the global music mini-player when a full-screen video player opens —
  // audio + video playback at the same time is bad UX.
  const closeMusic = useMusicPlayerStore((s) => s.close);
  useEffect(() => { closeMusic(); }, [closeMusic]);

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
            onClose={() => {
              if (router.history.canGoBack()) {
                router.history.back();
              } else {
                void router.navigate({ to: "/media/$mediaId", params: { mediaId } });
              }
            }}
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

  // Leaving the player (back button, navigation, close) stops playback and ends
  // the session immediately — no mini-player keeps it running in the background.
  useEffect(() => {
    usePlayerStore.getState().play(mediaId);
    return () => {
      const state = usePlayerStore.getState();
      if (state.isActive) state.stop();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaId]);

  useEffect(() => {
    if (sessionId) usePlayerStore.getState().setSessionId(sessionId);
  }, [sessionId]);

  useEffect(() => {
    // Expose the session-stop fn so store.stop() (called on unmount/close AND on
    // every episode change via the [mediaId] effect cleanup) can tear down the
    // server session. store.stop() NULLS _stopSessionFn after firing, and this
    // component does NOT remount on episode change (only HLSPlayer is keyed by
    // mediaId) — so we must re-arm whenever a new session starts, or every
    // intermediate episode in a binge leaks an orphaned transcode session on the
    // server (A stops via the still-armed fn, the last via usePlaySession's
    // unmount cleanup, but B..N-1 never do). sessionId in the deps re-arms it.
    usePlayerStore.getState().setStopSessionFn(stopSession);
  }, [stopSession, sessionId]);

  const handleStreamTokenRefresh = useCallback(() => fetchStreamToken(mediaId), [fetchStreamToken, mediaId]);
  const handleCastTokenRequest = useCallback(() => fetchStreamToken(mediaId, { cast: true }), [fetchStreamToken, mediaId]);

  const [syncState, syncControls] = useSyncPlay({
    onPlay: (positionMs) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = positionMs / 1000;
      v.play().catch((err) => console.warn("v.play failed", err));
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
      syncControls.joinRoom(roomParam).catch((err) => console.warn("joinRoom failed", err));
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
    // router.history.back() is a no-op when the player was reached via a
    // deep link, a hard refresh, or any path that left the SPA history
    // empty — which is exactly when users complain "back does nothing".
    // Fall back to the media detail page of whatever is playing so the
    // close button always lands somewhere sane.
    if (router.history.canGoBack()) {
      router.history.back();
    } else {
      void router.navigate({ to: "/media/$mediaId", params: { mediaId } });
    }
  }, [router, mediaId]);

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
          isForced: s.isForced,
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
    } catch (err) {
      if (import.meta.env.DEV) console.error("SyncPlay createRoom failed:", err);
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
            <Users size={16}  aria-hidden="true" />
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
            onCastTokenRequest={handleCastTokenRequest}
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
                setPlayerCleanup(() => {
                  // play() returns a Promise that rejects with AbortError
                  // when pause() interrupts an in-flight fetch (esp. on
                  // HLS reload mid-seek). Swallow it — by the time we're
                  // here the user has closed the player, no UX impact.
                  try { el.pause(); } catch { /* ignore */ }
                  try {
                    el.removeAttribute("src");
                    el.load();
                  } catch { /* ignore */ }
                });
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
              className="rounded-xl p-4 sm:p-6 text-white flex flex-col gap-4 max-w-sm w-full mx-4 max-h-[90vh] overflow-y-auto"
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
