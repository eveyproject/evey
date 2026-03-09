import { createLazyFileRoute, useRouter } from "@tanstack/react-router";
import { useCallback, useMemo, useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem, libraryServiceListMediaItems } from "@evey/api";
import { usePlaySession } from "@/hooks/usePlaySession";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { Spinner } from "@/components/ui/Spinner";
import type { AudioStream, SubtitleStream } from "@/features/playback/HLSPlayer";

export const Route = createLazyFileRoute("/_auth/player/$mediaId")({
  component: PlayerPage,
});

function PlayerPage() {
  const { mediaId } = Route.useParams();
  const router = useRouter();
  const { streamInfo, sessionId, isLoading, progress, stopSession } = usePlaySession(mediaId);
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

  const subtitleStreams: SubtitleStream[] =
    streamInfo?.mediaStreams
      ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
      .map((s, i) => ({
        index: s.index ?? 0,
        trackIndex: i,
        language: s.language,
        title: s.title,
        isDefault: s.isDefault,
        isExternal: s.isExternal,
        externalPath: s.externalPath,
      })) ?? [];

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
      className="fixed inset-0 bg-black z-50 flex items-center justify-center"
    >
      {isLoading || !streamInfo || !sessionId ? (
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
        />
      )}
    </div>
  );
}
