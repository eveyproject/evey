import { useEffect, useMemo, useState } from "react";
import { useInfiniteQuery, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  eveyFetch,
  formatDurationMs,
  getNotificationServiceGetUnreadCountQueryKey,
  getNotificationServiceListNotificationsQueryKey,
  notificationLabel,
  useLibraryServiceListMediaItems,
  useNotificationServiceListNotifications,
  useNotificationServiceMarkAllRead,
  useNotificationServiceMarkRead,
  type NotificationLabelInput,
  type V1Notification,
  getLibraryServiceListGenresQueryOptions,
  getPlaybackServiceGetWatchHistoryQueryKey,
  playbackServiceGetWatchHistory,
  useLibraryServiceListLibraries,
  useLibraryServiceSearchMedia,
  usePlaybackServiceMarkAsUnwatched,
  watchHistoryLabel,
} from "@evey/api";
import {
  CARD_WIDTH_GRID,
  collectionSubtitle,
  mergeGenres,
  progressPercent,
  genreTranslationKey,
  mediaTypeLabelKey,
  useCollection,
  useCollections,
  useFavoritesList,
  usePlaylistDetail,
  usePlaylistList,
} from "@evey/client";
import { toast } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { ScreenState } from "../components/ScreenState";
import { GridCard } from "../components/cards";
import { navigate } from "../app/router";
import { posterSrc, serverImageSrc } from "../lib/media";
import { Icon } from "../components/icons";

const HISTORY_PAGE_SIZE = 30;
const FILMOGRAPHY_PAGE = 24;


const LIBRARY_TYPE_KEYS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "media.library.movies",
  LIBRARY_TYPE_SHOWS: "media.library.tvShows",
  LIBRARY_TYPE_MUSIC: "media.library.music",
  LIBRARY_TYPE_PHOTOS: "media.library.photos",
};

/** The Libraries tab: the cross-library views, then one card per library. */
export function LibrariesScreen() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useLibraryServiceListLibraries();
  const libs = (data?.libraries ?? []).filter((l) => l.id);
  if (isLoading) return <ScreenState loading />;
  if (isError) return <ScreenState error onRetry={() => void refetch()} />;
  return (
    <FocusRoot autoFocus className="tab-page">
      <h1 className="tab-title">{t("common.nav.libraries")}</h1>
      <FocusRoot className="link-row">
        <Pressable className="tv-card link-card" autoFocus={libs.length === 0} onPress={() => navigate({ name: "collections" })}>{t("common.collections")}</Pressable>
        <Pressable className="tv-card link-card" onPress={() => navigate({ name: "playlists" })}>{t("common.playlists")}</Pressable>
        <Pressable className="tv-card link-card" onPress={() => navigate({ name: "genres" })}>{t("common.nav.genres")}</Pressable>
      </FocusRoot>
      <FocusRoot className="library-list">
        {libs.map((l, i) => (
          <Pressable key={l.id} focusKey={`libraries:${l.id}`} className="tv-card library-card" autoFocus={i === 0} label={l.name} onPress={() => navigate({ name: "library", libraryId: l.id! })}>
            <span className="library-name">{l.name}</span>
            <span className="library-type">{t((l.type && LIBRARY_TYPE_KEYS[l.type]) || "media.library.title")}</span>
          </Pressable>
        ))}
      </FocusRoot>
    </FocusRoot>
  );
}

function EmptyWithAction({ message, actionLabel, onAction }: { message: string; actionLabel: string; onAction: () => void }) {
  return (
    <FocusRoot autoFocus className="center-fill pad-top">
      <p className="muted big">{message}</p>
      <Pressable className="tv-card empty-action" autoFocus onPress={onAction}>{actionLabel}</Pressable>
    </FocusRoot>
  );
}

