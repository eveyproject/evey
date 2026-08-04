import { useState } from "react";
import { Folder, Download, X, ChevronUp, ChevronDown } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAuthStore, toast } from "@evey/store";
import { startTokenDownload } from "@/utils/downloadToken";
import { useCollection, useDeleteCollection, useRemoveCollectionItem, useReorderCollectionItems } from "@/features/collection/api";
import { moveItem } from "@/features/collection/reorder";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCardWithModals as MediaCard } from "@/features/library/MediaCardWithModals";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
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
  const { data, isLoading, isError, error, refetch } = useCollection(collectionId, pageSize, (page - 1) * pageSize);
  const deleteMut = useDeleteCollection();
  const navigate = Route.useNavigate();
  const accessToken = useAuthStore((s) => s.accessToken);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const removeItem = useRemoveCollectionItem(collectionId);
  const reorder = useReorderCollectionItems(collectionId);

  const col = data?.collection;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / pageSize);


  const handleDelete = () => {
    if (confirm(t("media.collections.deleteConfirm"))) {
      deleteMut.mutate(collectionId, { onSuccess: () => navigate({ to: "/collections" }) });
    }
  };

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
              {isAdmin && (
                <Button
                  variant="danger"
                  className="absolute top-4 right-4"
                  onClick={handleDelete}
                  data-testid="collection-delete-btn"
                >
                  {t("common.delete")}
                </Button>
              )}
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
              {isAdmin && (
                <Button variant="danger" onClick={handleDelete} data-testid="collection-delete-btn">
                  {t("common.delete")}
                </Button>
              )}
            </div>
          )}

          <div className="flex items-center justify-between gap-3 mb-4">
            <p className="text-xs text-[var(--text-muted)]">
              {t("common.item", { count: total })}
            </p>
            {total > 0 && accessToken && (
              <button
                type="button"
                onClick={async () => {
                  // Click-time mint: the account JWT must not ride in the URL
                  // (proxy logs, history) - see utils/downloadToken.ts.
                  try {
                    await startTokenDownload(`/api/v1/collections/${collectionId}/download`, collectionId, `${col.name ?? "collection"}.zip`);
                  } catch {
                    toast.error(t("common.error"));
                  }
                }}
                data-testid="collection-download-btn"
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-medium bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)] transition-colors-fast cursor-pointer"
              >
                <Download size={16} />
                {t("media.collections.download")}
              </button>
            )}
          </div>
        </>
      )}

      {isLoading ? (
        <MediaGrid isLoading>{null}</MediaGrid>
      ) : isError ? (
        // A collection that failed to load is not a collection someone emptied.
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <FetchErrorState error={error} onRetry={refetch} />
        </div>
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
              <div key={item.id} className="relative group">
                <MediaCard item={item} lite />
                {isAdmin && items.length > 1 && (
                  <div className="absolute top-1 left-1 flex flex-col gap-0.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
                    <button
                      type="button"
                      aria-label={t("media.collections.moveUp")}
                      title={t("media.collections.moveUp")}
                      data-testid={`collection-up-${item.id}`}
                      disabled={reorder.isPending || items[0]?.id === item.id}
                      onClick={() =>
                        reorder.mutate({
                          mediaItemIds: moveItem(items.map((i) => i.id), item.id, "up"),
                          name: col?.name ?? "",
                        })
                      }
                      className="rounded-[var(--radius-md)] bg-[var(--surface)] text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-40 p-1 cursor-pointer"
                    >
                      <ChevronUp size={14} />
                    </button>
                    <button
                      type="button"
                      aria-label={t("media.collections.moveDown")}
                      title={t("media.collections.moveDown")}
                      data-testid={`collection-down-${item.id}`}
                      disabled={reorder.isPending || items[items.length - 1]?.id === item.id}
                      onClick={() =>
                        reorder.mutate({
                          mediaItemIds: moveItem(items.map((i) => i.id), item.id, "down"),
                          name: col?.name ?? "",
                        })
                      }
                      className="rounded-[var(--radius-md)] bg-[var(--surface)] text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-40 p-1 cursor-pointer"
                    >
                      <ChevronDown size={14} />
                    </button>
                  </div>
                )}
                {isAdmin && (
                  <button
                    type="button"
                    aria-label={t("media.collections.removeItem")}
                    title={t("media.collections.removeItem")}
                    data-testid={`collection-remove-${item.id}`}
                    disabled={removeItem.isPending}
                    onClick={() => {
                      if (!confirm(t("media.collections.removeItemConfirm", { title: item.title }))) return;
                      removeItem.mutate(item.id, {
                        onSuccess: () => toast.success(t("media.collections.itemRemoved")),
                        onError: () => toast.error(t("common.error")),
                      });
                    }}
                    className="absolute top-1 right-1 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity rounded-[var(--radius-md)] bg-[var(--surface)] text-[var(--text-muted)] hover:text-[var(--danger)] p-1 cursor-pointer"
                  >
                    <X size={16} />
                  </button>
                )}
              </div>
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
