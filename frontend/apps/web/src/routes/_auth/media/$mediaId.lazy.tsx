import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { useTranslation } from "react-i18next";
import { MediaDetailSkeleton } from "@/components/ui/Skeleton";
import { MediaDetail } from "@/features/library/MediaDetail";

export const Route = createLazyFileRoute("/_auth/media/$mediaId")({
  component: MediaPage,
});

function MediaPage() {
  const { t } = useTranslation("media");
  const { mediaId } = Route.useParams();
  const { edit } = Route.useSearch();

  const { data: item, isLoading, isError } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  if (isLoading) {
    return <MediaDetailSkeleton />;
  }

  if (isError || !item) {
    return (
      <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
        {t("detail.notFound")}
      </div>
    );
  }

  return <MediaDetail item={item} initialEdit={edit} />;
}
