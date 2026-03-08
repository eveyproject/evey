import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { usePlaySession } from "@/hooks/usePlaySession";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { Spinner } from "@/components/ui/Spinner";
import type { AudioStream, SubtitleStream } from "@/features/playback/HLSPlayer";

export const Route = createFileRoute("/_auth/player/$mediaId")({
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

  function handleClose() {
    stopSession();
    router.history.back();
  }

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
          audioStreams={audioStreams}
          subtitleStreams={subtitleStreams}
          title={mediaItem?.title}
          onClose={handleClose}
          onEnded={handleClose}
        />
      )}
    </div>
  );
}