/** Every collection, each a boxed card with its poster, as the television lists them. */
export function CollectionsScreen() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useCollections();
  const list = (data?.collections ?? []).filter((c) => c.id);
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title wide">{t("common.collections")}</h1>
      {list.length === 0 ? (
        <EmptyWithAction message={t("common.noCollections")} actionLabel={t("media.library.libraries")} onAction={() => navigate({ name: "libraries" })} />
      ) : (
        <FocusRoot autoFocus grid className="box-grid-wrap">
          <div className="grid box-grid">
            {list.map((c, i) => {
              const sub = collectionSubtitle(c.kind, c.itemCount, t);
              return (
                <Pressable key={c.id} focusKey={`collections:${c.id}`} className="tv-card box-card" autoFocus={i === 0} label={c.name} onPress={() => navigate({ name: "collection", collectionId: c.id })}>
                  <span className="box-poster">
                    {c.posterUrl ? <img src={serverImageSrc(c.posterUrl)} alt="" loading="lazy" /> : <span className="box-initial">{c.name[0]?.toUpperCase() ?? "?"}</span>}
                    {sub.badge ? <span className="box-badge">{sub.badge}</span> : null}
                  </span>
                  <span className="box-name">{c.name}</span>
                  {sub.count ? <span className="box-sub">{sub.count}</span> : null}
                </Pressable>
              );
            })}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}

export function CollectionScreen({ collectionId }: { collectionId: string }) {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch, hasNextPage, fetchNextPage, isFetchingNextPage } = useCollection(collectionId);
  const items = data?.items ?? [];
  const col = data?.collection;
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title">{col?.name ?? t("media.collections.title")}</h1>
      {col?.overview ? <p className="deep-overview">{col.overview}</p> : null}
      {items.length === 0 ? (
        <div className="center-fill pad-top"><p className="muted big">{t("common.noItems")}</p></div>
      ) : (
        <FocusRoot autoFocus grid className="box-grid-wrap">
          <div className="grid box-grid">
            {items.map((it, i) => (
              <Pressable
                key={it.id}
                focusKey={`collection:${it.id}`}
                className="tv-card box-card"
                autoFocus={i === 0}
                label={it.title}
                onFocus={() => { if (hasNextPage && !isFetchingNextPage && i >= items.length - 10) void fetchNextPage(); }}
                onPress={() => navigate({ name: "media", mediaId: it.id })}
              >
                <span className="box-poster"><img src={posterSrc(it.id, "poster", 320)} alt="" loading="lazy" /></span>
                <span className="box-name small">{it.title}</span>
                {it.year ? <span className="box-sub">{it.year}</span> : null}
              </Pressable>
            ))}
          </div>
          {isFetchingNextPage ? <Spinner /> : null}
        </FocusRoot>
      )}
    </div>
  );
}

export function PlaylistsScreen() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = usePlaylistList();
  const list = data ?? [];
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title wide">{t("common.playlists")}</h1>
      {list.length === 0 ? (
        <EmptyWithAction message={t("common.noPlaylists")} actionLabel={t("media.library.libraries")} onAction={() => navigate({ name: "libraries" })} />
      ) : (
        <FocusRoot autoFocus className="stack-list">
          {list.map((p, i) => (
            <Pressable key={p.id} className="tv-card playlist-card" autoFocus={i === 0} label={p.name} onPress={() => navigate({ name: "playlist", playlistId: p.id })}>
              <span className="playlist-icon"><Icon name="musical-note" size={21} /></span>
              <span className="playlist-text">
                <span className="playlist-name">{p.name}</span>
                {p.description ? <span className="playlist-desc">{p.description}</span> : null}
                {p.type === "smart" ? <span className="playlist-meta">{t("media.collections.smart")}</span> : null}
              </span>
              <span className="muted"><Icon name="chevron-forward" size={21} /></span>
            </Pressable>
          ))}
        </FocusRoot>
      )}
    </div>
  );
}

export function PlaylistScreen({ playlistId }: { playlistId: string }) {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = usePlaylistDetail(playlistId);
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  if (!data) return <div className="center-fill"><p className="muted big">{t("media.playlists.notFound")}</p></div>;
  const items = data.items ?? [];
  return (
    <div className="screen deep-list">
      <h1 className="deep-title tight">{data.name}</h1>
      {data.description ? <p className="deep-overview">{data.description}</p> : null}
      {items.length === 0 ? (
        <div className="center-fill pad-top"><p className="muted big">{t("common.noItems")}</p></div>
      ) : (
        <FocusRoot autoFocus className="stack-list rows">
          {items.map((it, i) => {
            const typeKey = mediaTypeLabelKey(it.type);
            return (
              <Pressable key={it.media_item_id} className="tv-card playlist-row" autoFocus={i === 0} label={it.title ?? it.media_item_id} onPress={() => navigate({ name: "media", mediaId: it.media_item_id })}>
                <span className="playlist-order">{it.sort_order != null ? it.sort_order + 1 : "-"}</span>
                <span className="playlist-text">
                  <span className="playlist-row-title">{it.title ?? it.media_item_id}</span>
                  <span className="playlist-row-meta">
                    {typeKey ? <span>{t(typeKey)}</span> : null}
                    {it.year ? <span>{it.year}</span> : null}
                  </span>
                </span>
                <span className="muted"><Icon name="chevron-forward" size={21} /></span>
              </Pressable>
            );
          })}
          <p className="playlist-footer">{t("common.item", { count: items.length })}</p>
        </FocusRoot>
      )}
    </div>
  );
}

