import { useState, useCallback, useRef, useEffect } from "react";
import { displayTitle } from "@evey/api";
import {
  View,
  Text,
  TextInput,
  FlatList,
  TouchableOpacity,
  Pressable,
  ScrollView,
  StyleSheet,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Image } from "expo-image";
import { Skeleton } from "../../../src/components/Skeleton";
import { useRouter, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import { useAllGenres } from "../../../src/hooks/useAllGenres";
import type { V1MediaItem, V1Track } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { artworkUrl } from "../../../src/lib/artwork";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import { searchTrackMusicSource } from "../../../src/components/music/musicSource";
import { MAX_SEARCH_HISTORY, pushSearchHistory, removeSearchHistoryEntry } from "../../../src/components/library/searchHistory";
import * as SecureStore from "expo-secure-store";

const HISTORY_KEY = "evey-search-history";

function loadSearchHistory(): string[] {
  try {
    const raw = SecureStore.getItem(HISTORY_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveSearchHistory(history: string[]) {
  try {
    SecureStore.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_SEARCH_HISTORY)));
  } catch (err) { console.warn("caught", err); }
}

type MediaTypeFilter = "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";
type SortField = "MEDIA_SORT_FIELD_TITLE" | "MEDIA_SORT_FIELD_YEAR" | "MEDIA_SORT_FIELD_DATE_ADDED";
type SortOrder = "SORT_ORDER_ASC" | "SORT_ORDER_DESC";

function formatType(type?: string): string {
  if (!type) return "";
  return type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();
}

function searchKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

export default function SearchScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const params = useLocalSearchParams<{ q?: string; kind?: string; focus?: string }>();
  const [query, setQuery] = useState(typeof params.q === "string" ? params.q : "");
  // Trim before debouncing, like web (search.lazy.tsx): " from" and "from"
  // are the same query and must share a cache entry.
  const debouncedQuery = useDebounce(query.trim(), 300);
  const inputRef = useRef<TextInput>(null);

  // The header search field is a trigger that navigates here with a fresh
  // `focus` param (native toolbars clip any in-header dropdown, so typing
  // happens on this screen). Focus the input each time it fires.
  useEffect(() => {
    if (params.focus) inputRef.current?.focus();
  }, [params.focus]);
  // Search scope: video catalog (default) or music.
  const [searchKind, setSearchKind] = useState<"media" | "music">(params.kind === "music" ? "music" : "media");
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const authHeaders = useAuthHeaders();
  const [searchHistory, setSearchHistory] = useState<string[]>(() => loadSearchHistory());
  const lastSubmitted = useRef("");

  const [typeFilter, setTypeFilter] = useState<MediaTypeFilter | undefined>(
    undefined,
  );
  const [genre, setGenre] = useState("");
  const [yearMin, setYearMin] = useState("");
  const [yearMax, setYearMax] = useState("");
  const [showFilters, setShowFilters] = useState(false);
  // Real genres from the user's libraries, not a hardcoded English list: the
  // filter matches the stored genre string exactly, so English chips returned
  // nothing at all on a French library.
  const { data: availableGenres = [] } = useAllGenres(showFilters);
  const [sortField, setSortField] = useState<SortField | undefined>(undefined);
  const [sortOrder, setSortOrder] = useState<SortOrder | undefined>(undefined);

  const sortOptions: Array<{
    label: string;
    field: SortField | undefined;
    order: SortOrder | undefined;
  }> = [
    {
      label: t("media.search.relevance"),
      field: undefined,
      order: undefined,
    },
    {
      label: t("media.search.titleAZ"),
      field: "MEDIA_SORT_FIELD_TITLE",
      order: "SORT_ORDER_ASC",
    },
    {
      label: t("media.search.yearNewest"),
      field: "MEDIA_SORT_FIELD_YEAR",
      order: "SORT_ORDER_DESC",
    },
    {
      label: t("media.search.dateAdded"),
      field: "MEDIA_SORT_FIELD_DATE_ADDED",
      order: "SORT_ORDER_DESC",
    },
  ];

  const addToHistory = useCallback((q: string) => {
    const trimmed = q.trim();
    if (!trimmed || trimmed === lastSubmitted.current) return;
    lastSubmitted.current = trimmed;
    setSearchHistory((prev) => {
      const updated = pushSearchHistory(prev, trimmed);
      saveSearchHistory(updated);
      return updated;
    });
  }, []);

  const clearHistory = useCallback(() => {
    setSearchHistory([]);
    saveSearchHistory([]);
  }, []);

  // Removes a single recent-search entry, mirroring web's per-entry remove
  // button (search.lazy.tsx:354-360) - mobile previously only had "clear all".
  const removeHistoryEntry = useCallback((entry: string) => {
    setSearchHistory((prev) => {
      const updated = removeSearchHistoryEntry(prev, entry);
      saveSearchHistory(updated);
      return updated;
    });
  }, []);

  // Save to history when the debounced query stabilizes. Using useEffect
  // (rather than calling setSearchHistory inline during render) keeps React
  // happy and avoids "Cannot update a component while rendering" warnings.
  useEffect(() => {
    if (debouncedQuery.length > 1) addToHistory(debouncedQuery);
  }, [debouncedQuery, addToHistory]);

  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    {
      query: debouncedQuery,
      ...(typeFilter ? { type: typeFilter } : {}),
      ...(genre ? { genre } : {}),
      ...(yearMin ? { yearMin: parseInt(yearMin) } : {}),
      ...(yearMax ? { yearMax: parseInt(yearMax) } : {}),
      ...(sortField ? { sortField, sortOrder } : {}),
      limit: 40,
    },
    { query: { enabled: debouncedQuery.length > 1 } },
  );

  const { data: musicData, isLoading: musicLoading } = useMusicServiceSearchMusic(
    { query: debouncedQuery, limit: 40 },
    { query: { enabled: debouncedQuery.length > 1 && searchKind === "music" } },
  );
  const musicItems = musicData?.tracks ?? [];

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;
  const hasQuery = debouncedQuery.length > 1;

  const playMusicTrack = useCallback((tr: V1Track) => {
    setQueue([{
      id: tr.id ?? "",
      title: tr.title ?? "",
      artist: tr.artist ?? "",
      album: tr.album ?? "",
      duration: tr.duration ?? 0,
    }], 0, searchTrackMusicSource(tr.albumId, tr.album));
  }, [setQueue]);

  const renderMusicItem = useCallback(({ item }: { item: V1Track }) => {
    const cover = item.coverArtUrl
      ? (/^https?:/i.test(item.coverArtUrl) ? item.coverArtUrl : `${serverUrl ?? ""}${item.coverArtUrl}`)
      : undefined;
    return (
      <TouchableOpacity
        activeOpacity={0.7}
        onPress={() => playMusicTrack(item)}
        style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      >
        <View style={[styles.musicThumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised, justifyContent: "center", alignItems: "center" }]}>
          {cover ? (
            <Image source={{ uri: cover, headers: authHeaders }} style={{ width: 48, height: 48, borderRadius: theme.radii.sm }} contentFit="cover" transition={150} />
          ) : (
            <Ionicons name="musical-notes" size={20} color={theme.colors.textMuted} />
          )}
        </View>
        <View style={styles.info}>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>{item.title}</Text>
          <Text style={[styles.meta, { color: theme.colors.textMuted }]} numberOfLines={1}>
            {[item.artist, item.album].filter(Boolean).join(" · ")}
          </Text>
        </View>
        <Ionicons name="play" size={18} color={theme.colors.accent} />
      </TouchableOpacity>
    );
  }, [theme, serverUrl, authHeaders, playMusicTrack]);
  const hasActiveFilters = !!typeFilter || !!genre || !!yearMin || !!yearMax || !!sortField;

  const clearAllFilters = useCallback(() => {
    setTypeFilter(undefined);
    setGenre("");
    setYearMin("");
    setYearMax("");
    setSortField(undefined);
    setSortOrder(undefined);
  }, []);

  const typeFilters: Array<{
    label: string;
    value: MediaTypeFilter | undefined;
  }> = [
    { label: t("media.search.all"), value: undefined },
    { label: t("media.search.movies"), value: "MEDIA_ITEM_TYPE_MOVIE" },
    { label: t("media.search.series"), value: "MEDIA_ITEM_TYPE_SERIES" },
  ];

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 200) : undefined;
      return (
        <TouchableOpacity
          style={[
            styles.row,
            {
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.id}`)}
          accessible={true}
          accessibilityLabel={displayTitle(item)}
          accessibilityRole="button"
        >
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
              style={[styles.thumb, { borderRadius: theme.radii.sm }]}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={[
                styles.thumb,
                {
                  borderRadius: theme.radii.sm,
                  backgroundColor: theme.colors.surfaceRaised,
                  justifyContent: "center",
                  alignItems: "center",
                },
              ]}
            >
              <Text
                style={{ color: theme.colors.textMuted, fontSize: 10 }}
              >
                ?
              </Text>
            </View>
          )}
          <View style={styles.info}>
            <Text
              style={[styles.title, { color: theme.colors.text }]}
              numberOfLines={1}
            >
              {displayTitle(item)}
            </Text>
            <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
              {[item.year, formatType(item.type)].filter(Boolean).join(" · ")}
            </Text>
          </View>
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, serverUrl],
  );

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <TextInput
        ref={inputRef}
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            marginTop: theme.spacing.xl,
          },
        ]}
        placeholder={t("media.search.placeholder")}
        placeholderTextColor={theme.colors.textMuted}
        value={query}
        onChangeText={setQuery}
        autoCapitalize="none"
        autoCorrect={false}
        returnKeyType="search"
        accessible={true}
        accessibilityLabel="Search media"
      />

      {/* Scope selector: video catalog (default) vs music */}
      {hasQuery && (
        <View style={{ flexDirection: "row", paddingHorizontal: 16, marginBottom: 8, gap: 6 }}>
          {(["media", "music"] as const).map((k) => (
            <TouchableOpacity
              key={k}
              activeOpacity={0.7}
              onPress={() => setSearchKind(k)}
              style={[styles.chip, { backgroundColor: searchKind === k ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm, marginRight: 0 }]}
            >
              <Text style={{ color: searchKind === k ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                {k === "media" ? t("media.search.sectionMedia") : t("media.search.sectionMusic")}
              </Text>
            </TouchableOpacity>
          ))}
        </View>
      )}

      {/* Type filter chips + filter toggle */}
      {hasQuery && searchKind === "media" && (
        <View style={{ paddingHorizontal: 16, marginBottom: 4 }}>
          <ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            style={{ marginBottom: 6 }}
          >
            {typeFilters.map((f) => (
              <TouchableOpacity
                key={f.value ?? "all"}
                style={[
                  styles.chip,
                  {
                    backgroundColor:
                      typeFilter === f.value
                        ? theme.colors.accent
                        : theme.colors.surfaceRaised,
                    borderRadius: theme.radii.sm,
                  },
                ]}
                activeOpacity={0.7}
                onPress={() => setTypeFilter(f.value)}
              >
                <Text
                  style={{
                    color:
                      typeFilter === f.value
                        ? theme.colors.onAccent
                        : theme.colors.text,
                    fontSize: 13,
                  }}
                >
                  {f.label}
                </Text>
              </TouchableOpacity>
            ))}

            <TouchableOpacity
              style={[
                styles.chip,
                {
                  backgroundColor: showFilters
                    ? theme.colors.accent
                    : theme.colors.surfaceRaised,
                  borderRadius: theme.radii.sm,
                },
              ]}
              activeOpacity={0.7}
              onPress={() => setShowFilters((v) => !v)}
            >
              <Text
                style={{
                  color: showFilters ? theme.colors.onAccent : theme.colors.text,
                  fontSize: 13,
                }}
              >
                {t("media.search.filters")}
                {hasActiveFilters && !showFilters ? " ●" : ""}
              </Text>
            </TouchableOpacity>

            {hasActiveFilters && (
              <TouchableOpacity
                style={[
                  styles.chip,
                  {
                    backgroundColor: theme.colors.surfaceRaised,
                    borderRadius: theme.radii.sm,
                  },
                ]}
                activeOpacity={0.7}
                onPress={clearAllFilters}
              >
                <Text
                  style={{ color: theme.colors.textMuted, fontSize: 12 }}
                >
                  {t("media.search.clearFilters")}
                </Text>
              </TouchableOpacity>
            )}
          </ScrollView>

          {/* Expanded filters panel */}
          {showFilters && (
            <View
              style={[
                styles.filtersPanel,
                {
                  backgroundColor: theme.colors.surface,
                  borderRadius: theme.radii.md,
                },
              ]}
            >
              {/* Genre */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted },
                ]}
              >
                {t("media.search.genre")}
              </Text>
              <ScrollView
                horizontal
                showsHorizontalScrollIndicator={false}
                style={{ marginBottom: 10 }}
              >
                <TouchableOpacity
                  style={[
                    styles.chip,
                    {
                      backgroundColor: !genre
                        ? theme.colors.accent
                        : theme.colors.surfaceRaised,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                  activeOpacity={0.7}
                  onPress={() => setGenre("")}
                >
                  <Text
                    style={{
                      color: !genre ? theme.colors.onAccent : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    {t("media.search.allGenres")}
                  </Text>
                </TouchableOpacity>
                {availableGenres.map((g) => (
                  <TouchableOpacity
                    key={g}
                    style={[
                      styles.chip,
                      {
                        backgroundColor:
                          genre === g
                            ? theme.colors.accent
                            : theme.colors.surfaceRaised,
                        borderRadius: theme.radii.sm,
                      },
                    ]}
                    activeOpacity={0.7}
                    onPress={() => setGenre(genre === g ? "" : g)}
                  >
                    <Text
                      style={{
                        color:
                          genre === g ? theme.colors.onAccent : theme.colors.text,
                        fontSize: 13,
                      }}
                    >
                      {g}
                    </Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              {/* Sort */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted },
                ]}
              >
                {t("media.search.sortBy")}
              </Text>
              <ScrollView
                horizontal
                showsHorizontalScrollIndicator={false}
              >
                {sortOptions.map((opt, i) => {
                  const active =
                    sortField === opt.field &&
                    sortOrder === opt.order;
                  return (
                    <TouchableOpacity
                      key={i}
                      style={[
                        styles.chip,
                        {
                          backgroundColor: active
                            ? theme.colors.accent
                            : theme.colors.surfaceRaised,
                          borderRadius: theme.radii.sm,
                        },
                      ]}
                      activeOpacity={0.7}
                      onPress={() => {
                        setSortField(opt.field);
                        setSortOrder(opt.order);
                      }}
                    >
                      <Text
                        style={{
                          color: active
                            ? theme.colors.onAccent
                            : theme.colors.text,
                          fontSize: 13,
                        }}
                      >
                        {opt.label}
                      </Text>
                    </TouchableOpacity>
                  );
                })}
              </ScrollView>

              {/* Year range */}
              <Text style={[styles.filterLabel, { color: theme.colors.textMuted, marginTop: 10 }]}>
                {t("media.search.year")}
              </Text>
              <View style={{ flexDirection: "row", gap: 8 }}>
                <TextInput
                  placeholder={t("media.search.fromYear")}
                  placeholderTextColor={theme.colors.textMuted}
                  value={yearMin}
                  onChangeText={(v) => setYearMin(v.replace(/\D/g, ""))}
                  keyboardType="number-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
                <TextInput
                  placeholder={t("media.search.toYear")}
                  placeholderTextColor={theme.colors.textMuted}
                  value={yearMax}
                  onChangeText={(v) => setYearMax(v.replace(/\D/g, ""))}
                  keyboardType="number-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
              </View>
            </View>
          )}
        </View>
      )}

      {searchKind === "media" && isError && debouncedQuery.length > 0 ? (
        <View
          style={{
            flex: 1,
            justifyContent: "center",
            alignItems: "center",
          }}
        >
          <Text
            style={{ color: theme.colors.textMuted, marginBottom: 12 }}
          >
            {t("common.loadError")}
          </Text>
          <Pressable onPress={() => refetch()}>
            <Text style={{ color: theme.colors.accent }}>
              {t("common.retry")}
            </Text>
          </Pressable>
        </View>
      ) : (searchKind === "media" ? isLoading : musicLoading) && hasQuery ? (
        <View style={{ padding: theme.spacing.md }}>
          {Array.from({ length: 6 }).map((_, i) => (
            <View
              key={i}
              style={{
                flexDirection: "row",
                alignItems: "center",
                padding: 10,
                marginBottom: theme.spacing.sm,
              }}
            >
              <Skeleton
                width={48}
                height={72}
                borderRadius={theme.radii.sm}
              />
              <View style={{ flex: 1, marginLeft: 12 }}>
                <Skeleton width="70%" height={15} borderRadius={4} />
                <Skeleton
                  width="40%"
                  height={12}
                  borderRadius={4}
                  style={{ marginTop: 6 }}
                />
              </View>
            </View>
          ))}
        </View>
      ) : !hasQuery ? (
        searchHistory.length > 0 ? (
          <View style={{ paddingHorizontal: 16, paddingTop: 16 }}>
            <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600" }}>
                {t("media.search.recentSearches") || "Recent Searches"}
              </Text>
              <Pressable onPress={clearHistory}>
                <Text style={{ color: theme.colors.accent, fontSize: 13 }}>{t("common.clearAll") || "Clear"}</Text>
              </Pressable>
            </View>
            {searchHistory.map((h) => (
              <View
                key={h}
                style={{ flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: theme.colors.border }}
              >
                <Pressable
                  onPress={() => setQuery(h)}
                  style={{ flexDirection: "row", alignItems: "center", gap: 10, flex: 1 }}
                >
                  <Ionicons name="time-outline" size={16} color={theme.colors.textMuted} />
                  <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>{h}</Text>
                  <Ionicons name="arrow-up-outline" size={14} color={theme.colors.textMuted} style={{ transform: [{ rotate: "-45deg" }] }} />
                </Pressable>
                <Pressable
                  onPress={() => removeHistoryEntry(h)}
                  accessibilityRole="button"
                  accessibilityLabel={t("common.removeItem", { item: h })}
                  hitSlop={14}
                >
                  <Ionicons name="close" size={16} color={theme.colors.textMuted} />
                </Pressable>
              </View>
            ))}
          </View>
        ) : (
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>
              {t("media.search.searchLibrary")}
            </Text>
          </View>
        )
      ) : searchKind === "music" ? (
        musicItems.length === 0 ? (
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted }}>{t("media.search.noResults")}</Text>
          </View>
        ) : (
          <>
            <View style={{ paddingHorizontal: 16, paddingBottom: 6 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
                {t("media.search.resultCount", { count: musicItems.length })}{" "}
                {t("media.search.resultFor", { query: debouncedQuery })}
              </Text>
            </View>
            <FlatList
              data={musicItems}
              keyExtractor={(it) => it.id ?? ""}
              renderItem={renderMusicItem}
              contentContainerStyle={{ padding: theme.spacing.md }}
              ItemSeparatorComponent={() => <View style={styles.separator} />}
              keyboardShouldPersistTaps="handled"
              initialNumToRender={10}
              maxToRenderPerBatch={12}
              windowSize={5}
              removeClippedSubviews={true}
            />
          </>
        )
      ) : items.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted }}>
            {t("media.search.noResults")}
          </Text>
        </View>
      ) : (
        <>
          <View style={{ paddingHorizontal: 16, paddingBottom: 6 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
              {t("media.search.resultCount", { count: totalHits })}{" "}
              {t("media.search.resultFor", { query: debouncedQuery })}
            </Text>
          </View>
          <FlatList
            data={items}
            keyExtractor={searchKeyExtractor}
            renderItem={renderItem}
            contentContainerStyle={{ padding: theme.spacing.md }}
            ItemSeparatorComponent={() => <View style={styles.separator} />}
            keyboardShouldPersistTaps="handled"
            initialNumToRender={8}
            maxToRenderPerBatch={10}
            windowSize={5}
            removeClippedSubviews={true}
          />
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  input: {
    marginHorizontal: 16,
    marginBottom: 8,
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 16,
  },
  chip: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginRight: 6,
  },
  filtersPanel: {
    padding: 12,
    marginBottom: 6,
  },
  filterLabel: {
    fontSize: 11,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 6,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    padding: 10,
  },
  thumb: { width: 48, height: 72, marginRight: 12 },
  musicThumb: { width: 48, height: 48, marginRight: 12 },
  info: { flex: 1 },
  title: { fontSize: 15, fontWeight: "600" },
  meta: { fontSize: 12, marginTop: 2 },
  separator: { height: 8 },
});
