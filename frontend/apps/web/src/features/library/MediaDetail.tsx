import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceGetSeriesAggregate,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
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

  const accessToken = useAuthStore((s) => s.accessToken);

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
    // Use the widths the server actually has (185/342/500): w=300 and w=400
    // both snapped to 342, so the third entry was a duplicate and a retina
    // poster never got a sharper file.
    return `${posterUrl}${sep}w=185 185w, ${posterUrl}${sep}w=342 342w, ${posterUrl}${sep}w=500 500w`;
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

  return (
    <div className="evey-media-detail" data-testid="media-detail">
      <MediaDetailHero
        item={item}
        backdropUrl={backdropUrl}
        posterUrl={posterUrl}
        duration={duration}
        parentItem={parentItem}
        onEditToggle={() => setEditing((v) => !v)}
      />

      {/* The poster is pulled UP so it overlaps the backdrop instead of sitting
          in a separate band below it - the layout Jellyfin uses, and the reason
          its detail page reads as one composition rather than an image with a
          form underneath. The hero reserves matching space on its left
          (md:pl-[22rem]) so the title never collides with it.

          The negative margin is desktop-only: on a narrow screen there is no
          room beside the poster, so the sections stay stacked. */}
      <div className="flex gap-8 mt-6 items-start">
        <div className="flex-shrink-0 w-80 hidden md:block md:-mt-[18rem] relative z-raised">
          <div
            className="evey-poster relative w-80 overflow-hidden rounded-[var(--radius-md)] aspect-[2/3] bg-[var(--surface-raised)] shadow-[var(--shadow-lg)] ring-1 ring-[var(--border)]"
          >
            {posterUrl ? (
              <img
                src={posterUrl}
                srcSet={posterSrcSet}
                sizes="320px"
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

        {/* Pushed down to clear the backdrop the poster now overlaps, so the
            synopsis and actions start below the artwork rather than on top of
            it. */}
        <div className="flex-1 min-w-0">
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
      </div>

      <MediaDetailFileInfo item={item} />

      {/* Seasons and episodes come BEFORE the cast: on a series page they are
          what the viewer came for, and burying them under a cast carousel meant
          scrolling past people to reach the thing you want to watch. */}
      {isSeries && <div data-testid="seasons-section"><SeriesSection item={item} /></div>}
      {isSeason && <SeasonEpisodesSection item={item} />}

      <div data-testid="cast-section"><CastCarousel people={actors} title={t("media.detail.cast")} /></div>
      {crew.length > 0 && <CastCarousel people={crew} title={t("media.detail.crew")} />}

      {item.id && !isSeason && <SimilarSection item={item} />}

      <PluginSlot name="media.detail.sidebar" context={{ mediaId: item.id }} />
    </div>
  );
}