/** Every browsable library's genres, merged and deduplicated, as tiles. */
export function GenresScreen() {
  const { t } = useTranslation();
  const { data: libsData, isLoading: libsLoading, isError: libsError, refetch } = useLibraryServiceListLibraries();
  const libs = (libsData?.libraries ?? []).filter((l) => l.id && (l.type === "LIBRARY_TYPE_MOVIES" || l.type === "LIBRARY_TYPE_SHOWS"));
  const queries = useQueries({ queries: libs.map((l) => getLibraryServiceListGenresQueryOptions(l.id!)) });
  const genres = useMemo(
    () => mergeGenres(queries.map((q) => (q.data as { genres?: string[] } | undefined)?.genres)),
    // The query objects are new on every render; their data is what matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queries.map((q) => (q.data as { genres?: string[] } | undefined)?.genres?.join("|")).join("~")],
  );
  if (libsError || queries.some((q) => q.isError)) {
    return <ScreenState error onRetry={() => { void refetch(); queries.forEach((q) => void q.refetch()); }} />;
  }
  if (libsLoading || queries.some((q) => q.isLoading)) return <ScreenState loading />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title wide">{t("common.nav.genres")}</h1>
      {genres.length === 0 ? (
        <EmptyWithAction message={t("common.noGenres")} actionLabel={t("media.library.libraries")} onAction={() => navigate({ name: "libraries" })} />
      ) : (
        <FocusRoot autoFocus grid className="box-grid-wrap">
          <div className="grid genre-grid">
            {genres.map((g, i) => (
              <Pressable key={g} focusKey={`genres:${g}`} className="tv-card genre-tile" autoFocus={i === 0} onPress={() => navigate({ name: "genre", genre: g })}>
                {t(genreTranslationKey(g), g)}
              </Pressable>
            ))}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}

export function GenreScreen({ genre }: { genre: string }) {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia({ genre, limit: 120 });
  const items = (data?.items ?? []).filter((it) => it.id);
  if (isError) return <ScreenState error onRetry={() => void refetch()} />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title">{t(genreTranslationKey(genre), genre)}</h1>
      {isLoading ? (
        <ScreenState loading />
      ) : items.length === 0 ? (
        <EmptyWithAction message={t("media.library.noItemsFiltered")} actionLabel={t("common.nav.genres")} onAction={() => navigate({ name: "genres" })} />
      ) : (
        <FocusRoot autoFocus grid className="library-grid-wrap">
          <div className="grid library-grid">
            {items.map((it, i) => (
              <GridCard
                key={it.id}
                focusKey={`genre:${it.id}`}
                title={it.title ?? ""}
                imageUrl={posterSrc(it.id, "poster", CARD_WIDTH_GRID * 2)}
                rating={it.communityRating}
                autoFocus={i === 0}
                onPress={() => navigate({ name: "media", mediaId: it.id! })}
              />
            ))}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}

export function FavoritesScreen() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useFavoritesList();
  const items = data ?? [];
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <FocusRoot autoFocus className="tab-page">
      <h1 className="tab-title">{t("media.favorites.title")}</h1>
      {items.length === 0 ? (
        <FocusRoot className="center-fill pad-top">
          <p className="muted big">{t("media.favorites.empty")}</p>
          <Pressable className="tv-card empty-action" onPress={() => navigate({ name: "libraries" })}>{t("media.library.libraries")}</Pressable>
        </FocusRoot>
      ) : (
        <FocusRoot grid className="library-grid-wrap">
          <div className="grid library-grid">
            {items.map((it, i) => (
              <GridCard
                key={it.mediaItemId}
                focusKey={`favorites:${it.mediaItemId}`}
                title={it.title ?? t("common.untitled", "Untitled")}
                subtitle={it.year != null ? String(it.year) : undefined}
                imageUrl={posterSrc(it.mediaItemId, "poster", CARD_WIDTH_GRID * 2)}
                autoFocus={i === 0}
                onPress={() => navigate({ name: "media", mediaId: it.mediaItemId })}
              />
            ))}
          </div>
        </FocusRoot>
      )}
    </FocusRoot>
  );
}

function historyDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

/** What was watched, newest first; Remove is its own stop, a remote has no swipe. */
export function HistoryScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: [...getPlaybackServiceGetWatchHistoryQueryKey(), "pages"],
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) => playbackServiceGetWatchHistory(
      { "pagination.pageSize": HISTORY_PAGE_SIZE, ...(pageParam ? { "pagination.cursor": pageParam } : {}) },
      signal,
    ),
    getNextPageParam: (last) => last.pagination?.nextCursor || undefined,
  });
  const rows = (data?.pages ?? []).flatMap((p) => p.items ?? []);
  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() }); },
      onError: () => toast.error(t("auth.watchHistory.clearFailed", "Could not remove from history")),
    },
  });

  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <div className="screen deep-list">
      <h1 className="deep-title wide">{t("common.watchHistory.title")}</h1>
      {rows.length === 0 ? (
        <EmptyWithAction message={t("auth.watchHistory.noHistory")} actionLabel={t("media.library.libraries")} onAction={() => navigate({ name: "libraries" })} />
      ) : (
        <FocusRoot autoFocus className="stack-list rows wide">
          {rows.map((row, i) => {
            const pct = progressPercent(row.percentComplete);
            const label = watchHistoryLabel(row) ?? t("auth.watchHistory.unknownMedia", "Unknown media");
            return (
              <FocusRoot key={`${row.id ?? row.mediaItemId}-${i}`} className="history-wrap">
                <Pressable
                  className="tv-card history-row"
                  autoFocus={i === 0}
                  label={label}
                  onFocus={() => { if (hasNextPage && !isFetchingNextPage && i >= rows.length - 8) void fetchNextPage(); }}
                  onPress={() => { if (row.mediaItemId) navigate({ name: "media", mediaId: row.mediaItemId }); }}
                >
                  <span className="history-title">{label}{row.playCount && row.playCount > 1 ? ` (x${row.playCount})` : ""}</span>
                  <span className="history-meta">{historyDate(row.lastPlayedAt)}</span>
                  {row.durationMs ? <span className="history-meta">{formatDurationMs(row.positionMs) || "0min"} / {formatDurationMs(row.durationMs)}</span> : null}
                  <span className="history-progress">
                    <span className="history-bar"><i style={{ width: `${pct}%` }} /></span>
                    <span className="history-pct">{Math.round(pct)}%</span>
                  </span>
                </Pressable>
                <Pressable className={`tv-card history-remove ${unwatch.isPending ? "dim" : ""}`} label={t("auth.watchHistory.removeFromHistory", "Remove from history")} onPress={() => { if (!unwatch.isPending && row.mediaItemId) unwatch.mutate({ mediaItemId: row.mediaItemId }); }}>
                  {t("common.remove", "Remove")}
                </Pressable>
              </FocusRoot>
            );
          })}
          {isFetchingNextPage ? <Spinner /> : null}
        </FocusRoot>
      )}
    </div>
  );
}

