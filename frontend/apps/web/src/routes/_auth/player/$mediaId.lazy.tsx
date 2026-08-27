import { TriangleAlert } from "lucide-react";
import { createLazyFileRoute, useRouter } from "@tanstack/react-router";
import { useCallback, useMemo, useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem, libraryServiceListMediaItems } from "@evey/api";
import { usePlaySession } from "@/hooks/usePlaySession";
import { useChapters } from "@/hooks/useChapters";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { playerExit, resolveStartPositionMs } from "@/features/playback/playerUtils";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import type { AudioStream, SubtitleStream } from "@/features/playback/HLSPlayer";
import { useSyncPlay } from "@/features/syncplay/useSyncPlay";
import { SyncPlayBar } from "@/features/syncplay/SyncPlayBar";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { useParentalGate, markUnlocked } from "@/hooks/useParentalGate";
import { ParentalPINDialog } from "@/components/ParentalPINDialog";
import { useFocusTrap } from "@/hooks/useFocusTrap";
import { useAuthStore, usePlayerStore, setPlayerCleanup, useMusicPlayerStore, toast } from "@evey/store";

// Seasons of a series: far beyond any real count, stated rather than implied.
const SEASONS_PAGE_SIZE = 100;

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

export const Route = createLazyFileRoute("/_auth/player/$mediaId")({
  component: PlayerPage,
});

