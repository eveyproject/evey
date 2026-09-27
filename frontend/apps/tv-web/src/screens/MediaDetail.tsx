import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  formatAudioTrackLabel,
  formatDurationMs,
  formatTimeMs,
  getRatingServiceGetMyThumbRatingQueryKey,
  itemLabel,
  ratingServicePutThumbRating,
  seasonLabel,
  trackDisplayName,
  trackPickQuery,
  useFeedServiceSimilarItems,
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceMarkAsWatched,
  useRatingServiceGetMyThumbRating,
  type TrackPick,
  type V1MediaItem,
  type V1MediaPerson,
  type V1MediaStream,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import {
  CARD_WIDTH_FEATURED,
  CARD_WIDTH_GRID,
  POSTER_RATIO,
  RESUME_PROGRESS_QUERY,
  personTypeLabelKey,
  resolvePlayTarget,
  sortByIndexNumber,
  sortTrailers,
  trailerUrl,
  useIsFavorite,
  useMediaItemCollections,
  useParentalGate,
  useToggleFavorite,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Pressable } from "../components/ui";
import { Icon } from "../components/icons";
import { ScreenState } from "../components/ScreenState";
import { AddToPlaylistSheet } from "../components/AddToPlaylistSheet";
import { ParentalPINModal } from "../components/ParentalPINModal";
import { navigate, replace } from "../app/router";
import { posterSrc } from "../lib/media";
import { PluginSlot } from "../plugins/PluginSlot";

// Sized so the action row lands on the screen (Android, measured on the box).
const HERO_HEIGHT = 300;
const EPISODE_THUMB_WIDTH = 240;
// The fetch stays whole (500) so counts and next-episode math see every
// sibling; only the render pages.
const EPISODE_PAGE_SIZE = 20;
// The scope the page lands on: Resume (or Play) is its leftmost button.
const ACTIONS_KEY = "media-detail-actions";

function isNotFound(error: unknown): boolean {
  return typeof error === "object" && error !== null && (error as { httpStatus?: number }).httpStatus === 404;
}

function stars(rating: number): string {
  const full = Math.round(rating / 2);
  return "★".repeat(full) + "☆".repeat(Math.max(0, 5 - full));
}

function Section({ title, count, children }: { title: string; count?: string; children: React.ReactNode }) {
  return (
    <FocusRoot className="detail-section">
      <div className="section-head">
        <h2 className="section-title">{title}</h2>
        {count ? <span className="section-count">{count}</span> : null}
      </div>
      {children}
    </FocusRoot>
  );
}

function PeopleShelf({ title, people, crew }: { title: string; people: V1MediaPerson[]; crew?: boolean }) {
  const { t } = useTranslation();
  if (people.length === 0) return null;
  return (
    <Section title={title}>
      <div className="shelf people">
        {people.map((p, i) => {
          const roleKey = crew ? personTypeLabelKey(p.type) : null;
          const role = roleKey ? t(roleKey) : p.role;
          return (
            <Pressable
              key={`${p.id ?? p.name ?? i}-${p.type ?? ""}`}
              className="tv-card person-card"
              label={p.name}
              onPress={() => { if (p.id) navigate({ name: "person", personId: p.id }); }}
            >
              <span className="person-avatar">{p.thumbUrl ? <img src={p.thumbUrl} alt="" loading="lazy" /> : "?"}</span>
              <span className="person-name">{p.name}</span>
              {role ? <span className="person-role">{role}</span> : null}
            </Pressable>
          );
        })}
      </div>
    </Section>
  );
}

function PosterShelf({ title, items }: { title: string; items: { id: string; title?: string }[] }) {
  if (items.length === 0) return null;
  const w = CARD_WIDTH_GRID / 10;
  return (
    <Section title={title}>
      <div className="shelf">
        {items.map((it) => (
          <Pressable key={it.id} className="tv-card shelf-card" label={it.title} onPress={() => navigate({ name: "media", mediaId: it.id })}>
            <span className="poster" style={{ width: `${w}rem`, height: `${w * POSTER_RATIO}rem` }}>
              <img src={posterSrc(it.id, "poster", CARD_WIDTH_GRID * 2)} alt="" loading="lazy" />
            </span>
            <span className="shelf-label" style={{ width: `${w}rem` }}>{it.title}</span>
          </Pressable>
        ))}
      </div>
    </Section>
  );
}