interface PersonInfo {
  id?: string;
  name?: string;
  thumbUrl?: string;
  biography?: string;
  birthday?: string;
  deathday?: string;
  placeOfBirth?: string;
}

function longDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" });
}

/** A person's page: the portrait, the life dates, the biography, what they are in. */
export function PersonScreen({ personId }: { personId: string }) {
  const { t } = useTranslation();
  const [bioOpen, setBioOpen] = useState(false);
  const [shown, setShown] = useState(FILMOGRAPHY_PAGE);
  useEffect(() => { setShown(FILMOGRAPHY_PAGE); }, [personId]);
  const person = useQuery<PersonInfo>({
    queryKey: ["person", personId],
    queryFn: () => eveyFetch<PersonInfo>({ url: `/api/v1/people/${personId}`, method: "GET" }),
    enabled: !!personId,
  });
  const media = useLibraryServiceListMediaItems({ personId, "pagination.pageSize": 500 }, { query: { enabled: !!personId } });
  const items = (media.data?.items ?? []).filter((it) => it.id);
  const p = person.data;
  const name = p?.name ?? t("common.unknown");
  if (person.isLoading || media.isLoading || person.isError || media.isError) {
    return (
      <ScreenState
        loading={person.isLoading || media.isLoading}
        error={person.isError || media.isError}
        onRetry={() => { if (person.isError) void person.refetch(); if (media.isError) void media.refetch(); }}
      />
    );
  }
  const age = p?.birthday && !p.deathday ? Math.floor((Date.now() - new Date(p.birthday).getTime()) / 31557600000) : null;
  return (
    <FocusRoot autoFocus className="screen person-page">
      <div className="person-head">
        <span className="person-portrait">{p?.thumbUrl ? <img src={p.thumbUrl} alt="" /> : name[0]?.toUpperCase() ?? "?"}</span>
        <div className="person-info">
          <h1 className="person-title">{name}</h1>
          {p?.birthday ? (
            <p className="person-dates">
              {longDate(p.birthday)}
              {p.deathday ? ` - ${longDate(p.deathday)}` : ` (${age} ${t("media.person.yearsOld")})`}
            </p>
          ) : null}
          {p?.placeOfBirth ? <p className="person-place">{p.placeOfBirth}</p> : null}
        </div>
      </div>
      {p?.biography ? (
        <FocusRoot>
          <Pressable className="tv-card person-bio" label={bioOpen ? t("common.showLess") : t("common.readMore")} onPress={() => setBioOpen((o) => !o)}>
            <span className={`person-bio-text ${bioOpen ? "open" : ""}`}>{p.biography}</span>
            <span className="person-bio-more">{bioOpen ? t("common.showLess") : t("common.readMore")}</span>
          </Pressable>
        </FocusRoot>
      ) : null}
      {items.length > 0 ? (
        <>
          <h2 className="section-title person-section">{t("media.person.filmography")} ({items.length})</h2>
          <FocusRoot grid>
            <div className="grid person-grid">
              {items.slice(0, shown).map((it, i) => (
                <Pressable key={it.id} focusKey={`person:${it.id}`} className="tv-card person-film" autoFocus={i === 0} label={it.title} onPress={() => navigate({ name: "media", mediaId: it.id! })}>
                  <span className="person-film-poster"><img src={posterSrc(it.id, "poster", 420)} alt="" loading="lazy" /></span>
                  <span className="person-film-title">{it.title}</span>
                  {it.year ? <span className="person-film-year">{it.year}</span> : null}
                </Pressable>
              ))}
              {items.length > shown ? (
                <Pressable className="tv-card person-more" onPress={() => setShown((n) => n + FILMOGRAPHY_PAGE)}>
                  {t("media.detail.showMoreEpisodes", "Show more")}
                </Pressable>
              ) : null}
            </div>
          </FocusRoot>
        </>
      ) : (
        <p className="person-empty">{t("media.person.noFilmography")}</p>
      )}
    </FocusRoot>
  );
}

