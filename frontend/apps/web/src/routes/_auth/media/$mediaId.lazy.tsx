import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { MediaDetailSkeleton } from "@/components/ui/Skeleton";
import { MediaDetail } from "@/features/library/MediaDetail";
import { EmptyState } from "@/components/ui/EmptyState";
import { classifyError } from "@/utils/errorClassification";

export const Route = createLazyFileRoute("/_auth/media/$mediaId")({
  component: MediaPage,
});

function MediaPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { mediaId } = Route.useParams();
  const { edit } = Route.useSearch();

  const { data: item, isLoading, isError, error, refetch } = useQuery({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  if (isLoading) {
    return <MediaDetailSkeleton />;
  }

  // A genuinely missing item (404, or the server returning nothing without
  // erroring) is a dead end - no retry will fix it, so the only issue out is
  // going back to something that still exists. A server error or a dropped
  // connection is different: the item may well exist, so offer a retry instead
  // of telling the user it's gone.
  if (isError) {
    const kind = classifyError(error);
    if (kind === "not-found") {
      return (
        <EmptyState
          title={t("media.detail.notFound")}
          action={{ label: t("common.errorBoundary.goHome"), onClick: () => void navigate({ to: "/home" }) }}
        />
      );
    }
    return (
      <EmptyState
        icon={<TriangleAlert size={48} strokeWidth={1.5} />}
        title={kind === "offline" ? t("common.offline.title", "Offline") : t("common.errorBoundary.title")}
        description={
          kind === "offline"
            ? t("common.offline.description", "Check your internet connection and try again.")
            : t("common.errorBoundary.description")
        }
        action={{ label: t("common.retry", "Retry"), onClick: () => void refetch() }}
      />
    );
  }

  if (!item) {
    return (
      <EmptyState
        title={t("media.detail.notFound")}
        action={{ label: t("common.errorBoundary.goHome"), onClick: () => void navigate({ to: "/home" }) }}
      />
    );
  }

  return <MediaDetail item={item} initialEdit={edit} />;
}
