import { createLazyFileRoute, useRouter } from "@tanstack/react-router";
import { useCallback, useMemo, useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem, libraryServiceListMediaItems } from "@evey/api";
import { usePlaySession } from "@/hooks/usePlaySession";
import { useChapters } from "@/hooks/useChapters";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { Spinner } from "@/components/ui/Spinner";
import type { AudioStream, SubtitleStream } from "@/features/playback/HLSPlayer";

export const Route = createLazyFileRoute("/_auth/player/$mediaId")({
  component: PlayerPage,
});

function PlayerPage() {
  const { mediaId } = Route.useParams();
  const router = useRouter();
  const [theaterMode, setTheaterMode] = useState(false);
  const { streamInfo, sessionId, isLoading, isError, progress, stopSession } = usePlaySession(mediaId);
  const { data: mediaItem } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  const handleClose = useCallback(() => {
    stopSession();
    router.history.back();
  }, [stopSession, router]);

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

  const IMAGE_SUBTITLE_CODECS = new Set([
    "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
  ]);

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
    stopSession();
    router.navigate({ to: "/player/$mediaId", params: { mediaId: episodeId } });
  }

  const hasNextEpisode = useMemo(() => {
    if (!isEpisode || episodes.length === 0) return false;
    const idx = episodes.findIndex((e) => e.id === mediaId);
    return idx >= 0 && idx < episodes.length - 1;
  }, [isEpisode, episodes, mediaId]);

  const handleEnded = useCallback(() => {
    if (!hasNextEpisode) handleClose();
  }, [hasNextEpisode, handleClose]);

  return (
    <div
      className={`fixed inset-0 bg-black z-50 flex items-center justify-center${theaterMode ? " theater-mode" : ""}`}
      style={theaterMode ? { padding: 0 } : undefined}
    >
      <button
        aria-label="Exit player"
        onClick={handleClose}
        className="absolute top-4 left-4 z-[60] flex items-center justify-center w-9 h-9 rounded-full text-white transition-opacity hover:opacity-80"
        style={{ backgroundColor: "rgba(0,0,0,0.5)", border: "1px solid rgba(255,255,255,0.2)" }}
      >
        <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
      {isError ? (
        <div className="flex flex-col items-center gap-4 text-white">
          <svg className="w-12 h-12 text-red-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
          </svg>
          <p className="text-lg font-medium">Failed to start playback</p>
          <div className="flex gap-3">
            <button
              onClick={() => window.location.reload()}
              className="px-4 py-2 rounded text-sm font-medium"
              style={{ backgroundColor: "rgba(255,255,255,0.25)" }}
            >
              Retry
            </button>
            <button
              onClick={handleClose}
              className="px-4 py-2 rounded text-sm font-medium"
              style={{ backgroundColor: "rgba(255,255,255,0.15)" }}
            >
              Go back
            </button>
          </div>
        </div>
      ) : isLoading || !streamInfo || !sessionId ? (
        <Spinner />
      ) : (
        <HLSPlayer
          streamInfo={streamInfo}
          sessionId={sessionId}
          mediaId={mediaId}
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
          onTheaterModeChange={setTheaterMode}
        />
      )}
    </div>
  );
}
