import { useEffect, useState, useCallback, useMemo, useRef, memo } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  FlatList,
  ActivityIndicator,
  StyleSheet,
  Linking,
  Dimensions,
  Share,
  Alert,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { ProgressBar } from "../../../src/components/ProgressBar";
import { MediaCard } from "../../../src/components/MediaCard";
import { useLocalSearchParams, router, useFocusEffect } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useFeedServiceSimilarItems,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useShareServiceCreateShareLink,
  useRatingServiceGetMyThumbRating,
  ratingServicePutThumbRating,
  getRatingServiceGetMyThumbRatingQueryKey,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson, V1Trailer } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { certInfoForLocale } from "../../../src/lib/certifications";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { startDownload, getDownloadPath, deleteDownload, downloadErrorMessageKey, subscribeToProgress, type ProgressBytes } from "../../../src/lib/downloads";
import { downloadStageLabel } from "../../../src/lib/downloadStageLabel";
import { chooseDownloadQuality } from "../../../src/lib/downloadQualityGate";
import { startReducedDownload } from "../../../src/lib/reducedDownload";
import { confirmDownloadOnMobileData } from "../../../src/lib/mobileDataGate";
import { formatBytes, formatDurationMs, formatTimeMs } from "@evey/api";
import { buildDownloadAuthContext } from "../../../src/lib/downloadContext";
import { toast } from "@evey/store";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { BulkAddToCollectionSheet } from "../../../src/components/library/BulkAddSheets";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { MediaHeroBackdrop } from "../../../src/components/media/MediaHeroBackdrop";
import { backdropWidthForViewport } from "../../../src/components/media/backdropWidth";
import { hasMediaFile } from "../../../src/components/media/mediaFile";
import { buildPublicShareUrl } from "../../../src/components/media/shareLink";
import { MetadataEditSheet } from "../../../src/components/media/MetadataEditSheet";
import { PlayOnSheet } from "../../../src/components/media/PlayOnSheet";
import { SagaSection } from "../../../src/components/media/SagaSection";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";

const POSTER_WIDTH = 100;
const POSTER_HEIGHT = POSTER_WIDTH * 1.5;
const EPISODE_PAGE_SIZE = 20;

// Audio codecs of the item's source file, captured into the download index so
// the player can tell a hearable local copy from one that would play silent
// (raw remux with e.g. AC-3 on a device without that decoder).
function audioCodecsOf(item?: { streams?: { type?: string; codec?: string }[] }): string[] | undefined {
  const codecs = (item?.streams ?? [])
    .filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.codec)
    .map((s) => s.codec as string);
  return codecs.length > 0 ? codecs : undefined;
}

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

function isEpisodeType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_EPISODE";
}

function formatEpisodeLabel(seasonIndex?: number, episodeIndex?: number): string | null {
  const s = seasonIndex ?? 0;
  const e = episodeIndex ?? 0;
  if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
  if (e > 0) return `E${String(e).padStart(2, "0")}`;
  return null;
}

function StarIcons({ rating }: { rating: number }) {
  const theme = useTheme();
  // rating out of 10, show 5 stars
  const filled = Math.round(rating / 2);
  return (
    <View style={{ flexDirection: "row", gap: 1 }}>
      {[1, 2, 3, 4, 5].map((i) => (
        <Ionicons
          key={i}
          name={i <= filled ? "star" : "star-outline"}
          size={12}
          color={i <= filled ? "#f5c518" : theme.colors.textMuted}
        />
      ))}
    </View>
  );
}

const CastCard = memo(function CastCard({ person }: { person: V1MediaPerson }) {
  const theme = useTheme();
  const personType = person.type?.replace("PERSON_TYPE_", "") ?? "";
  const roleLabel = personType.charAt(0) + personType.slice(1).toLowerCase();

  return (
    <Pressable
      onPress={() => person.id && router.push(`/(auth)/person/${person.id}`)}
      style={{ width: 80, marginRight: theme.spacing.sm, alignItems: "center" }}
      accessible={true}
      accessibilityLabel={person.name ?? "Person"}
      accessibilityRole="button"
    >
      {person.thumbUrl ? (
        <Image
          source={{ uri: person.thumbUrl }}
          style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface }}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>
            {person.name ? person.name[0].toUpperCase() : "?"}
          </Text>
        </View>
      )}
      <Text style={{ color: theme.colors.text, fontSize: 11, marginTop: 4, textAlign: "center" }} numberOfLines={1}>
        {person.name}
      </Text>
      {person.role ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {person.role}
        </Text>
      ) : personType && personType !== "ACTOR" ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {roleLabel}
        </Text>
      ) : null}
    </Pressable>
  );
});

const EpisodeRow = memo(function EpisodeRow({
  item, serverUrl, theme, authHeaders, downloadProgress, onDownload, onPlay,
}: {
  item: V1MediaItem;
  serverUrl: string;
  theme: ReturnType<typeof useTheme>;
  authHeaders?: { Authorization: string };
  downloadProgress?: number;
  onDownload?: (item: V1MediaItem) => void;
  onPlay: (item: V1MediaItem) => void;
}) {
  const { t } = useTranslation();
  // "backdrop", not "poster": an episode's still lives under the backdrop type
  // (same as the web episode rows). Asking for a poster gave an empty 16:9 box
  // on most episodes, since episodes rarely carry one.
  const episodeUri = item.id ? artworkUrl(serverUrl, item.id, "backdrop", 240) : undefined;
  const { data: progress } = usePlaybackServiceGetWatchProgress(item.id ?? "", undefined, {
    query: { enabled: !!item.id, retry: false },
  });
  const posMs = Number(progress?.positionMs ?? 0);
  const durMs = Number(progress?.durationMs ?? 0);
  const percent = durMs > 0 ? (posMs / durMs) * 100 : 0;
  // A fully-watched episode has its resume position reset, so `percent` is 0 -
  // it would look identical to a never-watched one. `played` is the truth for
  // "seen in full"; show the same check badge as the cards, and keep the
  // partial bar only for an episode left mid-way.
  const watched = progress?.played === true;
  const downloadable = !!onDownload && hasMediaFile(item);

  return (
    <Pressable
      onPress={() => item.id && onPlay(item)}
      style={{
        flexDirection: "row",
        padding: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={item.title ?? "Episode"}
      accessibilityRole="button"
    >
      <View style={{ width: 120, height: 68, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }}>
        <Image
          source={episodeUri ? { uri: episodeUri, headers: authHeaders } : undefined}
          style={{ width: 120, height: 68 }}
          contentFit="cover"
          transition={200}
        />
        {!watched && percent > 0 && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: 2 }}>
            <ProgressBar percent={percent} />
          </View>
        )}
        {watched && (
          <View style={{
            position: "absolute", top: 4, right: 4,
            width: 20, height: 20, borderRadius: 10,
            backgroundColor: theme.colors.success,
            alignItems: "center", justifyContent: "center",
          }}>
            <Ionicons name="checkmark" size={13} color={theme.colors.onSuccess} />
          </View>
        )}
      </View>
      <View style={{ flex: 1, marginLeft: theme.spacing.sm, justifyContent: "center" }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
          {item.indexNumber != null ? `${item.indexNumber}. ` : ""}{item.title}
        </Text>
        {(() => {
          const runtime = formatDurationMs(item.durationMs);
          return runtime ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
              {runtime}
            </Text>
          ) : null;
        })()}
        {item.overview ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }} numberOfLines={2}>
            {item.overview}
          </Text>
        ) : null}
      </View>
      {downloadable ? (
        <Pressable
          onPress={(e) => { e.stopPropagation(); onDownload!(item); }}
          disabled={downloadProgress !== undefined}
          hitSlop={8}
          style={{ justifyContent: "center", paddingHorizontal: theme.spacing.sm }}
          accessible={true}
          accessibilityLabel={t("common.download") || "Download"}
          accessibilityRole="button"
        >
          {downloadProgress !== undefined ? (
            Number.isFinite(downloadProgress) ? (
              <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "700" }}>
                {Math.round(downloadProgress * 100)}%
              </Text>
            ) : (
              <Ionicons name="cloud-download" size={18} color={theme.colors.accent} />
            )
          ) : (
            <Ionicons name="download-outline" size={18} color={theme.colors.textMuted} />
          )}
        </Pressable>
      ) : null}
    </Pressable>
  );
});