function PlayerPage() {
  const { mediaId } = Route.useParams();
  const router = useRouter();
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);

  // Close the global music mini-player when a full-screen video player opens -
  // audio + video playback at the same time is bad UX.
  const closeMusic = useMusicPlayerStore((s) => s.close);
  useEffect(() => { closeMusic(); }, [closeMusic]);

  const { data: gateItem, isLoading: gateLoading } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  const { needsPin, settingsLoading } = useParentalGate(gateItem?.officialRating);
  const [pinVerified, setPinVerified] = useState(false);
  // Hold the spinner until the parental settings landed too: evaluating
  // needsPin() while they were in flight returned false, mounted the player
  // and started a transcode session BEFORE the PIN dialog appeared.
  const gatePending = gateLoading || settingsLoading;
  const blocked = !gatePending && needsPin() && !pinVerified;

  // Leaving the gate before the player mounts: back if we can, else fall to the
  // media page. Shared by the gate spinner's escape and the PIN dialog's close.
  const goBackFromGate = useCallback(() => {
    if (router.history.canGoBack()) {
      router.history.back();
    } else {
      void router.navigate({ to: "/media/$mediaId", params: { mediaId } });
    }
  }, [router, mediaId]);

  // A hung media-item or parental-settings request would otherwise leave a
  // black screen with no way out (the inner player-loading branch already has
  // this safety net; the gate stage lacked it).
  const [gateTimedOut, setGateTimedOut] = useState(false);
  useEffect(() => {
    if (!gatePending) { setGateTimedOut(false); return; }
    const timer = setTimeout(() => setGateTimedOut(true), 30_000);
    return () => clearTimeout(timer);
  }, [gatePending]);

  if (gatePending) {
    return (
      <div className="fixed inset-0 bg-black z-overlay flex flex-col items-center justify-center gap-4">
        {gateTimedOut ? (
          <>
            <TriangleAlert size={48} style={{ color: "var(--color-danger)" }} strokeWidth={1.5} />
            <p className="text-lg font-medium text-white">{t("player.failedToStart")}</p>
            <div className="flex gap-3">
              <Button variant="overlay-accent" onClick={() => window.location.reload()}>{t("player.retry")}</Button>
              <Button variant="overlay" onClick={goBackFromGate}>{t("player.goBack")}</Button>
            </div>
          </>
        ) : (
          <>
            <Spinner />
            <Button variant="overlay" className="mt-4" onClick={goBackFromGate} data-testid="gate-loading-back">{t("player.goBack")}</Button>
          </>
        )}
      </div>
    );
  }

  if (blocked) {
    return (
      <div className="fixed inset-0 bg-black z-overlay flex items-center justify-center">
        {profileId && (
          <ParentalPINDialog
            open
            onClose={goBackFromGate}
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
  const { room: roomParam, restart } = Route.useSearch();
  const router = useRouter();
  const [showShareDialog, setShowShareDialog] = useState(false);
  const [shareRoomId, setShareRoomId] = useState<string | null>(null);
  const shareDialogRef = useRef<HTMLDivElement>(null);
  const shareOpen = showShareDialog && !!shareRoomId;
  useFocusTrap(shareDialogRef, shareOpen);
  // Escape closes the share dialog. Capture phase + stopImmediatePropagation so
  // it wins over the player's bubble-phase window keydown listener, which would
  // otherwise exit the player entirely.
  useEffect(() => {
    if (!shareOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopImmediatePropagation();
        setShowShareDialog(false);
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [shareOpen]);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { streamInfo, sessionId, streamToken, fetchStreamToken, isLoading, isError, progress, stopSession } = usePlaySession(mediaId);
  const [loadingTimedOut, setLoadingTimedOut] = useState(false);
  useEffect(() => {
    if (!isLoading) { setLoadingTimedOut(false); return; }
    const timer = setTimeout(() => setLoadingTimedOut(true), 30_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  // Leaving the player (back button, navigation, close) stops playback and ends
  // the session immediately - no mini-player keeps it running in the background.
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
    // mediaId) - so we must re-arm whenever a new session starts, or every
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
    // Closing POPS the player's entry. Navigating forward to the media page
    // instead left the player in the back stack, and the media page's own
    // "Retour" then walked right back into it and started the episode again.
    // See playerExit for why back() is safe here (episode changes replace).
    const exit = playerExit(router.history.canGoBack(), mediaId, mediaItem);
    if (exit.action === "back") {
      router.history.back();
      return;
    }
    void router.navigate({
      to: "/media/$mediaId",
      params: { mediaId: exit.mediaId },
      replace: true,
    });
  }, [router, mediaId, mediaItem?.type, mediaItem?.parentId]);

  // Memoise so these arrays keep a stable identity across re-renders driven by
  // unrelated state (share dialog, syncplay ws, season selection): they flow
  // into PlayerProvider, whose own value memo exists precisely to avoid churning
  // every player consumer, and a fresh array every render defeats it.
  const audioStreams: AudioStream[] = useMemo(
    () =>
      streamInfo?.mediaStreams
        ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
        .map((s, i) => ({
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          codec: s.codec,
          channels: s.channels,
        })) ?? [],
    [streamInfo?.mediaStreams],
  );

  const subtitleStreams: SubtitleStream[] = useMemo(
    () =>
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
        }) ?? [],
    [streamInfo?.mediaStreams],
  );

  const { data: chapters } = useChapters(mediaId);
  const startPositionMs = resolveStartPositionMs(!!restart, progress);

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
    // Seasons, not episodes: 100 is far beyond any real series, and stating it
    // beats leaving the handler default to be read as either a decision or an
    // oversight.
    queryFn: ({ signal }) => libraryServiceListMediaItems({ parentId: seriesId, "pagination.pageSize": SEASONS_PAGE_SIZE }, signal),
    enabled: !!seriesId,
  });

  const [selectedSeasonId, setSelectedSeasonId] = useState(seasonId);
  useEffect(() => { if (seasonId) setSelectedSeasonId(seasonId); }, [seasonId]);

  const { data: episodesData } = useQuery({
    queryKey: ["/api/v1/media", { parentId: selectedSeasonId }],
    // Episodes have no server-side sort key, so this comes back in sort_title
    // order and is re-sorted by index just below. Without an explicit size the
    // handler serves 50, which past a 50-episode season fills the player's
    // episode picker with whichever titles sort first alphabetically. The
    // mobile player already asks for 500 here.
    queryFn: ({ signal }) => libraryServiceListMediaItems({ parentId: selectedSeasonId, "pagination.pageSize": 500 }, signal),
    enabled: !!selectedSeasonId,
  });

  const seasons = useMemo(
    () =>
      [...(seasonsData?.items ?? [])]
        .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0))
        .map((s) => ({ id: s.id!, title: s.title, indexNumber: s.indexNumber })),
    [seasonsData?.items],
  );

  const episodes = useMemo(
    () =>
      [...(episodesData?.items ?? [])]
        .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0))
        .map((ep) => ({ id: ep.id!, title: ep.title ?? `Episode ${ep.indexNumber}`, indexNumber: ep.indexNumber })),
    [episodesData?.items],
  );

  const handleEpisodeChange = useCallback(
    (episodeId: string) => {
      usePlayerStore.getState().stop();
      // REPLACE, never push: this is both the autoplay chain and the episode
      // picker, and one entry per episode is what made "back" walk a binge
      // backwards through the season. Replacing keeps exactly one player entry,
      // which is also the invariant playerExit's back() relies on.
      void router.navigate({ to: "/player/$mediaId", params: { mediaId: episodeId }, replace: true });
    },
    [router],
  );

  const handleEnded = useCallback(() => {
    handleClose();
  }, [handleClose]);

  async function handleWatchTogether() {
    try {
      const roomId = await syncControls.createRoom(mediaId);
      setShareRoomId(roomId);
      setShowShareDialog(true);
    } catch (err) {
      // In a production build this catch was empty: the dialog simply never
      // opened and nothing explained why.
      console.warn("SyncPlay createRoom failed", err);
      toast.error(t("common.error"));
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black z-overlay flex flex-col"
    >
      <div className="flex-1 relative flex items-center justify-center">
        {/* Watch Together moved into HLSPlayer's top-right cluster (next to the
            cast icon); the trigger + visibility are passed down below. */}
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
            onWatchTogether={() => void handleWatchTogether()}
            showWatchTogether={!syncState.roomId}
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
                  // HLS reload mid-seek). Swallow it - by the time we're
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
              ref={shareDialogRef}
              role="dialog"
              aria-modal="true"
              aria-labelledby="share-dialog-title"
              tabIndex={-1}
              className="rounded-xl p-4 sm:p-6 text-white flex flex-col gap-4 max-w-sm w-full mx-4 max-h-[90vh] overflow-y-auto"
              style={{ background: "var(--bg-gradient-mid)", border: "1px solid var(--overlay-border-subtle)" }}
              onClick={(e) => e.stopPropagation()}
            >
              <h2 id="share-dialog-title" className="text-lg font-semibold">{t("player.watchTogether")}</h2>
              <p className="text-sm text-white/70">{t("player.codeShareDesc", "Share this code - anyone can enter it to join and watch with you.")}</p>
              <div
                className="rounded px-3 py-3 text-center font-mono text-3xl tracking-[0.3em] font-bold select-all"
                style={{ background: "var(--overlay-border-subtle)" }}
              >
                {syncState.roomCode}
              </div>
              <div className="flex gap-3">
                <Button
                  variant="overlay"
                  className="flex-1"
                  onClick={() => { if (syncState.roomCode) void navigator.clipboard.writeText(syncState.roomCode); }}
                >
                  {t("player.copyCode", "Copy code")}
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
          onResync={() => syncControls.sendResync(Math.floor((videoRef.current?.currentTime ?? 0) * 1000))}
        />
      )}
      <PluginSlot name="player.overlay" context={{ mediaId }} />
    </div>
  );
}
