import { useCallback, useMemo, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  TouchableOpacity,
  Pressable,
  ScrollView,
  Dimensions,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import { useAuthStore, useDemoStore, toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { invalidateWatchedState } from "@evey/client";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { useFavoritesList, type FavoriteItem } from "../../../src/hooks/useFavorite";
import { BulkAddToPlaylistSheet, BulkAddToCollectionSheet } from "../../../src/components/library/BulkAddSheets";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;
const ROW_HEIGHT = POSTER_HEIGHT + 30 + SPACING; // poster + text + margin

type SortKey = "addedAt" | "titleAsc" | "titleDesc" | "yearNewest" | "yearOldest" | "rating";
type TypeFilter = "all" | "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";

function favKeyExtractor(item: FavoriteItem) {
  return item.mediaItemId;
}

// FlatList hands getItemLayout the ROW index when numColumns > 1, so dividing by
// NUM_COLUMNS here divided it twice: consecutive rows reported the same offset,
// and the grid scrolled in jumps instead of flowing. Same defect as the library
// screen, fixed together on 2026-09-04.
const favGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * index,
  index,
});

export default function FavoritesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const { data: favorites, isLoading, isError, isRefetching, refetch } = useFavoritesList();

  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const [sortKey, setSortKey] = useState<SortKey>("addedAt");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [bulkMode, setBulkMode] = useState(false);
  const demo = useDemoStore((st) => st.demoMode);
  // One bulk action at a time: the handlers are sequential per-item loops, and
  // a second press while one runs interleaves two loops over the same
  // selection (half watched / half unwatched - a state nobody asked for).
  const [bulkBusy, setBulkBusy] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [showBulkPlaylist, setShowBulkPlaylist] = useState(false);
  const [showBulkCollection, setShowBulkCollection] = useState(false);

  const qc = useQueryClient();
  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: () => toast.success(t("media.library.markedWatched", "Marked watched")) } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: () => toast.success(t("media.library.markedUnwatched", "Marked unwatched")) } });

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      if (next.size === 0) setBulkMode(false);
      return next;
    });
  }, []);

  const exitBulk = useCallback(() => { setBulkMode(false); setSelected(new Set()); }, []);

  const bulkMarkWatched = useCallback(async () => {
    if (bulkBusy) return;
    setBulkBusy(true);
    let failed = 0;
    for (const id of selected) {
      try { await markWatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    invalidateWatchedState(qc);
    setBulkBusy(false);
    exitBulk();
  }, [selected, markWatched, profileId, exitBulk, t, bulkBusy, qc]);

  const bulkMarkUnwatched = useCallback(async () => {
    if (bulkBusy) return;
    setBulkBusy(true);
    let failed = 0;
    for (const id of selected) {
      try { await markUnwatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    invalidateWatchedState(qc);
    setBulkBusy(false);
    exitBulk();
  }, [selected, markUnwatched, profileId, exitBulk, t, bulkBusy, qc]);

  const sortOptions: Array<{ key: SortKey; label: string }> = [
    { key: "addedAt", label: t("media.search.dateAdded") || "Date added" },
    { key: "titleAsc", label: t("media.search.titleAZ") || "Title A-Z" },
    { key: "titleDesc", label: t("media.favorites.titleZA") || "Title Z-A" },
    { key: "yearNewest", label: t("media.search.yearNewest") || "Year newest" },
    { key: "yearOldest", label: t("media.favorites.yearOldest") || "Year oldest" },
    { key: "rating", label: t("media.favorites.rating") || "Rating" },
  ];

  const typeOptions: Array<{ key: TypeFilter; label: string }> = [
    { key: "all", label: t("media.search.all") || "All" },
    { key: "MEDIA_ITEM_TYPE_MOVIE", label: t("media.search.movies") || "Movies" },
    { key: "MEDIA_ITEM_TYPE_SERIES", label: t("media.search.series") || "Series" },
  ];

  const displayItems = useMemo(() => {
    let items = favorites ?? [];

    if (typeFilter !== "all") {
      items = items.filter((i) => i.type === typeFilter);
    }

    if (debouncedSearch.trim()) {
      const lower = debouncedSearch.toLowerCase();
      items = items.filter((i) => i.title?.toLowerCase().includes(lower));
    }

    const sorted = [...items];
    switch (sortKey) {
      case "addedAt":
        sorted.sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
        break;
      case "titleAsc":
        sorted.sort((a, b) => (a.title ?? "").localeCompare(b.title ?? ""));
        break;
      case "titleDesc":
        sorted.sort((a, b) => (b.title ?? "").localeCompare(a.title ?? ""));
        break;
      case "yearNewest":
        sorted.sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
        break;
      case "yearOldest":
        sorted.sort((a, b) => (a.year ?? 0) - (b.year ?? 0));
        break;
      case "rating":
        sorted.sort((a, b) => (b.communityRating ?? 0) - (a.communityRating ?? 0));
        break;
    }
    return sorted;
  }, [favorites, typeFilter, debouncedSearch, sortKey]);

  const renderItem = useCallback(
    ({ item }: { item: FavoriteItem }) => {
      const posterUri = serverUrl
        ? `${serverUrl}/api/v1/media/${item.mediaItemId}/artwork/poster?w=400`
        : undefined;
      const isSelected = bulkMode && selected.has(item.mediaItemId);
      return (
        <TouchableOpacity
          style={[styles.gridItem, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING, opacity: bulkMode && !isSelected ? 0.5 : 1 }]}
          activeOpacity={0.7}
          onPress={() => bulkMode ? toggleSelect(item.mediaItemId) : router.push(`/media/${item.mediaItemId}`)}
          // No bulk selection on the demo: its only actions (watched marks)
          // are the shared account's, which the server refuses.
          onLongPress={() => !bulkMode && !demo && (setBulkMode(true), setSelected(new Set([item.mediaItemId])))}
          accessible={true}
          accessibilityLabel={item.title ?? "Untitled"}
          accessibilityRole="button"
        >
          {isSelected && (
            <View style={styles.checkmark}>
              <Ionicons name="checkmark-circle" size={22} color={theme.colors.accent} />
            </View>
          )}
          {posterUri ? (
            // Decorative: the enclosing TouchableOpacity already carries the
            // media title as its accessibilityLabel.
            <Image
              source={{ uri: posterUri, headers: authHeaders }}
              style={{ width: ITEM_WIDTH, height: POSTER_HEIGHT, borderRadius: theme.radii.sm }}
              contentFit="cover"
              transition={200}
              accessible={false}
              accessibilityElementsHidden={true}
              importantForAccessibility="no-hide-descendants"
            />
          ) : (
            <View
              style={{
                width: ITEM_WIDTH,
                height: POSTER_HEIGHT,
                borderRadius: theme.radii.sm,
                backgroundColor: theme.colors.surfaceRaised,
                justifyContent: "center",
                alignItems: "center",
              }}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("common.noPoster")}</Text>
            </View>
          )}
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {item.title ?? "Untitled"}
          </Text>
          {!!item.year && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, textAlign: "center" }}>
              {item.year}
            </Text>
          )}
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, serverUrl, t, bulkMode, selected, toggleSelect, demo],
  );

  if (isError) {
    return <LoadFailed onRetry={() => void refetch()} />;
  }

  if (isLoading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl }]}>
          {t("media.favorites.title")}
        </Text>
        <View style={{ flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING }}>
          {Array.from({ length: 9 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING, alignItems: "center", width: ITEM_WIDTH }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl }]}>
        {t("media.favorites.title")}
      </Text>

      {/* Search input */}
      <TextInput
        style={[
          styles.searchInput,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.sm,
          },
        ]}
        placeholder={t("media.library.filterByName") || "Search..."}
        placeholderTextColor={theme.colors.textMuted}
        value={searchText}
        onChangeText={setSearchText}
        autoCapitalize="none"
        autoCorrect={false}
        accessible={true}
        accessibilityLabel={t("media.library.filterByName") || "Search..."}
      />

      {/* Type filter chips */}
      <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.chipRow}>
        {typeOptions.map((opt) => (
          <TouchableOpacity
            key={opt.key}
            style={[
              styles.chip,
              {
                backgroundColor: typeFilter === opt.key ? theme.colors.accent : theme.colors.surfaceRaised,
                borderRadius: theme.radii.sm,
              },
            ]}
            activeOpacity={0.7}
            onPress={() => setTypeFilter(opt.key)}
            accessible={true}
            accessibilityLabel={opt.label}
            accessibilityRole="radio"
            accessibilityState={{ selected: typeFilter === opt.key }}
          >
            <Text style={{ color: typeFilter === opt.key ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>
              {opt.label}
            </Text>
          </TouchableOpacity>
        ))}
      </ScrollView>

      {/* Sort chips */}
      <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.chipRow}>
        {sortOptions.map((opt) => (
          <TouchableOpacity
            key={opt.key}
            style={[
              styles.chip,
              {
                backgroundColor: sortKey === opt.key ? theme.colors.accent : theme.colors.surfaceRaised,
                borderRadius: theme.radii.sm,
              },
            ]}
            activeOpacity={0.7}
            onPress={() => setSortKey(opt.key)}
            accessible={true}
            accessibilityLabel={opt.label}
            accessibilityRole="radio"
            accessibilityState={{ selected: sortKey === opt.key }}
          >
            <Text style={{ color: sortKey === opt.key ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>
              {opt.label}
            </Text>
          </TouchableOpacity>
        ))}
      </ScrollView>

      {displayItems.length > 0 && (
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingHorizontal: 16, paddingTop: 8 }}>
          {t("common.itemCount", "{{count}} items", { count: displayItems.length })}
        </Text>
      )}

      <FlatList
        data={displayItems}
        keyExtractor={favKeyExtractor}
        renderItem={renderItem}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ paddingTop: SPACING }}
        refreshing={isRefetching}
        onRefresh={refetch}
        getItemLayout={favGetItemLayout}
        initialNumToRender={9}
        maxToRenderPerBatch={12}
        windowSize={5}
        removeClippedSubviews={true}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>{t("media.favorites.empty")}</Text>
          </View>
        }
      />
      {bulkMode && (
        <View style={[styles.bulkBar, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 12 }}>
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("common.selectedCount", "{{count}} selected", { count: selected.size })}</Text>
            <Pressable
              onPress={() => setSelected(new Set(displayItems.map((i) => i.mediaItemId)))}
              hitSlop={BULK_HIT_SLOP}
              accessibilityRole="button"
              accessibilityLabel={t("common.selectAll", "Select all")}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 12 }}>{t("common.selectAll", "Select all")}</Text>
            </Pressable>
          </View>
          <View style={{ flexDirection: "row", gap: 12 }}>
            <Pressable onPress={bulkMarkWatched} disabled={bulkBusy} hitSlop={BULK_HIT_SLOP} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible={true} accessibilityLabel={t("media.library.markWatched", "Mark watched")} accessibilityRole="button">
              <Ionicons name="checkmark-circle-outline" size={20} color={theme.colors.accent} />
            </Pressable>
            <Pressable onPress={bulkMarkUnwatched} disabled={bulkBusy} hitSlop={BULK_HIT_SLOP} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible={true} accessibilityLabel={t("media.library.markUnwatched", "Mark unwatched")} accessibilityRole="button">
              <Ionicons name="eye-off-outline" size={20} color={theme.colors.textMuted} />
            </Pressable>
            <Pressable onPress={() => setShowBulkPlaylist(true)} disabled={bulkBusy} hitSlop={BULK_HIT_SLOP} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible={true} accessibilityLabel={t("media.detail.addToPlaylist", "Playlist")} accessibilityRole="button">
              <Ionicons name="add-circle-outline" size={20} color={theme.colors.accent} />
            </Pressable>
            {isAdmin && (
              <Pressable onPress={() => setShowBulkCollection(true)} disabled={bulkBusy} hitSlop={BULK_HIT_SLOP} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible={true} accessibilityLabel={t("media.library.addToCollection", "Add to collection")} accessibilityRole="button">
                <Ionicons name="albums-outline" size={20} color={theme.colors.accent} />
              </Pressable>
            )}
            <Pressable onPress={exitBulk} hitSlop={BULK_HIT_SLOP} style={styles.bulkBtn} accessible={true} accessibilityLabel={t("common.cancel")} accessibilityRole="button">
              <Ionicons name="close" size={20} color={theme.colors.error} />
            </Pressable>
          </View>
        </View>
      )}
      {showBulkPlaylist && selected.size > 0 && (
        <BulkAddToPlaylistSheet
          visible={showBulkPlaylist}
          mediaItemIds={Array.from(selected)}
          onClose={() => { setShowBulkPlaylist(false); exitBulk(); }}
        />
      )}
      {showBulkCollection && selected.size > 0 && (
        <BulkAddToCollectionSheet
          visible={showBulkCollection}
          mediaItemIds={Array.from(selected)}
          onClose={() => { setShowBulkCollection(false); exitBulk(); }}
        />
      )}
    </View>
  );
}

