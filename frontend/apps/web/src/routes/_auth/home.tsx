import { useMemo } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  useFeedServiceContinueWatching,
  useFeedServiceRecentlyAdded,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import { MediaSlider } from "../../features/home/MediaSlider";
import { MediaCard } from "../../features/library/MediaCard";
import { FeedItemCard } from "../../features/home/FeedItemCard";

export const Route = createFileRoute("/_auth/home")({
  component: HomePage,
});

// ── Hero banner ────────────────────────────────────────────────────────────────

function HeroBanner() {
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];

  const hero = useMemo(() => {
    // pick first item that has a mediaItemId (backdrop fetched via API)
    const candidates = items.filter((i) => !!i.mediaItemId);
    if (!candidates.length) return null;
    return candidates[Math.floor(Math.random() * candidates.length)];
  }, [items]);

  if (isLoading) {
    return (
      <div
        className="w-full animate-pulse"
        style={{ height: "480px", backgroundColor: "var(--surface-raised)" }}
      />
    );
  }

  if (!hero || !hero.mediaItemId) return null;

  const backdropUrl = `/api/v1/media/${hero.mediaItemId}/artwork/backdrop`;
  const playerHref = `/player/${hero.mediaItemId}`;

  return (
    <div
      className="relative w-full overflow-hidden"
      style={{ height: "480px" }}
    >
      <img
        src={backdropUrl}
        alt={hero.title ?? ""}
        className="absolute inset-0 w-full h-full object-cover"
        onError={(e) => {
          (e.currentTarget as HTMLImageElement).style.display = "none";
        }}
      />
      {/* gradient overlay */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(to right, rgba(0,0,0,0.85) 0%, rgba(0,0,0,0.4) 60%, rgba(0,0,0,0.1) 100%)",
        }}
      />
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(to top, rgba(0,0,0,0.7) 0%, transparent 50%)",
        }}
      />

      <div className="absolute inset-0 flex flex-col justify-end px-10 pb-12 max-w-2xl">
        <h1
          className="text-4xl font-bold mb-3 leading-tight"
          style={{
            color: "#fff",
            textShadow: "0 2px 8px rgba(0,0,0,0.8)",
          }}
        >
          {hero.title}
        </h1>

        <div className="flex gap-3 mt-4">
          <a
            href={playerHref}
            className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
            style={{ backgroundColor: "var(--accent)", color: "#fff" }}
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
            Play
          </a>
          <Link
            to="/media/$mediaId"
            params={{ mediaId: hero.mediaItemId }}
            className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
            style={{ backgroundColor: "rgba(255,255,255,0.2)", color: "#fff" }}
          >
            More Info
          </Link>
        </div>
      </div>
    </div>
  );
}

// ── Library slider (one per library) ──────────────────────────────────────────

function LibrarySlider({ libraryId, name }: { libraryId: string; name: string }) {
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    "pagination.pageSize": 20,
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
  });

  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={name} seeAllHref={`/library/${libraryId}`} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} />
      ))}
    </MediaSlider>
  );
}

// ── Type slider (Movies / Series) ─────────────────────────────────────────────

function TypeSlider({
  title,
  type,
}: {
  title: string;
  type: "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";
}) {
  const { data, isLoading } = useLibraryServiceListMediaItems({
    type,
    "pagination.pageSize": 20,
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
  });

  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={title} seeAllHref="/library" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} />
      ))}
    </MediaSlider>
  );
}

// ── Home page ──────────────────────────────────────────────────────────────────

function ContinueWatchingSlider() {
  const { data, isLoading } = useFeedServiceContinueWatching();
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Continue Watching" isLoading={isLoading}>
      {items.map((item) => (
        <FeedItemCard key={item.mediaItemId} item={item} showProgress />
      ))}
    </MediaSlider>
  );
}

function RecentlyAddedSlider() {
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];

  return (
    <MediaSlider title="Recently Added" isLoading={isLoading}>
      {items.map((item) => (
        <FeedItemCard key={item.mediaItemId} item={item} />
      ))}
    </MediaSlider>
  );
}

function HomePage() {
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];

  return (
    <div>
      <HeroBanner />

      <div className="px-6 pt-6">
        <ContinueWatchingSlider />
        <RecentlyAddedSlider />
        <TypeSlider title="Films" type="MEDIA_ITEM_TYPE_MOVIE" />
        <TypeSlider title="Séries" type="MEDIA_ITEM_TYPE_SERIES" />
        {libraries.map((lib) =>
          lib.id ? (
            <LibrarySlider key={lib.id} libraryId={lib.id} name={lib.name ?? "Library"} />
          ) : null
        )}
      </div>
    </div>
  );
}
