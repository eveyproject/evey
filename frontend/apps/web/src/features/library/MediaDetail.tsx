import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceGetSeriesAggregate,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { slugify } from "@/utils/slugify";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { useStreamToken } from "@/hooks/useStreamToken";
import { formatDuration, formatSeriesRuntime, hasMediaFile } from "./mediaDetailUtils";
import { MediaDetailHero } from "./MediaDetailHero";
import { MediaDetailActions } from "./MediaDetailActions";
import { CastCarousel } from "./MediaDetailCast";
import { SeriesSection, SeasonEpisodesSection } from "./MediaDetailSeries";
import { SimilarSection } from "./MediaDetailSimilar";
import { MediaDetailFileInfo } from "./MediaDetailFileInfo";
import { artworkUrl } from "./artworkUrl";
import { PosterPlaceholder } from "./PosterPlaceholder";

interface MediaDetailProps {
  item: V1MediaItem;
  onMetadataUpdated?: () => void;
  initialEdit?: boolean;
}

export function MediaDetail({ item, onMetadataUpdated, initialEdit }: MediaDetailProps) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(initialEdit ?? false);
  const isSeries = item.type === "MEDIA_ITEM_TYPE_SERIES";
  const isSeason = item.type === "MEDIA_ITEM_TYPE_SEASON";
  const isEpisode = item.type === "MEDIA_ITEM_TYPE_EPISODE";
  const { streamToken, fetchStreamToken } = useStreamToken();

  // Fetch the stream token whenever the item is a real file - NOT gated on
  // item.filePath, which is admin-only (redacted for non-admins). Gating on
  // filePath here starved every non-admin of a stream token, which in turn hid
  // the download button (it needs the token). hasMediaFile uses fileSize, which
  // every viewer receives. See hasMediaFile.
  const itemHasFile = hasMediaFile(item);
  useEffect(() => {
    if (item.id && itemHasFile) {
      fetchStreamToken(item.id).catch((err) => console.warn("fetchStreamToken failed", err));
    }
  }, [item.id, itemHasFile, fetchStreamToken]);

  const { data: parentItem } = useLibraryServiceGetMediaItem(
    item.parentId ?? "",
    { query: { enabled: !!item.parentId && (item.type === "MEDIA_ITEM_TYPE_SEASON" || isEpisode) } } as never,
  );

  const { data: grandParentItem } = useLibraryServiceGetMediaItem(
    parentItem?.parentId ?? "",
    { query: { enabled: !!parentItem?.parentId && isEpisode } } as never,
  );

  const accessToken = useAuthStore((s) => s.accessToken);
  const { data: librariesData } = useLibraryServiceListLibraries();
  const library = librariesData?.libraries?.find((l) => l.id === item.libraryId);
  const librarySlug = library ? slugify(library.name ?? "") : null;

  // Always route through backend endpoint - see MediaCard for the ORB rationale.
  // The raw item.artwork[].url is the external provider URL (Amazon CDN for
  // OMDb, etc.) which Firefox blocks under Opaque Response Blocking. The ?v=
  // content version busts the immutable cache on a poster change; the token
  // rides along for the auth'd detail view (and incidentally also busts).
  const backdropUrl = item.id ? artworkUrl(item.id, "backdrop", item, accessToken || undefined) : undefined;
  const posterUrl = item.id ? artworkUrl(item.id, "poster", item, accessToken || undefined) : undefined;

  const posterSrcSet = useMemo(() => {
    if (!posterUrl) return undefined;
    const sep = posterUrl.includes("?") ? "&" : "?";
    return `${posterUrl}${sep}w=200 200w, ${posterUrl}${sep}w=300 300w, ${posterUrl}${sep}w=400 400w`;
  }, [posterUrl]);

  const { data: seriesAggregate } = useLibraryServiceGetSeriesAggregate(
    item.id ?? "",
    { query: { enabled: isSeries && !!item.id } } as never,
  );
  const duration = isSeries
    ? formatSeriesRuntime(seriesAggregate?.totalDurationMs, seriesAggregate?.episodeCount)
    : formatDuration(item.durationMs);
  const actors = item.people?.filter((p) => p.type === "PERSON_TYPE_ACTOR") ?? [];
  const directors = item.people?.filter((p) => p.type === "PERSON_TYPE_DIRECTOR") ?? [];
  const writers = item.people?.filter((p) => p.type === "PERSON_TYPE_WRITER") ?? [];
  const producers = item.people?.filter((p) => p.type === "PERSON_TYPE_PRODUCER") ?? [];
  const crew = [...directors, ...writers, ...producers];

  const breadcrumbItems = (() => {
    const crumbs: Array<{ label: string; to?: string }> = [{ label: t("common.nav.home"), to: "/home" }];
    if (librarySlug && library) {
      crumbs.push({ label: library.name ?? t("media.home.library"), to: `/library/${librarySlug}` });
    }
    if (isEpisode && grandParentItem) {
      crumbs.push({ label: grandParentItem.title ?? t("media.library.series"), to: `/media/${grandParentItem.id}` });
      if (parentItem) {
        const seasonLabel = parentItem.indexNumber != null ? t("media.season", { number: parentItem.indexNumber }) : (parentItem.title ?? t("media.season", { number: "" }));
        crumbs.push({ label: seasonLabel, to: `/media/${parentItem.id}` });
      }
      crumbs.push({ label: item.title ?? "" });
    } else if (isSeason && parentItem) {
      crumbs.push({ label: parentItem.title ?? t("media.library.series"), to: `/media/${parentItem.id}` });
      const seasonLabel = item.indexNumber != null ? t("media.season", { number: item.indexNumber }) : (item.title ?? t("media.season", { number: "" }));
      crumbs.push({ label: seasonLabel });
    } else {
      crumbs.push({ label: item.title ?? "" });
    }
    return crumbs;
  })();

  return (
    <div className="evey-media-detail" data-testid="media-detail">
      <MediaDetailHero
        item={item}
        backdropUrl={backdropUrl}
        posterUrl={posterUrl}
        duration={duration}
        breadcrumbItems={breadcrumbItems}
        parentItem={parentItem}
        onEditToggle={() => setEditing((v) => !v)}
      />

      <div className="flex gap-6 mt-6">
        <div className="flex-shrink-0 w-48 hidden md:block">
          <div
            className="evey-poster relative w-48 overflow-hidden rounded-[var(--radius-md)] aspect-[2/3] bg-[var(--surface-raised)]"
          >
            {posterUrl ? (
              <img
                src={posterUrl}
                srcSet={posterSrcSet}
                sizes="192px"
                alt={item.title ?? ""}
                fetchPriority="high"
                className="w-full h-full object-cover"
                data-testid="media-poster"
                onError={(e) => {
                  const t = e.currentTarget as HTMLImageElement;
                  t.style.display = "none";
                  const fb = t.nextElementSibling as HTMLElement | null;
                  if (fb) fb.style.display = "flex";
                }}
              />
            ) : null}
            <PosterPlaceholder
              title={item.title}
              type={item.type}
              iconSize={36}
              className="absolute inset-0"
              style={{ display: posterUrl ? "none" : "flex" }}
            />
          </div>
        </div>

        <MediaDetailActions
          item={item}
          isSeries={isSeries}
          isSeason={isSeason}
          directors={directors}
          writers={writers}
          editing={editing}
          setEditing={setEditing}
          onMetadataUpdated={onMetadataUpdated}
          streamToken={streamToken}
        />
      </div>

      <MediaDetailFileInfo item={item} />

      <div data-testid="cast-section"><CastCarousel people={actors} title={t("media.detail.cast")} /></div>
      {crew.length > 0 && <CastCarousel people={crew} title={t("media.detail.crew")} />}

      {isSeries && <div data-testid="seasons-section"><SeriesSection item={item} /></div>}
      {isSeason && <SeasonEpisodesSection item={item} />}

      {item.id && !isSeason && <SimilarSection item={item} />}

      <PluginSlot name="media.detail.sidebar" context={{ mediaId: item.id }} />
    </div>
  );
}
