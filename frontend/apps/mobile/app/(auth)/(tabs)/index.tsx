import { ScrollView, View, RefreshControl, Text, Pressable, Modal, Switch, StyleSheet, Dimensions } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useCallback, useMemo, useState, useEffect } from "react";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
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
function resolveMaxItems(maxItems: number | undefined, fallback: number): number {
  return maxItems && maxItems > 0 ? maxItems : fallback;
}
import { useAuthStore, toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { MediaRow } from "../../../src/components/MediaRow";
import { useWatchedMap } from "../../../src/hooks/useWatchedMap";
import { useCollections } from "../../../src/api/collections";
import { useMostPopular } from "../../../src/hooks/usePopular";
import { CollectionRow } from "../../../src/components/CollectionRow";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { useServerUrl } from "../../../src/hooks/useServerUrl";

const SCREEN_WIDTH = Dimensions.get("window").width;

interface MusicAlbumLite { id?: string; title?: string; artist?: string; coverArtUrl?: string }
interface PhotoAlbumLite { name?: string; coverPhotoId?: string; photoCount?: number }

function MusicLibraryRow({ libraryId, name, maxItems }: { libraryId: string; name: string; maxItems?: number }) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data, isLoading } = useQuery({
    queryKey: ["music", "albums", libraryId],
    queryFn: () => eveyFetch<{ albums: MusicAlbumLite[] }>({ url: `/api/v1/music/albums?library_id=${libraryId}`, method: "GET" }),
    enabled: !!libraryId,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
  const albums = (data ?? []).slice(0, resolveMaxItems(maxItems, 20));
  if (!isLoading && albums.length === 0) return null;
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  // The album card is 120dp - decoding the full-res original for a strip of
  // up to 20 thumbnails is the exact cost /api/v1/music/album-art/{hash}?w=
  // exists to avoid. 185 is the smallest resize bucket the endpoint supports,
  // still crisp at ~1.5x the card size.
  const artQuery = `?w=185${tokenParam ? `&${tokenParam.slice(1)}` : ""}`;
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
                  // The server returns a relative path like /api/v1/...; web's
                  // CSS background-url resolves that against the current origin
                  // automatically but expo-image refuses relative URIs, so we
                  // have to prefix the active server explicitly. ?w= only
                  // applies to our own album-art endpoint, not external URLs.
                  source={{
                    uri: /^https?:/i.test(a.coverArtUrl)
                      ? `${a.coverArtUrl}${tokenParam}`
                      : `${serverUrl ?? ""}${a.coverArtUrl}${artQuery}`,
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
  const serverUrl = useServerUrl();
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data, isLoading } = useQuery({
    queryKey: ["photos", "albums", libraryId],
    queryFn: () => eveyFetch<{ albums: PhotoAlbumLite[] }>({ url: `/api/v1/photos/albums?library_id=${libraryId}`, method: "GET" }),
    enabled: !!libraryId,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
  const albums = (data ?? []).slice(0, resolveMaxItems(maxItems, 20));
  if (!isLoading && albums.length === 0) return null;
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
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
                    source={{ uri: `${serverUrl}/api/v1/photos/${a.coverPhotoId}/serve${tokenParam}&w=400&fmt=webp` }}
                    style={{ width: "100%", height: "100%" }}
                    contentFit="cover"
                  />
                ) : null}
              </View>
              <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}>{last}</Text>
              <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {a.photoCount ?? 0} {(a.photoCount ?? 0) > 1 ? "photos" : "photo"}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

function LibraryFeedRow({ libraryId, name, libraryType, maxItems, onItemPress }: { libraryId: string; name: string; libraryType?: string; maxItems?: number; onItemPress: (item: V1MediaItem) => void }) {
  // Music + photo libraries don't live in the media_items table - they have
  // their own tables hit via /music/albums and /photos/albums. Fall back to
  // /library/list otherwise (movies/shows).
  if (libraryType === "LIBRARY_TYPE_MUSIC") return <MusicLibraryRow libraryId={libraryId} name={name} maxItems={maxItems} />;
  if (libraryType === "LIBRARY_TYPE_PHOTOS") return <PhotoLibraryRow libraryId={libraryId} name={name} maxItems={maxItems} />;

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
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter as never } : {}),
    "pagination.pageSize": Math.max(40, visibleTarget * 2),
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED" as never,
    sortOrder: "SORT_ORDER_DESC" as never,
  });

  // eslint-disable-next-line react-hooks/rules-of-hooks
  const watchedMap = useWatchedMap();

  // eslint-disable-next-line react-hooks/rules-of-hooks
  const items = useMemo(
    () =>
      (data?.items ?? [])
        .filter((i): i is V1MediaItem => !!i.id && !watchedMap.get(i.id))
        .slice(0, visibleTarget),
    [data?.items, watchedMap, visibleTarget],
  );

  if (!isLoading && items.length === 0) return null;

  return <MediaRow title={name} items={items} onItemPress={onItemPress} />;
}

