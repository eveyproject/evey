import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  formatDurationMs, getRatingServiceGetMyThumbRatingQueryKey, ratingServicePutThumbRating,
  useFeedServiceSimilarItems, useLibraryServiceGetMediaItem, useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress, usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched,
  useRatingServiceGetMyThumbRating, trackPickQuery, formatAudioTrackLabel,
  trackDisplayName,
  type LibraryServiceListMediaItemsType, type TrackPick, type V1MediaItem, type V1MediaPerson,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  resolvePlayTarget, sortByIndexNumber, sortTrailers, trailerUrl,
  useAddToPlaylist, useIsFavorite, useIsWatched, usePlaylistList, useToggleFavorite,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc, isPlayable, isSeries, isSeason } from "../lib/media";
import { PluginSlot } from "../plugins/PluginSlot";
import { ThumbDownIcon, ThumbUpIcon } from "../components/icons";

// The community rating as five stars, the 0-10 scale every client renders.
function stars(rating: number): string {
  const full = Math.round(rating / 2);
  return "★".repeat(full) + "☆".repeat(Math.max(0, 5 - full));
}

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

// The people under the picture, each one a card that opens their page, the
// way the television and the phone have them.
function People({ people, title }: { people: V1MediaPerson[]; title: string }) {
  if (people.length === 0) return null;
  return (
    <FocusRoot>
      <h2 className="row-title">{title}</h2>
      <div className="row">
        {people.slice(0, 24).map((p, i) => (
          <Card
            key={p.id ?? `${p.name}-${i}`}
            title={p.role ? `${p.name} (${p.role})` : p.name ?? ""}
            imageUrl={p.thumbUrl || undefined}
            onPress={() => { if (p.id) navigate({ name: "person", personId: p.id }); }}
          />
        ))}
      </div>
    </FocusRoot>
  );
}

/** The playlists this title can join, as a panel the remote walks. */
function AddToPlaylist({ mediaId, mediaType, onClose }: { mediaId: string; mediaType?: string; onClose: () => void }) {
  const { t } = useTranslation();
  const { data: playlists } = usePlaylistList();
  const add = useAddToPlaylist();
  const [done, setDone] = useState<string | null>(null);
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus className="modal-sheet">
        <h2 className="row-title">{t("media.playlist.addTitle")}</h2>
        <div className="track-list">
          {(playlists ?? []).map((pl, i) => (
            <Button
              key={pl.id}
              autoFocus={i === 0}
              variant={done === pl.id ? "primary" : "default"}
              onPress={() => { add.mutate({ playlistId: pl.id, mediaItemId: mediaId, mediaType }); setDone(pl.id); }}
            >
              {pl.name}{done === pl.id ? ` · ${t("media.playlist.addSuccess")}` : ""}
            </Button>
          ))}
          {(playlists ?? []).length === 0 ? <p className="muted">{t("common.noPlaylists")}</p> : null}
        </div>
        <div className="actions">
          <Button variant="primary" onPress={onClose}>{t("common.close")}</Button>
        </div>
      </FocusRoot>
    </div>
  );
}

// The scope the page lands on: Resume (or Play) is its leftmost button.
const ACTIONS_KEY = "media-detail-actions";

