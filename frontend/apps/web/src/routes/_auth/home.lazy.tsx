import { useState, useEffect, useCallback, useRef, useMemo, memo, type FC } from "react";
import { Folder, Play, Settings2 } from "lucide-react";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useQueries } from "@tanstack/react-query";
import {
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecentlyAdded,
  useFeedServiceResumePlayback,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  libraryServiceGetMediaItem,
  useMusicServiceGetAlbums,
  usePhotoServiceListPhotos,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { MediaSlider } from "../../features/home/MediaSlider";
import { MediaCardWithModals as MediaCard } from "../../features/library/MediaCardWithModals";
import { EmptyState } from "@/components/ui/EmptyState";
import { slugify } from "@/utils/slugify";
import { useCollections } from "../../features/collection/api";
import { CollectionCard } from "../../features/collection/CollectionCard";
import { useMostPopular } from "@/hooks/usePopular";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { CustomizePanel } from "@/features/home/CustomizePanel";
import { NextUpRow } from "@/features/home/NextUpRow";

export const Route = createLazyFileRoute("/_auth/home")({
  component: HomePage,
});

const HERO_COUNT = 5;
const HERO_INTERVAL = 8000;

const HeroBanner = memo(function HeroBanner() {
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
        className="w-full animate-shimmer h-[clamp(320px,50vh,600px)]"
      />
    );
  }

  if (heroIds.length === 0) return null;

  return (
    <div
      className="relative w-full overflow-hidden h-[clamp(320px,50vh,600px)]"
      data-testid="hero-banner"
      onMouseEnter={() => { pausedRef.current = true; }}
      onMouseLeave={() => { pausedRef.current = false; }}
    >
      {heroIds.map((id, idx) => {
        const detail = details[idx]?.data;
        const candidate = candidates[idx];
        const backdropUrl = `/api/v1/media/${id}/artwork/backdrop`;
        const backdropSrcSet = `${backdropUrl}?w=800 800w, ${backdropUrl}?w=1280 1280w, ${backdropUrl}?w=1920 1920w`;
        const displayTitle = candidate?.title ?? detail?.title ?? "";
        const overview = detail?.overview;
        const year = detail?.year;
        const genres = detail?.genres ?? [];
        const rating = detail?.communityRating;
        const isActive = idx === activeIndex;

        return (
          <div
            key={id}
            className="absolute inset-0 transition-opacity-fast duration-1000"
            style={{ opacity: isActive ? 1 : 0, pointerEvents: isActive ? "auto" : "none" }}
          >
            {seenSlides.has(idx) && (
              <img
                src={backdropUrl}
                srcSet={backdropSrcSet}
                sizes="100vw"
                alt=""
                aria-hidden="true"
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
                className="text-2xl sm:text-4xl font-bold mb-2 leading-tight text-[var(--overlay-text)] tracking-[-0.02em]"
                style={{
                  textShadow: "0 2px 8px var(--overlay-bg-strong)",
                }}
              >
                {displayTitle}
              </h1>

              <div className="flex items-center gap-3 flex-wrap mb-2">
                {year && (
                  <span className="text-sm font-medium text-[var(--overlay-text-muted)]">
                    {year}
                  </span>
                )}
                {rating !== undefined && rating > 0 && (
                  <span className="text-sm text-[var(--overlay-text-muted)]">
                    {"\u2605"} {rating.toFixed(1)}
                  </span>
                )}
                {genres.slice(0, 3).map((g) => (
                  <span
                    key={g}
                    className="text-xs px-2 py-0.5 font-medium hidden sm:inline-block bg-[var(--glass-bg)] text-[var(--overlay-text-bright)] rounded-full border border-[var(--glass-border)] backdrop-blur-[var(--glass-blur)]"
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
                  className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity-fast hover:opacity-90 bg-[var(--accent)] text-[var(--filter-active-text)]"
                >
                  <Play size={16} fill="currentColor" />
                  {t("media.home.play")}
                </Link>
                <Link
                  to="/media/$mediaId"
                  params={{ mediaId: id }}
                  className="flex items-center gap-2 px-5 py-2.5 rounded-[var(--radius-md)] font-semibold text-sm transition-opacity-fast hover:opacity-90 bg-[var(--glass-bg)] text-[var(--overlay-text)] border border-[var(--glass-border)] backdrop-blur-[var(--glass-blur)]"
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
});

const MusicLibrarySlider = memo(function MusicLibrarySlider({ libraryId, name }: { libraryId: string; name: string }) {
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const { data, isLoading } = useMusicServiceGetAlbums({ libraryId });
  const albums = (data?.albums ?? []).slice(0, 20);
  if (!isLoading && albums.length === 0) return null;
  return (
    <MediaSlider title={name} seeAllHref="/music" isLoading={isLoading}>
      {albums.map((a) => (
        <Link key={a.id} to="/music" className="block w-[160px] flex-shrink-0">
          <div
            className="aspect-square rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] bg-cover bg-center"
            style={{ backgroundImage: a.coverArtUrl ? `url(${a.coverArtUrl}${tokenParam})` : undefined }}
          />
          <p className="text-sm font-medium text-[var(--text)] truncate mt-2">{a.title}</p>
          <p className="text-xs text-[var(--text-muted)] truncate">{a.artist}</p>
        </Link>
      ))}
    </MediaSlider>
  );
});

const PhotoLibrarySlider = memo(function PhotoLibrarySlider({ libraryId, name }: { libraryId: string; name: string }) {
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const { data, isLoading } = usePhotoServiceListPhotos({ libraryId, limit: 20 });
  const photos = data?.photos ?? [];
  if (!isLoading && photos.length === 0) return null;
  return (
    <MediaSlider title={name} seeAllHref="/photos" isLoading={isLoading}>
      {photos.map((p) => (
        <Link key={p.id} to="/photos" className="block w-[160px] flex-shrink-0">
          <div
            className="aspect-square rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] bg-cover bg-center"
            style={{ backgroundImage: `url(/api/v1/photos/${p.id}/serve${tokenParam}&w=400&fmt=webp)` }}
          />
        </Link>
      ))}
    </MediaSlider>
  );
});

const LibrarySlider = memo(function LibrarySlider({ libraryId, name, libraryType }: { libraryId: string; name: string; libraryType?: string }) {
  if (libraryType === "LIBRARY_TYPE_MUSIC") return <MusicLibrarySlider libraryId={libraryId} name={name} />;
  if (libraryType === "LIBRARY_TYPE_PHOTOS") return <PhotoLibrarySlider libraryId={libraryId} name={name} />;
  return <MediaItemsLibrarySlider libraryId={libraryId} name={name} libraryType={libraryType} />;
});

const MediaItemsLibrarySlider = memo(function MediaItemsLibrarySlider({ libraryId, name, libraryType }: { libraryId: string; name: string; libraryType?: string }) {
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
  const { data, isLoading } = useFeedServiceResumePlayback();
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
  next_up: NextUpRow,
  collections: CollectionsSection,
  recommended: RecommendedSlider,
  most_popular: MostPopularSlider,
};

// Keep in sync with KNOWN_FEED_TYPES in features/home/feedTypes.ts.
const FALLBACK_ORDER = [
  "resume_playback", "next_up", "collections",
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

  const allSections = useMemo(() => {
    const configs = feedConfig?.configs;
    if (!configs || configs.length === 0 || feedConfigError) {
      return FALLBACK_ORDER.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i }));
    }
    return [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0));
  }, [feedConfig, feedConfigError]);

  const sections = useMemo(() => allSections.filter((c) => c.enabled), [allSections]);

  return (
    <div className="animate-fade-in" data-testid="home-page">
      <HeroBanner />

      <div className="px-4 sm:px-6 pt-6">
        <div className="flex justify-end mb-2">
          <button
            onClick={() => setCustomizeOpen(true)}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded-[var(--radius-sm)] transition-colors-fast hover:opacity-80 text-[var(--text-muted)]"
            aria-label={t("common.feeds.customize")}
          >
            <Settings2 size={14}  aria-hidden="true" />
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
              .filter((lib) => lib.id && !allSections.some((s) => s.feedType === `library:${lib.id}`))
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
