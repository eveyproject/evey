import { useState, useEffect, useCallback, useRef, useMemo, memo, type FC } from "react";
import { Folder, Play, Settings2 } from "lucide-react";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useQueries, useQuery } from "@tanstack/react-query";
import {
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecentlyAdded,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  libraryServiceGetMediaItem,
  eveyFetch,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { MediaSlider } from "../../features/home/MediaSlider";
import { MediaCard } from "../../features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { slugify } from "@/utils/slugify";
import { useCollections } from "../../features/collection/api";
import { CollectionCard } from "../../features/collection/CollectionCard";
import { useMostPopular } from "@/hooks/usePopular";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { CustomizePanel } from "@/features/home/CustomizePanel";

export const Route = createLazyFileRoute("/_auth/home")({
  component: HomePage,
});

const HERO_COUNT = 5;
const HERO_INTERVAL = 8000;

function HeroBanner() {
  const { t } = useTranslation();
  const { data, isLoading } = useFeedServiceRecentlyAdded(undefined, { query: { staleTime: 5 * 60_000 } });
  const items = data?.items ?? [];

  const candidates = items.filter((i) => !!i.mediaItemId).slice(0, HERO_COUNT);
  const heroIds = candidates.map((c) => c.mediaItemId!);

  const details = useQueries({
    queries: heroIds.map((id) => ({
      queryKey: [`/api/v1/media/${id}`],
      queryFn: ({ signal }: { signal: AbortSignal }) => libraryServiceGetMediaItem(id, signal),
      enabled: !!id,
      staleTime: 5 * 60_000,
    })),
  });

  const [activeIndex, setActiveIndex] = useState(0);
  const pausedRef = useRef(false);
  const [seenSlides, setSeenSlides] = useState<Set<number>>(() => new Set([0]));

  const goToSlide = useCallback((idx: number) => {
    setActiveIndex(idx);
    setSeenSlides((prev) => { if (prev.has(idx)) return prev; const next = new Set(prev); next.add(idx); return next; });
  }, []);

  const advance = useCallback(() => {
    if (heroIds.length <= 1) return;
    const next = (activeIndex + 1) % heroIds.length;
    goToSlide(next);
  }, [heroIds.length, activeIndex, goToSlide]);

  useEffect(() => {
    if (heroIds.length <= 1) return;
    const id = setInterval(() => {
      if (!pausedRef.current) advance();
    }, HERO_INTERVAL);
    return () => clearInterval(id);
  }, [advance, heroIds.length]);

  if (isLoading && heroIds.length === 0) {
    return (
      <div
        className="w-full animate-shimmer"
        style={{ height: "clamp(320px, 50vh, 600px)" }}
      />
    );
  }

  if (heroIds.length === 0) return null;

  return (
    <div
      className="relative w-full overflow-hidden"
      style={{ height: "clamp(320px, 50vh, 600px)" }}
      data-testid="hero-banner"
      onMouseEnter={() => { pausedRef.current = true; }}
      onMouseLeave={() => { pausedRef.current = false; }}
    >
      {heroIds.map((id, idx) => {
        const detail = details[idx]?.data;
        const candidate = candidates[idx];
        const backdropUrl = `/api/v1/media/${id}/artwork/backdrop`;
        const displayTitle = candidate?.title ?? detail?.title ?? "";
        const overview = detail?.overview;
        const year = detail?.year;
        const genres = detail?.genres ?? [];
        const rating = detail?.communityRating;
        const isActive = idx === activeIndex;

        return (
          <div
            key={id}
            className="absolute inset-0 transition-opacity duration-1000"
            style={{ opacity: isActive ? 1 : 0, pointerEvents: isActive ? "auto" : "none" }}
          >
            {seenSlides.has(idx) && (
              <img
                src={backdropUrl}
                alt={displayTitle}
                className="absolute inset-0 w-full h-full object-cover animate-ken-burns"
                loading={isActive ? "eager" : "lazy"}
                fetchPriority={isActive ? "high" : "low"}
                onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
              />
            )}
            <div
              className="absolute inset-0"
              style={{
                background: "linear-gradient(to right, color-mix(in srgb, var(--bg) 90%, transparent) 0%, color-mix(in srgb, var(--bg) 40%, transparent) 60%, transparent 100%)",
              }}
            />
            <div
              className="absolute inset-0"
              style={{
                background: "linear-gradient(to top, var(--bg) 0%, color-mix(in srgb, var(--bg) 40%, transparent) 40%, transparent 100%)",
              }}
            />

            <div className="absolute inset-0 flex flex-col justify-end px-6 sm:px-10 pb-8 sm:pb-12 max-w-2xl">
              <h1
                className="text-2xl sm:text-4xl font-bold mb-2 leading-tight"
                style={{
                  color: "var(--overlay-text)",
                  textShadow: "0 2px 8px var(--overlay-bg-strong)",
                  letterSpacing: "-0.02em",
                }}
              >
                {displayTitle}
              </h1>

              <div className="flex items-center gap-3 flex-wrap mb-2">
                {year && (
                  <span className="text-sm font-medium" style={{ color: "var(--overlay-text-muted)" }}>
                    {year}
                  </span>
                )}
                {rating !== undefined && rating > 0 && (
                  <span className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>
                    {"\u2605"} {rating.toFixed(1)}
                  </span>
                )}
                {genres.slice(0, 3).map((g) => (
                  <span
                    key={g}
                    className="text-xs px-2 py-0.5 font-medium hidden sm:inline-block"
                    style={{
                      backgroundColor: "var(--glass-bg)",
                      color: "var(--overlay-text-bright)",
                      borderRadius: "9999px",
                      backdropFilter: `blur(var(--glass-blur))`,
                      border: "1px solid var(--glass-border)",
                    }}
                  >
                    {g}
                  </span>
                ))}
              </div>

              {overview && (
                <p
                  className="text-sm leading-relaxed mb-4 hidden sm:line-clamp-3 text-white/90"
                  style={{ textShadow: "0 1px 6px var(--overlay-scrim)" }}
                >
                  {overview}
                </p>
              )}

              <div className="flex gap-3 mt-2">
                <Link
                  to="/player/$mediaId"
                  params={{ mediaId: id }}
                  className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
                  style={{ backgroundColor: "var(--accent)", color: "var(--filter-active-text)" }}
                >
                  <Play size={16} fill="currentColor" />
                  {t("media.home.play")}
                </Link>
                <Link
                  to="/media/$mediaId"
                  params={{ mediaId: id }}
                  className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity hover:opacity-90"
                  style={{ backgroundColor: "var(--glass-bg)", color: "var(--overlay-text)", backdropFilter: `blur(var(--glass-blur))`, border: "1px solid var(--glass-border)" }}
                >
                  {t("media.home.moreInfo")}
                </Link>
              </div>
            </div>
          </div>
        );
      })}

      {heroIds.length > 1 && (
        <div className="absolute bottom-4 right-6 flex gap-2 z-raised">
          {heroIds.map((_, idx) => (
            <button
              key={idx}
              onClick={() => goToSlide(idx)}
              className="w-2 h-2 rounded-full transition-all duration-300"
              style={{
                backgroundColor: idx === activeIndex ? "var(--accent)" : "var(--overlay-dot-inactive)",
                transform: idx === activeIndex ? "scale(1.3)" : "scale(1)",
              }}
              aria-label={`Slide ${idx + 1}`}
            />
          ))}
        </div>
      )}
    </div>
  );
}

const LibrarySlider = memo(function LibrarySlider({ libraryId, name, libraryType }: { libraryId: string; name: string; libraryType?: string }) {
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
        <MediaCard key={item.id} item={item} lite />
      ))}
    </MediaSlider>
  );
});

