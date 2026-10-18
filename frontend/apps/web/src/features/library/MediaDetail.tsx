import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceGetSeriesAggregate,
} from "@evey/api";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { useStreamToken } from "@/hooks/useStreamToken";
import { formatDurationMs } from "@evey/api";
import { formatSeriesRuntime, hasMediaFile } from "./mediaDetailUtils";
import { MediaDetailHero } from "./MediaDetailHero";
import { MediaDetailActions } from "./MediaDetailActions";
import { CastCarousel } from "./MediaDetailCast";
import { SeriesSection, SeasonEpisodesSection } from "./MediaDetailSeries";
import { SimilarSection } from "./MediaDetailSimilar";
import { SagaSection } from "./MediaDetailSagas";
import { MediaDetailFileInfo } from "./MediaDetailFileInfo";
import { artworkUrl } from "./artworkUrl";

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

  // Always route through backend endpoint - see MediaCard for the ORB rationale.
  // The raw item.artwork[].url is the external provider URL (Amazon CDN for
  // OMDb, etc.) which Firefox blocks under Opaque Response Blocking. The ?v=
  // content version busts the immutable cache on a poster change.
  const backdropUrl = item.id ? artworkUrl(item.id, "backdrop", item) : undefined;
  const posterUrl = item.id ? artworkUrl(item.id, "poster", item) : undefined;

  const { data: seriesAggregate } = useLibraryServiceGetSeriesAggregate(
    item.id ?? "",
    { query: { enabled: isSeries && !!item.id } } as never,
  );
  const duration = isSeries
    ? formatSeriesRuntime(seriesAggregate?.totalDurationMs, seriesAggregate?.episodeCount)
    : formatDurationMs(item.durationMs);
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
      >
        {/* Rendered inside the hero's right column: the poster, the title and
            the actions belong to one flex row, so the poster's height is part
            of the row and the sections below clear it without a hand-tuned
            margin. */}
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
      </MediaDetailHero>

      <MediaDetailFileInfo item={item} />

      {/* Seasons and episodes come BEFORE the cast: on a series page they are
          what the viewer came for, and burying them under a cast carousel meant
          scrolling past people to reach the thing you want to watch. */}
      {isSeries && <div data-testid="seasons-section"><SeriesSection item={item} /></div>}
      {isSeason && <SeasonEpisodesSection item={item} />}

      <div data-testid="cast-section"><CastCarousel people={actors} title={t("media.detail.cast")} /></div>
      {crew.length > 0 && <CastCarousel people={crew} title={t("media.detail.crew")} />}

      {item.id && !isSeason && <SagaSection mediaItemId={item.id} />}
      {item.id && !isSeason && <SimilarSection item={item} />}

      <PluginSlot name="media.detail.sidebar" context={{ mediaId: item.id }} />
    </div>
  );
}
