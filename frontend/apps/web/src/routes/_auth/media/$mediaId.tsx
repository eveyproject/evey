import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { MediaDetail } from "@/features/library/MediaDetail";

export const Route = createFileRoute("/_auth/media/$mediaId")({
  component: MediaPage,
});

function MediaPage() {
  const { mediaId } = Route.useParams();

  const { data: item, isLoading, isError } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
        Media item not found.
      </div>
    );
  }

  return <MediaDetail item={item} />;
}