export function MediaDetailScreen({ mediaId }: { mediaId: string }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId) ?? undefined;
  const { data: item, isLoading, isError } = useLibraryServiceGetMediaItem(mediaId);
  const { data: fav } = useIsFavorite(mediaId);
  const toggleFavorite = useToggleFavorite(mediaId);
  const watched = useIsWatched(mediaId);
  const invalidateWatched = () => {
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/feeds/resume-playback"] });
  };
  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: invalidateWatched } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: invalidateWatched } });
  const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, {
    query: { enabled: isPlayable(item), retry: false },
  });
  const [playlistOpen, setPlaylistOpen] = useState(false);
  const [trackPick, setTrackPick] = useState<TrackPick>({});
  const { data: similarData } = useFeedServiceSimilarItems(mediaId);
  const { data: myRating } = useRatingServiceGetMyThumbRating(mediaId, {
    query: { enabled: !!mediaId, select: (d) => d.value ?? 0, staleTime: 60_000 },
  });
  const setThumb = useMutation({
    mutationFn: (value: number) => ratingServicePutThumbRating(mediaId, { value }),
    onSettled: () => { void qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) }); },
  });
  const positionMs = Number(progress?.positionMs ?? 0);
  const durationMs = Number(item?.durationMs ?? progress?.durationMs ?? 0);
  // Only the title itself can be resumed: a series page plays its first
  // episode, which has a position of its own.
  const canResume = isPlayable(item) && positionMs > 0 && (durationMs === 0 || positionMs < durationMs - 15_000);

  // A series and a season own no file: playing their own id can only fail, so
  // the button starts on the first episode, the way every other client does.
  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: mediaId, "pagination.pageSize": 500 },
    { query: { enabled: isSeries(item) || isSeason(item) } },
  );
  const { data: firstSeasonEpisodes } = useLibraryServiceListMediaItems(
    { parentId: sortByIndexNumber(childrenData?.items ?? [])[0]?.id ?? "", "pagination.pageSize": 500 },
    { query: { enabled: isSeries(item) && !!(childrenData?.items ?? []).length } },
  );
  const episodes = useMemo(
    () => sortByIndexNumber(isSeason(item) ? (childrenData?.items ?? []) : (firstSeasonEpisodes?.items ?? [])),
    [item, childrenData, firstSeasonEpisodes],
  );
  const playTarget = resolvePlayTarget({
    id: mediaId,
    isSeries: isSeries(item),
    isSeason: isSeason(item),
    episodeIds: episodes.map((e) => e.id),
  });
  // The track pick belongs to THIS page's streams: an episode started from a
  // series page keeps the player's own resolution.
  const openPlayer = (restart: boolean) => {
    if (!playTarget) return;
    const pick = playTarget === mediaId ? trackPick : undefined;
    const q = trackPickQuery(pick);
    const params = new URLSearchParams(q);
    navigate({
      name: "player",
      mediaId: playTarget,
      restart,
      ...(params.get("audio") ? { audio: Number(params.get("audio")) } : {}),
      ...(params.get("subtitle") ? { subtitle: params.get("subtitle") === "off" ? "off" as const : Number(params.get("subtitle")) } : {}),
    });
  };

  if (isLoading) return <div className="screen screen-center"><Spinner /></div>;
  if (isError || !item) return <div className="screen screen-center"><p className="error">{t("common.error", "Something went wrong.")}</p></div>;

  const meta = [item.year, formatDurationMs(item.durationMs), item.officialRating, ...(item.genres ?? []).slice(0, 3)].filter(Boolean).join("  ·  ");
  const trailers = sortTrailers(item.trailers ?? []);
  const trailerHref = trailers.map(trailerUrl).find(Boolean) ?? null;
  const similar = (similarData?.items ?? []).filter((s) => s.mediaItemId);
  const audioStreams = (item.streams ?? []).filter((st) => st.type === "MEDIA_STREAM_TYPE_AUDIO");
  const subtitleStreams = (item.streams ?? []).filter((st) => st.type === "MEDIA_STREAM_TYPE_SUBTITLE");
  const people = item.people ?? [];
  const actors = people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");
  const crew = people.filter((p) => p.type !== "PERSON_TYPE_ACTOR" && p.type !== "PERSON_TYPE_GUEST_STAR");
  const isFav = !!fav;
  const busy = markWatched.isPending || markUnwatched.isPending;

  return (
    <FocusRoot autoFocus className="screen" preferredChildFocusKey={ACTIONS_KEY}>
      <div className="hero">
        <img src={posterSrc(item.id, "poster", 600)} alt="" />
        <div>
          {item.seriesTitle ? <p className="muted" style={{ margin: 0 }}>{item.seriesTitle}{item.seasonTitle ? ` · ${item.seasonTitle}` : ""}</p> : null}
          <h1 className="title">{item.title}</h1>
          {meta ? <p className="muted" style={{ margin: "0 0 0.4vw" }}>{meta}</p> : null}
          {item.studios && item.studios.length > 0 ? <p className="muted" style={{ margin: "0 0 0.4vw" }}>{t("media.detail.studio")}{item.studios.join(", ")}</p> : null}
          {item.communityRating ? <p className="muted" style={{ margin: "0 0 1vw" }}>{stars(item.communityRating)} {item.communityRating.toFixed(1)}/10</p> : null}
          {item.overview ? <p className="overview">{item.overview}</p> : null}
          {playTarget ? (
            <FocusRoot focusKey={ACTIONS_KEY}>
              <div className="actions">
                {canResume ? (
                  <Button variant="primary" autoFocus onPress={() => openPlayer(false)}>
                    {t("media.detail.resume", "Resume")} · {formatDurationMs(positionMs)}
                  </Button>
                ) : null}
                <Button variant={canResume ? "default" : "primary"} autoFocus={!canResume} onPress={() => openPlayer(canResume)}>
                  {canResume ? t("media.playFromStart", "Play from the beginning") : t("common.play", "Play")}
                </Button>
                <Button onPress={() => toggleFavorite.mutate()}>
                  {isFav ? "★ " : "☆ "}{isFav ? t("media.detail.favorited", "Favorited") : t("media.detail.favorite", "Favorite")}
                </Button>
                <Button disabled={busy} onPress={() => (watched ? markUnwatched : markWatched).mutate({ mediaItemId: mediaId, params: { profileId } })}>
                  {watched ? t("media.detail.markUnwatched", "Mark Unwatched") : t("media.detail.markWatched", "Mark Watched")}
                </Button>
                <Button onPress={() => setPlaylistOpen(true)}>{t("media.playlist.addTitle")}</Button>
                <Button variant={myRating === 1 ? "primary" : "default"} label={t("media.rating.like")} onPress={() => setThumb.mutate(myRating === 1 ? 0 : 1)}><ThumbUpIcon /></Button>
                <Button variant={myRating === -1 ? "primary" : "default"} label={t("media.rating.dislike")} onPress={() => setThumb.mutate(myRating === -1 ? 0 : -1)}><ThumbDownIcon /></Button>
                {trailerHref ? <Button onPress={() => window.open(trailerHref, "_blank", "noopener")}>{t("media.detail.trailer")}</Button> : null}
              </div>
            </FocusRoot>
          ) : (
            <FocusRoot focusKey={ACTIONS_KEY}>
              <div className="actions">
                <Button autoFocus onPress={() => toggleFavorite.mutate()}>
                  {isFav ? "★ " : "☆ "}{isFav ? t("media.detail.favorited", "Favorited") : t("media.detail.favorite", "Favorite")}
                </Button>
              </div>
            </FocusRoot>
          )}
        </div>
      </div>
      {audioStreams.length > 1 || subtitleStreams.length > 0 ? (
        <FocusRoot>
          <h2 className="row-title">{t("media.streams.audio")}</h2>
          <div className="chips">
            <Button variant={trackPick.audio === undefined ? "primary" : "default"} onPress={() => setTrackPick((p) => ({ ...p, audio: undefined }))}>{t("media.streams.auto")}</Button>
            {audioStreams.map((st) => (
              <Button key={st.index} variant={trackPick.audio === st.index ? "primary" : "default"} onPress={() => setTrackPick((p) => ({ ...p, audio: st.index }))}>
                {formatAudioTrackLabel(st, t("media.track", { number: st.index }))}
              </Button>
            ))}
          </div>
          <h2 className="row-title">{t("media.streams.subtitles")}</h2>
          <div className="chips">
            <Button variant={trackPick.subtitle === undefined ? "primary" : "default"} onPress={() => setTrackPick((p) => ({ ...p, subtitle: undefined }))}>{t("media.streams.auto")}</Button>
            <Button variant={trackPick.subtitle === "off" ? "primary" : "default"} onPress={() => setTrackPick((p) => ({ ...p, subtitle: "off" }))}>{t("media.streams.none")}</Button>
            {subtitleStreams.map((st) => (
              <Button key={st.index} variant={trackPick.subtitle === st.index ? "primary" : "default"} onPress={() => setTrackPick((p) => ({ ...p, subtitle: st.index }))}>
                {`${trackDisplayName(st, t("media.track", { number: st.index }))}${st.isForced ? ` - ${t("media.streams.forced")}` : ""}`}
              </Button>
            ))}
          </div>
        </FocusRoot>
      ) : null}
      {isSeries(item) ? <Children parentId={mediaId} type="MEDIA_ITEM_TYPE_SEASON" title={t("media.detail.seasons", "Seasons")} /> : null}
      {isSeason(item) ? <Children parentId={mediaId} type="MEDIA_ITEM_TYPE_EPISODE" title={t("media.detail.episodes", "Episodes")} /> : null}
      <People people={actors} title={t("media.detail.cast", "Cast")} />
      <People people={crew} title={t("media.detail.crew", "Crew")} />
      {similar.length > 0 ? (
        <FocusRoot>
          <h2 className="row-title">{t("media.detail.similarTitles")}</h2>
          <div className="row">
            {similar.map((sim) => (
              <Card key={sim.mediaItemId} title={sim.title ?? ""} imageUrl={posterSrc(sim.mediaItemId)} onPress={() => navigate({ name: "media", mediaId: sim.mediaItemId! })} />
            ))}
          </div>
        </FocusRoot>
      ) : null}
      <div className="plugin-row">
        <PluginSlot name="media.detail.sidebar" context={{ mediaId, mediaType: item.type }} />
      </div>
      {playlistOpen ? <AddToPlaylist mediaId={mediaId} mediaType={item.type} onClose={() => setPlaylistOpen(false)} /> : null}
    </FocusRoot>
  );
}
