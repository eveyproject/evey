import { useState } from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useCollections, useDeleteCollection } from "@/features/collection/api";
import type { Collection } from "@/features/collection/api";
import { EmptyState } from "@/components/ui/EmptyState";
import { Button } from "@/components/ui/Button";
import { SmartCollectionDialog } from "@/features/collection/SmartCollectionDialog";
import type { SmartCollection } from "@/features/collection/SmartCollectionDialog";

export const Route = createLazyFileRoute("/_auth/collections/")({
  component: CollectionsPage,
});

function CollectionCardSkeleton() {
  return (
    <div
      className="animate-pulse rounded-[var(--radius-lg)] overflow-hidden"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", aspectRatio: "2/3" }}
    />
  );
}

function CollectionPosterCard({ col }: { col: Collection }) {
  const deleteMut = useDeleteCollection();
  const initial = col.name[0]?.toUpperCase() ?? "?";

  return (
    <div className="group relative">
      <Link
        to="/collections/$collectionId"
        params={{ collectionId: col.id }}
        className="block overflow-hidden transition-transform duration-200 group-hover:scale-105"
        style={{ borderRadius: "var(--radius-lg)" }}
      >
        <div style={{ aspectRatio: "2/3", position: "relative" }}>
          {col.posterUrl ? (
            <img
              src={col.posterUrl}
              alt={col.name}
              loading="lazy"
              className="w-full h-full object-cover"
            />
          ) : (
            <div
              className="w-full h-full flex items-center justify-center text-4xl font-bold"
              style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
            >
              {initial}
            </div>
          )}
          <div
            className="absolute inset-0"
            style={{ background: "linear-gradient(to top, rgba(0,0,0,0.7) 0%, transparent 50%)" }}
          />
          <p
            className="absolute bottom-2 left-2 right-2 text-sm font-semibold text-white truncate"
          >
            {col.name}
          </p>
        </div>
      </Link>

      <button
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          if (confirm("Delete this collection?")) {
            deleteMut.mutate(col.id);
          }
        }}
        className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity p-1.5 rounded-full bg-black/60 text-white"
        aria-label="Delete collection"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
        </svg>
      </button>
    </div>
  );
}

function collectionToSmartCollection(col: Collection): SmartCollection {
  return {
    id: col.id,
    name: col.name,
    rules: (col.rules ?? []) as SmartCollection["rules"],
    sortBy: col.sortBy,
    createdAt: col.createdAt,
  };
}

function SmartFilterCard({ col, onEdit, onDelete }: { col: Collection; onEdit: () => void; onDelete: () => void }) {
  const rulesLabel = (col.rules ?? [])
    .map((r) => {
      if (r.field === "genre") return `genre = ${r.value}`;
      if (r.field === "year") return `year ${r.operator} ${r.value}`;
      if (r.field === "rating_gte") return `rating >= ${r.value}`;
      if (r.field === "library_id") return `library = ${r.value}`;
      const typeLabel = r.value.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();
      return `type = ${typeLabel}`;
    })
    .join(", ");

  return (
    <div
      className="group relative flex flex-col gap-2 p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className="font-semibold truncate" style={{ color: "var(--text)" }}>
            {col.name}
          </p>
          {rulesLabel && (
            <p className="text-xs mt-0.5 truncate" style={{ color: "var(--text-muted)" }}>
              {rulesLabel}
            </p>
          )}
        </div>
        <div className="flex gap-1 flex-shrink-0">
          <button
            onClick={onEdit}
            aria-label="Edit filter"
            className="p-1.5 rounded hover:bg-[var(--surface-raised)] transition-colors"
            style={{ color: "var(--text-muted)" }}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
            </svg>
          </button>
          <button
            onClick={onDelete}
            aria-label="Delete filter"
            className="p-1.5 rounded hover:bg-[var(--surface-raised)] transition-colors"
            style={{ color: "var(--text-muted)" }}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
            </svg>
          </button>
        </div>
      </div>
      <Link
        to="/collections/smart/$filterId"
        params={{ filterId: col.id }}
        className="text-xs font-medium transition-colors"
        style={{ color: "var(--accent)" }}
      >
        Browse results &rarr;
      </Link>
    </div>
  );
}

function CollectionsPage() {
  const { data, isLoading } = useCollections();
  const deleteMut = useDeleteCollection();
  const allCollections = data?.collections ?? [];

  const smartCollections = allCollections.filter((c) => (c.rules?.length ?? 0) > 0);
  const tmdbCollections = allCollections.filter((c) => (c.rules?.length ?? 0) === 0);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<SmartCollection | undefined>(undefined);

  function openCreate() { setEditing(undefined); setDialogOpen(true); }
  function openEdit(col: Collection) { setEditing(collectionToSmartCollection(col)); setDialogOpen(true); }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Collections
        </h1>
        <Button variant="secondary" onClick={openCreate}>
          <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          New smart collection
        </Button>
      </div>

      {/* Smart collections section */}
      {smartCollections.length > 0 && (
        <div className="mb-8">
          <h2 className="text-sm font-semibold uppercase tracking-wider mb-3" style={{ color: "var(--text-muted)" }}>
            Smart Collections
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-3">
            {smartCollections.map((col) => (
              <SmartFilterCard
                key={col.id}
                col={col}
                onEdit={() => openEdit(col)}
                onDelete={() => { if (confirm("Delete this smart collection?")) deleteMut.mutate(col.id); }}
              />
            ))}
          </div>
        </div>
      )}

      {/* TMDB collections */}
      <div>
        {smartCollections.length > 0 && (
          <h2 className="text-sm font-semibold uppercase tracking-wider mb-3" style={{ color: "var(--text-muted)" }}>
            TMDB Collections
          </h2>
        )}
        {isLoading ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-4">
            {Array.from({ length: 10 }).map((_, i) => (
              <CollectionCardSkeleton key={i} />
            ))}
          </div>
        ) : tmdbCollections.length === 0 ? (
          <div
            className="rounded-[var(--radius-lg)]"
            style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          >
            <EmptyState
              icon="library"
              title="No collections yet"
              description="Collections are automatically populated from TMDB sagas."
            />
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-4">
            {tmdbCollections.map((col) => (
              <CollectionPosterCard key={col.id} col={col} />
            ))}
          </div>
        )}
      </div>

      <SmartCollectionDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onSaved={() => {}}
        editing={editing}
      />
    </div>
  );
}
