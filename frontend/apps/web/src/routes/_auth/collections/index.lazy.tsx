import { useState } from "react";
import { Folder, Trash2, Pencil, Plus } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useCollections, useDeleteCollection } from "@/features/collection/api";
import type { Collection } from "@/features/collection/api";
import { EmptyState } from "@/components/ui/EmptyState";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { SmartCollectionDialog } from "@/features/collection/SmartCollectionDialog";
import type { SmartCollection } from "@/features/collection/SmartCollectionDialog";

export const Route = createLazyFileRoute("/_auth/collections/")({
  component: CollectionsPage,
});

export interface CollectionSubtitleParts {
  badge?: string;
  count?: string;
}

export function collectionSubtitle(
  kind: string | undefined,
  itemCount: number | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
): CollectionSubtitleParts {
  const parts: CollectionSubtitleParts = {};
  if (kind === "saga") parts.badge = t("media.collections.saga");
  if (itemCount !== undefined && itemCount > 0) parts.count = t("media.person.titleCount", { count: itemCount });
  return parts;
}

function CollectionCardSkeleton() {
  return (
    <div
      className="animate-pulse rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)] aspect-[2/3]"
    />
  );
}

function CollectionPosterCard({ col }: { col: Collection }) {
  const { t } = useTranslation();
  const deleteMut = useDeleteCollection();
  const initial = col.name[0]?.toUpperCase() ?? "?";
  const subtitle = collectionSubtitle(col.kind, col.itemCount, t);

  return (
    <div className="group relative">
      <Link
        to="/collections/$collectionId"
        params={{ collectionId: col.id }}
        className="block overflow-hidden transition-transform duration-200 group-hover:scale-105 rounded-[var(--radius-lg)]"
      >
        <div className="aspect-[2/3] relative">
          {col.posterUrl ? (
            <img
              src={col.posterUrl}
              alt={col.name}
              loading="lazy"
              className="w-full h-full object-cover"
            />
          ) : (
            <div
              className="w-full h-full flex items-center justify-center text-4xl font-bold bg-[var(--surface-raised)] text-[var(--text-muted)]"
            >
              {initial}
            </div>
          )}
          <div
            className="absolute inset-0 gradient-collection-fade"
          />
          {subtitle.badge && (
            <Badge variant="default" className="absolute top-2 left-2">
              {subtitle.badge}
            </Badge>
          )}
          <div className="absolute bottom-2 left-2 right-2">
            <p className="text-sm font-semibold text-[var(--overlay-text)] truncate">
              {col.name}
            </p>
            {subtitle.count && (
              <p className="text-xs text-[var(--overlay-text)] opacity-80 truncate">
                {subtitle.count}
              </p>
            )}
          </div>
        </div>
      </Link>

      <button
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          if (confirm(t("media.collections.deleteConfirm"))) {
            deleteMut.mutate(col.id);
          }
        }}
        className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity-fast p-1.5 rounded-full bg-black/60 text-[var(--overlay-text)]"
        aria-label={t("common.delete")}
      >
        <Trash2 size={16}  aria-hidden="true" />
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
  const { t } = useTranslation();
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
      className="group relative flex flex-col gap-2 p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className="font-semibold truncate text-[var(--text)]">
            {col.name}
          </p>
          {rulesLabel && (
            <p className="text-xs mt-0.5 truncate text-[var(--text-muted)]">
              {rulesLabel}
            </p>
          )}
        </div>
        <div className="flex gap-1 flex-shrink-0">
          <button
            onClick={onEdit}
            aria-label={t("common.edit")}
            className="p-1.5 rounded hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
          >
            <Pencil size={16} strokeWidth={1.5}  aria-hidden="true" />
          </button>
          <button
            onClick={onDelete}
            aria-label={t("common.delete")}
            className="p-1.5 rounded hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
          >
            <Trash2 size={16} strokeWidth={1.5}  aria-hidden="true" />
          </button>
        </div>
      </div>
      <Link
        to="/collections/smart/$filterId"
        params={{ filterId: col.id }}
        className="text-xs font-medium transition-colors-fast text-[var(--accent)]"
      >
        {t("media.collections.browseResults")}
      </Link>
    </div>
  );
}

function CollectionsPage() {
  const { t } = useTranslation();
  const { data, isLoading, isError } = useCollections();
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
        <h1 className="text-2xl font-bold text-[var(--text)]">
          {t("media.collections.title")}
        </h1>
        <Button variant="secondary" onClick={openCreate}>
          <Plus size={16} />
          {t("media.collections.newSmartCollection")}
        </Button>
      </div>

      {/* Smart collections section */}
      {smartCollections.length > 0 && (
        <div className="mb-8">
          <h2 className="text-sm font-semibold uppercase tracking-wider mb-3 text-[var(--text-muted)]">
            {t("media.collections.smartCollections")}
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-3">
            {smartCollections.map((col) => (
              <SmartFilterCard
                key={col.id}
                col={col}
                onEdit={() => openEdit(col)}
                onDelete={() => { if (confirm(t("media.collections.deleteSmartConfirm"))) deleteMut.mutate(col.id); }}
              />
            ))}
          </div>
        </div>
      )}

      {/* TMDB collections */}
      <div>
        {smartCollections.length > 0 && (
          <h2 className="text-sm font-semibold uppercase tracking-wider mb-3 text-[var(--text-muted)]">
            {t("media.collections.tmdbCollections")}
          </h2>
        )}
        {isLoading ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-4">
            {Array.from({ length: 10 }).map((_, i) => (
              <CollectionCardSkeleton key={i} />
            ))}
          </div>
        ) : isError ? (
          <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
            <EmptyState
              icon={<Folder size={48} strokeWidth={1.5} />}
              title={t("common.error")}
              description={t("common.errorBoundary.description")}
            />
          </div>
        ) : tmdbCollections.length === 0 ? (
          <div
            className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
          >
            <EmptyState
              icon={<Folder size={48} strokeWidth={1.5} />}
              title={t("media.collections.noCollections")}
              description={t("media.collections.noCollectionsDesc")}
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
