import { createFileRoute, useRouter } from "@tanstack/react-router";
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

  function handleClose() {
    stopSession();
    router.history.back();
  }

  const audioStreams: AudioStream[] =
    streamInfo?.mediaStreams
      ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
      .map((s) => ({
        index: s.index ?? 0,
        language: s.language,
        title: s.title,
        isDefault: s.isDefault,
      })) ?? [];

  const subtitleStreams: SubtitleStream[] =
    streamInfo?.mediaStreams
      ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
      .map((s) => ({
        index: s.index ?? 0,
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
          startPositionMs={startPositionMs}
          audioStreams={audioStreams}
          subtitleStreams={subtitleStreams}
          onClose={handleClose}
          onEnded={handleClose}
        />
      )}
    </div>
  );
}