const ResumePlaybackSlider = memo(function ResumePlaybackSlider() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery<{ items?: { mediaItemId: string; title: string; type: string; positionMs: number }[] }>({
    queryKey: ["/api/v1/feeds/resume-playback"],
    queryFn: ({ signal }) => eveyFetch({ url: "/api/v1/feeds/resume-playback", method: "GET", signal }),
  });
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("common.feeds.labels.resume_playback")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
          showProgress={!!item.positionMs}
          lite
        />
      ))}
    </MediaSlider>
  );
});

const NextEpisodeSlider = memo(function NextEpisodeSlider() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery<{ items?: { mediaItemId: string; parentId?: string; title: string; type: string }[] }>({
    queryKey: ["/api/v1/feeds/next-episode"],
    queryFn: ({ signal }) => eveyFetch({ url: "/api/v1/feeds/next-episode", method: "GET", signal }),
  });
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("common.feeds.labels.next_episode")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
          showType
          lite
        />
      ))}
    </MediaSlider>
  );
});

const MostPopularSlider = memo(function MostPopularSlider() {
  const { t } = useTranslation();
  const { data, isLoading } = useMostPopular(20);
  const items = data ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.mostPopular")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showType
          lite
        />
      ))}
    </MediaSlider>
  );
});

const RecommendedSlider = memo(function RecommendedSlider() {
  const { t } = useTranslation();
  const { data, isLoading } = useFeedServiceRecommended({ limit: 20 });
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.recommendedForYou")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showType
          lite
        />
      ))}
    </MediaSlider>
  );
});