const TrailerButton = memo(function TrailerButton({ trailer }: { trailer: V1Trailer }) {
  const { t } = useTranslation();
  const theme = useTheme();

  const url = (() => {
    if (!trailer.key || !trailer.site) return null;
    switch (trailer.site.toLowerCase()) {
      case "youtube":
        return `https://www.youtube.com/watch?v=${trailer.key}`;
      case "vimeo":
        return `https://vimeo.com/${trailer.key}`;
      default:
        return null;
    }
  })();

  if (!url) return null;

  return (
    <Pressable
      onPress={() => Linking.openURL(url)}
      style={{
        flexDirection: "row",
        alignItems: "center",
        gap: 6,
        paddingHorizontal: 12,
        paddingVertical: 8,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surface,
        borderWidth: StyleSheet.hairlineWidth,
        borderColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={t("media.detail.trailer")}
      accessibilityRole="link"
    >
      <Ionicons name="play" size={14} color={theme.colors.text} />
      <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "500" }}>
        {t("media.detail.trailer")}
      </Text>
    </Pressable>
  );
});

const SimilarSection = memo(function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const screenW = Dimensions.get("window").width;
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const firstGenre = item.genres?.[0];
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never,
  );

  const cards: V1MediaItem[] = useMemo(() => {
    const fromFeed = (similarData?.items ?? [])
      .filter((fi: any) => !!fi.mediaItemId)
      .map((fi: any) => ({ id: fi.mediaItemId!, title: fi.title }));

    const fromGenre = (byGenreData?.items ?? [])
      .filter((m) => m.id && m.id !== item.id)
      .map((m) => ({ id: m.id!, title: m.title, year: m.year, type: m.type }));

    return (fromFeed.length ? fromFeed : fromGenre) as V1MediaItem[];
  }, [similarData?.items, byGenreData?.items, item.id]);

  const renderCard = useCallback(
    ({ item: c }: { item: V1MediaItem }) => (
      <MediaCard
        item={c}
        onPress={() => c.id && router.push(`/(auth)/media/${c.id}`)}
        width={screenW * 0.28}
      />
    ),
    [screenW],
  );

  if (!cards.length) return null;

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
        {t("media.detail.similarTitles")}
      </Text>
      <FlatList
        horizontal
        data={cards}
        keyExtractor={similarKeyExtractor}
        renderItem={renderCard}
        showsHorizontalScrollIndicator={false}
      />
    </View>
  );
});

function similarKeyExtractor(c: V1MediaItem) {
  return c.id ?? "";
}

function personKeyExtractor(p: V1MediaPerson) {
  return p.id ?? p.name ?? "";
}

function renderCastItem({ item }: { item: V1MediaPerson }) {
  return <CastCard person={item} />;
}

function FileInfoSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [open, setOpen] = useState(false);

  if (!item.filePath && !item.fileSize && !item.containerFormat && !(item.streams && item.streams.length > 0)) {
    return null;
  }

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Pressable
        onPress={() => setOpen((v) => !v)}
        style={{ flexDirection: "row", alignItems: "center", gap: 6, marginBottom: theme.spacing.sm }}
        accessible={true}
        accessibilityLabel={t("media.detail.fileInfo")}
        accessibilityRole="button"
      >
        <Ionicons
          name={open ? "chevron-down" : "chevron-forward"}
          size={16}
          color={theme.colors.textMuted}
        />
        <Text style={{ color: theme.colors.textMuted, fontSize: 14, fontWeight: "600" }}>
          {t("media.detail.fileInfo")}
        </Text>
      </Pressable>
      {open && (
        <View style={{ gap: 8 }}>
          {item.filePath ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={2}>
              {item.filePath}
            </Text>
          ) : null}
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
            {item.fileSize ? <InfoBadge label={formatBytes(item.fileSize)} /> : null}
            {item.containerFormat ? <InfoBadge label={item.containerFormat.toUpperCase()} /> : null}
            {item.bitrate ? <InfoBadge label={`${Math.round(parseInt(item.bitrate) / 1000)} kbps`} /> : null}
            {(() => { const r = formatDurationMs(item.durationMs); return r ? <InfoBadge label={r} /> : null; })()}
          </View>
          {item.streams && item.streams.length > 0 ? (
            <View style={{ gap: 4, marginTop: 4 }}>
              {item.streams.map((s, i) => {
                const typeLabel = s.type?.replace("MEDIA_STREAM_TYPE_", "") ?? "";
                const res = s.width && s.height ? `${s.width}x${s.height}` : undefined;
                return (
                  <View key={i} style={{ flexDirection: "row", gap: 8 }}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", width: 60 }}>
                      {typeLabel}
                    </Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, flex: 1 }} numberOfLines={1}>
                      {[s.codec, s.language, s.title, res].filter(Boolean).join(" - ")}
                    </Text>
                  </View>
                );
              })}
            </View>
          ) : null}
        </View>
      )}
    </View>
  );
}