// "recently_added" is intentionally absent: it duplicated the per-library
// rows below (which already sort by date added), matching web's home which
// never rendered it as a dedicated shelf. Dropping it from FEED_TYPES also
// makes the customize modal treat any already-persisted "recently_added"
// row as an orphan and prune it automatically.
const FEED_TYPES = [
  "resume_playback",
  "next_episode",
  "collections",
  "recommended",
  "most_popular",
] as const;

const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
  recently_added: "media.home.recentlyAdded",
};

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

  const persist = useCallback(async (updated: FeedConfig[]) => {
    let failed = 0;
    for (const c of updated) {
      if (!c.feedType) continue;
      try {
        await updateFeed.mutateAsync({
          feedType: c.feedType,
          data: { profileId, enabled: c.enabled, displayOrder: c.displayOrder },
        });
      } catch { failed++; }
    }
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
    if (failed > 0) toast.error(t("media.home.customizeSaveFailed", "Failed to save some changes"));
  }, [updateFeed, profileId, qc, t]);

  const toggle = (idx: number) => {
    setLocal((prev) => {
      const next = prev.map((c, i) => (i === idx ? { ...c, enabled: !c.enabled } : c));
      persist(next);
      return next;
    });
  };

  const move = (idx: number, dir: -1 | 1) => {
    const target = idx + dir;
    if (target < 0 || target >= local.length) return;
    setLocal((prev) => {
      const arr = [...prev];
      [arr[idx], arr[target]] = [arr[target], arr[idx]];
      const next = arr.map((c, i) => ({ ...c, displayOrder: i }));
      persist(next);
      return next;
    });
  };

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
            onPress={onClose}
            style={[feedStyles.cancelBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, marginTop: 16 }]}
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

  function renderSection(cfg: FeedConfig) {
    const feedType = cfg.feedType ?? "";
    const maxItems = cfg.maxItems && cfg.maxItems > 0 ? cfg.maxItems : undefined;
    switch (feedType) {
      case "resume_playback":
      case "continue_watching":
        return <MediaRow key={feedType} title={t("common.feeds.labels.resume_playback")} items={resume.data?.items ?? []} onItemPress={handleItemPress} watchPercents={watchPercents} />;
      case "next_episode":
        return <MediaRow key={feedType} title={t("media.home.upNext")} items={nextUp.data?.items ?? []} onItemPress={handleItemPress} />;
      case "collections": {
        const allCollections = collections.data?.collections ?? [];
        const cols = maxItems ? allCollections.slice(0, maxItems) : allCollections;
        return <CollectionRow key={feedType} title={t("media.home.collections")} collections={cols} />;
      }
      case "recommended":
        return <MediaRow key={feedType} title={t("media.home.recommendedForYou")} items={recommendedItems} onItemPress={handleItemPress} />;
      case "most_popular":
        return <MediaRow key={feedType} title={t("media.home.mostPopular")} items={popularItems} onItemPress={handleItemPress} />;
      default:
        if (feedType.startsWith("library:")) {
          const libId = feedType.replace("library:", "");
          const lib = (librariesData?.libraries ?? []).find((l) => l.id === libId);
          if (!lib) return null;
          return <LibraryFeedRow key={feedType} libraryId={libId} name={lib.name ?? ""} libraryType={lib.type} maxItems={maxItems} onItemPress={handleItemPress} />;
        }
        if (feedType.startsWith("plugin:")) {
          const pluginId = feedType.replace("plugin:", "");
          return (
            <View key={feedType} style={{ marginVertical: theme.spacing.sm, marginHorizontal: theme.spacing.md }}>
              <PluginSlot name="home.section" pluginId={pluginId} />
            </View>
          );
        }
        return null;
    }
  }

  if (isLoading && !refreshing) {
    const skeletonW = SCREEN_WIDTH * 0.35;
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
              <LibraryFeedRow key={`library:${lib.id}`} libraryId={lib.id!} name={lib.name ?? ""} libraryType={lib.type} onItemPress={handleItemPress} />
            ))}
        </View>
      </ScrollView>
      <FeedCustomizeModal
        visible={showCustomize}
        onClose={() => setShowCustomize(false)}
        configs={feedConfigData?.configs ?? []}
        profileId={profileId}
        libraryNames={libraryNames}
        pluginNames={pluginNames}
      />
    </View>
  );
}

