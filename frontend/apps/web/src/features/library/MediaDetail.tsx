import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { slugify } from "@/utils/slugify";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { useStreamToken } from "@/hooks/useStreamToken";
import { formatDuration } from "./mediaDetailUtils";
import { MediaDetailHero } from "./MediaDetailHero";
import { MediaDetailActions } from "./MediaDetailActions";
import { CastCarousel } from "./MediaDetailCast";
import { SeriesSection, SeasonEpisodesSection } from "./MediaDetailSeries";
import { SimilarSection } from "./MediaDetailSimilar";
import { MediaDetailFileInfo } from "./MediaDetailFileInfo";

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

  useEffect(() => {
    if (item.id && item.filePath) {
      fetchStreamToken(item.id).catch(() => {});
    }
  }, [item.id, item.filePath, fetchStreamToken]);

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

  const artTokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const backdropUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.url
    ?? (item.id ? `/api/v1/media/${item.id}/artwork/backdrop${artTokenParam}` : undefined);
  const posterUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url
    ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster${artTokenParam}` : undefined);

  const posterSrcSet = useMemo(() => {
    if (!posterUrl) return undefined;
    const sep = posterUrl.includes("?") ? "&" : "?";
    return `${posterUrl}${sep}w=200 200w, ${posterUrl}${sep}w=300 300w, ${posterUrl}${sep}w=400 400w`;
  }, [posterUrl]);

  const duration = formatDuration(item.durationMs);
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
    <div data-testid="media-detail">
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
            className="w-48 overflow-hidden rounded-[var(--radius-md)] aspect-[2/3] bg-[var(--surface-raised)]"
          >
            {posterUrl ? (
              <img src={posterUrl} srcSet={posterSrcSet} sizes="192px" alt={item.title ?? ""} fetchPriority="high" className="w-full h-full object-cover" data-testid="media-poster" onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }} />
            ) : (
              <div className="w-full h-full flex items-center justify-center text-3xl font-bold text-[var(--text-muted)]">
                {item.title?.[0]?.toUpperCase() ?? "?"}
              </div>
            )}
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

      <div data-testid="cast-section"><CastCarousel people={actors} title={t("media.detail.cast")} /></div>
      {crew.length > 0 && <CastCarousel people={crew} title={t("media.detail.crew")} />}

      {isSeries && <div data-testid="seasons-section"><SeriesSection item={item} /></div>}
      {isSeason && <SeasonEpisodesSection item={item} />}

      {item.id && !isSeason && <SimilarSection item={item} />}

      <MediaDetailFileInfo item={item} />
      <PluginSlot name="media.detail.sidebar" context={{ mediaId: item.id }} />
    </div>
  );
}