// The bulk bar's buttons are a 20px icon in 6px of padding: 32dp, under the
// 44dp minimum. The bar's own gap is 12, so 6 on each side reaches 44 without
// two neighbouring targets ever overlapping.
const BULK_HIT_SLOP = { top: 8, bottom: 8, left: 6, right: 6 };

const styles = StyleSheet.create({
  container: { flex: 1 },
  header: { fontSize: 28, fontWeight: "700", paddingHorizontal: 16 },
  searchInput: {
    marginHorizontal: SPACING,
    marginTop: 8,
    marginBottom: 4,
    paddingHorizontal: 12,
    paddingVertical: 8,
    fontSize: 14,
  },
  // flexShrink 0: the grid below always wants more height than the screen, and
  // Yoga would otherwise squeeze the chip rows until they are clipped.
  chipRow: {
    paddingHorizontal: SPACING,
    marginBottom: 4,
    flexGrow: 0,
    flexShrink: 0,
  },
  chip: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginRight: 6,
  },
  gridItem: { alignItems: "center" },
  title: { fontSize: 12, fontWeight: "500", marginTop: 4, textAlign: "center" },
  empty: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  checkmark: { position: "absolute", top: 4, right: 4, zIndex: 1, backgroundColor: "rgba(0,0,0,0.5)", borderRadius: 12 },
  bulkBar: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 16, paddingVertical: 10, borderTopWidth: StyleSheet.hairlineWidth },
  // padding, not just gap: the icons are 20px and sit side by side, so without
  // it the real touch target is ~20px and a mis-tap hits the neighbour (worst
  // case: "cancel" instead of "mark watched" in bulk selection).
  bulkBtn: { alignItems: "center", gap: 2, padding: 6 },
});
