import { useCallback, useEffect, useState } from "react";
import { View, Text, ScrollView, Pressable, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, router } from "expo-router";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson } from "@evey/api";
import { useAuthStore } from "@evey/store";
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

const HERO_HEIGHT = 420;
const POSTER_WIDTH = 220;
const POSTER_HEIGHT = 330;

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
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(false); invalidateWatched(); },
    },
  });

  const isSeries = isSeriesType(media?.type);
  const isSeason = isSeasonType(media?.type);

  const people = media?.people ?? [];
  const actors = people.filter((p: V1MediaPerson) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");

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
          <Text style={styles.playText}>
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
                  <Text style={{ color: active ? "#fff" : theme.colors.text, fontSize: 18, fontWeight: "600" }}>
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
          {episodes.map((ep, index) => (
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
  playText: { color: "#fff", fontWeight: "700", fontSize: 22 },
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
  castCard: { width: 120, alignItems: "center" },
  castAvatar: { width: 80, height: 80, borderRadius: 40, justifyContent: "center", alignItems: "center" },
});
