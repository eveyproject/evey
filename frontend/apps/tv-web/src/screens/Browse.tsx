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
  mergeGenres,
  progressPercent,
  genreTranslationKey,
  useCollection,
  useCollections,
  useFavoritesList,
  usePlaylistDetail,
  usePlaylistList,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc } from "../lib/media";

const HISTORY_PAGE_SIZE = 40;
const FILMOGRAPHY_PAGE = 24;

function Screen({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="screen">
      <h1 className="title">{title}</h1>
      {children}
    </div>
  );
}

/** Every library, with the shortcuts to the cross-library views. */
export function LibrariesScreen() {
  const { t } = useTranslation();
  const { data, isLoading } = useLibraryServiceListLibraries();
  const libs = (data?.libraries ?? []).filter((l) => l.id);
  return (
    <Screen title={t("media.library.libraries")}>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus className="chips">
        <Button onPress={() => navigate({ name: "collections" })}>{t("common.collections")}</Button>
        <Button onPress={() => navigate({ name: "playlists" })}>{t("common.playlists")}</Button>
        <Button onPress={() => navigate({ name: "genres" })}>{t("common.nav.genres")}</Button>
        <Button onPress={() => navigate({ name: "favorites" })}>{t("media.favorites.title")}</Button>
        <Button onPress={() => navigate({ name: "history" })}>{t("common.watchHistory.title")}</Button>
        <Button onPress={() => navigate({ name: "photos" })}>{t("media.photos.title")}</Button>
      </FocusRoot>
      <FocusRoot>
        <div className="grid">
          {libs.map((l) => (
            <Card
              key={l.id}
              title={l.name ?? ""}
              onPress={() => (l.type === "LIBRARY_TYPE_MUSIC"
                ? navigate({ name: "music", libraryId: l.id })
                : l.type === "LIBRARY_TYPE_PHOTOS"
                  ? navigate({ name: "photos", libraryId: l.id })
                  : navigate({ name: "library", libraryId: l.id! }))}
            />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

export function CollectionsScreen() {
  const { t } = useTranslation();
  const { data, isLoading } = useCollections();
  const list = (data?.collections ?? []).filter((c) => c.id);
  return (
    <Screen title={t("common.collections")}>
      {isLoading ? <Spinner /> : null}
      {!isLoading && list.length === 0 ? <p className="muted">{t("common.noCollections")}</p> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {list.map((c, i) => (
            <Card key={c.id} title={`${c.name}${c.itemCount ? ` (${c.itemCount})` : ""}`} imageUrl={c.posterUrl} autoFocus={i === 0} onPress={() => navigate({ name: "collection", collectionId: c.id })} />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

export function CollectionScreen({ collectionId }: { collectionId: string }) {
  const { t } = useTranslation();
  const { data, isLoading, hasNextPage, fetchNextPage, isFetchingNextPage } = useCollection(collectionId);
  const items = data?.items ?? [];
  const name = data?.collection?.name ?? "";
  return (
    <Screen title={name || t("common.collections")}>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {items.map((it, i) => (
            <Card
              key={it.id}
              title={it.title ?? ""}
              imageUrl={posterSrc(it.id)}
              autoFocus={i === 0}
              onFocus={() => { if (hasNextPage && !isFetchingNextPage && i >= items.length - 8) void fetchNextPage(); }}
              onPress={() => navigate({ name: "media", mediaId: it.id })}
            />
          ))}
        </div>
        {isFetchingNextPage ? <Spinner /> : null}
      </FocusRoot>
    </Screen>
  );
}

export function PlaylistsScreen() {
  const { t } = useTranslation();
  const { data, isLoading } = usePlaylistList();
  const list = data ?? [];
  return (
    <Screen title={t("common.playlists")}>
      {isLoading ? <Spinner /> : null}
      {!isLoading && list.length === 0 ? <p className="muted">{t("common.noPlaylists")}</p> : null}
      <FocusRoot autoFocus>
        <div className="track-list">
          {list.map((p, i) => (
            <Button key={p.id} autoFocus={i === 0} onPress={() => navigate({ name: "playlist", playlistId: p.id })}>
              <span className="track-title">{p.name}</span>
              <span className="muted">{p.type === "smart" ? t("media.collections.smart") : ""}</span>
            </Button>
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

export function PlaylistScreen({ playlistId }: { playlistId: string }) {
  const { t } = useTranslation();
  const { data, isLoading } = usePlaylistDetail(playlistId);
  const items = data?.items ?? [];
  return (
    <Screen title={data?.name ?? t("common.playlists")}>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {items.map((it, i) => (
            <Card key={it.media_item_id} title={it.title ?? ""} imageUrl={posterSrc(it.media_item_id)} autoFocus={i === 0} onPress={() => navigate({ name: "media", mediaId: it.media_item_id })} />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

/** Every browsable library's genres, merged and deduplicated. */
export function GenresScreen() {
  const { t } = useTranslation();
  const { data: libsData } = useLibraryServiceListLibraries();
  const libs = (libsData?.libraries ?? []).filter((l) => l.id && (l.type === "LIBRARY_TYPE_MOVIES" || l.type === "LIBRARY_TYPE_SHOWS"));
  const queries = useQueries({ queries: libs.map((l) => getLibraryServiceListGenresQueryOptions(l.id!)) });
  const genres = useMemo(
    () => mergeGenres(queries.map((q) => (q.data as { genres?: string[] } | undefined)?.genres)),
    // The query objects are new on every render; their data is what matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queries.map((q) => (q.data as { genres?: string[] } | undefined)?.genres?.join("|")).join("~")],
  );
  return (
    <Screen title={t("common.nav.genres")}>
      <FocusRoot autoFocus>
        <div className="grid">
          {genres.map((g, i) => (
            <Card key={g} title={t(genreTranslationKey(g), g)} autoFocus={i === 0} onPress={() => navigate({ name: "genre", genre: g })} />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

export function GenreScreen({ genre }: { genre: string }) {
  const { t } = useTranslation();
  const { data, isLoading } = useLibraryServiceSearchMedia({ genre, limit: 120 });
  const items = (data?.items ?? []).filter((it) => it.id);
  return (
    <Screen title={t(genreTranslationKey(genre), genre)}>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {items.map((it, i) => (
            <Card key={it.id} title={it.title ?? ""} imageUrl={posterSrc(it.id)} autoFocus={i === 0} onPress={() => navigate({ name: "media", mediaId: it.id! })} />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

export function FavoritesScreen() {
  const { t } = useTranslation();
  const { data, isLoading } = useFavoritesList();
  const items = data ?? [];
  return (
    <Screen title={t("media.favorites.title")}>
      {isLoading ? <Spinner /> : null}
      {!isLoading && items.length === 0 ? <p className="muted">{t("media.favorites.empty")}</p> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {items.map((it, i) => (
            <Card key={it.mediaItemId} title={it.title ?? ""} imageUrl={posterSrc(it.mediaItemId)} autoFocus={i === 0} onPress={() => navigate({ name: "media", mediaId: it.mediaItemId })} />
          ))}
        </div>
      </FocusRoot>
    </Screen>
  );
}

function historyDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

/** What was watched, newest first, with the same unwatch action the television has. */
export function HistoryScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const markUnwatched = usePlaybackServiceMarkAsUnwatched();
  const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: [...getPlaybackServiceGetWatchHistoryQueryKey(), "pages"],
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) => playbackServiceGetWatchHistory(
      { "pagination.pageSize": HISTORY_PAGE_SIZE, ...(pageParam ? { "pagination.cursor": pageParam } : {}) },
      signal,
    ),
    getNextPageParam: (last) => last.pagination?.nextCursor || undefined,
  });
  const rows = (data?.pages ?? []).flatMap((p) => p.items ?? []);

  const unwatch = async (mediaItemId: string) => {
    try {
      await markUnwatched.mutateAsync({ mediaItemId });
      await qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
    } catch { /* the row stays as it was */ }
  };

  return (
    <Screen title={t("common.watchHistory.title")}>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus>
        <div className="track-list">
          {rows.map((row, i) => (
            <div className="feed-row" key={`${row.mediaItemId}-${i}`}>
              <Button
                autoFocus={i === 0}
                onFocus={() => { if (hasNextPage && !isFetchingNextPage && i >= rows.length - 8) void fetchNextPage(); }}
                onPress={() => { if (row.mediaItemId) navigate({ name: "media", mediaId: row.mediaItemId }); }}
              >
                <span className="track-title">{watchHistoryLabel(row) ?? ""}</span>
                <span className="muted">
                  {historyDate(row.lastPlayedAt)} · {progressPercent(row.percentComplete)}% · {formatDurationMs(row.positionMs) || "0min"} / {formatDurationMs(row.durationMs)}
                </span>
              </Button>
              <Button onPress={() => row.mediaItemId && void unwatch(row.mediaItemId)}>{t("media.library.markUnwatched")}</Button>
            </div>
          ))}
        </div>
        {isFetchingNextPage ? <Spinner /> : null}
      </FocusRoot>
    </Screen>
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

/** A person's page: the picture, the life dates, and what they are in. */
export function PersonScreen({ personId }: { personId: string }) {
  const { t } = useTranslation();
  const { data: person } = useQuery<PersonInfo>({
    queryKey: ["person", personId],
    queryFn: () => eveyFetch<PersonInfo>({ url: `/api/v1/people/${personId}`, method: "GET" }),
    enabled: !!personId,
  });
  // The fetch stays whole so the count in the heading is honest; only the
  // render pages, as on the television - a prolific actor otherwise mounts
  // hundreds of posters before first paint.
  const [shown, setShown] = useState(FILMOGRAPHY_PAGE);
  useEffect(() => { setShown(FILMOGRAPHY_PAGE); }, [personId]);
  const { data, isLoading } = useLibraryServiceListMediaItems(
    { personId, "pagination.pageSize": 500 },
    { query: { enabled: !!personId } },
  );
  const items = (data?.items ?? []).filter((it) => it.id);
  const dates = [person?.birthday, person?.deathday].filter(Boolean).join(" - ");

  return (
    <div className="screen">
      <div className="hero">
        {person?.thumbUrl ? <img src={person.thumbUrl} alt="" /> : <div className="hero-placeholder">☺</div>}
        <div>
          <h1 className="title">{person?.name ?? ""}</h1>
          {dates ? <p className="muted">{dates}</p> : null}
          {person?.placeOfBirth ? <p className="muted">{person.placeOfBirth}</p> : null}
          {person?.biography ? <p className="overview">{person.biography}</p> : null}
        </div>
      </div>
      <h2 className="row-title">{t("media.person.titleCount", { count: items.length })}</h2>
      {isLoading ? <Spinner /> : null}
      <FocusRoot autoFocus>
        <div className="grid">
          {items.slice(0, shown).map((it, i) => (
            <Card
              key={it.id}
              title={it.title ?? ""}
              imageUrl={posterSrc(it.id)}
              autoFocus={i === 0}
              onFocus={() => { if (i >= shown - 6) setShown((n) => Math.min(items.length, n + FILMOGRAPHY_PAGE)); }}
              onPress={() => navigate({ name: "media", mediaId: it.id! })}
            />
          ))}
        </div>
      </FocusRoot>
    </div>
  );
}

/** What the server has to say, with the read state the other clients keep. */
export function NotificationsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useNotificationServiceListNotifications();
  const list = data?.notifications ?? [];
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };
  const markRead = useNotificationServiceMarkRead({ mutation: { onSuccess: invalidate } });
  const markAllRead = useNotificationServiceMarkAllRead({ mutation: { onSuccess: invalidate } });
  const hasUnread = list.some((n) => !n.readAt);

  const open = (n: V1Notification) => {
    if (!n.readAt && n.id) markRead.mutate({ id: n.id });
    if (n.mediaItemId) navigate({ name: "media", mediaId: n.mediaItemId });
  };

  return (
    <div className="screen">
      <FocusRoot autoFocus className="header">
        <h1 className="title">{t("common.notifications.title")}</h1>
        {hasUnread ? <Button onPress={() => markAllRead.mutate({ data: {} })}>{t("common.notifications.markAllRead")}</Button> : null}
      </FocusRoot>
      {isLoading ? <Spinner /> : null}
      {!isLoading && list.length === 0 ? <p className="muted">{t("common.notifications.noNotifications")}</p> : null}
      <FocusRoot>
        <div className="track-list">
          {list.map((n, i) => (
            <Button key={n.id ?? i} autoFocus={i === 0} variant={n.readAt ? "default" : "primary"} onPress={() => open(n)}>
              <span className="track-title">{notificationLabel(t, n as NotificationLabelInput)}</span>
              <span className="muted">{n.createdAt ? new Date(n.createdAt).toLocaleString() : ""}</span>
            </Button>
          ))}
        </div>
      </FocusRoot>
    </div>
  );
}
