import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { formatDurationMs, formatTimeMs, itemLabel, seasonLabel, trackPickParams, type TrackPick } from "@evey/api";
import { TrackPickRow } from "../../../src/components/TrackPickRow";
import { View, Text, ScrollView, Pressable, FlatList, StyleSheet, Linking } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useLocalSearchParams, router } from "expo-router";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useFeedServiceSimilarItems,
  useRatingServiceGetMyThumbRating,
  ratingServicePutThumbRating,
  getRatingServiceGetMyThumbRatingQueryKey,
  WATCH_HISTORY_OVERLAY_LIMIT,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson, V1Trailer } from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useAuthStore, useDemoStore, toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { RESUME_PROGRESS_QUERY, buildWatchProgressMaps, invalidateWatchedState, useSeriesPlayTarget } from "@evey/client";
import { useRefetchOnReturn } from "@evey/client/native/useRefetchOnReturn";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import Svg, { Defs, LinearGradient, Rect, Stop } from "react-native-svg";
import { artworkUrl } from "../../../src/lib/artwork";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { CARD_WIDTH_FEATURED, POSTER_RATIO, CARD_WIDTH_GRID } from "../../../src/lib/tvLayout";
import { resolvePlayTarget, sortByIndexNumber } from "../../../src/lib/playTarget";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { ScreenState } from "../../../src/components/ScreenState";
import { SagaSection } from "../../../src/components/SagaSection";
import { trailerUrl, sortTrailers } from "../../../src/lib/trailerUrl";
import { personTypeLabelKey } from "../../../src/lib/personRole";

// Sized so the action row lands ON the screen. At 420 the backdrop pushed
// "Reprendre" to y=1027 of a 1080 panel: the viewer arrived on a media page
// whose focused primary button was cut in half, before overscan even had its
// say (measured on the living-room box, 2026-09-19).
const HERO_HEIGHT = 300;
// Android TV renders at 1080p whatever the panel: a hero backdrop wider than
// the screen is decoded then downscaled for nothing (audit perf 2026-08-25).
const HERO_BACKDROP_WIDTH = 1920;
const POSTER_WIDTH = CARD_WIDTH_FEATURED;
// 16:9 still next to the episode text. Wide enough to read at three metres,
// narrow enough that the title and synopsis keep the room. Served as
// "backdrop" - that is the artwork type an episode still lives under, same as
// the web episode rows.
const EPISODE_THUMB_WIDTH = 240;
// Client-side page size for the episode list - the fetch stays whole (500) so
// counts and next-episode math keep seeing every sibling; only the RENDER pages.
const EPISODE_PAGE_SIZE = 20;
const POSTER_HEIGHT = POSTER_WIDTH * POSTER_RATIO;

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

// Mirrors web's classifyError (routes/_auth/media/$mediaId.lazy.tsx): a 404
// is a dead end no retry can fix (the gRPC-gateway maps NOT_FOUND to it), so
// it gets its own branch instead of the generic offline/server-error one a
// Retry button implies.
function isNotFoundError(error: unknown): boolean {
  if (typeof error !== "object" || error === null) return false;
  return (error as { httpStatus?: number }).httpStatus === 404;
}

// Community rating as five stars, the same 0-10 scale web and mobile render.
function StarRating({ rating, color }: { rating: number; color: string }) {
  const full = Math.round(rating / 2);
  return (
    <Text style={{ color, fontSize: 18 }} accessible={false}>
      {"★".repeat(full)}{"☆".repeat(Math.max(0, 5 - full))}
    </Text>
  );
}

// Mirrors web's useRating and mobile's inlined pair: like=1, dislike=-1,
// 0=cleared. The optimistic write is what keeps the button from feeling dead
// on a remote, where the round trip is the only feedback.
function useMyThumbRating(mediaId: string | undefined) {
  return useRatingServiceGetMyThumbRating(mediaId ?? "", {
    query: { enabled: !!mediaId, select: (d) => d.value ?? 0, staleTime: 60_000 },
  });
}

function useSetThumbRating(mediaId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: number) => ratingServicePutThumbRating(mediaId ?? "", { value }),
    onMutate: async (value: number) => {
      if (!mediaId) return { prev: undefined };
      const key = getRatingServiceGetMyThumbRatingQueryKey(mediaId);
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<{ value?: number }>(key);
      qc.setQueryData(key, { value });
      return { prev };
    },
    onError: (_err: Error, _v: number, ctx?: { prev?: { value?: number } }) => {
      if (mediaId && ctx?.prev !== undefined) {
        qc.setQueryData(getRatingServiceGetMyThumbRatingQueryKey(mediaId), ctx.prev);
      }
    },
    onSettled: () => {
      if (mediaId) void qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) });
    },
  });
}

