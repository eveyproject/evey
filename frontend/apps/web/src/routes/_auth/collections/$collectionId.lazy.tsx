import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useCollection, useUpdateCollection, useDeleteCollection } from "@/features/collection/api";
import { CollectionModal } from "@/features/collection/CollectionModal";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Pagination } from "@/components/ui/Pagination";
import { BackButton } from "@/components/ui/BackButton";

export const Route = createLazyFileRoute("/_auth/collections/$collectionId")({
  component: CollectionDetailPage,
});

function CollectionDetailPage() {
  const { collectionId } = Route.useParams();
  const [page, setPage] = useState(1);
  const pageSize = 50;
  const [editOpen, setEditOpen] = useState(false);
  const { data, isLoading } = useCollection(collectionId, pageSize, (page - 1) * pageSize);
  const updateMut = useUpdateCollection();
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
        <div className="flex items-start justify-between mb-6 mt-4">
          <div>
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
                {col.name}
              </h1>
              <Badge variant={col.type === "smart" ? "info" : "default"}>
                {col.type}
              </Badge>
            </div>
            {col.overview && (
              <p className="text-sm" style={{ color: "var(--text-muted)" }}>
                {col.overview}
              </p>
            )}
            <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>
              {total} {total === 1 ? "item" : "items"}
              {col.type === "smart" && col.rules.length > 0 && (
                <span>
                  {" "} &mdash; {col.rules.map((r) => `${r.field} ${r.operator} ${r.value}`).join(", ")}
                </span>
              )}
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setEditOpen(true)}>
              Edit
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                if (confirm("Delete this collection?")) {
                  deleteMut.mutate(collectionId, {
                    onSuccess: () => navigate({ to: "/collections" }),
                  });
                }
              }}
            >
              Delete
            </Button>
          </div>
        </div>
      )}

      {isLoading ? (
        <MediaGrid isLoading>{null}</MediaGrid>
      ) : items.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="library"
            title="No matching items"
            description={
              col?.type === "smart"
                ? "No media items match your rules. Try adjusting the filters."
                : "Add items to this collection."
            }
          />
        </div>
      ) : (
        <>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} />
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

      {col && (
        <CollectionModal
          open={editOpen}
          onClose={() => setEditOpen(false)}
          initial={col}
          onSubmit={(data) => updateMut.mutate({ id: collectionId, ...data })}
        />
      )}
    </div>
  );
}