// "More like this": the recommendation feed, else a same-genre listing, so
// the page never ends on an empty shelf.
function SimilarShelf({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: similar } = useFeedServiceSimilarItems(item.id ?? "");
  const hasSimilar = (similar?.items?.length ?? 0) > 0;
  const firstGenre = item.genres?.[0];
  const { data: byGenre } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never,
  );
  const cards = useMemo(() => {
    const fromFeed = (similar?.items ?? []).filter((s) => !!s.mediaItemId).map((s) => ({ id: s.mediaItemId as string, title: s.title }));
    if (fromFeed.length > 0) return fromFeed;
    return (byGenre?.items ?? []).filter((m) => m.id && m.id !== item.id).map((m) => ({ id: m.id as string, title: m.title }));
  }, [similar, byGenre, item.id]);
  return <PosterShelf title={t("media.detail.similarTitles")} items={cards} />;
}

function SagaSection({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const { data } = useMediaItemCollections(mediaItemId);
  const collections = data?.collections ?? [];
  if (collections.length === 0) return null;
  return (
    <Section title={t("media.collections.partOf")}>
      <div className="shelf">
        {collections.map((c) => (
          <Pressable key={c.id} className="tv-card saga-card" label={c.name} onPress={() => navigate({ name: "collection", collectionId: c.id })}>
            <span className="saga-name">{c.name}</span>
            {(c.itemCount ?? 0) > 0 ? <span className="saga-count">{t("media.person.titleCount", { count: c.itemCount })}</span> : null}
          </Pressable>
        ))}
      </div>
    </Section>
  );
}

// Audio and subtitle chosen before playback; "Auto" is the player's own choice.
function TrackPickRow({ streams, pick, onChange }: { streams: V1MediaStream[]; pick: TrackPick; onChange: (p: TrackPick) => void }) {
  const { t } = useTranslation();
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.index !== undefined);
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE" && s.index !== undefined);
  if (audio.length < 2 && subtitles.length === 0) return null;
  const pill = (key: string, label: string, selected: boolean, onPress: () => void) => (
    <Pressable key={key} className={`tv-card pill ${selected ? "active" : ""}`} label={label} onPress={onPress}>{label}</Pressable>
  );
  return (
    <div className="track-pick">
      {audio.length > 1 ? (
        <FocusRoot className="track-pick-row">
          <span className="track-pick-title">{t("media.streams.audio")}</span>
          {pill("a:auto", t("media.streams.auto"), pick.audio === undefined, () => onChange({ ...pick, audio: undefined }))}
          {audio.map((s) => pill(`a:${s.index}`, formatAudioTrackLabel(s, t("media.track", { number: s.index })), pick.audio === s.index, () => onChange({ ...pick, audio: s.index })))}
        </FocusRoot>
      ) : null}
      {subtitles.length > 0 ? (
        <FocusRoot className="track-pick-row">
          <span className="track-pick-title">{t("media.streams.subtitles")}</span>
          {pill("s:auto", t("media.streams.auto"), pick.subtitle === undefined, () => onChange({ ...pick, subtitle: undefined }))}
          {pill("s:off", t("media.streams.none"), pick.subtitle === "off", () => onChange({ ...pick, subtitle: "off" }))}
          {subtitles.map((s) => pill(
            `s:${s.index}`,
            `${trackDisplayName(s, t("media.track", { number: s.index }))}${s.isForced ? ` - ${t("media.streams.forced")}` : ""}`,
            pick.subtitle === s.index,
            () => onChange({ ...pick, subtitle: s.index }),
          ))}
        </FocusRoot>
      ) : null}
    </div>
  );
}

