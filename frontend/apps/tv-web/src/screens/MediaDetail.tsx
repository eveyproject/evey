import { useTranslation } from "react-i18next";
import { formatDurationMs, useLibraryServiceGetMediaItem, useLibraryServiceListMediaItems, usePlaybackServiceGetWatchProgress, type LibraryServiceListMediaItemsType, type V1MediaItem } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc, isPlayable, isSeries, isSeason } from "../lib/media";

function Children({ parentId, type, title }: { parentId: string; type: LibraryServiceListMediaItemsType; title: string }) {
  const { data } = useLibraryServiceListMediaItems({ parentId, type, "pagination.pageSize": 500 });
  const items = (data?.items ?? [])
    .filter((it): it is V1MediaItem & { id: string } => !!it.id)
    .sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  if (items.length === 0) return null;
  return (
    <FocusRoot>
      <h2 className="row-title">{title}</h2>
      <div className="row">
        {items.map((item) => (
          <Card
            key={item.id}
            title={item.indexNumber ? `${item.indexNumber}. ${item.title ?? ""}` : item.title ?? ""}
            imageUrl={posterSrc(item.id, type === "MEDIA_ITEM_TYPE_EPISODE" ? "backdrop" : "poster", type === "MEDIA_ITEM_TYPE_EPISODE" ? 640 : 400)}
            wide={type === "MEDIA_ITEM_TYPE_EPISODE"}
            onPress={() => navigate({ name: "media", mediaId: item.id })}
          />
        ))}
      </div>
    </FocusRoot>
  );
}

export function MediaDetailScreen({ mediaId }: { mediaId: string }) {
  const { t } = useTranslation();
  const { data: item, isLoading, isError } = useLibraryServiceGetMediaItem(mediaId);
  const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, {
    query: { enabled: isPlayable(item), retry: false },
  });
  const positionMs = Number(progress?.positionMs ?? 0);
  const durationMs = Number(item?.durationMs ?? progress?.durationMs ?? 0);
  const canResume = isPlayable(item) && positionMs > 0 && (durationMs === 0 || positionMs < durationMs - 15_000);

  if (isLoading) return <div className="screen screen-center"><Spinner /></div>;
  if (isError || !item) return <div className="screen screen-center"><p className="error">{t("common.error", "Something went wrong.")}</p></div>;

  const meta = [item.year, formatDurationMs(item.durationMs), item.officialRating, ...(item.genres ?? []).slice(0, 3)].filter(Boolean).join("  ·  ");

  return (
    <FocusRoot autoFocus className="screen">
      <div className="hero">
        <img src={posterSrc(item.id, "poster", 600)} alt="" />
        <div>
          {item.seriesTitle ? <p className="muted" style={{ margin: 0 }}>{item.seriesTitle}{item.seasonTitle ? ` · ${item.seasonTitle}` : ""}</p> : null}
          <h1 className="title">{item.title}</h1>
          {meta ? <p className="muted" style={{ margin: "0 0 1vw" }}>{meta}</p> : null}
          {item.overview ? <p className="overview">{item.overview}</p> : null}
          {isPlayable(item) ? (
            <FocusRoot>
              <div className="actions">
                {canResume ? (
                  <Button variant="primary" autoFocus onPress={() => navigate({ name: "player", mediaId })}>
                    {t("player.resume", "Resume")} · {formatDurationMs(positionMs)}
                  </Button>
                ) : null}
                <Button variant={canResume ? "default" : "primary"} autoFocus={!canResume} onPress={() => navigate({ name: "player", mediaId, restart: canResume })}>
                  {canResume ? t("player.startOver", "Start over") : t("common.play", "Play")}
                </Button>
              </div>
            </FocusRoot>
          ) : null}
        </div>
      </div>
      {isSeries(item) ? <Children parentId={mediaId} type="MEDIA_ITEM_TYPE_SEASON" title={t("media.seasons", "Seasons")} /> : null}
      {isSeason(item) ? <Children parentId={mediaId} type="MEDIA_ITEM_TYPE_EPISODE" title={t("media.episodes", "Episodes")} /> : null}
    </FocusRoot>
  );
}
