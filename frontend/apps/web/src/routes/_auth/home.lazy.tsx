import { useState } from "react";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  useFeedServiceContinueWatching,
  useFeedServiceRecentlyAdded,
  useFeedServiceRecommended,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetNextUp,
  libraryServiceGetMediaItem,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { MediaSlider } from "../../features/home/MediaSlider";
import { MediaCard } from "../../features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { slugify } from "@/utils/slugify";
import { useCollections } from "../../features/collection/api";
import { CollectionCard } from "../../features/collection/CollectionCard";
import { useMostPopular } from "@/hooks/usePopular";
import { PluginSlot } from "@/features/plugins/PluginSlot";

export const Route = createLazyFileRoute("/_auth/home")({
  component: HomePage,
});

function HeroBanner() {
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];

  const [heroIndex] = useState(() => Math.floor(Math.random() * 1000));
  const candidates = items.filter((i) => !!i.mediaItemId);
  const hero = candidates.length
    ? candidates[heroIndex % candidates.length]
    : null;

  const heroId = hero?.mediaItemId ?? "";
  const { data: heroDetail } = useQuery({
    queryKey: [`/api/v1/media/${heroId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(heroId, signal),
    enabled: !!heroId,
  });

  if (isLoading) {
    return (
      <div
        className="w-full animate-pulse"
        style={{ height: "clamp(320px, 56vw, 520px)", backgroundColor: "var(--surface-raised)" }}
      />
    );
  }

  if (!hero || !hero.mediaItemId) return null;

  const backdropUrl = `/api/v1/media/${hero.mediaItemId}/artwork/backdrop`;
  const overview = heroDetail?.overview;
  const year = heroDetail?.year;
  const genres = heroDetail?.genres ?? [];
  const rating = heroDetail?.communityRating;

  return (
    <div
      className="relative w-full overflow-hidden"
      style={{ height: "clamp(320px, 56vw, 520px)" }}
      data-testid="hero-banner"
    >
      <img
        src={backdropUrl}
        alt={hero.title ?? ""}
        className="absolute inset-0 w-full h-full object-cover animate-ken-burns"
        onError={(e) => {
          (e.currentTarget as HTMLImageElement).style.display = "none";
        }}
      />
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
            "linear-gradient(to top, var(--bg) 0%, rgba(0,0,0,0.4) 40%, transparent 100%)",
        }}
      />

      <div className="absolute inset-0 flex flex-col justify-end px-6 sm:px-10 pb-8 sm:pb-12 max-w-2xl">
        <h1
          className="text-2xl sm:text-4xl font-bold mb-2 leading-tight"
          style={{
            color: "var(--overlay-text)",
            textShadow: "0 2px 8px rgba(0,0,0,0.8)",
          }}
        >
          {hero.title}
        </h1>

        <div className="flex items-center gap-3 flex-wrap mb-2">
          {year && (
            <span className="text-sm font-medium" style={{ color: "var(--overlay-text-muted)" }}>
              {year}
            </span>
          )}
          {rating !== undefined && rating > 0 && (
            <span className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>
              \u2605 {rating.toFixed(1)}
            </span>
          )}
          {genres.slice(0, 3).map((g) => (
            <span
              key={g}
              className="text-xs px-2 py-0.5 font-medium hidden sm:inline-block"
              style={{
                backgroundColor: "rgba(255,255,255,0.15)",
                color: "rgba(255,255,255,0.9)",
                borderRadius: "9999px",
                backdropFilter: "blur(4px)",
              }}
            >
              {g}
            </span>
          ))}
        </div>

        {overview && (
          <p
            className="text-sm leading-relaxed mb-4 hidden sm:line-clamp-3 text-white/90"
            style={{
              textShadow: "0 1px 6px rgba(0,0,0,0.9)",
            }}
          >
            {overview}
          </p>
        )}

        <div className="flex gap-3 mt-2">
          <Link
            to="/player/$mediaId"
            params={{ mediaId: hero.mediaItemId }}
            className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
            style={{ backgroundColor: "var(--accent)", color: "var(--filter-active-text)" }}
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
            Play
          </Link>
          <Link
            to="/media/$mediaId"
            params={{ mediaId: hero.mediaItemId }}
            className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
            style={{ backgroundColor: "var(--overlay-btn-bg)", color: "var(--overlay-text)", backdropFilter: "blur(4px)" }}
          >
            More Info
          </Link>
        </div>
      </div>
    </div>
  );
}

function LibrarySlider({ libraryId, name, libraryType }: { libraryId: string; name: string; libraryType?: string }) {
  const typeFilter = libraryType === "LIBRARY_TYPE_SHOWS"
    ? "MEDIA_ITEM_TYPE_SERIES"
    : libraryType === "LIBRARY_TYPE_MOVIES"
      ? "MEDIA_ITEM_TYPE_MOVIE"
      : undefined;
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter as never } : {}),
    "pagination.pageSize": 20,
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
  });

  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={name} seeAllHref={`/library/${slugify(name)}`} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} />
      ))}
    </MediaSlider>
  );
}

function ContinueWatchingSlider() {
  const { data, isLoading } = useFeedServiceContinueWatching();
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Continue Watching" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
          showProgress={!!item.positionMs}
        />
      ))}
    </MediaSlider>
  );
}

function UpNextSlider() {
  const profileId = useAuthStore((s) => s.profileId);
  const { data, isLoading } = usePlaybackServiceGetNextUp(
    { profileId: profileId ?? undefined, limit: 15 },
    { query: { enabled: !!profileId } },
  );
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Up Next" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.id}
          item={{
            id: item.id,
            title: item.title,
            type: item.type,
            year: item.year,
            communityRating: item.communityRating,
            overview: item.overview,
            genres: item.genres,
            artwork: item.artwork,
          }}
          showYear={false}
          showType
        />
      ))}
    </MediaSlider>
  );
}

function RecentlyAddedSlider() {
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Recently Added" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
          showType
        />
      ))}
    </MediaSlider>
  );
}

function MostPopularSlider() {
  const { data, isLoading } = useMostPopular(20);
  const items = data ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Most Popular" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showType
        />
      ))}
    </MediaSlider>
  );
}

function RecommendedSlider() {
  const { data, isLoading } = useFeedServiceRecommended({ limit: 20 });
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title="Recommended For You" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showType
        />
      ))}
    </MediaSlider>
  );
}

function CollectionsSection() {
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];
  if (!isLoading && collections.length === 0) return null;

  return (
    <MediaSlider title="Collections" seeAllHref="/collections" isLoading={isLoading} skeletonCount={6}>
      {collections.map((col) => (
        <CollectionCard key={col.id} collection={col} />
      ))}
    </MediaSlider>
  );
}

function HomePage() {
  const navigate = useNavigate();
  const { data: librariesData, isLoading: librariesLoading } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];
  const { data: recentlyAdded, isLoading: recentLoading } = useFeedServiceRecentlyAdded();
  const hasContent = (recentlyAdded?.items ?? []).length > 0 || libraries.length > 0;
  const isLoading = librariesLoading || recentLoading;

  return (
    <div className="animate-fade-in" data-testid="home-page">
      <HeroBanner />

      <div className="px-4 sm:px-6 pt-6">
        {!isLoading && !hasContent ? (
          <EmptyState
            icon="library"
            title="Your library is empty"
            description="Go to Admin \u2192 Libraries to add a media folder."
            action={{ label: "Go to Libraries", onClick: () => navigate({ to: "/admin/libraries" }) }}
          />
        ) : (
          <>
            <ContinueWatchingSlider />
            <UpNextSlider />
            <CollectionsSection />
            <RecentlyAddedSlider />
            <RecommendedSlider />
            <MostPopularSlider />
            {libraries.map((lib) =>
              lib.id ? (
                <LibrarySlider key={lib.id} libraryId={lib.id} name={lib.name ?? "Library"} libraryType={lib.type} />
              ) : null
            )}
            <PluginSlot name="home.section" />
          </>
        )}
      </div>
    </div>
  );
}
