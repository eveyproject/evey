import { Film, Monitor, Music, Folder, ChevronRight } from "lucide-react";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceListLibraries, useLibraryServiceListMediaItems, useMusicServiceListTracks, usePhotoServiceListPhotos } from "@evey/api";
import type { V1Library, V1LibraryType } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { Skeleton } from "@/components/ui/Skeleton";
import { slugify } from "@/utils/slugify";

export const Route = createLazyFileRoute("/_auth/library/")({
  component: LibraryIndex,
});

const LIBRARY_TYPE_KEYS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "media.library.movies",
  LIBRARY_TYPE_SHOWS: "media.library.tvShows",
  LIBRARY_TYPE_MUSIC: "media.library.music",
  LIBRARY_TYPE_PHOTOS: "media.library.photos",
  LIBRARY_TYPE_BOOKS: "media.library.books",
};

function libraryTypeIcon(type?: V1LibraryType) {
  switch (type) {
    case "LIBRARY_TYPE_MOVIES":
      return <Film size={32} strokeWidth={1.5} />;
    case "LIBRARY_TYPE_SHOWS":
      return <Monitor size={32} strokeWidth={1.5} />;
    case "LIBRARY_TYPE_MUSIC":
      return <Music size={32} strokeWidth={1.5} />;
    default:
      return <Folder size={32} strokeWidth={1.5} />;
  }
}

function LibraryCardSkeleton() {
  return (
    <div
      className="p-6 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)]"
    >
      <div className="flex items-start gap-4">
        <Skeleton className="w-14 h-14 rounded-xl flex-shrink-0" />
        <div className="flex-1 min-w-0 space-y-2">
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-4 w-1/3" />
          <Skeleton className="h-3 w-1/2 mt-3" />
        </div>
      </div>
    </div>
  );
}

function MusicItemCount({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  const { data } = useMusicServiceListTracks({ libraryId, limit: 1 });
  const total = data?.total;
  if (total === undefined) return null;
  return <span className="text-sm text-[var(--text-muted)]">{t("common.item", { count: total })}</span>;
}

function PhotoItemCount({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  const { data } = usePhotoServiceListPhotos({ libraryId, limit: 1 });
  const total = data?.total;
  if (total === undefined) return null;
  return <span className="text-sm text-[var(--text-muted)]">{t("common.item", { count: total })}</span>;
}

function LibraryItemCount({ libraryId, libraryType }: { libraryId: string; libraryType?: V1LibraryType }) {
  const { t } = useTranslation();
  // Music + photo libraries live in their own tables, not media_items.
  if (libraryType === "LIBRARY_TYPE_MUSIC") return <MusicItemCount libraryId={libraryId} />;
  if (libraryType === "LIBRARY_TYPE_PHOTOS") return <PhotoItemCount libraryId={libraryId} />;
  const { data } = useLibraryServiceListMediaItems({
    libraryId,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const total = data?.pagination?.totalItems;
  if (total === undefined) return null;
  return (
    <span className="text-sm text-[var(--text-muted)]">
      {t("common.item", { count: total })}
    </span>
  );
}

function LibraryCard({ library }: { library: V1Library }) {
  const { t } = useTranslation();
  const typeKey = LIBRARY_TYPE_KEYS[library.type ?? ""] ?? "media.library.title";
  const label = t(typeKey);
  const icon = libraryTypeIcon(library.type);

  return (
    <Link
      to="/library/$libraryId"
      params={{ libraryId: slugify(library.name ?? "") }}
      className="group block p-6 transition-all duration-200"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.backgroundColor = "var(--surface-raised)";
        (e.currentTarget as HTMLElement).style.borderColor = "var(--accent)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.backgroundColor = "var(--surface)";
        (e.currentTarget as HTMLElement).style.borderColor = "var(--border)";
      }}
    >
      <div className="flex items-start gap-4">
        <div
          className="flex-shrink-0 w-14 h-14 rounded-xl flex items-center justify-center transition-colors-fast duration-200 bg-[var(--surface-raised)] text-[var(--accent)]"
        >
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <h2 className="text-lg font-semibold truncate text-[var(--text)]">
              {library.name}
            </h2>
            <Badge variant="info">{label}</Badge>
          </div>
          {library.id && <LibraryItemCount libraryId={library.id} libraryType={library.type} />}
          {library.updatedAt && (
            <p className="text-xs mt-2 text-[var(--text-muted)]">
              {t("media.updated")} {new Date(library.updatedAt).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
            </p>
          )}
        </div>
        <ChevronRight
          size={20}
          className="flex-shrink-0 mt-1 opacity-0 group-hover:opacity-100 transition-opacity-fast text-[var(--text-muted)]"
        />
      </div>
    </Link>
  );
}

function LibraryIndex() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data, isLoading, isError, error, refetch } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-[var(--text)]">
          {t("media.library.libraries")}
        </h1>
        {!isLoading && libraries.length > 0 && (
          <span className="text-sm text-[var(--text-muted)]">
            {libraries.length === 1
              ? `1 ${t("media.library.title").toLowerCase()}`
              : `${libraries.length} ${t("media.library.libraries").toLowerCase()}`}
          </span>
        )}
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <LibraryCardSkeleton key={i} />
          ))}
        </div>
      ) : isError ? (
        // A failed fetch must never be shown as "no libraries" - the user would
        // believe their collection is gone rather than the server being unreachable.
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <FetchErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : libraries.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.library.noLibraries")}
            description={t("media.library.noLibrariesDesc")}
            action={{
              label: t("media.library.goToAdmin"),
              onClick: () => void navigate({ to: "/admin/libraries" }),
            }}
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {libraries.map((lib) => (
            <LibraryCard key={lib.id} library={lib} />
          ))}
        </div>
      )}
    </div>
  );
}