export function MediaDetailScreen({ mediaId }: { mediaId: string }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const { data: media, isLoading, error, refetch } = useLibraryServiceGetMediaItem(mediaId);
  const { data: watchProgress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, { query: RESUME_PROGRESS_QUERY });
  const { data: isFavorite } = useIsFavorite(mediaId);
  const toggleFavorite = useToggleFavorite(mediaId);
  const [playlistOpen, setPlaylistOpen] = useState(false);
  const [visibleEpisodes, setVisibleEpisodes] = useState(EPISODE_PAGE_SIZE);
  useEffect(() => { setVisibleEpisodes(EPISODE_PAGE_SIZE); }, [mediaId]);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);
  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [pendingPlay, setPendingPlay] = useState<{ id: string; restart: boolean } | null>(null);
  const [trackPick, setTrackPick] = useState<TrackPick>({});

  const invalidateWatched = () => {
    void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${mediaId}/progress`] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/feeds/resume-playback"] });
  };
  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(true); invalidateWatched(); },
      onError: () => toast.error(t("media.detail.markWatchedFailed", "Could not mark as watched")),
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(false); invalidateWatched(); },
      onError: () => toast.error(t("media.detail.markUnwatchedFailed", "Could not mark as unwatched")),
    },
  });

  // like = 1, dislike = -1, cleared = 0, written optimistically: on a remote
  // the round trip is the only feedback.
  const { data: myRating, isError: ratingUnknown } = useRatingServiceGetMyThumbRating(mediaId, {
    query: { enabled: !!mediaId, select: (d) => d.value ?? 0, staleTime: 60_000 },
  });
  const setThumb = useMutation({
    mutationFn: (value: number) => ratingServicePutThumbRating(mediaId, { value }),
    onMutate: async (value: number) => {
      const key = getRatingServiceGetMyThumbRatingQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<{ value?: number }>(key);
      qc.setQueryData(key, { value });
      return { prev };
    },
    onError: (_e: Error, _v: number, ctx?: { prev?: { value?: number } }) => {
      if (ctx?.prev !== undefined) qc.setQueryData(getRatingServiceGetMyThumbRatingQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => { void qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) }); },
  });
  const thumbsBusy = setThumb.isPending || ratingUnknown;

  const isSeries = media?.type === "MEDIA_ITEM_TYPE_SERIES";
  const isSeason = media?.type === "MEDIA_ITEM_TYPE_SEASON";
  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: mediaId, "pagination.pageSize": 500 },
    { query: { enabled: !!mediaId && (isSeries || isSeason) } },
  );
  const children = useMemo(() => childrenData?.items ?? [], [childrenData?.items]);
  const seasons = useMemo(() => (isSeries ? sortByIndexNumber(children) : []), [isSeries, children]);
  const firstSeasonId = seasons[0]?.id ?? "";
  const { data: firstSeasonEpisodes } = useLibraryServiceListMediaItems(
    { parentId: firstSeasonId, "pagination.pageSize": 500 },
    { query: { enabled: isSeries && !!firstSeasonId } },
  );
  const episodes = useMemo(
    () => sortByIndexNumber(isSeason ? children : (firstSeasonEpisodes?.items ?? [])),
    [isSeason, children, firstSeasonEpisodes?.items],
  );
  const playTarget = resolvePlayTarget({ id: mediaId, isSeries, isSeason, episodeIds: episodes.map((e) => e.id) });

  // `restart` and the track pick survive the PIN detour. The pick belongs to
  // THIS page's streams: an episode started from a series keeps the player's.
  const openPlayer = (target: string, restart: boolean) => {
    const params = new URLSearchParams(trackPickQuery(target === media?.id ? trackPick : undefined));
    navigate({
      name: "player",
      mediaId: target,
      restart,
      ...(params.get("audio") ? { audio: Number(params.get("audio")) } : {}),
      ...(params.get("subtitle") ? { subtitle: params.get("subtitle") === "off" ? "off" as const : Number(params.get("subtitle")) } : {}),
    });
  };
  const play = (restart: boolean) => {
    if (!playTarget) return;
    if (needsPin()) {
      setPendingPlay({ id: playTarget, restart });
      return;
    }
    openPlayer(playTarget, restart);
  };

  if (isLoading || !media) {
    if (!isLoading && isNotFound(error)) {
      return (
        <FocusRoot autoFocus className="screen-state">
          <p className="state-title big">{t("media.detail.notFound")}</p>
          <Pressable className="tv-card state-btn primary" autoFocus onPress={() => replace({ name: "home" })}>
            {t("common.errorBoundary.goHome", "Go Home")}
          </Pressable>
        </FocusRoot>
      );
    }
    return <ScreenState loading={isLoading} error={!isLoading} onRetry={() => void refetch()} />;
  }

  const meta = [media.year, formatDurationMs(media.durationMs), media.officialRating].filter(Boolean).join("  •  ");
  const resumeMs = Number(watchProgress?.positionMs ?? 0);
  const hasProgress = resumeMs > 0 && !watchProgress?.played;
  const people = media.people ?? [];
  const actors = people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");
  const crew = people.filter((p) => p.type === "PERSON_TYPE_DIRECTOR" || p.type === "PERSON_TYPE_WRITER" || p.type === "PERSON_TYPE_PRODUCER");
  const trailer = sortTrailers(media.trailers ?? []).map(trailerUrl).find(Boolean) ?? null;
  const posterW = CARD_WIDTH_FEATURED / 10;
  const posterH = posterW * POSTER_RATIO;
  const seasonW = CARD_WIDTH_GRID / 10;

  const action = (key: string, label: string, onPress: () => void, active = false, disabled = false) => (
    <Pressable key={key} className={`tv-card action-btn ${active ? "on" : ""} ${disabled ? "dim" : ""}`} label={label} onPress={() => { if (!disabled) onPress(); }}>
      {label}
    </Pressable>
  );

  return (
    <FocusRoot autoFocus className="detail-page" preferredChildFocusKey={ACTIONS_KEY}>
      <div className="detail-hero" style={{ height: `${HERO_HEIGHT / 10}rem` }}>
        <img src={posterSrc(media.id, "backdrop", 1920)} alt="" />
        <div className="detail-fade"><i /><i /><i /><i /></div>
      </div>
      <div className="detail-head" style={{ marginTop: `${-posterH / 2}rem` }}>
        <img className="detail-poster" src={posterSrc(media.id, "poster", CARD_WIDTH_FEATURED * 2)} alt="" style={{ width: `${posterW}rem`, height: `${posterH}rem` }} />
        <div className="detail-titles">
          {!isSeries && !isSeason && media.seriesTitle ? <p className="detail-eyebrow">{media.seriesTitle}</p> : null}
          <h1 className="detail-title">{itemLabel(media, t)}</h1>
          {media.tagline ? <p className="detail-tagline">{media.tagline}</p> : null}
          {meta ? <p className="detail-meta">{meta}</p> : null}
          {media.genres && media.genres.length > 0 ? <p className="detail-line">{media.genres.join(", ")}</p> : null}
          {media.studios && media.studios.length > 0 ? <p className="detail-line">{t("media.detail.studio")}{media.studios.join(", ")}</p> : null}
          {media.communityRating != null && media.communityRating > 0 ? (
            <p className="detail-rating" aria-label={t("media.detail.communityRatingA11y", { rating: media.communityRating.toFixed(1) })}>
              <span className="accent">{stars(media.communityRating)}</span>
              <span className="muted">{media.communityRating.toFixed(1)}</span>
            </p>
          ) : null}
        </div>
      </div>

      <div className="detail-body">
        <FocusRoot focusKey={ACTIONS_KEY} className="detail-actions">
          <div className="play-group">
            <Pressable
              className={`tv-card play-btn ${hasProgress ? "left" : ""}`}
              autoFocus
              label={hasProgress ? `${t("media.resume", "Resume")} ${formatTimeMs(resumeMs)}` : t("player.controls.play")}
              onPress={() => play(false)}
            >
              {hasProgress ? `${t("media.resume", "Resume")} ${formatTimeMs(resumeMs)}` : t("player.controls.play")}
            </Pressable>
            {hasProgress ? (
              <Pressable className="tv-card play-restart" label={t("media.playFromStart", "Play from the beginning")} onPress={() => play(true)}>
                <Icon name="refresh" size={26} />
              </Pressable>
            ) : null}
          </div>
          {action("fav", isFavorite ? t("media.detail.favorited") : t("media.detail.favorite"), () => toggleFavorite.mutate(), !!isFavorite, toggleFavorite.isPending)}
          {action(
            "watched",
            watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched"),
            () => {
              if (!profileId) return;
              (watched ? markUnwatched : markWatched).mutate({ mediaItemId: mediaId, params: { profileId } });
            },
            watched,
            markWatched.isPending || markUnwatched.isPending,
          )}
          {action("playlist", t("media.detail.addToPlaylist"), () => setPlaylistOpen(true))}
          {action("like", t("media.rating.like", "Like"), () => setThumb.mutate(myRating === 1 ? 0 : 1), myRating === 1, thumbsBusy)}
          {action("dislike", t("media.rating.dislike", "Dislike"), () => setThumb.mutate(myRating === -1 ? 0 : -1), myRating === -1, thumbsBusy)}
          {trailer ? action("trailer", t("media.detail.trailer"), () => { window.open(trailer, "_blank", "noopener"); }) : null}
        </FocusRoot>

        {media.streams && media.id === playTarget ? <TrackPickRow streams={media.streams} pick={trackPick} onChange={setTrackPick} /> : null}

        {media.overview ? <p className="detail-overview">{media.overview}</p> : null}

        {isSeries && seasons.length > 0 ? (
          <Section title={t("media.detail.seasons")} count={t("media.detail.seasonCount", { count: seasons.length })}>
            <div className="shelf">
              {seasons.map((s) => (
                <Pressable key={s.id} className="tv-card shelf-card" label={seasonLabel(s, t)} onPress={() => s.id && navigate({ name: "media", mediaId: s.id })}>
                  <span className="poster" style={{ width: `${seasonW}rem`, height: `${seasonW * POSTER_RATIO}rem` }}>
                    <img src={posterSrc(s.id, "poster", CARD_WIDTH_GRID * 2)} alt="" loading="lazy" />
                  </span>
                  <span className="shelf-label big" style={{ width: `${seasonW}rem` }}>{seasonLabel(s, t)}</span>
                </Pressable>
              ))}
            </div>
          </Section>
        ) : null}

        {isSeason && episodes.length > 0 ? (
          <Section title={t("media.detail.episodes")} count={t("media.detail.episodeCount", { count: episodes.length })}>
            <div className="episode-list">
              {episodes.slice(0, visibleEpisodes).map((ep) => (
                <Pressable
                  key={ep.id}
                  className="tv-card episode-row"
                  label={ep.indexNumber != null ? `${ep.indexNumber}. ${ep.title}` : ep.title}
                  onPress={() => ep.id && navigate({ name: "media", mediaId: ep.id })}
                >
                  <span className="episode-thumb" style={{ width: `${EPISODE_THUMB_WIDTH / 10}rem`, height: `${(EPISODE_THUMB_WIDTH * 9) / 160}rem` }}>
                    <img src={posterSrc(ep.id, "backdrop", EPISODE_THUMB_WIDTH * 2)} alt="" loading="lazy" />
                  </span>
                  <span className="episode-text">
                    <span className="episode-title">{ep.indexNumber != null ? `${ep.indexNumber}. ` : ""}{ep.title}</span>
                    {ep.durationMs ? <span className="episode-meta">{formatDurationMs(ep.durationMs)}</span> : null}
                    {ep.overview ? <span className="episode-overview">{ep.overview}</span> : null}
                  </span>
                </Pressable>
              ))}
              {episodes.length > visibleEpisodes ? (
                <Pressable className="tv-card episode-row more" onPress={() => setVisibleEpisodes((c) => c + EPISODE_PAGE_SIZE)}>
                  {t("media.detail.showMoreEpisodes", "Show more")}
                </Pressable>
              ) : null}
            </div>
          </Section>
        ) : null}

        <PeopleShelf title={t("media.detail.cast")} people={actors} />
        <PeopleShelf title={t("media.detail.crew")} people={crew} crew />
        {media.id ? <SagaSection mediaItemId={media.id} /> : null}
        {!isSeason ? <SimilarShelf item={media} /> : null}
        <PluginSlot name="media.detail.sidebar" context={{ mediaId: media.id }} />
      </div>

      {playlistOpen ? <AddToPlaylistSheet mediaItemId={mediaId} mediaType={media.type} onClose={() => setPlaylistOpen(false)} /> : null}
      {pendingPlay && gateProfileId ? (
        <ParentalPINModal
          profileId={gateProfileId}
          onClose={() => setPendingPlay(null)}
          onVerified={() => { const p = pendingPlay; setPendingPlay(null); openPlayer(p.id, p.restart); }}
        />
      ) : null}
    </FocusRoot>
  );
}
