import { ScrollView, View, RefreshControl, Text, Pressable, Modal, Switch, StyleSheet, Alert } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { toggleFeed, moveFeed, changedFeeds } from "../../../src/lib/feedConfigEdits";
import { useCallback, useMemo, useRef, useState, useEffect } from "react";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
  useFeedServiceResetProfileFeedConfig,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  eveyFetch,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";

interface FeedConfig {
  feedType?: string;
  enabled?: boolean;
  displayOrder?: number;
  maxItems?: number;
}

// Normalizes a configured row size to a usable positive int, falling back to
// the historical default (20) when unset/invalid. Mirrors web's home.lazy.tsx
// (`cfg.maxItems && cfg.maxItems > 0 ? cfg.maxItems : undefined`).
import { useAuthStore, toast, useDemoStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { MediaRow } from "../../../src/components/MediaRow";
import { MediaActionsSheet } from "../../../src/components/MediaActionsSheet";
import { useWatchedMap } from "../../../src/hooks/useWatchedMap";
import { useCollections } from "../../../src/api/collections";
import { useMostPopular } from "../../../src/hooks/usePopular";
import { CollectionRow } from "../../../src/components/CollectionRow";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { coverArtUri } from "../../../src/lib/coverArtUri";
import { FEED_TYPES, resolveMaxItems } from "@evey/client";
import { useRefetchOnReturn } from "@evey/client/native/useRefetchOnReturn";
import { ROW_CARD_WIDTH } from "../../../src/lib/rowCardWidth";


interface MusicAlbumLite { id?: string; title?: string; artist?: string; coverArtUrl?: string }
interface PhotoAlbumLite { name?: string; coverPhotoId?: string; photoCount?: number }

function MusicLibraryRow({ libraryId, name, maxItems }: { libraryId: string; name: string; maxItems?: number }) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["music", "albums", libraryId],
    queryFn: () => eveyFetch<{ albums: MusicAlbumLite[] }>({ url: `/api/v1/music/albums?library_id=${libraryId}`, method: "GET" }),
    enabled: !!libraryId,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
  const albums = (data ?? []).slice(0, resolveMaxItems(maxItems, 20));
  // A failed shelf renders LoadFailed IN PLACE of just that row rather than
  // silently disappearing - matches the convention every other row follows
  // (see renderSection above).
  if (isError) return <LoadFailed inline onRetry={() => void refetch()} />;
  if (!isLoading && albums.length === 0) return null;
  return (
    <View style={{ marginVertical: 8 }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", marginHorizontal: 16, marginBottom: 8 }}>{name}</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 12, gap: 12 }}>
        {albums.map((a) => (
          <Pressable
            key={a.id}
            onPress={() => router.push("/(auth)/(tabs)/music")}
            style={{ width: 120 }}
            accessibilityRole="button"
            accessibilityLabel={a.title ?? ""}
          >
            <View style={{ width: 120, height: 120, borderRadius: theme.radii.md, backgroundColor: theme.colors.surfaceRaised, overflow: "hidden" }}>
              {a.coverArtUrl ? (
                <Image
                  cachePolicy="memory-disk"
                  // The music screen's rule: the server prefixed, a display-
                  // sized ?w= on our own art, another host's cover untouched,
                  // and the token in the header - in the query it reached every
                  // proxy log and changed the URL at each refresh, so the image
                  // cache missed (the posters' fix, 9ab318eeb).
                  source={{
                    uri: coverArtUri(serverUrl, a.coverArtUrl, 120),
                    headers: /^https?:/i.test(a.coverArtUrl) ? undefined : authHeaders,
                  }}
                  style={{ width: "100%", height: "100%" }}
                  contentFit="cover"
                />
              ) : null}
            </View>
            <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}>{a.title}</Text>
            <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: 11 }}>{a.artist}</Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}

function PhotoLibraryRow({ libraryId, name, maxItems }: { libraryId: string; name: string; maxItems?: number }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["photos", "albums", libraryId],
    queryFn: () => eveyFetch<{ albums: PhotoAlbumLite[] }>({ url: `/api/v1/photos/albums?library_id=${libraryId}`, method: "GET" }),
    enabled: !!libraryId,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
  const albums = (data ?? []).slice(0, resolveMaxItems(maxItems, 20));
  // A failed shelf renders LoadFailed IN PLACE of just that row rather than
  // silently disappearing - matches the convention every other row follows
  // (see renderSection above).
  if (isError) return <LoadFailed inline onRetry={() => void refetch()} />;
  if (!isLoading && albums.length === 0) return null;
  return (
    <View style={{ marginVertical: 8 }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", marginHorizontal: 16, marginBottom: 8 }}>{name}</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 12, gap: 12 }}>
        {albums.map((a) => {
          const last = (a.name ?? "").split("/").pop() ?? "";
          return (
            <Pressable
              key={a.name}
              onPress={() => router.push("/(auth)/(tabs)/photos")}
              style={{ width: 120 }}
              accessibilityRole="button"
              accessibilityLabel={last}
            >
              <View style={{ width: 120, height: 120, borderRadius: theme.radii.md, backgroundColor: theme.colors.surfaceRaised, overflow: "hidden" }}>
                {a.coverPhotoId && serverUrl ? (
                  <Image
                    cachePolicy="memory-disk"
                    // Token in the header, like the photos screen: with no
                    // token the query started with "&" and never reached ?w=.
                    source={{ uri: `${serverUrl}/api/v1/photos/${a.coverPhotoId}/serve?w=400&fmt=webp`, headers: authHeaders }}
                    style={{ width: "100%", height: "100%" }}
                    contentFit="cover"
                  />
                ) : null}
              </View>
              <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}>{last}</Text>
              <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {t("media.photos.photoCount", { count: a.photoCount ?? 0 })}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

// Pure dispatch, no hooks: which row component a library gets. The media
// row's hooks live in MediaLibraryRow so no hook ever sits below an early
// return - a library whose type changes under a mounted row would otherwise
// crash with "Rendered fewer hooks than expected" (the eslint-disable comments
// that used to sit here hid exactly that from the lint gate).
function LibraryFeedRow({ libraryId, name, libraryType, maxItems, onItemPress, onItemLongPress }: { libraryId: string; name: string; libraryType?: string; maxItems?: number; onItemPress: (item: V1MediaItem) => void; onItemLongPress?: (item: V1MediaItem) => void }) {
  // Music + photo libraries don't live in the media_items table - they have
  // their own tables hit via /music/albums and /photos/albums. Fall back to
  // /library/list otherwise (movies/shows).
  if (libraryType === "LIBRARY_TYPE_MUSIC") return <MusicLibraryRow libraryId={libraryId} name={name} maxItems={maxItems} />;
  if (libraryType === "LIBRARY_TYPE_PHOTOS") return <PhotoLibraryRow libraryId={libraryId} name={name} maxItems={maxItems} />;
  return <MediaLibraryRow libraryId={libraryId} name={name} libraryType={libraryType} maxItems={maxItems} onItemPress={onItemPress} onItemLongPress={onItemLongPress} />;
}

function MediaLibraryRow({ libraryId, name, libraryType, maxItems, onItemPress, onItemLongPress }: { libraryId: string; name: string; libraryType?: string; maxItems?: number; onItemPress: (item: V1MediaItem) => void; onItemLongPress?: (item: V1MediaItem) => void }) {
  const typeFilter = libraryType === "LIBRARY_TYPE_SHOWS"
    ? "MEDIA_ITEM_TYPE_SERIES"
    : libraryType === "LIBRARY_TYPE_MOVIES"
      ? "MEDIA_ITEM_TYPE_MOVIE"
      : undefined;
  const visibleTarget = resolveMaxItems(maxItems, 20);
  // Over-fetch: the server list doesn't filter by watched state, so pull a
  // wider page than the row actually shows and filter client-side below,
  // keeping the row full after watched items are dropped. The fetched window
  // scales with maxItems so a configured larger row doesn't come up short
  // after the watched-filter. Mirrors web's MediaItemsLibrarySlider (home.lazy.tsx).
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter as never } : {}),
    "pagination.pageSize": Math.max(40, visibleTarget * 2),
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED" as never,
    sortOrder: "SORT_ORDER_DESC" as never,
  });

  const watchedMap = useWatchedMap();

  const items = useMemo(
    () =>
      (data?.items ?? [])
        .filter((i): i is V1MediaItem => !!i.id && !watchedMap.get(i.id))
        .slice(0, visibleTarget),
    [data?.items, watchedMap, visibleTarget],
  );

  if (!isLoading && items.length === 0) return null;

  // "See all" jumps to the full library grid (web parity: per-library sliders
  // link to /library/<id>).
  return <MediaRow title={name} seeAllHref={`/library/${libraryId}`} items={items} onItemPress={onItemPress} onItemLongPress={onItemLongPress} />;
}

// "recently_added" is intentionally absent: it duplicated the per-library
// rows below (which already sort by date added), matching web's home which
// never rendered it as a dedicated shelf. Dropping it from FEED_TYPES also
// makes the customize modal treat any already-persisted "recently_added"
// row as an orphan and prune it automatically.

const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
  recently_added: "media.home.recentlyAdded",
};

// One array, shared, for "the feed config has not arrived yet". Written as a
// module constant because `?? []` builds a NEW array on every render, and this
// value is a dependency of the modal's init effect: with the panel OPEN and
// the config still loading, every parent render handed it a different empty
// array, the effect re-ran, setLocal re-rendered, and round it went. React
// stops that with "Maximum update depth exceeded" - the screen dies rather
// than spins, and it dies exactly for the viewer who opened Customize before
// the query came back.
const NO_FEED_CONFIGS: FeedConfig[] = [];

function FeedCustomizeModal({
  visible,
  onClose,
  configs,
  profileId,
  libraryNames,
  pluginNames,
}: {
  visible: boolean;
  onClose: () => void;
  configs: FeedConfig[];
  profileId: string;
  libraryNames: Record<string, string>;
  pluginNames: Record<string, string>;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const updateFeed = useFeedServiceUpdateProfileFeedConfig();
  const resetFeed = useFeedServiceResetProfileFeedConfig();

  const [local, setLocal] = useState<FeedConfig[]>([]);

  useEffect(() => {
    if (visible) {
      // Drop orphan rows the user can no longer act on - without this filter
      // the customize panel surfaces stale feed types as raw UUIDs/IDs with
      // no way to dismiss them. Mirrors the web fix in 928a9b9d6.
      const knownTypes = new Set<string>(FEED_TYPES);
      const filtered = configs.filter((c) => {
        const ft = c.feedType ?? "";
        if (knownTypes.has(ft)) return true;
        if (ft.startsWith("library:")) return !!libraryNames[ft.replace("library:", "")];
        if (ft.startsWith("plugin:")) return !!pluginNames[ft.replace("plugin:", "")];
        return false;
      });

      // Prepend installed home.section plugins that are not yet in the
      // feed config. Negative displayOrder puts them at the top so the user
      // sees newly-installed plugins immediately; the persist step
      // renumbers contiguously when they reorder. Mirrors web behaviour.
      const haveAlready = new Set(filtered.map((c) => c.feedType));
      const minOrder = filtered.reduce(
        (m, c) => Math.min(m, c.displayOrder ?? 0),
        0,
      );
      const installedHomeSlots = Object.keys(pluginNames);
      const newPlugins: FeedConfig[] = installedHomeSlots
        .filter((id) => !haveAlready.has("plugin:" + id))
        .map((id, i) => ({
          feedType: "plugin:" + id,
          enabled: true,
          displayOrder: minOrder - installedHomeSlots.length + i,
        }));

      const merged = [...newPlugins, ...filtered];
      const sorted = [...merged].sort(
        (a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0),
      );
      if (sorted.length === 0) {
        setLocal(FEED_TYPES.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i })));
      } else {
        setLocal(sorted);
      }
    }
  }, [visible, configs, libraryNames, pluginNames]);

  // Persists the rows a click changed - one PUT per changed feed, in
  // parallel - and never from inside a setState updater (StrictMode runs
  // updaters twice, which doubled every PUT before).
  const persist = useCallback(async (changed: FeedConfig[]) => {
    if (changed.length === 0) return;
    const results = await Promise.allSettled(
      changed
        .filter((c) => !!c.feedType)
        .map((c) => updateFeed.mutateAsync({
          feedType: c.feedType as string,
          data: { profileId, enabled: c.enabled, displayOrder: c.displayOrder },
        })),
    );
    const failed = results.filter((r) => r.status === "rejected").length;
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
    if (failed > 0) toast.error(t("media.home.customizeSaveFailed", "Failed to save some changes"));
  }, [updateFeed, profileId, qc, t]);

  // Two taps can land in one React batch (a fast double tap): each handler
  // would then see the same `local` and the second would silently discard
  // the first. The ref is the handlers' source of truth between renders - it
  // is advanced synchronously by each tap and re-synced from state on every
  // render (a reset or a config reload lands in it too).
  const localRef = useRef(local);
  localRef.current = local;

  const toggle = (idx: number) => {
    const prev = localRef.current;
    const next = toggleFeed(prev, idx);
    localRef.current = next;
    setLocal(next);
    void persist(changedFeeds(prev, next));
  };

  const move = (idx: number, dir: -1 | 1) => {
    const prev = localRef.current;
    const next = moveFeed(prev, idx, dir);
    if (next === prev) return;
    localRef.current = next;
    setLocal(next);
    void persist(changedFeeds(prev, next));
  };

  const resetAll = useCallback(() => {
    Alert.alert(
      t("common.feeds.resetDefaults", "Reset to Defaults"),
      t("common.feeds.resetConfirm", "Reset your feed customization to defaults?"),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.feeds.resetDefaults", "Reset to Defaults"),
          style: "destructive",
          onPress: async () => {
            for (const c of local) {
              if (!c.feedType) continue;
              try {
                await resetFeed.mutateAsync({ feedType: c.feedType });
              } catch { /* best-effort, mirrors persist() above */ }
            }
            await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
          },
        },
      ],
    );
  }, [local, resetFeed, qc, t]);

  const feedLabel = (feedType: string) => {
    if (feedType.startsWith("library:")) {
      const libId = feedType.replace("library:", "");
      return libraryNames[libId] || libId;
    }
    if (feedType.startsWith("plugin:")) {
      const pluginId = feedType.replace("plugin:", "");
      const raw = pluginNames[pluginId] || pluginId;
      // Plugin manifests don't ship a separate display name yet, so the
      // raw `name` field bleeds through into the UI ("weather" → "Weather").
      // Title-case it client-side so the customize panel doesn't show
      // lower-case identifiers next to nicely formatted shelf labels.
      return raw.charAt(0).toUpperCase() + raw.slice(1);
    }
    return t(FEED_LABELS[feedType] ?? feedType);
  };

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={[feedStyles.backdrop, { backgroundColor: "rgba(0,0,0,0.5)" }]}>
        <View style={[feedStyles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[feedStyles.sheetTitle, { color: theme.colors.text }]}>
            {t("media.home.customizeFeed") || "Customize Feed"}
          </Text>
          <ScrollView style={{ maxHeight: 400 }}>
            {local.map((c, i) => (
              <View key={c.feedType} style={[feedStyles.row, { borderBottomColor: theme.colors.border }]}>
                <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                  <Pressable
                    onPress={() => move(i, -1)}
                    disabled={i === 0}
                    hitSlop={8}
                    accessible
                    accessibilityRole="button"
                    accessibilityLabel={t("common.feeds.moveUp", "Move section up")}
                  >
                    <Ionicons name="chevron-up" size={18} color={i === 0 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                  <Pressable
                    onPress={() => move(i, 1)}
                    disabled={i === local.length - 1}
                    hitSlop={8}
                    accessible
                    accessibilityRole="button"
                    accessibilityLabel={t("common.feeds.moveDown", "Move section down")}
                  >
                    <Ionicons name="chevron-down" size={18} color={i === local.length - 1 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                </View>
                <Text style={[feedStyles.feedLabel, { color: theme.colors.text }]}>
                  {feedLabel(c.feedType ?? "")}
                </Text>
                <Switch
                  value={c.enabled}
                  onValueChange={() => toggle(i)}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>
            ))}
          </ScrollView>
          <Pressable
            onPress={resetAll}
            style={[feedStyles.resetBtn, { marginTop: 16 }]}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("common.feeds.resetDefaults", "Reset to Defaults")}
          >
            <Ionicons name="refresh-outline" size={14} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("common.feeds.resetDefaults", "Reset to Defaults")}</Text>
          </Pressable>
          <Pressable
            onPress={onClose}
            style={[feedStyles.cancelBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, marginTop: 8 }]}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("common.close") || "Close"}
          >
            <Text style={{ color: theme.colors.text, fontWeight: "500", fontSize: 15 }}>{t("common.close") || "Close"}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const feedStyles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { padding: 20, paddingBottom: 32 },
  sheetTitle: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, gap: 12 },
  feedLabel: { flex: 1, fontSize: 14, fontWeight: "500" },
  saveBtn: { paddingVertical: 12, alignItems: "center" },
  cancelBtn: { paddingVertical: 12, alignItems: "center" },
  resetBtn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, paddingVertical: 8 },
});

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId) ?? "";

  const { data: feedConfigData } = useFeedServiceGetFeedConfig({ profileId });
  const { data: librariesData } = useLibraryServiceListLibraries();

  // Computed ahead of the section data hooks below so their `limit`/`maxItems`
  // can honor the admin-configured row size instead of a figée 20 - the user
  // sets this per-section in Customize and previously saw no effect (see
  // web's home.lazy.tsx, which already honors cfg.maxItems).
  const feedConfigs = useMemo(() => {
    const configs = feedConfigData?.configs ?? [];
    if (configs.length === 0) {
      return FEED_TYPES.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i, maxItems: undefined as number | undefined }));
    }
    return [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0));
  }, [feedConfigData]);

  const maxItemsByType = useMemo(() => {
    const map: Record<string, number> = {};
    for (const c of feedConfigs) {
      if (c.feedType && c.maxItems && c.maxItems > 0) map[c.feedType] = c.maxItems;
    }
    return map;
  }, [feedConfigs]);

  const resume = usePlaybackServiceGetResumeItems({ limit: resolveMaxItems(maxItemsByType.resume_playback, 20) });
  const nextUp = usePlaybackServiceGetNextUp({ limit: resolveMaxItems(maxItemsByType.next_episode, 20) });
  // The home tab is never unmounted and the player only invalidates these rows
  // when a title is marked watched: after a PARTIAL viewing the resume point
  // and the next episode on screen were the ones from before the film.
  useRefetchOnReturn(() => {
    void resume.refetch();
    void nextUp.refetch();
  });
  const recommended = useFeedServiceRecommended({ limit: resolveMaxItems(maxItemsByType.recommended, 20) });
  const popular = useMostPopular(resolveMaxItems(maxItemsByType.most_popular, 20));
  const collections = useCollections();
  const libraryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const lib of librariesData?.libraries ?? []) {
      if (lib.id && lib.name) map[lib.id] = lib.name;
    }
    return map;
  }, [librariesData]);

  // List of currently-installed plugins that ship a `home.section` frontend
  // module (anything else can't render in the home feed). Used by the
  // customize panel to (a) auto-prepend newly-installed plugins, (b) drop
  // stale plugin: rows whose plugin was uninstalled, and (c) show a
  // human-readable name instead of the raw plg-* id.
  type PluginEntry = {
    id?: string;
    name?: string;
    frontend?: { slot?: string }[];
  };
  const { data: pluginsData } = useQuery<{ plugins?: PluginEntry[] }>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
  });
  const pluginNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const p of pluginsData?.plugins ?? []) {
      if (!p.id || !p.name) continue;
      if (!(p.frontend ?? []).some((m) => m.slot === "home.section")) continue;
      map[p.id] = p.name;
    }
    return map;
  }, [pluginsData]);

  const [refreshing, setRefreshing] = useState(false);
  const [showCustomize, setShowCustomize] = useState(false);
  const demo = useDemoStore((st) => st.demoMode);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([
      resume.refetch(),
      nextUp.refetch(),
      recommended.refetch(),
      popular.refetch(),
      collections.refetch(),
    ]);
    setRefreshing(false);
  }, [resume, nextUp, recommended, popular, collections]);

  const isLoading =
    resume.isLoading ||
    nextUp.isLoading ||
    recommended.isLoading ||
    popular.isLoading ||
    collections.isLoading;

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) {
        map[p.mediaItemId] = p.percentComplete;
      }
    });
    return map;
  }, [resume.data?.progress]);

  // Synthesize a minimal artwork[] from the FeedItem.primaryBlurhash so the
  // MediaCard placeholder path (which reads item.artwork) works without the
  // mobile MediaRow having to grow a parallel blurhash prop. The card's
  // hash extractor is already tolerant of unknown fields on artwork.
  const recommendedItems = useMemo(
    () =>
      (recommended.data?.items ?? [])
        .filter((i) => !!i.mediaItemId)
        .map((i) => ({
          id: i.mediaItemId,
          title: i.title,
          type: i.type,
          artwork: i.primaryBlurhash ? [{ type: "ARTWORK_TYPE_PRIMARY", blurhash: i.primaryBlurhash }] : undefined,
        }) as V1MediaItem),
    [recommended.data?.items],
  );

  const popularItems = useMemo(
    () =>
      (popular.data ?? []).map(
        (i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem,
      ),
    [popular.data],
  );

  const handleItemPress = useCallback((item: V1MediaItem) => {
    router.push(`/media/${item.id}`);
  }, []);
  // Long press on a card: the web card's context menu, as a sheet.
  const [actionItem, setActionItem] = useState<V1MediaItem | null>(null);
  const closeActions = useCallback(() => setActionItem(null), []);

  // A failed shelf renders LoadFailed IN PLACE of just that row rather than
  // hiding the whole home screen (renderSection is called per-section) - one
  // dead endpoint (e.g. recommendations) must not take resume/collections/etc
  // down with it. Each row keeps its own retry, wired to that row's refetch.
  function renderSection(cfg: FeedConfig) {
    const feedType = cfg.feedType ?? "";
    const maxItems = cfg.maxItems && cfg.maxItems > 0 ? cfg.maxItems : undefined;
    switch (feedType) {
      case "resume_playback":
      case "continue_watching":
        if (resume.isError) return <LoadFailed key={feedType} inline onRetry={() => void resume.refetch()} />;
        return <MediaRow key={feedType} title={t("common.feeds.labels.resume_playback")} items={resume.data?.items ?? []} onItemPress={handleItemPress} onItemLongPress={setActionItem} watchPercents={watchPercents} />;
      case "next_episode":
        if (nextUp.isError) return <LoadFailed key={feedType} inline onRetry={() => void nextUp.refetch()} />;
        return <MediaRow key={feedType} title={t("media.home.upNext")} items={nextUp.data?.items ?? []} onItemPress={handleItemPress} onItemLongPress={setActionItem} />;
      case "collections": {
        if (collections.isError) return <LoadFailed key={feedType} inline onRetry={() => void collections.refetch()} />;
        const allCollections = collections.data?.collections ?? [];
        const cols = maxItems ? allCollections.slice(0, maxItems) : allCollections;
        return <CollectionRow key={feedType} title={t("media.home.collections")} collections={cols} seeAllHref="/collections" />;
      }
      case "recommended":
        if (recommended.isError) return <LoadFailed key={feedType} inline onRetry={() => void recommended.refetch()} />;
        return <MediaRow key={feedType} title={t("media.home.recommendedForYou")} items={recommendedItems} onItemPress={handleItemPress} onItemLongPress={setActionItem} />;
      case "most_popular":
        if (popular.isError) return <LoadFailed key={feedType} inline onRetry={() => void popular.refetch()} />;
        return <MediaRow key={feedType} title={t("media.home.mostPopular")} items={popularItems} onItemPress={handleItemPress} onItemLongPress={setActionItem} />;
      default:
        if (feedType.startsWith("library:")) {
          const libId = feedType.replace("library:", "");
          const lib = (librariesData?.libraries ?? []).find((l) => l.id === libId);
          if (!lib) return null;
          return <LibraryFeedRow key={feedType} libraryId={libId} name={lib.name ?? ""} libraryType={lib.type} maxItems={maxItems} onItemPress={handleItemPress} onItemLongPress={setActionItem} />;
        }
        if (feedType.startsWith("plugin:")) {
          const pluginId = feedType.replace("plugin:", "");
          // A row whose plugin is gone (every past install of a widget leaves
          // its plugin:<id> behind in the profile's feed config) must not keep
          // its wrapper: five such rows put 80 dp of nothing above the first
          // widget. The slot itself renders null, so the margins were the gap.
          if (pluginsData && !pluginNames[pluginId]) return null;
          return (
            // The rows' own bottom gap (MediaRow: spacing.lg), so the next row
            // does not sit flush under the widget.
            <View key={feedType} testID="home-plugin-section" style={{ marginBottom: theme.spacing.lg, marginHorizontal: theme.spacing.md }}>
              <PluginSlot name="home.section" pluginId={pluginId} />
            </View>
          );
        }
        return null;
    }
  }

  // Nothing to show once everything has loaded: no library, no feed row, no
  // plugin section. Web shows an EmptyState here rather than a blank page under
  // the Customize button (home.lazy.tsx hasContent). A plugin home section can
  // still carry its own content, so its presence counts too.
  const homeHasContent =
    (resume.data?.items?.length ?? 0) > 0 ||
    (nextUp.data?.items?.length ?? 0) > 0 ||
    recommendedItems.length > 0 ||
    popularItems.length > 0 ||
    (collections.data?.collections?.length ?? 0) > 0 ||
    (librariesData?.libraries?.length ?? 0) > 0 ||
    Object.keys(pluginNames).length > 0;

  if (isLoading && !refreshing) {
    const skeletonW = ROW_CARD_WIDTH;
    const skeletonH = skeletonW * 1.5;
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <View style={{ paddingTop: theme.spacing.lg }}>
          {Array.from({ length: 4 }).map((_, row) => (
            <View key={row} style={{ marginBottom: theme.spacing.lg }}>
              <Skeleton width={140} height={18} borderRadius={4} style={{ marginLeft: theme.spacing.md, marginBottom: theme.spacing.sm }} />
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: theme.spacing.md }}>
                {Array.from({ length: 4 }).map((_, col) => (
                  <View key={col} style={{ marginRight: theme.spacing.sm }}>
                    <Skeleton width={skeletonW} height={skeletonH} borderRadius={theme.radii.md} />
                    <Skeleton width={skeletonW * 0.7} height={13} borderRadius={4} style={{ marginTop: 4 }} />
                  </View>
                ))}
              </ScrollView>
            </View>
          ))}
        </View>
      </View>
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <ScrollView
        style={{ flex: 1 }}
        contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
      >
        <View style={{ paddingTop: theme.spacing.md }}>
          {/* The home layout is the shared profile's on the demo: not offered. */}
          {demo ? null : (
          <Pressable
            onPress={() => setShowCustomize(true)}
            style={{ alignSelf: "flex-end", marginRight: theme.spacing.md, marginBottom: theme.spacing.sm, flexDirection: "row", alignItems: "center", gap: 4 }}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("common.feeds.customize")}
          >
            <Ionicons name="options-outline" size={16} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("media.home.customize") || "Customize"}</Text>
          </Pressable>
          )}
          {/* Plugins mounting the page hero - same strip-above-every-feed
              placement as the web home. */}
          <View style={{ marginHorizontal: theme.spacing.md }}>
            <PluginSlot name="home.hero" />
          </View>
          {/* Installed home.section plugins the user has NOT yet added to their
              feed render at the TOP, matching both the web home and the
              customize modal (which auto-prepends new plugins with a negative
              displayOrder). Rendering them at the bottom made the weather
              widget land under every feed row - the user had to reorder it by
              hand. A home.section plugin is a hero widget; it belongs first. */}
          {Object.keys(pluginNames)
            .filter((id) => !feedConfigs.some((c) => c.feedType === `plugin:${id}`))
            .map((id) => (
              <View key={`plugin:${id}`} style={{ marginVertical: theme.spacing.sm, marginHorizontal: theme.spacing.md }}>
                <PluginSlot name="home.section" pluginId={id} />
              </View>
            ))}
          {feedConfigs.filter((c) => c.enabled !== false).map((c) => renderSection(c))}
          {(librariesData?.libraries ?? [])
            .filter((lib) => lib.id && !feedConfigs.some((c) => c.feedType === `library:${lib.id}`))
            .map((lib) => (
              <LibraryFeedRow key={`library:${lib.id}`} libraryId={lib.id!} name={lib.name ?? ""} libraryType={lib.type} onItemPress={handleItemPress} onItemLongPress={setActionItem} />
            ))}
          {!homeHasContent && (
            <View style={{ alignItems: "center", paddingVertical: theme.spacing.xl * 2, paddingHorizontal: theme.spacing.xl }}>
              <Ionicons name="film-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginTop: theme.spacing.md, textAlign: "center" }}>
                {t("media.home.emptyLibrary")}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 6, textAlign: "center" }}>
                {t("media.home.emptyLibraryDesc")}
              </Text>
              <Pressable
                onPress={() => router.push("/(auth)/(tabs)/library")}
                style={{ marginTop: theme.spacing.lg, backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: theme.spacing.sm, paddingHorizontal: theme.spacing.lg }}
                accessibilityRole="button"
                accessibilityLabel={t("media.home.goToLibraries")}
              >
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("media.home.goToLibraries")}</Text>
              </Pressable>
            </View>
          )}
        </View>
      </ScrollView>
      <FeedCustomizeModal
        visible={showCustomize}
        onClose={() => setShowCustomize(false)}
        configs={feedConfigData?.configs ?? NO_FEED_CONFIGS}
        profileId={profileId}
        libraryNames={libraryNames}
        pluginNames={pluginNames}
      />
      <MediaActionsSheet item={actionItem} onClose={closeActions} />
    </View>
  );
}

