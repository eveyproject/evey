import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useCollections, useDeleteCollection } from "@/features/collection/api";
import type { Collection } from "@/features/collection/api";
import { EmptyState } from "@/components/ui/EmptyState";

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

function CollectionsPage() {
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Collections
        </h1>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-4">
          {Array.from({ length: 10 }).map((_, i) => (
            <CollectionCardSkeleton key={i} />
          ))}
        </div>
      ) : collections.length === 0 ? (
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
          {collections.map((col) => (
            <CollectionPosterCard key={col.id} col={col} />
          ))}
        </div>
      )}
    </div>
  );
}