// A horizontal shelf of poster cards, used by "similar titles". Kept local:
// the home screen's row is tuned for its own card size and focus rules.
function PosterShelf({
  title,
  items,
  serverUrl,
  authHeaders,
}: {
  title: string;
  items: { id: string; title?: string }[];
  serverUrl: string;
  authHeaders?: { Authorization: string };
}) {
  const theme = useTheme();
  if (items.length === 0) return null;
  const w = CARD_WIDTH_GRID;
  const h = w * POSTER_RATIO;
  return (
    <View style={{ marginTop: 32 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>{title}</Text>
      <FlatList
        horizontal
        data={items}
        keyExtractor={(it) => it.id}
        showsHorizontalScrollIndicator={false}
        ItemSeparatorComponent={() => <View style={{ width: 16 }} />}
        renderItem={({ item }) => (
          <FocusableCard
            onPress={() => router.push(`/media/${item.id}`)}
            style={{ width: w }}
            accessibilityLabel={item.title}
          >
            <Image
              source={{ uri: artworkUrl(serverUrl, item.id, "poster", w * 2), headers: authHeaders }}
              style={{ width: w, height: h, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }}
              contentFit="cover"
              transition={200}
            />
            <Text style={{ color: theme.colors.text, fontSize: 15, marginTop: 8 }} numberOfLines={1}>
              {item.title}
            </Text>
          </FocusableCard>
        )}
      />
    </View>
  );
}

// "More like this": the server's list or no shelf. A same-genre fallback
// filled it with titles sharing one catch-all genre (2026-10-01).
function SimilarShelf({
  item,
  serverUrl,
  authHeaders,
}: {
  item: V1MediaItem;
  serverUrl: string;
  authHeaders?: { Authorization: string };
}) {
  const { t } = useTranslation();
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const cards = useMemo(
    () =>
      (similarData?.items ?? [])
        .filter((fi) => !!fi.mediaItemId)
        .map((fi) => ({ id: fi.mediaItemId as string, title: fi.title })),
    [similarData],
  );

  return (
    <PosterShelf
      title={t("media.detail.similarTitles")}
      items={cards}
      serverUrl={serverUrl}
      authHeaders={authHeaders}
    />
  );
}

export default function MediaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  // The FIRST season, not a user selection: since the series page became a
  // grid of season posters there is nothing to select here. It is still needed
  // to answer "what does Play start?" on a series - the first episode of the
  // first season.
  const [firstSeasonId, setFirstSeasonId] = useState<string | null>(null);
  const [focusedBtn, setFocusedBtn] = useState<string | null>(null);

  const { data: media, isLoading, error, refetch } = useLibraryServiceGetMediaItem(id ?? "");
  // Re-arm the play button's preferred focus AFTER the media loads: applied
  // statically at mount it intermittently lands while the view is not
  // attachable yet, native focus is lost and every select goes nowhere.
  const [armPlayFocus, setArmPlayFocus] = useState(false);
  const playBtnRef = useRef<View>(null);
  const { data: watchProgress, refetch: refetchProgress } = usePlaybackServiceGetWatchProgress(id ?? "", undefined, {
    query: RESUME_PROGRESS_QUERY,
  });
  // The player stacks on top of this screen and autoplay moves on from the
  // episode it opened with: back here, the Resume label must be re-read.
  useRefetchOnReturn(refetchProgress);
  const profileId = useAuthStore((s) => s.profileId);
  const demo = useDemoStore((s) => s.demoMode);
  const qc = useQueryClient();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);

  const [playlistSheetOpen, setPlaylistSheetOpen] = useState(false);
  // A long season used to mount its full episode list at once (pageSize 500):
  // one layout burst on the most visited pre-playback screen. Same paging as
  // mobile: slice + a focusable "show more".
  const [visibleEpisodeCount, setVisibleEpisodeCount] = useState(EPISODE_PAGE_SIZE);
  // expo-router reuses this screen across ids: reset the paging when the
  // viewer moves to another season, or the new one opens pre-expanded.
  useEffect(() => {
    setVisibleEpisodeCount(EPISODE_PAGE_SIZE);
  }, [id]);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayTarget, setPendingPlayTarget] = useState<{ id: string; restart: boolean } | null>(null);
  // Audio/subtitle chosen before playback; empty = the player decides.
  const [trackPick, setTrackPick] = useState<TrackPick>({});

  const invalidateWatched = useCallback(() => {
    if (id) void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${id}/progress`] });
    // History, Up Next, Continue Watching and the home feeds, from the one list.
    invalidateWatchedState(qc);
  }, [id, qc]);

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

  const isSeries = isSeriesType(media?.type);
  const isSeason = isSeasonType(media?.type);

  const people = media?.people ?? [];
  const actors = people.filter((p: V1MediaPerson) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");
  // Web groups director/writer/producer into one "crew" shelf; same grouping
  // here so the same person appears in the same place on both.
  const crew = people.filter((p: V1MediaPerson) =>
    p.type === "PERSON_TYPE_DIRECTOR" || p.type === "PERSON_TYPE_WRITER" || p.type === "PERSON_TYPE_PRODUCER");
  const { data: myRating, isError: ratingUnknown } = useMyThumbRating(id);
  const setThumbRating = useSetThumbRating(id);
  // A failed read leaves the thumbs on NEUTRAL, which is a statement ("no
  // opinion"), not an absence - and pressing OK from there writes it over an
  // opinion the viewer was never shown. Same guard on web and mobile.
  const thumbsBusy = setThumbRating.isPending || ratingUnknown;

  const { data: childrenData } = useLibraryServiceListMediaItems(
    // Serves both the season list of a series and the episode list of a
    // season, so it takes the episode ceiling rather than the season one.
    { parentId: id, "pagination.pageSize": 500 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = useMemo(() => childrenData?.items ?? [], [childrenData?.items]);

  // The screen is reused from one title to the next (a series opened from the
  // row of another): the first season belongs to the id on screen, never to
  // the one before it.
  useEffect(() => {
    setFirstSeasonId(null);
  }, [id]);

  useEffect(() => {
    if (isSeries && children.length > 0 && !firstSeasonId) {
      setFirstSeasonId(children[0]?.id ?? null);
    }
  }, [isSeries, children, firstSeasonId]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: firstSeasonId ?? "", "pagination.pageSize": 500 },
    { query: { enabled: !!(firstSeasonId && isSeries) } },
  );

  // There is no server-side sort key for the episode index (SortColumnMap knows
  // title, added_at, year, premiere_date, community_rating), so ListMediaItems
  // answers in sort_title order and the ordering has to be restored here. This
  // screen used to render whatever order arrived: episodes listed
  // alphabetically instead of 1, 2, 3, and - because resolvePlayTarget takes
  // the first id in the array - the Play button started on whichever episode
  // sorted first by TITLE. Web and mobile already sorted; only the TV did not.
  const episodes = useMemo(
    () => sortByIndexNumber(isSeason ? children : (episodesData?.items ?? [])),
    [isSeason, children, episodesData?.items],
  );

  // Which episodes of the season are seen, from one history fetch (as on the
  // phone), plus the marks made from the rows before that history refetches.
  const { data: episodeHistory } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000, enabled: isSeason } },
  );
  const [episodeMarks, setEpisodeMarks] = useState<ReadonlyMap<string, boolean>>(new Map());
  // Same map back when there is nothing to drop, so a re-render is not one.
  useEffect(() => setEpisodeMarks((m) => (m.size === 0 ? m : new Map())), [episodeHistory]);
  const episodeWatched = useMemo(() => {
    const { watchedMap } = buildWatchProgressMaps(episodeHistory?.items ?? []);
    for (const [epId, played] of episodeMarks) watchedMap.set(epId, played);
    return watchedMap;
  }, [episodeHistory, episodeMarks]);
  const onEpisodeMarked = useCallback((epId: string, played: boolean) => {
    setEpisodeMarks((m) => new Map(m).set(epId, played));
    invalidateWatchedState(qc, epId);
  }, [qc]);
  const markEpisodeWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: (_d, vars) => onEpisodeMarked(vars.mediaItemId, true),
      onError: () => toast.error(t("media.detail.markWatchedFailed", "Could not mark as watched")),
    },
  });
  const markEpisodeUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: (_d, vars) => onEpisodeMarked(vars.mediaItemId, false),
      onError: () => toast.error(t("media.detail.markUnwatchedFailed", "Could not mark as unwatched")),
    },
  });
  const episodeMarkPending = markEpisodeWatched.isPending || markEpisodeUnwatched.isPending;
  const toggleEpisodeWatched = (ep: V1MediaItem) => {
    if (!ep.id || !profileId) return;
    const vars = { mediaItemId: ep.id, params: { profileId } };
    if (episodeWatched.get(ep.id)) markEpisodeUnwatched.mutate(vars);
    else markEpisodeWatched.mutate(vars);
  };
  const seasons = useMemo(() => (isSeries ? sortByIndexNumber(children) : []), [isSeries, children]);

  // A series plays where the profile is in it (the server's answer); the
  // first episode stays the fallback until that answer is in.
  const seriesTarget = useSeriesPlayTarget(isSeries ? id : undefined);
  const seriesStarted = isSeries && seriesTarget.started;
  const playTarget = useMemo(
    () => (isSeries && seriesTarget.episode?.id) || resolvePlayTarget({ id, isSeries, isSeason, episodeIds: episodes.map((e) => e.id) }),
    [id, isSeries, isSeason, episodes, seriesTarget.episode?.id],
  );
  const getPlayTarget = useCallback((): string | null => playTarget, [playTarget]);

  // Re-arm the play button's preferred focus AFTER the media loads: applied
  // statically at mount it intermittently lands while the view is not
  // attachable yet, native focus is lost and every select goes nowhere. On a
  // series, `playTarget` resolves later than `media` (it depends on a second
  // request for the episode list), so the retry must also fire once
  // `playTarget` itself changes - otherwise Play stays disabled through both
  // timers and the remote never gets a focused target.
  useEffect(() => {
    if (!media) return;
    // Imperative on purpose: the declarative hasTVPreferredFocus races the
    // mount and intermittently leaves NO focused native view, at which point
    // Android stops delivering key events - the page goes dead to the remote.
    const timers = [50, 400].map((ms) =>
      setTimeout(() => {
        setArmPlayFocus(true);
        (playBtnRef.current as unknown as { requestTVFocus?: () => void })?.requestTVFocus?.();
      }, ms),
    );
    return () => timers.forEach(clearTimeout);
  }, [media, playTarget]);

  // `restart` travels all the way to the player, PIN prompt included: the
  // choice must survive the detour, or entering a PIN would silently resume
  // the thing the viewer asked to watch from the start.
  const openPlayer = useCallback((target: string, restart: boolean) => {
    // The track pick belongs to THIS file's streams: an episode launched from
    // a series page keeps the player's own resolution.
    const query = new URLSearchParams(trackPickParams(target === media?.id ? trackPick : undefined));
    if (restart) query.set("restart", "1");
    const q = query.toString();
    router.push(q ? `/(auth)/player/${target}?${q}` : `/(auth)/player/${target}`);
  }, [media?.id, trackPick]);

  const handlePlay = useCallback((restart = false) => {
    const target = getPlayTarget();
    if (!target) return;
    if (needsPin()) {
      setPendingPlayTarget({ id: target, restart });
      setShowPinModal(true);
      return;
    }
    openPlayer(target, restart);
  }, [getPlayTarget, needsPin, openPlayer]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayTarget) {
      openPlayer(pendingPlayTarget.id, pendingPlayTarget.restart);
      setPendingPlayTarget(null);
    }
  }, [pendingPlayTarget, openPlayer]);

  // A failed request used to land here with isLoading false and media
  // undefined, so the spinner never stopped - on a television that is
  // indistinguishable from a frozen app.
  if (isLoading || !media) {
    // A 404 is not "the network dropped" - no amount of retrying brings back
    // an item that no longer exists, so offer a way out instead of a button
    // that can only fail again.
    if (!isLoading && isNotFoundError(error)) {
      return (
        <View style={[styles.center, { backgroundColor: theme.colors.bg, gap: 12, padding: 48 }]}>
          <Text style={[styles.notFoundTitle, { color: theme.colors.text }]}>
            {t("media.detail.notFound")}
          </Text>
          <FocusableCard
            onPress={() => router.replace("/(auth)/(tabs)")}
            hasTVPreferredFocus={isNotFoundError(error)}
            accessibilityLabel={t("common.errorBoundary.goHome", "Go Home")}
            style={[styles.notFoundButton, { backgroundColor: theme.colors.accent }]}
          >
            <Text style={[styles.notFoundButtonLabel, { color: theme.colors.onAccent }]}>
              {t("common.errorBoundary.goHome", "Go Home")}
            </Text>
          </FocusableCard>
        </View>
      );
    }
    return <ScreenState loading={isLoading} error={!isLoading} onRetry={() => void refetch()} />;
  }

  const meta = [media.year, formatDurationMs(media.durationMs), media.officialRating]
    .filter(Boolean)
    .join("  \u2022  ");

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  const primaryBlurhash = (media.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;
  const backdropBlurhash = (media.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.blurhash;

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingBottom: 48 }}>
      {/* Backdrop */}
      <View style={{ width: "100%", height: HERO_HEIGHT }}>
        {serverUrl && id ? (
          <Image
            source={{ uri: artworkUrl(serverUrl, id, "backdrop", HERO_BACKDROP_WIDTH, media), headers: authHeaders }}
            style={StyleSheet.absoluteFillObject}
            contentFit="cover"
            transition={300}
            placeholder={backdropBlurhash ? { blurhash: backdropBlurhash } : undefined}
            placeholderContentFit="cover"
          />
        ) : null}
        {/* Fade to bg so the poster/title stay legible over any backdrop; the
            stops trace the curve the old 16 flat bands sampled (0.05 to 0.95). */}
        <Svg style={StyleSheet.absoluteFillObject} width="100%" height="100%" pointerEvents="none">
          <Defs>
            <LinearGradient id="hero-fade" x1="0" y1="0" x2="0" y2="1">
              <Stop offset="0" stopColor={theme.colors.bg} stopOpacity={0.05} />
              <Stop offset="0.25" stopColor={theme.colors.bg} stopOpacity={0.18} />
              <Stop offset="0.5" stopColor={theme.colors.bg} stopOpacity={0.39} />
              <Stop offset="0.75" stopColor={theme.colors.bg} stopOpacity={0.65} />
              <Stop offset="1" stopColor={theme.colors.bg} stopOpacity={0.95} />
            </LinearGradient>
          </Defs>
          <Rect x="0" y="0" width="100%" height="100%" fill="url(#hero-fade)" />
        </Svg>
      </View>

      {/* Poster + title row, overlapping the backdrop */}
      <View style={{ paddingHorizontal: 48, marginTop: -POSTER_HEIGHT * 0.5, flexDirection: "row", gap: 32 }}>
        {serverUrl && id ? (
          <Image
            source={{ uri: artworkUrl(serverUrl, id, "poster", POSTER_WIDTH * 2, media), headers: authHeaders }}
            style={{
              width: POSTER_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.surface,
            }}
            contentFit="cover"
            transition={200}
            placeholder={primaryBlurhash ? { blurhash: primaryBlurhash } : undefined}
            placeholderContentFit="cover"
          />
        ) : null}
        <View style={{ flex: 1, justifyContent: "flex-end" }}>
          {/* Surtitle: series/season stamped by the server on an episode
              payload (both), so an episode reads "Series - Season 2" above
              its own title instead of a bare episode name with no context. */}
          {!isSeries && !isSeason && media.seriesTitle ? (
            <Text style={[styles.eyebrow, { color: theme.colors.textMuted }]} numberOfLines={1}>
              {media.seriesTitle}
            </Text>
          ) : null}
          <Text style={[styles.title, { color: theme.colors.text }]}>{itemLabel(media, t)}</Text>
          {media.tagline ? (
            <Text style={[styles.tagline, { color: theme.colors.textMuted }]}>{media.tagline}</Text>
          ) : null}
          {meta ? <Text style={[styles.meta, { color: theme.colors.textMuted }]}>{meta}</Text> : null}
          {media.genres && media.genres.length > 0 && (
            <Text style={[styles.genres, { color: theme.colors.textMuted }]}>{media.genres.join(", ")}</Text>
          )}
          {media.studios && media.studios.length > 0 && (
            <Text style={[styles.studios, { color: theme.colors.textMuted }]}>
              {t("media.detail.studio")}{media.studios.join(", ")}
            </Text>
          )}
          {media.communityRating != null && media.communityRating > 0 ? (
            <View
              style={styles.ratingLine}
              accessible={true}
              accessibilityLabel={t("media.detail.communityRatingA11y", "Rated {{rating}} out of 10", {
                rating: media.communityRating.toFixed(1),
              })}
            >
              <StarRating rating={media.communityRating} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>{media.communityRating.toFixed(1)}</Text>
            </View>
          ) : null}
        </View>
      </View>

      <View style={{ paddingHorizontal: 48 }}>
      {/* Action buttons */}
      <View style={styles.actions}>
        {/* One control, two halves. The wide half resumes, the narrow one starts
            over; the divider is what says they belong together rather than
            reading as two rival Play buttons. With no progress to resume there
            is nothing to choose, so the second half is simply absent and the
            button is whole again. */}
        <View style={styles.playGroup}>
          <Pressable
            ref={playBtnRef}
            onPress={() => handlePlay(false)}
            disabled={!playTarget}
            onFocus={() => setFocusedBtn("play")}
            onBlur={() => setFocusedBtn(null)}
            hasTVPreferredFocus={armPlayFocus}
            accessible={true}
            accessibilityLabel={
              hasProgress
                ? `${t("media.resume", "Resume")} ${formatTimeMs(Number(resumeMs))}`
                : seriesStarted ? t("media.resume", "Resume") : t("player.controls.play")
            }
            accessibilityRole="button"
            style={[
              styles.playButton,
              hasProgress && styles.playButtonLeft,
              {
                backgroundColor: theme.colors.accent,
                borderColor: focusedBtn === "play" ? "#fff" : "transparent",
              },
            ]}
          >
            <Text style={[styles.playText, { color: theme.colors.onAccent }]}>
              {hasProgress ? `${t("media.resume", "Resume")} ${formatTimeMs(Number(resumeMs))}` : seriesStarted ? t("media.resume", "Resume") : t("player.controls.play")}
            </Text>
          </Pressable>

          {hasProgress ? (
            <Pressable
              onPress={() => handlePlay(true)}
              onFocus={() => setFocusedBtn("restart")}
              onBlur={() => setFocusedBtn(null)}
              accessible={true}
              accessibilityLabel={t("media.playFromStart", "Play from the beginning")}
              accessibilityRole="button"
              style={[
                styles.playRestart,
                {
                  backgroundColor: theme.colors.accent,
                  borderColor: focusedBtn === "restart" ? "#fff" : "transparent",
                  borderLeftColor: theme.colors.onAccent + "55",
                },
              ]}
            >
              <Ionicons name="refresh" size={26} color={theme.colors.onAccent} />
            </Pressable>
          ) : null}
        </View>

        {/* On the demo the favourites, watched marks, playlists and ratings are
            the shared account's and the server refuses them: not offered, since
            a disabled button that still takes the focus is a trap on a remote. */}
        {demo ? null : (
          <>
        <Pressable
          onPress={() => toggleFavorite.mutate()}
          disabled={toggleFavorite.isPending}
          onFocus={() => setFocusedBtn("fav")}
          onBlur={() => setFocusedBtn(null)}
          accessible={true}
          accessibilityLabel={isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
          accessibilityRole="button"
          style={[
            styles.actionBtn,
            {
              backgroundColor: theme.colors.surface,
              borderColor: focusedBtn === "fav" ? theme.colors.accent : "transparent",
            },
          ]}
        >
          <Text style={[styles.actionText, { color: isFavorite ? theme.colors.accent : theme.colors.text }]}>
            {isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
          </Text>
        </Pressable>

        {/* No series/season guard: marking a whole season watched from the
            couch is a normal TV use, and the web equivalent
            (MediaDetailActions.tsx) has no such guard either. */}
        <Pressable
          onPress={() => {
            if (!profileId) return;
            if (watched) {
              markUnwatched.mutate({ mediaItemId: id!, params: { profileId } });
            } else {
              markWatched.mutate({ mediaItemId: id!, params: { profileId } });
            }
          }}
          disabled={markWatched.isPending || markUnwatched.isPending}
          onFocus={() => setFocusedBtn("watched")}
          onBlur={() => setFocusedBtn(null)}
          accessible={true}
          accessibilityLabel={watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
          accessibilityRole="button"
          style={[
            styles.actionBtn,
            {
              backgroundColor: theme.colors.surface,
              borderColor: focusedBtn === "watched" ? theme.colors.accent : "transparent",
            },
          ]}
        >
          <Text style={[styles.actionText, { color: watched ? theme.colors.accent : theme.colors.text }]}>
            {watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
          </Text>
        </Pressable>

        <Pressable
          onPress={() => setPlaylistSheetOpen(true)}
          onFocus={() => setFocusedBtn("playlist")}
          onBlur={() => setFocusedBtn(null)}
          accessible={true}
          accessibilityLabel={t("media.detail.addToPlaylist")}
          accessibilityRole="button"
          style={[
            styles.actionBtn,
            {
              backgroundColor: theme.colors.surface,
              borderColor: focusedBtn === "playlist" ? theme.colors.accent : "transparent",
            },
          ]}
        >
          <Text style={[styles.actionText, { color: theme.colors.text }]}>{t("media.detail.addToPlaylist")}</Text>
        </Pressable>

        {/* Thumb rating: same like/dislike/clear contract as web and mobile
            (1 / -1 / 0), so a rating given in the living room shows up in the
            browser. Two separate buttons rather than one toggle - the D-pad
            has no way to express "the other direction" on a single control. */}
        {/* No series/season guard here either, matching web's RatingButtons
            (MediaDetailActions.tsx:266). */}
            <Pressable
              onPress={() => setThumbRating.mutate(myRating === 1 ? 0 : 1)}
              disabled={thumbsBusy}
              onFocus={() => setFocusedBtn("like")}
              onBlur={() => setFocusedBtn(null)}
              accessible={true}
              accessibilityLabel={t("media.rating.like", "Like")}
              accessibilityRole="button"
              accessibilityState={{ selected: myRating === 1 }}
              style={[
                styles.actionBtn,
                {
                  backgroundColor: theme.colors.surface,
                  borderColor: focusedBtn === "like" ? theme.colors.accent : "transparent",
                  opacity: thumbsBusy ? 0.5 : 1,
                },
              ]}
            >
              <Text style={[styles.actionText, { color: myRating === 1 ? theme.colors.accent : theme.colors.text }]}>
                {t("media.rating.like", "Like")}
              </Text>
            </Pressable>
            <Pressable
              onPress={() => setThumbRating.mutate(myRating === -1 ? 0 : -1)}
              disabled={thumbsBusy}
              onFocus={() => setFocusedBtn("dislike")}
              onBlur={() => setFocusedBtn(null)}
              accessible={true}
              accessibilityLabel={t("media.rating.dislike", "Dislike")}
              accessibilityRole="button"
              accessibilityState={{ selected: myRating === -1 }}
              style={[
                styles.actionBtn,
                {
                  backgroundColor: theme.colors.surface,
                  borderColor: focusedBtn === "dislike" ? theme.colors.accent : "transparent",
                  opacity: thumbsBusy ? 0.5 : 1,
                },
              ]}
            >
              <Text style={[styles.actionText, { color: myRating === -1 ? theme.colors.accent : theme.colors.text }]}>
                {t("media.rating.dislike", "Dislike")}
              </Text>
            </Pressable>
          </>
        )}

        {/* Trailers hand off to whatever app the television has for the site.
            Only rendered when a link can actually be built, and a device with
            nothing registered gets a message instead of a silent no-op. */}
        {sortTrailers(media.trailers ?? [])
          .map((tr) => ({ tr, url: trailerUrl(tr) }))
          .filter((x): x is { tr: V1Trailer; url: string } => !!x.url)
          .slice(0, 1)
          .map(({ url }) => (
            <Pressable
              key={url}
              onPress={() => {
                void Linking.openURL(url).catch(() =>
                  toast.error(t("media.detail.trailerUnavailable", "No app on this device can open the trailer")),
                );
              }}
              onFocus={() => setFocusedBtn("trailer")}
              onBlur={() => setFocusedBtn(null)}
              accessible={true}
              accessibilityLabel={t("media.detail.trailer")}
              accessibilityRole="button"
              style={[
                styles.actionBtn,
                {
                  backgroundColor: theme.colors.surface,
                  borderColor: focusedBtn === "trailer" ? theme.colors.accent : "transparent",
                },
              ]}
            >
              <Text style={[styles.actionText, { color: theme.colors.text }]}>{t("media.detail.trailer")}</Text>
            </Pressable>
          ))}
      </View>

      {media.streams && media.id === playTarget ? (
        <TrackPickRow streams={media.streams} pick={trackPick} onChange={setTrackPick} />
      ) : null}

      {/* Overview */}
      {media.overview ? (
        <Text style={[styles.overview, { color: theme.colors.text }]}>{media.overview}</Text>
      ) : null}

      {/* Seasons, as poster cards - the web layout, chosen over the phone's
          chip row. A series page now shows the seasons and nothing else; you
          open one to get its episodes, exactly as on the web. */}
      {isSeries && seasons.length > 0 && (
        <View style={{ marginTop: 32 }}>
          <View style={styles.sectionHeaderRow}>
            <Text style={[styles.sectionTitle, { color: theme.colors.text, marginBottom: 0 }]}>
              {t("media.detail.seasons")}
            </Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>
              {t("media.detail.seasonCount", { count: seasons.length })}
            </Text>
          </View>
          <FlatList
            horizontal
            data={seasons}
            keyExtractor={(sn) => sn.id ?? ""}
            showsHorizontalScrollIndicator={false}
            ItemSeparatorComponent={() => <View style={{ width: 16 }} />}
            renderItem={({ item: season }) => (
              <FocusableCard
                onPress={() => router.push(`/(auth)/media/${season.id}`)}
                style={{ width: CARD_WIDTH_GRID }}
                accessibilityLabel={seasonLabel(season, t)}
              >
                <Image
                  source={{ uri: artworkUrl(serverUrl, season.id ?? "", "poster", CARD_WIDTH_GRID * 2, season), headers: authHeaders }}
                  style={{
                    width: CARD_WIDTH_GRID,
                    height: CARD_WIDTH_GRID * POSTER_RATIO,
                    borderRadius: theme.radii.sm,
                    backgroundColor: theme.colors.surface,
                  }}
                  contentFit="cover"
                  transition={200}
                />
                <Text style={{ color: theme.colors.text, fontSize: 17, marginTop: 8 }} numberOfLines={1}>
                  {seasonLabel(season, t)}
                </Text>
              </FocusableCard>
            )}
          />
        </View>
      )}

      {/* Episodes - only on a season page now, since a series page sends you
          to a season first. Each row carries its still: a list of numbered
          titles gives no idea what the episode is, and every other client
          shows one. */}
      {isSeason && episodes.length > 0 && (
        <View style={{ marginTop: 32 }}>
          <View style={styles.sectionHeaderRow}>
            <Text style={[styles.sectionTitle, { color: theme.colors.text, marginBottom: 0 }]}>
              {t("media.detail.episodes")}
            </Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>
              {t("media.detail.episodeCount", { count: episodes.length })}
            </Text>
          </View>
          {episodes.slice(0, visibleEpisodeCount).map((ep) => {
            const watched = ep.id ? episodeWatched.get(ep.id) === true : false;
            return (
            <View key={ep.id} style={{ flexDirection: "row", alignItems: "center" }}>
            <FocusableCard
              onPress={() => router.push(`/(auth)/media/${ep.id}`)}
              style={[styles.episodeRow, { flex: 1, borderBottomColor: theme.colors.border }]}
              accessibilityLabel={ep.indexNumber != null ? `${ep.indexNumber}. ${ep.title}` : ep.title}
            >
              <View>
              <Image
                source={{ uri: artworkUrl(serverUrl, ep.id ?? "", "backdrop", EPISODE_THUMB_WIDTH * 2, ep), headers: authHeaders }}
                style={{
                  width: EPISODE_THUMB_WIDTH,
                  height: EPISODE_THUMB_WIDTH * (9 / 16),
                  borderRadius: theme.radii.sm,
                  backgroundColor: theme.colors.surface,
                }}
                contentFit="cover"
                transition={200}
              />
              {watched ? (
                <View style={[styles.episodeWatchedBadge, { backgroundColor: theme.colors.success }]}>
                  <Ionicons name="checkmark" size={16} color={theme.colors.onSuccess} />
                </View>
              ) : null}
              </View>
              <View style={{ flex: 1, marginLeft: 20 }}>
                <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "600" }} numberOfLines={1}>
                  {ep.indexNumber != null ? `${ep.indexNumber}. ` : ""}{ep.title}
                </Text>
                {ep.durationMs ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 2 }}>
                    {formatDurationMs(ep.durationMs)}
                  </Text>
                ) : null}
                {ep.overview ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 6 }} numberOfLines={2}>
                    {ep.overview}
                  </Text>
                ) : null}
              </View>
            </FocusableCard>
            {/* Its own stop for the remote, right of the row: a press on the
                row opens the episode, this one only marks it. */}
            {demo ? null : (
              <FocusableCard
                onPress={() => toggleEpisodeWatched(ep)}
                disabled={episodeMarkPending}
                style={styles.episodeWatchedToggle}
                // Names the episode: every row's stop says the same action.
                accessibilityLabel={`${watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}: ${ep.indexNumber != null ? `${ep.indexNumber}. ` : ""}${ep.title ?? ""}`}
                accessibilityState={{ checked: watched }}
              >
                <Ionicons
                  name={watched ? "checkmark-circle" : "checkmark-circle-outline"}
                  size={34}
                  color={watched ? theme.colors.success : theme.colors.textMuted}
                />
              </FocusableCard>
            )}
            </View>
            );
          })}
          {episodes.length > visibleEpisodeCount ? (
            <FocusableCard
              onPress={() => setVisibleEpisodeCount((c) => c + EPISODE_PAGE_SIZE)}
              style={[styles.episodeRow, { borderBottomColor: theme.colors.border, justifyContent: "center" }]}
              accessibilityLabel={t("media.detail.showMoreEpisodes", "Show more")}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>
                {t("media.detail.showMoreEpisodes", "Show more")}
              </Text>
            </FocusableCard>
          ) : null}
        </View>
      )}

      {/* Cast */}
      {actors.length > 0 && (
        <View style={{ marginTop: 32 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>{t("media.detail.cast")}</Text>
          <FlatList
            horizontal
            data={actors}
            keyExtractor={(p) => p.id ?? p.name ?? ""}
            showsHorizontalScrollIndicator={false}
            renderItem={({ item: person }) => (
              <FocusableCard
                onPress={() => person.id && router.push(`/(auth)/person/${person.id}`)}
                // No id, no destination: a card the remote can select but
                // that does nothing reads as a dead button. The name still
                // renders below - it's information even without a link.
                disabled={!person.id}
                style={styles.castCard}
                accessibilityLabel={person.name}
              >
                {person.thumbUrl ? (
                  <Image
                    source={{ uri: person.thumbUrl }}
                    style={styles.castAvatar}
                    contentFit="cover"
                    transition={200}
                  />
                ) : (
                  <View style={[styles.castAvatar, { backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }]}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>?</Text>
                  </View>
                )}
                <Text style={{ color: theme.colors.text, fontSize: 15, marginTop: 6, textAlign: "center" }} numberOfLines={1}>
                  {person.name}
                </Text>
                {person.role ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center" }} numberOfLines={1}>
                    {person.role}
                  </Text>
                ) : null}
              </FocusableCard>
            )}
            ItemSeparatorComponent={() => <View style={{ width: 12 }} />}
          />
        </View>
      )}

      {/* Crew, after the cast, exactly as web orders them. */}
      {crew.length > 0 && (
        <View style={{ marginTop: 32 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>{t("media.detail.crew")}</Text>
          <FlatList
            horizontal
            data={crew}
            keyExtractor={(p) => `${p.id ?? p.name ?? ""}-${p.type ?? ""}`}
            showsHorizontalScrollIndicator={false}
            ItemSeparatorComponent={() => <View style={{ width: 12 }} />}
            renderItem={({ item: person }) => (
              <FocusableCard
                onPress={() => person.id && router.push(`/(auth)/person/${person.id}`)}
                // No id, no destination: a card the remote can select but
                // that does nothing reads as a dead button. The name still
                // renders below - it's information even without a link.
                disabled={!person.id}
                style={styles.castCard}
                accessibilityLabel={person.name}
              >
                {person.thumbUrl ? (
                  <Image source={{ uri: person.thumbUrl }} style={styles.castAvatar} contentFit="cover" transition={200} />
                ) : (
                  <View style={[styles.castAvatar, { backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }]}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>?</Text>
                  </View>
                )}
                <Text style={{ color: theme.colors.text, fontSize: 15, marginTop: 6, textAlign: "center" }} numberOfLines={1}>
                  {person.name}
                </Text>
                {/* The crew's job, translated from its enum - `role` here is
                    the raw database role_type ("director"), unlike the cast
                    where it is the character name. */}
                {personTypeLabelKey(person.type) || person.role ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center" }} numberOfLines={1}>
                    {(() => {
                      const key = personTypeLabelKey(person.type);
                      return key ? t(key) : person.role;
                    })()}
                  </Text>
                ) : null}
              </FocusableCard>
            )}
          />
        </View>
      )}

      {/* Sagas this title belongs to - just before Similar, as on the web. */}
      {media.id ? <SagaSection mediaItemId={media.id} /> : null}

      {/* A season page has no "similar seasons" to offer - same exclusion web
          makes. */}
      {media.id && !isSeason ? (
        <SimilarShelf item={media} serverUrl={serverUrl} authHeaders={authHeaders} />
      ) : null}

      <PluginSlot name="media.detail.sidebar" context={{ mediaId: media.id }} />

      <View style={{ height: 64 }} />
      </View>

      {id ? (
        <AddToPlaylistSheet
          visible={playlistSheetOpen}
          mediaItemId={id}
          onClose={() => setPlaylistSheetOpen(false)}
        />
      ) : null}

      {gateProfileId ? (
        <ParentalPINModal
          visible={showPinModal}
          onClose={() => { setShowPinModal(false); setPendingPlayTarget(null); }}
          onVerified={handlePinVerified}
          profileId={gateProfileId}
        />
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  notFoundTitle: { fontSize: 24, fontWeight: "600", textAlign: "center" },
  notFoundButton: { paddingVertical: 10, paddingHorizontal: 36, borderRadius: 8 },
  notFoundButtonLabel: { fontSize: 18, fontWeight: "600" },
  eyebrow: { fontSize: 18, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  title: { fontSize: 42, fontWeight: "bold" },
  tagline: { fontSize: 20, fontStyle: "italic", marginTop: 6 },
  meta: { fontSize: 18, marginTop: 8 },
  genres: { fontSize: 17, marginTop: 4 },
  studios: { fontSize: 17, marginTop: 4 },
  actions: { flexDirection: "row", gap: 12, marginTop: 24 },
  playGroup: { flexDirection: "row", alignItems: "stretch" },
  playButton: {
    paddingVertical: 11,
    paddingHorizontal: 48,
    borderRadius: 8,
    borderWidth: 3,
  },
  // Square off the inner corners so the two halves read as one control.
  playButtonLeft: { borderTopRightRadius: 0, borderBottomRightRadius: 0, borderRightWidth: 0 },
  playRestart: {
    justifyContent: "center",
    paddingHorizontal: 20,
    borderRadius: 8,
    borderTopLeftRadius: 0,
    borderBottomLeftRadius: 0,
    borderWidth: 3,
    borderLeftWidth: 2,
  },
  playText: { fontWeight: "700", fontSize: 20 },
  actionBtn: {
    paddingVertical: 11,
    paddingHorizontal: 32,
    borderRadius: 8,
    borderWidth: 3,
  },
  actionText: { fontWeight: "600", fontSize: 18 },
  overview: { fontSize: 18, lineHeight: 30, marginTop: 24, maxWidth: 800 },
  seasonRow: { marginTop: 32 },
  seasonPill: {
    paddingHorizontal: 24,
    paddingVertical: 9,
    marginRight: 12,
    borderRadius: 8,
    borderWidth: 3,
  },
  sectionTitle: { fontSize: 24, fontWeight: "600", marginBottom: 12 },
  sectionHeaderRow: { flexDirection: "row", alignItems: "baseline", gap: 12, marginBottom: 12 },
  episodeRow: {
    paddingVertical: 11,
    paddingHorizontal: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  episodeWatchedBadge: {
    position: "absolute", top: 6, right: 6,
    width: 26, height: 26, borderRadius: 13,
    alignItems: "center", justifyContent: "center",
  },
  episodeWatchedToggle: { padding: 12, marginLeft: 12 },
  ratingLine: { flexDirection: "row", alignItems: "center", gap: 8, marginTop: 8 },
  castCard: { width: 120, alignItems: "center" },
  castAvatar: { width: 80, height: 80, borderRadius: 40, justifyContent: "center", alignItems: "center" },
});
