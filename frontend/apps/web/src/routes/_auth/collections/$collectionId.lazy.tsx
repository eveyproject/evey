import { useState } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useCollection, useDeleteCollection } from "@/features/collection/api";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Pagination } from "@/components/ui/Pagination";
import { BackButton } from "@/components/ui/BackButton";

export const Route = createLazyFileRoute("/_auth/collections/$collectionId")({
  component: CollectionDetailPage,
});

function CollectionDetailPage() {
  const { t } = useTranslation();
  const { collectionId } = Route.useParams();
  const [page, setPage] = useState(1);
  const pageSize = 50;
  const { data, isLoading } = useCollection(collectionId, pageSize, (page - 1) * pageSize);
  const deleteMut = useDeleteCollection();
  const navigate = Route.useNavigate();

  const col = data?.collection;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / pageSize);

  return (
    <div>
      <BackButton fallback="/collections" />

      {col && (
        <>
          {col.backdropUrl && (
            <div
              className="relative w-full overflow-hidden mb-6 mt-4 rounded-[var(--radius-lg)] h-[clamp(180px,30vw,320px)]"
            >
              <img
                src={col.backdropUrl}
                alt={col.name}
                className="w-full h-full object-cover"
              />
              <div
                className="absolute inset-0 gradient-collection-fade"
              />
              <div className="absolute bottom-4 left-4 right-4">
                <h1 className="text-3xl font-bold text-white">{col.name}</h1>
                {col.overview && (
                  <p className="text-sm text-white/80 mt-1 line-clamp-2">{col.overview}</p>
                )}
              </div>
            </div>
          )}

          {!col.backdropUrl && (
            <div className="flex items-start justify-between mb-6 mt-4">
              <div>
                <h1 className="text-2xl font-bold mb-1 text-[var(--text)]">
                  {col.name}
                </h1>
                {col.overview && (
                  <p className="text-sm text-[var(--text-muted)]">
                    {col.overview}
                  </p>
                )}
              </div>
              <Button
                variant="danger"
                onClick={() => {
                  if (confirm(t("media.collections.deleteConfirm"))) {
                    deleteMut.mutate(collectionId, {
                      onSuccess: () => navigate({ to: "/collections" }),
                    });
                  }
                }}
              >
                {t("common.delete")}
              </Button>
            </div>
          )}

          <p className="text-xs mb-4 text-[var(--text-muted)]">
            {t("common.item", { count: total })}
          </p>
        </>
      )}

      {isLoading ? (
        <MediaGrid isLoading>{null}</MediaGrid>
      ) : items.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.collections.noItems")}
            description={t("media.collections.noItemsDesc")}
          />
        </div>
      ) : (
        <>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} lite />
            ))}
          </MediaGrid>
          {totalPages > 1 && (
            <div className="mt-6">
              <Pagination
                page={page}
                totalPages={totalPages}
                onPageChange={setPage}
              />
            </div>
          )}
        </>
      )}
    </div>
  );
}
