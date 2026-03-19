import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceListLibraries, useLibraryServiceListMediaItems } from "@evey/api";
import type { V1Library, V1LibraryType } from "@evey/api/src/generated/library.schemas";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
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
      return (
        <svg className="w-8 h-8" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h1.5C5.496 19.5 6 18.996 6 18.375m-2.625 0V5.625m0 12.75v-1.5c0-.621.504-1.125 1.125-1.125m18.375 2.625V5.625m0 12.75c0 .621-.504 1.125-1.125 1.125m1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125m0 3.75h-1.5A1.125 1.125 0 0118 18.375M20.625 4.5H3.375m17.25 0c.621 0 1.125.504 1.125 1.125M20.625 4.5h-1.5C18.504 4.5 18 5.004 18 5.625m3.75 0v1.5c0 .621-.504 1.125-1.125 1.125M3.375 4.5c-.621 0-1.125.504-1.125 1.125M3.375 4.5h1.5C5.496 4.5 6 5.004 6 5.625m-3.75 0v1.5c0 .621.504 1.125 1.125 1.125m0 0h1.5m-1.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m1.5-3.75C5.496 8.25 6 7.746 6 7.125v-1.5M4.875 8.25C5.496 8.25 6 8.754 6 9.375v1.5m0-5.25v5.25m0-5.25C6 5.004 6.504 4.5 7.125 4.5h9.75c.621 0 1.125.504 1.125 1.125m1.125 2.625h1.5m-1.5 0A1.125 1.125 0 0118 7.125v-1.5m1.125 2.625c-.621 0-1.125.504-1.125 1.125v1.5m2.625-2.625c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125M18 5.625v5.25M7.125 12h9.75m-9.75 0A1.125 1.125 0 016 10.875M7.125 12C6.504 12 6 12.504 6 13.125m0-2.25C6 11.496 5.496 12 4.875 12M18 10.875c0 .621-.504 1.125-1.125 1.125M18 10.875c0 .621.504 1.125 1.125 1.125m-2.25 0c.621 0 1.125.504 1.125 1.125m-12 5.25v-5.25m0 5.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125m-12 0v-1.5c0-.621-.504-1.125-1.125-1.125M18 18.375v-5.25m0 5.25v-1.5c0-.621.504-1.125 1.125-1.125M18 13.125v1.5c0 .621.504 1.125 1.125 1.125M18 13.125c0-.621.504-1.125 1.125-1.125M6 13.125v1.5c0 .621-.504 1.125-1.125 1.125M6 13.125C6 12.504 5.496 12 4.875 12m-1.5 0h1.5m-1.5 0c-.621 0-1.125-.504-1.125-1.125v-1.5c0-.621.504-1.125 1.125-1.125m1.5 3.75c-.621 0-1.125-.504-1.125-1.125v-1.5c0-.621.504-1.125 1.125-1.125" />
        </svg>
      );
    case "LIBRARY_TYPE_SHOWS":
      return (
        <svg className="w-8 h-8" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 20.25h12m-7.5-3v3m3-3v3m-10.125-3h17.25c.621 0 1.125-.504 1.125-1.125V4.875c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125z" />
        </svg>
      );
    case "LIBRARY_TYPE_MUSIC":
      return (
        <svg className="w-8 h-8" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 9l10.5-3m0 6.553v3.75a2.25 2.25 0 01-1.632 2.163l-1.32.377a1.803 1.803 0 11-.99-3.467l2.31-.66a2.25 2.25 0 001.632-2.163zm0 0V2.25L9 5.25v10.303m0 0v3.75a2.25 2.25 0 01-1.632 2.163l-1.32.377a1.803 1.803 0 01-.99-3.467l2.31-.66A2.25 2.25 0 009 15.553z" />
        </svg>
      );
    default:
      return (
        <svg className="w-8 h-8" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 9.776c.112-.017.227-.026.344-.026h15.812c.117 0 .232.009.344.026m-16.5 0a2.25 2.25 0 00-1.883 2.542l.857 6a2.25 2.25 0 002.227 1.932H19.05a2.25 2.25 0 002.227-1.932l.857-6a2.25 2.25 0 00-1.883-2.542m-16.5 0V6A2.25 2.25 0 016 3.75h3.879a1.5 1.5 0 011.06.44l2.122 2.12a1.5 1.5 0 001.06.44H18A2.25 2.25 0 0120.25 9v.776" />
        </svg>
      );
  }
}

function LibraryCardSkeleton() {
  return (
    <div
      className="p-6"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
      }}
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

function LibraryItemCount({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  const { data } = useLibraryServiceListMediaItems({
    libraryId,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const total = data?.pagination?.totalItems;
  if (total === undefined) return null;
  return (
    <span className="text-sm" style={{ color: "var(--text-muted)" }}>
      {total === 1 ? t("common.item", { count: total }) : t("common.items", { count: total })}
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
          className="flex-shrink-0 w-14 h-14 rounded-xl flex items-center justify-center transition-colors duration-200"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--accent)" }}
        >
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <h2 className="text-lg font-semibold truncate" style={{ color: "var(--text)" }}>
              {library.name}
            </h2>
            <Badge variant="info">{label}</Badge>
          </div>
          {library.id && <LibraryItemCount libraryId={library.id} />}
          {library.updatedAt && (
            <p className="text-xs mt-2" style={{ color: "var(--text-muted)" }}>
              {t("media.updated")} {new Date(library.updatedAt).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
            </p>
          )}
        </div>
        <svg
          className="w-5 h-5 flex-shrink-0 mt-1 opacity-0 group-hover:opacity-100 transition-opacity"
          style={{ color: "var(--text-muted)" }}
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
        </svg>
      </div>
    </Link>
  );
}

function LibraryIndex() {
  const { t } = useTranslation();
  const { data, isLoading } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          {t("media.library.libraries")}
        </h1>
        {!isLoading && libraries.length > 0 && (
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
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
      ) : libraries.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="library"
            title={t("media.library.noLibraries")}
            description={t("media.library.noLibrariesDesc")}
            action={{
              label: t("media.library.goToAdmin"),
              onClick: () => { window.location.href = "/admin/libraries"; },
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