const CollectionsSection = memo(function CollectionsSection() {
  const { t } = useTranslation();
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];
  if (!isLoading && collections.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.collections")} seeAllHref="/collections" isLoading={isLoading} skeletonCount={6}>
      {collections.map((col) => (
        <CollectionCard key={col.id} collection={col} />
      ))}
    </MediaSlider>
  );
});

const SECTION_REGISTRY: Record<string, FC> = {
  resume_playback: ResumePlaybackSlider,
  collections: CollectionsSection,
  recommended: RecommendedSlider,
  most_popular: MostPopularSlider,
};

const FALLBACK_ORDER = [
  "resume_playback", "collections",
  "recommended", "most_popular",
];

function HomePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const profileId = useAuthStore((s) => s.profileId);
  const [customizeOpen, setCustomizeOpen] = useState(false);

  const { data: librariesData, isLoading: librariesLoading } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];
  const { data: recentlyAdded, isLoading: recentLoading } = useFeedServiceRecentlyAdded();
  const { data: feedConfig, isError: feedConfigError } = useFeedServiceGetFeedConfig(
    { profileId: profileId ?? undefined },
    { query: { retry: 1, staleTime: 60_000 } },
  );
  const hasContent = (recentlyAdded?.items ?? []).length > 0 || libraries.length > 0;
  const isLoading = librariesLoading || recentLoading;

  const sections = useMemo(() => {
    const configs = feedConfig?.configs;
    if (!configs || configs.length === 0 || feedConfigError) {
      return FALLBACK_ORDER.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i }));
    }
    return [...configs]
      .sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0))
      .filter((c) => c.enabled);
  }, [feedConfig, feedConfigError]);

  return (
    <div className="animate-fade-in" data-testid="home-page">
      <HeroBanner />

      <div className="px-4 sm:px-6 pt-6">
        <div className="flex justify-end mb-2">
          <button
            onClick={() => setCustomizeOpen(true)}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded-[var(--radius-sm)] transition-colors hover:opacity-80"
            style={{ color: "var(--text-muted)" }}
            aria-label={t("common.feeds.customize")}
          >
            <Settings2 size={14} />
            {t("common.feeds.customize")}
          </button>
        </div>

        {!isLoading && !hasContent ? (
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.home.emptyLibrary")}
            description={t("media.home.emptyLibraryDesc")}
            action={{ label: t("media.home.goToLibraries"), onClick: () => navigate({ to: "/admin/libraries" }) }}
          />
        ) : (
          <>
            {sections.map((cfg) => {
              const ft = cfg.feedType ?? "";
              if (ft.startsWith("library:")) {
                const libId = ft.replace("library:", "");
                const lib = libraries.find((l) => l.id === libId);
                return lib ? (
                  <LibrarySlider key={ft} libraryId={libId} name={lib.name ?? t("media.home.library")} libraryType={lib.type} />
                ) : null;
              }
              const Component = SECTION_REGISTRY[ft];
              return Component ? <Component key={ft} /> : null;
            })}
            {libraries
              .filter((lib) => lib.id && !sections.some((s) => s.feedType === `library:${lib.id}`))
              .map((lib) => (
                <LibrarySlider key={lib.id} libraryId={lib.id!} name={lib.name ?? t("media.home.library")} libraryType={lib.type} />
              ))}
            <PluginSlot name="home.section" />
          </>
        )}
      </div>

      <CustomizePanel open={customizeOpen} onClose={() => setCustomizeOpen(false)} />
    </div>
  );
}