function relativeTime(t: (k: string, o?: Record<string, unknown>) => string, iso: string | undefined): string {
  if (!iso) return "";
  const mins = Math.floor((Date.now() - new Date(iso).getTime()) / 60000);
  if (mins < 1) return t("common.notifications.justNow");
  if (mins < 60) return t("common.notifications.minutesAgo", { count: mins });
  const hours = Math.floor(mins / 60);
  if (hours < 24) return t("common.notifications.hoursAgo", { count: hours });
  const days = Math.floor(hours / 24);
  if (days < 7) return t("common.notifications.daysAgo", { count: days });
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

/** What the server has to say, with the read state the other clients keep. */
export function NotificationsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch } = useNotificationServiceListNotifications();
  const list = data?.notifications ?? [];
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };
  const markRead = useNotificationServiceMarkRead({
    mutation: { onSuccess: invalidate, onError: () => toast.error(t("common.notifications.markReadFailed", "Could not mark as read")) },
  });
  const markAllRead = useNotificationServiceMarkAllRead({
    mutation: { onSuccess: invalidate, onError: () => toast.error(t("common.notifications.markAllReadFailed", "Could not mark all as read")) },
  });
  const hasUnread = list.some((n) => !n.readAt);

  const open = (n: V1Notification) => {
    if (!n.readAt && n.id) markRead.mutate({ id: n.id });
    if (n.mediaItemId) navigate({ name: "media", mediaId: n.mediaItemId });
  };

  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  return (
    <FocusRoot autoFocus className="screen deep-list">
      <FocusRoot className="deep-head">
        <h1 className="deep-title-inline">{t("common.notifications.title")}</h1>
        {hasUnread ? <Pressable className="tv-card mark-all" onPress={() => markAllRead.mutate({ data: {} })}>{t("common.notifications.markAllRead")}</Pressable> : null}
      </FocusRoot>
      {list.length === 0 ? (
        <div className="center-fill pad-top"><p className="muted big">{t("common.noNotifications")}</p></div>
      ) : (
        <FocusRoot className="stack-list rows wide">
          {list.map((n, i) => {
            const unread = !n.readAt;
            const title = notificationLabel(t, n as NotificationLabelInput);
            return (
              <Pressable key={n.id ?? i} className={`tv-card notif-row ${unread ? "unread" : ""}`} autoFocus={i === 0} label={title} onPress={() => open(n)}>
                <span className="notif-icon">●</span>
                <span className="notif-text">
                  <span className="notif-title">{title}</span>
                  {n.message ? <span className="notif-message">{n.message}</span> : null}
                  <span className="notif-time">{relativeTime(t, n.createdAt)}</span>
                </span>
                {unread ? <span className="notif-dot" /> : null}
              </Pressable>
            );
          })}
        </FocusRoot>
      )}
    </FocusRoot>
  );
}
