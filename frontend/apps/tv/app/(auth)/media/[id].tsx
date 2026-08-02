import { useCallback, useEffect, useMemo, useState } from "react";
import { View, Text, ScrollView, Pressable, FlatList, ActivityIndicator, StyleSheet, Linking } from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, router } from "expo-router";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useFeedServiceSimilarItems,
  useRatingServiceGetMyThumbRating,
  ratingServicePutThumbRating,
  getRatingServiceGetMyThumbRatingQueryKey,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson, V1Trailer } from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useAuthStore, toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { CARD_WIDTH_FEATURED, POSTER_RATIO, CARD_WIDTH_GRID } from "../../../src/lib/tvLayout";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { trailerUrl, sortTrailers } from "../../../src/lib/trailerUrl";

const HERO_HEIGHT = 420;
const POSTER_WIDTH = CARD_WIDTH_FEATURED;
const POSTER_HEIGHT = POSTER_WIDTH * POSTER_RATIO;

function formatRuntime(ms?: string): string {
  if (!ms) return "";
  const totalMin = Math.floor(Number(ms) / 60000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function formatProgress(ms?: string): string {
  if (!ms) return "";
  const totalSec = Math.floor(Number(ms) / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

// Community rating as five stars, the same 0-10 scale web and mobile render.
function StarRating({ rating, color }: { rating: number; color: string }) {
  const full = Math.round(rating / 2);
  return (
    <Text style={{ color, fontSize: 20 }} accessible={false}>
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
            <Text style={{ color: theme.colors.text, fontSize: 16, marginTop: 8 }} numberOfLines={1}>
              {item.title}
            </Text>
          </FocusableCard>
        )}
      />
    </View>
  );
}

// "More like this". Same two-step as web and mobile: the recommendation feed
// first, and a same-genre listing when the feed has nothing to say - a page
// that ends on an empty shelf is a dead end for the remote.
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
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const firstGenre = item.genres?.[0];
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never,
  );

  const cards = useMemo(() => {
    const fromFeed = (similarData?.items ?? [])
      .filter((fi) => !!fi.mediaItemId)
      .map((fi) => ({ id: fi.mediaItemId as string, title: fi.title }));
    if (fromFeed.length > 0) return fromFeed;
    return (byGenreData?.items ?? [])
      .filter((m) => m.id && m.id !== item.id)
      .map((m) => ({ id: m.id as string, title: m.title }));
  }, [similarData, byGenreData, item.id]);

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
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);
  const [focusedBtn, setFocusedBtn] = useState<string | null>(null);

  const { data: media, isLoading } = useLibraryServiceGetMediaItem(id ?? "");
  const { data: watchProgress } = usePlaybackServiceGetWatchProgress(id ?? "");
  const profileId = useAuthStore((s) => s.profileId);
  const qc = useQueryClient();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);

  const [playlistSheetOpen, setPlaylistSheetOpen] = useState(false);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayTarget, setPendingPlayTarget] = useState<string | null>(null);

  const invalidateWatched = useCallback(() => {
    if (id) void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${id}/progress`] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/feeds/resume-playback"] });
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
  const { data: myRating } = useMyThumbRating(id);
  const setThumbRating = useSetThumbRating(id);

  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: id, "pagination.pageSize": 100 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = childrenData?.items ?? [];

  useEffect(() => {
    if (isSeries && children.length > 0 && !selectedSeason) {
      setSelectedSeason(children[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: selectedSeason ?? "", "pagination.pageSize": 100 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = isSeason ? children : (episodesData?.items ?? []);
  const seasons = isSeries ? children : [];

  const getPlayTarget = useCallback((): string | null => {
    if (!id) return null;
    if (isSeries && episodes.length > 0) return episodes[0]!.id!;
    if (isSeason && episodes.length > 0) return episodes[0]!.id!;
    return id;
  }, [id, isSeries, isSeason, episodes]);

  const handlePlay = useCallback(() => {
    const target = getPlayTarget();
    if (!target) return;
    if (needsPin()) {
      setPendingPlayTarget(target);
      setShowPinModal(true);
      return;
    }
    router.push(`/(auth)/player/${target}`);
  }, [getPlayTarget, needsPin]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayTarget) {
      router.push(`/(auth)/player/${pendingPlayTarget}`);
      setPendingPlayTarget(null);
    }
  }, [pendingPlayTarget]);

  if (isLoading || !media) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  const meta = [media.year, formatRuntime(media.durationMs), media.officialRating]
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
            source={{ uri: artworkUrl(serverUrl, id, "backdrop"), headers: authHeaders }}
            style={StyleSheet.absoluteFillObject}
            contentFit="cover"
            transition={300}
            placeholder={backdropBlurhash ? { blurhash: backdropBlurhash } : undefined}
            placeholderContentFit="cover"
          />
        ) : null}
        {/* Fade to bg so the poster/title stay legible over any backdrop. */}
        <View style={[StyleSheet.absoluteFillObject, { justifyContent: "flex-end" }]}>
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.1 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.35 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.7 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.95 }} />
        </View>
      </View>

      {/* Poster + title row, overlapping the backdrop */}
      <View style={{ paddingHorizontal: 48, marginTop: -POSTER_HEIGHT * 0.4, flexDirection: "row", gap: 32 }}>
        {serverUrl && id ? (
          <Image
            source={{ uri: artworkUrl(serverUrl, id, "poster"), headers: authHeaders }}
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
          <Text style={[styles.title, { color: theme.colors.text }]}>{media.title}</Text>
          {meta ? <Text style={[styles.meta, { color: theme.colors.textMuted }]}>{meta}</Text> : null}
          {media.genres && media.genres.length > 0 && (
            <Text style={[styles.genres, { color: theme.colors.textMuted }]}>{media.genres.join(", ")}</Text>
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
              <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>{media.communityRating.toFixed(1)}</Text>
            </View>
          ) : null}
        </View>
      </View>

      <View style={{ paddingHorizontal: 48 }}>
      {/* Action buttons */}
      <View style={styles.actions}>
        <Pressable
          onPress={handlePlay}
          onFocus={() => setFocusedBtn("play")}
          onBlur={() => setFocusedBtn(null)}
          hasTVPreferredFocus={true}
          accessible={true}
          accessibilityLabel={hasProgress ? t("media.resume", "Resume") : t("player.controls.play")}
          accessibilityRole="button"
          style={[
            styles.playButton,
            {
              backgroundColor: theme.colors.accent,
              borderColor: focusedBtn === "play" ? "#fff" : "transparent",
            },
          ]}
        >
          <Text style={[styles.playText, { color: theme.colors.onAccent }]}>
            {hasProgress ? `${t("media.resume", "Resume")} ${formatProgress(resumeMs)}` : t("player.controls.play")}
          </Text>
        </Pressable>

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

        {!isSeries && !isSeason && (
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
        )}

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
        {!isSeries && !isSeason && (
          <>
            <Pressable
              onPress={() => setThumbRating.mutate(myRating === 1 ? 0 : 1)}
              disabled={setThumbRating.isPending}
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
                  opacity: setThumbRating.isPending ? 0.5 : 1,
                },
              ]}
            >
              <Text style={[styles.actionText, { color: myRating === 1 ? theme.colors.accent : theme.colors.text }]}>
                {t("media.rating.like", "Like")}
              </Text>
            </Pressable>
            <Pressable
              onPress={() => setThumbRating.mutate(myRating === -1 ? 0 : -1)}
              disabled={setThumbRating.isPending}
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
                  opacity: setThumbRating.isPending ? 0.5 : 1,
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

      {/* Overview */}
      {media.overview ? (
        <Text style={[styles.overview, { color: theme.colors.text }]}>{media.overview}</Text>
      ) : null}

      {/* Season picker */}
      {isSeries && seasons.length > 0 && (
        <View style={styles.seasonRow}>
          <ScrollView horizontal showsHorizontalScrollIndicator={false}>
            {seasons.map((season) => {
              const active = season.id === selectedSeason;
              return (
                <Pressable
                  key={season.id}
                  onPress={() => setSelectedSeason(season.id ?? null)}
                  onFocus={() => setFocusedBtn(`season-${season.id}`)}
                  onBlur={() => setFocusedBtn(null)}
                  style={[
                    styles.seasonPill,
                    {
                      backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                      borderColor: focusedBtn === `season-${season.id}` ? "#fff" : "transparent",
                    },
                  ]}
                >
                  <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 18, fontWeight: "600" }}>
                    {season.title}
                  </Text>
                </Pressable>
              );
            })}
          </ScrollView>
        </View>
      )}

      {/* Episodes */}
      {(isSeries || isSeason) && episodes.length > 0 && (
        <View style={{ marginTop: 24 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>{t("media.detail.episodes")}</Text>
          {episodes.map((ep, _index) => (
            <FocusableCard
              key={ep.id}
              onPress={() => router.push(`/(auth)/media/${ep.id}`)}
              style={[styles.episodeRow, { borderBottomColor: theme.colors.border }]}
            >
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "600" }} numberOfLines={1}>
                  {ep.indexNumber != null ? `${ep.indexNumber}. ` : ""}{ep.title}
                </Text>
                {ep.durationMs && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: 2 }}>
                    {formatRuntime(ep.durationMs)}
                  </Text>
                )}
              </View>
            </FocusableCard>
          ))}
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
                style={styles.castCard}
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
                    <Text style={{ color: theme.colors.textMuted, fontSize: 28 }}>?</Text>
                  </View>
                )}
                <Text style={{ color: theme.colors.text, fontSize: 16, marginTop: 6, textAlign: "center" }} numberOfLines={1}>
                  {person.name}
                </Text>
                {person.role ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center" }} numberOfLines={1}>
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
                style={styles.castCard}
                accessibilityLabel={person.name}
              >
                {person.thumbUrl ? (
                  <Image source={{ uri: person.thumbUrl }} style={styles.castAvatar} contentFit="cover" transition={200} />
                ) : (
                  <View style={[styles.castAvatar, { backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }]}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 28 }}>?</Text>
                  </View>
                )}
                <Text style={{ color: theme.colors.text, fontSize: 16, marginTop: 6, textAlign: "center" }} numberOfLines={1}>
                  {person.name}
                </Text>
                {person.role ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center" }} numberOfLines={1}>
                    {person.role}
                  </Text>
                ) : null}
              </FocusableCard>
            )}
          />
        </View>
      )}

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
  title: { fontSize: 42, fontWeight: "bold" },
  meta: { fontSize: 20, marginTop: 8 },
  genres: { fontSize: 18, marginTop: 4 },
  actions: { flexDirection: "row", gap: 16, marginTop: 24 },
  playButton: {
    paddingVertical: 16,
    paddingHorizontal: 48,
    borderRadius: 8,
    borderWidth: 3,
  },
  playText: { fontWeight: "700", fontSize: 22 },
  actionBtn: {
    paddingVertical: 16,
    paddingHorizontal: 32,
    borderRadius: 8,
    borderWidth: 3,
  },
  actionText: { fontWeight: "600", fontSize: 20 },
  overview: { fontSize: 20, lineHeight: 30, marginTop: 24, maxWidth: 800 },
  seasonRow: { marginTop: 32 },
  seasonPill: {
    paddingHorizontal: 24,
    paddingVertical: 12,
    marginRight: 12,
    borderRadius: 8,
    borderWidth: 3,
  },
  sectionTitle: { fontSize: 28, fontWeight: "600", marginBottom: 12 },
  episodeRow: {
    paddingVertical: 16,
    paddingHorizontal: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  ratingLine: { flexDirection: "row", alignItems: "center", gap: 8, marginTop: 8 },
  castCard: { width: 120, alignItems: "center" },
  castAvatar: { width: 80, height: 80, borderRadius: 40, justifyContent: "center", alignItems: "center" },
});