// Mirrors apps/web/src/hooks/useRating.ts (like=1, dislike=-1, 0=cleared),
// inlined here rather than shared since only this file is in scope.
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
    onError: (_err: Error, _vars: number, ctx?: { prev?: { value?: number } }) => {
      if (ctx?.prev !== undefined && mediaId)
        qc.setQueryData(getRatingServiceGetMyThumbRatingQueryKey(mediaId), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) void qc.invalidateQueries({ queryKey: getRatingServiceGetMyThumbRatingQueryKey(mediaId) });
    },
  });
}

function InfoBadge({ label }: { label: string }) {
  const theme = useTheme();
  return (
    <View style={{
      paddingHorizontal: 8,
      paddingVertical: 3,
      borderRadius: theme.radii.sm,
      backgroundColor: theme.colors.surface,
    }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "500" }}>{label}</Text>
    </View>
  );
}

export default function MediaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const insets = useSafeAreaInsets();
  const [screenW, setScreenW] = useState(() => Dimensions.get("window").width);
  const [screenH, setScreenH] = useState(() => Dimensions.get("window").height);
  const dimsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const sub = Dimensions.addEventListener("change", ({ window }) => {
      if (dimsTimer.current) clearTimeout(dimsTimer.current);
      dimsTimer.current = setTimeout(() => {
        setScreenW(window.width);
        setScreenH(window.height);
      }, 300);
    });
    return () => { sub.remove(); if (dimsTimer.current) clearTimeout(dimsTimer.current); };
  }, []);
  const isLandscape = screenW > screenH;
  const heroHeight = isLandscape ? screenH * 0.55 : screenH * 0.4;
  // Dimensions().scale IS the device pixel ratio for the window - reused here
  // instead of importing PixelRatio separately since Dimensions is already
  // pulled in above. Falls back to 1x if a test double's Dimensions mock
  // omits it.
  const pixelRatio = Dimensions.get("window").scale || 1;
  const backdropWidth = backdropWidthForViewport(screenW, pixelRatio);
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);
  // Episodes render inside the screen's single outer ScrollView (see below), so a
  // nested FlatList would trip "VirtualizedLists should never be nested inside
  // plain ScrollViews" and gain nothing since it can't own its own scroll. Instead
  // cap the initial render and let the user page in more with a button, which keeps
  // a 100-episode season from mounting 100 rows (with progress queries each) at once.
  const [visibleEpisodeCount, setVisibleEpisodeCount] = useState(EPISODE_PAGE_SIZE);
  useEffect(() => { setVisibleEpisodeCount(EPISODE_PAGE_SIZE); }, [selectedSeason]);

  const { data: media, isLoading, isError, refetch } = useLibraryServiceGetMediaItem(id ?? "");

  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } } as never);
  const profileRegion = (profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0])?.region;

  const { data: watchProgress } = usePlaybackServiceGetWatchProgress(id ?? "", undefined, {
    query: { enabled: !!id, retry: false },
  });
  const profileId = useAuthStore((s) => s.profileId);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  // Same active-profile resolution as the player screen (player/[id].tsx's
  // activeProfile) - reused here so a download captures the exact profile
  // identity/restriction that getDownloadPath's offline gate will later check.
  const activeProfile = useMemo(
    () =>
      profilesResp?.profiles?.find((p) => p.id === profileId) ??
      profilesResp?.profiles?.find((p) => p.isDefault) ??
      profilesResp?.profiles?.[0],
    [profilesResp, profileId],
  );
  // Admin-only: mint a PUBLIC download link and hand it off via the native
  // share sheet - the mobile equivalent of the web's clipboard copy (there is
  // no clipboard API in this app; SyncPlayBar's room-code "copy" uses the same
  // Share.share() trick). The toast fires as soon as the link is ready, same
  // as the web's instant "copied" feedback, independent of what the user does
  // with the share sheet afterwards.
  const createShareLink = useShareServiceCreateShareLink({
    mutation: {
      onSuccess: async (res) => {
        const url = buildPublicShareUrl(serverUrl, res?.url);
        if (!url) {
          toast.error(t("media.library.shareFailed"));
          return;
        }
        toast.success(t("media.library.shareLinkCopied"));
        try {
          await Share.share({ message: url });
        } catch {
          // user dismissed the share sheet
        }
      },
      onError: (err: Error) => toast.error(err?.message || t("media.library.shareFailed")),
    },
  });
  const qc = useQueryClient();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);
  const { data: myRating = 0 } = useMyThumbRating(id);
  const setThumbRating = useSetThumbRating(id);

  const [playlistSheetOpen, setPlaylistSheetOpen] = useState(false);
  const [playOnSheetOpen, setPlayOnSheetOpen] = useState(false);
  const [collectionSheetOpen, setCollectionSheetOpen] = useState(false);
  const [metadataSheetOpen, setMetadataSheetOpen] = useState(false);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayTarget, setPendingPlayTarget] = useState<{ id: string; restart: boolean } | null>(null);
  // null = idle, 0..1 = downloading with known total, NaN = downloading
  // with unknown total (server omits Content-Length).
  const [downloadProgress, setDownloadProgress] = useState<number | null>(null);
  // Raw byte counter shown next to the percent: the integer percent of a
  // multi-GB file reads "0%" for minutes on a slow link, and the moving MB
  // figure is what tells that apart from a stuck download.
  const [downloadBytes, setDownloadBytes] = useState<ProgressBytes | null>(null);
  // Per-episode progress: a key is only present while that episode's
  // download is running (0..1 known total, NaN unknown) - absent means idle,
  // matching EpisodeRow's `downloadProgress === undefined` idle check.
  const [episodeDownloads, setEpisodeDownloads] = useState<Record<string, number>>({});
  // Mirror of episodeDownloads for the in-flight guard in downloadEpisodeFile,
  // so that callback can stay identity-stable across progress ticks.
  const episodeDownloadsRef = useRef(episodeDownloads);
  episodeDownloadsRef.current = episodeDownloads;
  const [bulkDownload, setBulkDownload] = useState<{ done: number; total: number } | null>(null);

  const invalidateWatched = useCallback(() => {
    if (id) void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${id}/progress`] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
    // All home feeds filter watched items server-side; refresh the lot so
    // the just-marked item disappears from the homepage immediately.
    void qc.invalidateQueries({
      predicate: (q) => {
        const k = q.queryKey?.[0];
        if (typeof k !== "string") return false;
        return k.startsWith("/api/v1/feeds/") || k === "/api/v1/activity/popular-media";
      },
    });
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
  const isEpisode = isEpisodeType(media?.type);

  const { data: parentItem } = useLibraryServiceGetMediaItem(
    media?.parentId ?? "",
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );

  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: id, "pagination.pageSize": 100 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = useMemo(() => childrenData?.items ?? [], [childrenData?.items]);

  // Navigating between two series reuses this screen (Expo Router re-renders
  // rather than remounting), so reset the selected season when the media id
  // changes - otherwise the guard below keeps the PREVIOUS series' season and
  // the episode query loads the wrong (or empty) episode list.
  useEffect(() => { setSelectedSeason(null); }, [id]);

  useEffect(() => {
    if (isSeries && children.length > 0 && !selectedSeason) {
      const sorted = [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
      setSelectedSeason(sorted[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
      // 500, not 100: episodes have no server-side sort key, so this list is
      // re-sorted by index on the device and a truncated page would put the
      // wrong episode first (the server answers in sort_title order).
    { parentId: selectedSeason ?? "", "pagination.pageSize": 500 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = useMemo(
    () =>
      [...(isSeason ? children : (episodesData?.items ?? []))].sort(
        (a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0),
      ),
    [isSeason, children, episodesData?.items],
  );
  const seasons = useMemo(
    () => (isSeries ? [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0)) : []),
    [isSeries, children],
  );

  // Downloads a single episode (single-file, same server endpoint as a
  // movie). Shared by the per-episode button and the "download all" loop
  // below; `silent` skips the per-item toasts during a bulk run so the user
  // gets one start/finish toast for the whole batch instead of N of them.
  // Tapping download on an already-downloaded item offers removal instead of
  // the old dead-end "already downloaded" toast - same dialog wording as the
  // Downloads screen so both paths read as one feature.
  const confirmDeleteDownload = useCallback((mediaItemId: string, title: string) => {
    Alert.alert(
      t("downloads.deleteTitle", "Delete download"),
      t("downloads.deleteConfirm", "Remove \"{{title}}\" from this device?", { title }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            await deleteDownload(mediaItemId);
            toast.success(t("downloads.deleted", "Download removed"));
          },
        },
      ],
    );
  }, [t]);

  const downloadEpisodeFile = useCallback(
    async (ep: V1MediaItem, opts?: { silent?: boolean }) => {
      if (!ep.id) return;
      // Read the in-flight set through a ref, not the state value: depending on
      // episodeDownloads would give this callback a new identity on every
      // progress tick, which in turn re-renders EVERY memoised EpisodeRow
      // instead of just the one that is downloading.
      if (episodeDownloadsRef.current[ep.id] !== undefined) return;
      const existing = await getDownloadPath(ep.id);
      if (existing) {
        // Bulk "download all" must stay quiet about episodes already on disk;
        // a direct tap on the row's button gets the delete dialog instead.
        if (!opts?.silent) confirmDeleteDownload(ep.id, ep.title ?? ep.id);
        return;
      }
      if (!(await confirmDownloadOnMobileData(t))) return;
      setEpisodeDownloads((s) => ({ ...s, [ep.id!]: NaN }));
      if (!opts?.silent) toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
      try {
        await startDownload(
          ep.id,
          ep.title ?? ep.id,
          (p) => { if (screenMountedRef.current) setEpisodeDownloads((s) => ({ ...s, [ep.id!]: p })); },
          {
            kind: "video",
            formatHint: ep.containerFormat,
            audioCodecs: audioCodecsOf(ep),
            // Captured here because offline there is no way to resolve an
            // episode's ancestors: this screen IS the series, and the picked
            // season carries the number.
            display: {
              seriesTitle: media?.title,
              seasonNumber: children.find((c) => c.id === selectedSeason)?.indexNumber,
              episodeNumber: ep.indexNumber,
            },
            // An episode rarely carries its own classification - it inherits
            // the series' rating, which is what this screen's own parental
            // gate above (useParentalGate(media?.officialRating)) already
            // relies on.
            ...buildDownloadAuthContext(activeProfile, {
              officialRating: ep.officialRating ?? media?.officialRating,
              certificationsByCountry: ep.certificationsByCountry ?? media?.certificationsByCountry,
            }),
          },
        );
        if (!opts?.silent) toast.success(t("media.detail.downloadComplete"));
      } catch (err) {
        if (!opts?.silent) {
          toast.error(t(downloadErrorMessageKey(err)));
        }
        throw err;
      } finally {
        setEpisodeDownloads((s) => {
          const next = { ...s };
          delete next[ep.id!];
          return next;
        });
      }
    },
    [t, activeProfile, media, confirmDeleteDownload],
  );

  // Stable prop for EpisodeRow. Passing an inline arrow here made the memo()
  // around EpisodeRow inert: every "Show more" tap re-rendered the whole list
  // (280 rows across four taps on a 100-episode season) instead of only the
  // twenty newly revealed ones.
  const handleEpisodeDownload = useCallback(
    (ep: V1MediaItem) => { void downloadEpisodeFile(ep); },
    [downloadEpisodeFile],
  );


  // "Download all" for the currently shown episode list (the active season,
  // for both a season screen and a series screen). Sequenced one at a time -
  // not a Promise.all - so a 24-episode season doesn't fire 24 concurrent
  // background downloads at once.
  const handleDownloadAllEpisodes = useCallback(async () => {
    if (bulkDownload) return;
    const targets = episodes.filter((ep) => ep.id && hasMediaFile(ep));
    if (targets.length === 0) return;
    // Asked once for the whole batch, not once per episode: a season is the
    // heaviest thing this app downloads, and prompting per episode would be
    // twenty dialogs.
    if (!(await confirmDownloadOnMobileData(t))) return;
    setBulkDownload({ done: 0, total: targets.length });
    toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
    let done = 0;
    for (const ep of targets) {
      try {
        await downloadEpisodeFile(ep, { silent: true });
      } catch {
        // One failed episode must not stop the rest of the season from queuing.
      }
      done += 1;
      setBulkDownload({ done, total: targets.length });
    }
    setBulkDownload(null);
    toast.success(t("media.detail.downloadComplete"));
  }, [episodes, downloadEpisodeFile, bulkDownload, t]);

  const people = media?.people ?? [];
  const actors = useMemo(
    () => people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR"),
    [people],
  );
  const directors = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_DIRECTOR"), [people]);
  const writers = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_WRITER"), [people]);
  const producers = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_PRODUCER"), [people]);
  const crew = useMemo(
    () => [...directors, ...writers, ...producers],
    [directors, writers, producers],
  );

  const getPlayTarget = useCallback((): string | null => {
    if (!id) return null;
    if (isSeries && episodes.length > 0) return episodes[0]!.id!;
    if (isSeason && episodes.length > 0) return episodes[0]!.id!;
    return id;
  }, [id, isSeries, isSeason, episodes]);

  // A double-tap on a play control fires this callback twice before the
  // first push unmounts the screen, mounting two player screens stacked on
  // top of each other. Latch until the screen regains focus (leaving the
  // player and coming back).
  const navigatingRef = useRef(false);
  useFocusEffect(
    useCallback(() => {
      navigatingRef.current = false;
    }, []),
  );

  // `restart` travels all the way to the player, PIN prompt included: the
  // choice must survive the detour, or entering a PIN would silently resume
  // the thing the viewer asked to watch from the start.
  const openPlayer = useCallback((target: string, restart: boolean) => {
    if (navigatingRef.current) return;
    navigatingRef.current = true;
    router.push(restart ? `/(auth)/player/${target}?restart=1` : `/(auth)/player/${target}`);
  }, []);

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

  // The download-progress callbacks below outlive this screen: startDownload
  // unsubscribes them when the DOWNLOAD settles, not when the screen unmounts,
  // so a download finishing minutes after navigation kept firing setState into
  // a dead screen for its whole duration (audit 2026-08-09). Gate them here.
  const screenMountedRef = useRef(true);
  useEffect(() => () => { screenMountedRef.current = false; }, []);

  // An episode tap is a PLAY action, so it takes the same PIN gate as the Play
  // button - the row used to router.push straight into the player, which has
  // no check of its own, leaving the gate on one button only. needsPin rides a
  // ref to keep this callback identity-stable (it is a prop of every memoised
  // EpisodeRow, see handleEpisodeDownload's comment).
  const needsPinRef = useRef(needsPin);
  needsPinRef.current = needsPin;
  const handleEpisodePlay = useCallback((ep: V1MediaItem) => {
    if (!ep.id) return;
    if (needsPinRef.current()) {
      setPendingPlayTarget({ id: ep.id, restart: false });
      setShowPinModal(true);
      return;
    }
    openPlayer(ep.id, false);
  }, [openPlayer]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayTarget) {
      openPlayer(pendingPlayTarget.id, pendingPlayTarget.restart);
      setPendingPlayTarget(null);
    }
  }, [pendingPlayTarget, openPlayer]);

  // The React Query client retries twice then gives up (except 401/403/404):
  // once exhausted, isLoading turns false but media stays undefined. Without an
  // error branch that left the most-visited screen stuck on the loading skeleton
  // forever, with no message or retry. Mirrors library/[libraryId].tsx.
  if (isError && !media) {
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg, justifyContent: "center", alignItems: "center" }}>
        <Text style={{ color: theme.colors.textMuted, marginBottom: 12 }}>{t("common.loadError")}</Text>
        <Pressable onPress={() => refetch()}>
          <Text style={{ color: theme.colors.accent }}>{t("common.retry")}</Text>
        </Pressable>
      </View>
    );
  }

  if (isLoading || !media) {
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <Skeleton width={screenW} height={heroHeight} borderRadius={0} />
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.md }}>
          <Skeleton width={screenW * 0.6} height={24} borderRadius={theme.radii.sm} />
          <Skeleton width={screenW * 0.4} height={14} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.sm }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={44} borderRadius={theme.radii.md} style={{ marginTop: theme.spacing.md }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={60} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.md }} />
        </View>
      </View>
    );
  }

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  const trailers = media.trailers ?? [];
  const sortedTrailers = [...trailers].sort((a, b) => {
    if (a.official && !b.official) return -1;
    if (!a.official && b.official) return 1;
    return 0;
  }).slice(0, 3);

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingLeft: insets.left, paddingRight: insets.right, paddingBottom: insets.bottom }}>
      {/* Hero backdrop */}
      <MediaHeroBackdrop
        uri={serverUrl && id ? artworkUrl(serverUrl, id, "backdrop", backdropWidth) : undefined}
        headers={authHeaders}
        height={heroHeight}
        onBack={() => router.back()}
        topInset={insets.top}
        leftInset={insets.left}
      />

      {/* Content overlapping hero */}
      <View style={{ paddingHorizontal: theme.spacing.md, marginTop: -POSTER_HEIGHT * 0.5, ...(isLandscape ? { flexDirection: "row", gap: theme.spacing.lg } : {}) }}>
        {/* Left column (landscape) / full width (portrait) */}
        <View style={isLandscape ? { flex: 3 } : {}}>
        {/* Poster + title row */}
        <View style={{ flexDirection: "row", gap: theme.spacing.md }}>
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
            />
          ) : (
            <View style={{
              width: POSTER_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.surface,
              justifyContent: "center",
              alignItems: "center",
            }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 28, fontWeight: "bold" }}>
                {media.title?.[0]?.toUpperCase() ?? "?"}
              </Text>
            </View>
          )}

          <View style={{ flex: 1, justifyContent: "flex-end" }}>
            {isEpisode && (() => {
              const epLabel = formatEpisodeLabel(parentItem?.indexNumber, media.indexNumber);
              const seasonName = parentItem?.title;
              const label = [epLabel, seasonName].filter(Boolean).join(" · ");
              if (!label) return null;
              // Reached from the Resume row an episode lands here with no way
              // back up to its season; parentId is always present on an episode,
              // so make the SxEy · season line a tap-back to the season page.
              if (!media.parentId) {
                return (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontWeight: "600", marginBottom: 2 }}>
                    {label}
                  </Text>
                );
              }
              return (
                <Pressable
                  onPress={() => router.push(`/(auth)/media/${media.parentId}`)}
                  hitSlop={8}
                  accessibilityRole="button"
                  accessibilityLabel={seasonName ?? epLabel ?? undefined}
                  style={{ flexDirection: "row", alignItems: "center", gap: 3, marginBottom: 2 }}
                >
                  <Ionicons name="chevron-back" size={14} color={theme.colors.accent} />
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
                    {label}
                  </Text>
                </Pressable>
              );
            })()}
            <Text style={{ color: theme.colors.text, fontSize: 22, fontWeight: "bold" }}>
              {media.title}
            </Text>

            {/* Meta: year, duration, rating */}
            <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6, marginTop: 4 }}>
              {media.year ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{media.year}</Text>
              ) : null}
              {(() => {
                const runtime = formatDurationMs(media.durationMs);
                return runtime
                  ? <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{runtime}</Text>
                  : null;
              })()}
              {(() => {
                const info = certInfoForLocale(media.certificationsByCountry, i18n.language, media.officialRating, profileRegion);
                if (!info) return null;
                const body = t(`media.certBody.${info.country}`, { defaultValue: info.country });
                return (
                  <View style={{
                    borderWidth: 1,
                    borderColor: theme.colors.textMuted,
                    borderRadius: theme.radii.sm,
                    paddingHorizontal: 5,
                    paddingVertical: 1,
                  }}>
                    <Text style={{ color: theme.colors.text, fontSize: 11, fontWeight: "700" }}>
                      {body} {info.rating}
                    </Text>
                  </View>
                );
              })()}
            </View>

            {/* Community rating stars */}
            {media.communityRating != null && media.communityRating > 0 ? (
              <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginTop: 4 }}>
                <StarIcons rating={media.communityRating} />
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                  {media.communityRating.toFixed(1)}
                </Text>
              </View>
            ) : null}

            {/* Provider badges: IMDb, TMDB */}
            <View style={{ flexDirection: "row", gap: 6, marginTop: 4 }}>
              {media.providerIds?.imdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.imdb.com/title/${media.providerIds!.imdb}/`)} accessible={true} accessibilityLabel="IMDb" accessibilityRole="link">
                  <View style={{ backgroundColor: "#f5c518", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#000", fontSize: 10, fontWeight: "800" }}>IMDb</Text>
                  </View>
                </Pressable>
              ) : null}
              {media.providerIds?.tmdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.themoviedb.org/${media.type === "MEDIA_ITEM_TYPE_MOVIE" ? "movie" : "tv"}/${media.providerIds!.tmdb}`)} accessible={true} accessibilityLabel="TMDB" accessibilityRole="link">
                  <View style={{ backgroundColor: "#01b4e4", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#fff", fontSize: 10, fontWeight: "800" }}>TMDB</Text>
                  </View>
                </Pressable>
              ) : null}
            </View>
          </View>
        </View>

        {/* Genres */}
        {media.genres && media.genres.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: theme.spacing.sm }}>
            {media.genres.map((g) => (
              <View key={g} style={{
                paddingHorizontal: 10,
                paddingVertical: 4,
                borderRadius: 16,
                backgroundColor: theme.colors.surface,
              }}>
                <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "500" }}>{g}</Text>
              </View>
            ))}
          </View>
        ) : null}

        {/* One control, two halves: the wide one resumes, the narrow one starts
            over. The divider is what makes them read as a single button rather
            than two rival Play buttons. With nothing to resume the second half
            is absent and the button is whole again. */}
        <View style={{ flexDirection: "row", marginTop: theme.spacing.md }}>
          <Pressable
            onPress={() => handlePlay(false)}
            style={{
              flex: 1,
              backgroundColor: theme.colors.accent,
              borderTopLeftRadius: theme.radii.md,
              borderBottomLeftRadius: theme.radii.md,
              borderTopRightRadius: hasProgress ? 0 : theme.radii.md,
              borderBottomRightRadius: hasProgress ? 0 : theme.radii.md,
              paddingVertical: theme.spacing.sm + 4,
              flexDirection: "row",
              alignItems: "center",
              justifyContent: "center",
              gap: 8,
            }}
            accessible={true}
            accessibilityLabel={
              hasProgress
                ? `${t("media.detail.resume")} ${formatTimeMs(Number(resumeMs))}`
                : t("media.detail.play")
            }
            accessibilityRole="button"
          >
            <Ionicons name="play" size={18} color={theme.colors.onAccent} />
            <Text style={{ color: theme.colors.onAccent, fontWeight: "700", fontSize: 16 }}>
              {hasProgress
                ? `${t("media.detail.resume")} ${formatTimeMs(Number(resumeMs))}`
                : t("media.detail.play")}
            </Text>
          </Pressable>

          {hasProgress ? (
            <Pressable
              onPress={() => handlePlay(true)}
              style={{
                backgroundColor: theme.colors.accent,
                borderTopRightRadius: theme.radii.md,
                borderBottomRightRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm + 4,
                paddingHorizontal: theme.spacing.md,
                justifyContent: "center",
                borderLeftWidth: StyleSheet.hairlineWidth,
                borderLeftColor: theme.colors.onAccent + "55",
              }}
              accessible={true}
              accessibilityLabel={t("media.playFromStart", "Play from the beginning")}
              accessibilityRole="button"
            >
              <Ionicons name="refresh" size={20} color={theme.colors.onAccent} />
            </Pressable>
          ) : null}
        </View>

        {/* Action row: favorite, watched, playlist */}
        <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.md, flexWrap: "wrap" }}>
          <Pressable
            onPress={() => toggleFavorite.mutate()}
            disabled={toggleFavorite.isPending}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: toggleFavorite.isPending ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
            accessibilityRole="button"
          >
            <Ionicons
              name={isFavorite ? "heart" : "heart-outline"}
              size={20}
              color={isFavorite ? theme.colors.accent : theme.colors.text}
            />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
            </Text>
          </Pressable>

          <Pressable
            onPress={() => setThumbRating.mutate(myRating === 1 ? 0 : 1)}
            disabled={setThumbRating.isPending}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: myRating === 1 ? theme.colors.success : theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: setThumbRating.isPending ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={myRating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
            accessibilityRole="button"
          >
            <Ionicons
              name={myRating === 1 ? "thumbs-up" : "thumbs-up-outline"}
              size={20}
              color={myRating === 1 ? theme.colors.onAccent : theme.colors.text}
            />
          </Pressable>

          <Pressable
            onPress={() => setThumbRating.mutate(myRating === -1 ? 0 : -1)}
            disabled={setThumbRating.isPending}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: myRating === -1 ? theme.colors.danger : theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: setThumbRating.isPending ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={myRating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
            accessibilityRole="button"
          >
            <Ionicons
              name={myRating === -1 ? "thumbs-down" : "thumbs-down-outline"}
              size={20}
              color={myRating === -1 ? theme.colors.onAccent : theme.colors.text}
            />
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
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: (markWatched.isPending || markUnwatched.isPending) ? 0.5 : 1,
              }}
              accessible={true}
              accessibilityLabel={watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              accessibilityRole="button"
            >
              {(markWatched.isPending || markUnwatched.isPending) ? (
                <ActivityIndicator size={18} color={theme.colors.text} />
              ) : (
                <Ionicons
                  name={watched ? "checkmark-circle" : "checkmark-circle-outline"}
                  size={20}
                  color={watched ? theme.colors.accent : theme.colors.text}
                />
              )}
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              </Text>
            </Pressable>
          )}

          <Pressable
            onPress={() => setPlaylistSheetOpen(true)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={t("media.detail.addToPlaylist")}
            accessibilityRole="button"
          >
            <Ionicons name="add" size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {t("media.detail.addToPlaylist")}
            </Text>
          </Pressable>

          {/* Sends this title's loadAndPlay command to one of the user's other
              connected Evey clients - the mobile counterpart to the web
              player's Cast/RemotePlaybackMenu "Play on" section. */}
          <Pressable
            onPress={() => setPlayOnSheetOpen(true)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={t("player.playOn.button")}
            accessibilityRole="button"
          >
            <Ionicons name="tv-outline" size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {t("player.playOn.title")}
            </Text>
          </Pressable>
          {/* Grouping a title into a saga is thought of here, on the title's own
              page, not from a grid card - but only the library's bulk action bar
              offered it. Admin-gated like the RPC (AddCollectionItem is RoleAdmin). */}
          {isAdmin && id && (
            <Pressable
              onPress={() => setCollectionSheetOpen(true)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("media.contextMenu.addToCollection")}
              accessibilityRole="button"
              testID="add-to-collection-button"
            >
              <Ionicons name="albums-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.contextMenu.addToCollection")}
              </Text>
            </Pressable>
          )}
          {!isSeries && !isSeason && hasMediaFile(media) && (
            <Pressable
              onPress={async () => {
                if (!id || !media?.title) return;
                if (downloadProgress !== null) return;
                const existing = await getDownloadPath(id);
                if (existing) { confirmDeleteDownload(id, media.title); return; }
                if (!(await confirmDownloadOnMobileData(t))) return;
                // A 4K remux can be tens of gigabytes: when the server offers
                // a lighter copy (and the operator turned the choice on), ask
                // before spending the space rather than after failing on it.
                const choice = await chooseDownloadQuality(t, id);
                if (choice.quality === "cancel") return;
                setDownloadProgress(NaN);
                toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
                try {
                  if (choice.quality === "reduced" && choice.plan) {
                    // The sliced path builds its own file, so it reports
                    // through the same progress stream rather than returning
                    // one task to await.
                    const unsubscribe = subscribeToProgress(id, (p, b) => {
                      if (!screenMountedRef.current) return;
                      setDownloadProgress(p);
                      if (b) setDownloadBytes(b);
                    });
                    try {
                      await startReducedDownload(id, media.title, choice.plan.durationMs, choice.plan.slices, {
                        authContext: buildDownloadAuthContext(activeProfile, {
                          officialRating: media.officialRating,
                          certificationsByCountry: media.certificationsByCountry,
                        }),
                      });
                    } finally {
                      unsubscribe();
                    }
                    toast.success(t("media.detail.downloadComplete"));
                    return;
                  }
                  await startDownload(id, media.title,
                    (p, b) => { if (!screenMountedRef.current) return; setDownloadProgress(p); if (b) setDownloadBytes(b); },
                    {
                      ...buildDownloadAuthContext(activeProfile, {
                        officialRating: media.officialRating,
                        certificationsByCountry: media.certificationsByCountry,
                      }),
                      // Without the hint the extension falls back to ".mp4"
                      // even for Matroska bytes (the episode site always
                      // passed it; this one forgot) - which misnames the file
                      // for VLC and used to make the audio pass skip it.
                      formatHint: media.containerFormat,
                      audioCodecs: audioCodecsOf(media),
                      // The server stamps seriesTitle on episode payloads, so
                      // a lone episode downloaded from its own page still
                      // names its show on the offline list.
                      display: {
                        seriesTitle: media.seriesTitle,
                        seasonNumber: parentItem?.indexNumber,
                        episodeNumber: media.indexNumber,
                      },
                    },
                  );
                  toast.success(t("media.detail.downloadComplete"));
                } catch (err) {
                  // Surface the real cause instead of a blanket "failed": a
                  // disk-space rejection right after the "started" toast reads
                  // as a contradictory success+error to the user (reported on
                  // a 2.8 GB file). NOT_ENOUGH_SPACE is the one client failure
                  // the user can act on.
                  toast.error(t(downloadErrorMessageKey(err)));
                } finally {
                  setDownloadProgress(null);
                  setDownloadBytes(null);
                }
              }}
              disabled={downloadProgress !== null}
              style={{
                position: "relative",
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                overflow: "hidden",
                opacity: downloadProgress !== null ? 0.85 : 1,
              }}
              accessible={true}
              accessibilityLabel={t("common.download") || "Download"}
              accessibilityRole="button"
            >
              {downloadProgress !== null && Number.isFinite(downloadProgress) && (
                <View style={{
                  position: "absolute",
                  left: 0,
                  top: 0,
                  bottom: 0,
                  width: `${Math.max(2, Math.min(100, downloadProgress * 100))}%`,
                  backgroundColor: theme.colors.accent,
                  opacity: 0.25,
                }} />
              )}
              <Ionicons
                name={downloadProgress !== null ? "cloud-download" : "download-outline"}
                size={20}
                color={theme.colors.text}
              />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {downloadProgress === null || !id
                  ? t("common.download") || "Download"
                  // Every "what is it doing right now" answer - server-side
                  // audio prep, 720p production, part N of M, assembling -
                  // comes from one place, shared with the Downloads screen.
                  : downloadStageLabel(t, id, downloadProgress, downloadBytes, 0, { formatSize: formatBytes })}
              </Text>
            </Pressable>
          )}
          {isAdmin && id && !isSeries && !isSeason && hasMediaFile(media) && (
            <Pressable
              onPress={() => createShareLink.mutate({ mediaItemId: id, data: { durationHours: 48 } })}
              disabled={createShareLink.isPending}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: createShareLink.isPending ? 0.6 : 1,
              }}
              accessible={true}
              accessibilityLabel={t("media.library.shareLink")}
              accessibilityRole="button"
            >
              <Ionicons name="share-social-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.library.shareLink")}
              </Text>
            </Pressable>
          )}
          {isAdmin && id && (
            <Pressable
              onPress={() => setMetadataSheetOpen(true)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("media.detail.editMetadata")}
              accessibilityRole="button"
            >
              <Ionicons name="create-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.detail.editMetadata")}
              </Text>
            </Pressable>
          )}
          {isAdmin && id && (
            <Pressable
              onPress={() => router.push(`/admin/media/${id}` as never)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("admin.segments.title", "Edit segments")}
              accessibilityRole="button"
            >
              <Ionicons name="settings-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("admin.segments.title", "Edit segments")}
              </Text>
            </Pressable>
          )}
        </View>
        </View>{/* end left column */}

        {/* Right column (landscape) / continues below (portrait) */}
        <View style={isLandscape ? { flex: 7 } : {}}>
        {/* Tagline */}
        {media.tagline ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontStyle: "italic", marginTop: theme.spacing.md }}>
            {media.tagline}
          </Text>
        ) : null}

        {/* Overview */}
        {media.overview ? (
          <Text style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20, marginTop: theme.spacing.sm }}>
            {media.overview}
          </Text>
        ) : null}

        {/* Directors */}
        {directors.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.sm }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.director")}</Text>
            {directors.map((d) => d.name).filter(Boolean).join(", ")}
          </Text>
        ) : null}

        {/* Writers */}
        {writers.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.writer")}</Text>
            {writers.map((w) => w.name).filter(Boolean).join(", ")}
          </Text>
        ) : null}

        {/* Studios */}
        {media.studios && media.studios.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.studio")}</Text>
            {media.studios.join(", ")}
          </Text>
        ) : null}

        {/* Trailers */}
        {sortedTrailers.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: theme.spacing.md }}>
            {sortedTrailers.map((tr, i) => (
              <TrailerButton key={`${tr.key}-${i}`} trailer={tr} />
            ))}
          </View>
        ) : null}

        {/* File info sits here, not at the bottom of the page: web puts it
            immediately after the hero (which carries the synopsis and the
            crew), before the seasons. Buried under cast and similar titles it
            was effectively unreachable on a phone. */}
        <FileInfoSection item={media} />

        {/* Season picker for series */}
        {isSeries && seasons.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.seasons")}
            </Text>
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {seasons.map((season) => {
                const active = season.id === selectedSeason;
                const label = season.indexNumber != null
                  ? t("media.season", { number: season.indexNumber })
                  : (season.title ?? "");
                return (
                  <Pressable
                    key={season.id}
                    onPress={() => setSelectedSeason(season.id ?? null)}
                    style={{
                      paddingHorizontal: theme.spacing.md,
                      paddingVertical: theme.spacing.sm,
                      marginRight: theme.spacing.xs,
                      borderRadius: theme.radii.md,
                      backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                    }}
                    accessible={true}
                    accessibilityLabel={label}
                    accessibilityRole="tab"
                  >
                    <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                      {label}
                    </Text>
                  </Pressable>
                );
              })}
            </ScrollView>
          </View>
        ) : null}

        {/* Episodes list */}
        {(isSeries || isSeason) && episodes.length > 0 ? (
          <View style={{ marginTop: theme.spacing.md }}>
            <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: theme.spacing.sm }}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600" }}>
                {t("media.detail.episodes")}
              </Text>
              {episodes.some((ep) => hasMediaFile(ep)) ? (
                <Pressable
                  onPress={() => { void handleDownloadAllEpisodes(); }}
                  disabled={bulkDownload !== null}
                  style={{ flexDirection: "row", alignItems: "center", gap: 4, opacity: bulkDownload !== null ? 0.6 : 1 }}
                  accessible={true}
                  accessibilityLabel={t("media.detail.downloadAllEpisodes", "Download all")}
                  accessibilityRole="button"
                >
                  {bulkDownload !== null ? (
                    <ActivityIndicator size={14} color={theme.colors.accent} />
                  ) : (
                    <Ionicons name="download-outline" size={16} color={theme.colors.accent} />
                  )}
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
                    {bulkDownload !== null
                      ? `${bulkDownload.done}/${bulkDownload.total}`
                      : t("media.detail.downloadAllEpisodes", "Download all")}
                  </Text>
                </Pressable>
              ) : null}
            </View>
            {episodes.slice(0, visibleEpisodeCount).map((ep) => (
              <EpisodeRow
                key={ep.id}
                item={ep}
                serverUrl={serverUrl}
                theme={theme}
                authHeaders={authHeaders}
                downloadProgress={ep.id ? episodeDownloads[ep.id] : undefined}
                onDownload={handleEpisodeDownload}
                onPlay={handleEpisodePlay}
              />
            ))}
            {episodes.length > visibleEpisodeCount ? (
              <Pressable
                onPress={() => setVisibleEpisodeCount((c) => c + EPISODE_PAGE_SIZE)}
                style={{ paddingVertical: theme.spacing.md, alignItems: "center" }}
                accessible={true}
                accessibilityLabel={t("media.detail.showMoreEpisodes", "Show more")}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.accent, fontSize: 14, fontWeight: "600" }}>
                  {t("media.detail.showMoreEpisodes", "Show more")}
                </Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}

        {/* Cast */}
        {actors.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.cast")}
            </Text>
            <FlatList
              horizontal
              data={actors}
              keyExtractor={personKeyExtractor}
              renderItem={renderCastItem}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Crew */}
        {crew.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.crew")}
            </Text>
            <FlatList
              horizontal
              data={crew}
              keyExtractor={personKeyExtractor}
              renderItem={renderCastItem}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Sagas this title belongs to - just before Similar, as on the web. */}
        {id ? <SagaSection mediaItemId={id} /> : null}

        {/* Similar content */}
        {id && !isSeason ? <SimilarSection item={media} /> : null}

        {/* Plugin modules declared for the media page - same placement as
            web's MediaDetail (after similar titles). */}
        {id ? <PluginSlot name="media.detail.sidebar" context={{ mediaId: id }} /> : null}

        {/* Bottom spacing */}
        <View style={{ height: theme.spacing.xl * 2 }} />
        </View>{/* end right column */}
      </View>

      {id ? (
        <AddToPlaylistSheet
          visible={playlistSheetOpen}
          mediaItemId={id}
          onClose={() => setPlaylistSheetOpen(false)}
        />
      ) : null}

      {id ? (
        <PlayOnSheet
          visible={playOnSheetOpen}
          mediaId={id}
          onClose={() => setPlayOnSheetOpen(false)}
        />
      ) : null}

      {/* Defense in depth: re-check isAdmin at the mount point, not just at the
          toggle button, so a stale collectionSheetOpen=true can never render the
          sheet for a non-admin (e.g. after an admin demotes their own account
          mid-session). */}
      {isAdmin && id ? (
        <BulkAddToCollectionSheet
          visible={collectionSheetOpen}
          mediaItemIds={[id]}
          onClose={() => setCollectionSheetOpen(false)}
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

      {/* Defense in depth: re-check isAdmin at the mount point, not just at the
          toggle button, so a stale metadataSheetOpen=true can never render the
          editor for a non-admin (e.g. after an admin demotes their own account
          mid-session). */}
      {isAdmin && media ? (
        <MetadataEditSheet
          visible={metadataSheetOpen}
          onClose={() => setMetadataSheetOpen(false)}
          item={media}
          onSaved={() => void refetch()}
        />
      ) : null}
    </ScrollView>
  );
}
