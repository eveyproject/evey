import { useState, useCallback } from "react";
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
import { Image } from "expo-image";
import { Skeleton } from "../../../src/components/Skeleton";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { artworkUrl } from "../../../src/lib/artwork";

type MediaTypeFilter = "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";
type SortField = "MEDIA_SORT_FIELD_TITLE" | "MEDIA_SORT_FIELD_YEAR" | "MEDIA_SORT_FIELD_DATE_ADDED";
type SortOrder = "SORT_ORDER_ASC" | "SORT_ORDER_DESC";

const COMMON_GENRES = [
  "Action", "Adventure", "Animation", "Comedy", "Crime", "Documentary",
  "Drama", "Fantasy", "Horror", "Mystery", "Romance", "Sci-Fi",
  "Thriller", "War", "Western",
];

function getPoster(item: V1MediaItem): string | undefined {
  return item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
}

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
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);

  const [typeFilter, setTypeFilter] = useState<MediaTypeFilter | undefined>(
    undefined,
  );
  const [genre, setGenre] = useState("");
  const [yearMin, setYearMin] = useState("");
  const [yearMax, setYearMax] = useState("");
  const [showFilters, setShowFilters] = useState(false);
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

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;
  const hasQuery = debouncedQuery.length > 1;
  const hasActiveFilters = !!typeFilter || !!genre || !!yearMin || !!yearMax || !!sortField;

  const authHeaders = useAuthHeaders();

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
      const poster = getPoster(item) ?? (serverUrl && item.id ? artworkUrl(serverUrl, item.id) : undefined);
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
          accessibilityLabel={item.title}
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
              {item.title}
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

      {/* Type filter chips + filter toggle */}
      {hasQuery && (
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
                        ? "#fff"
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
                  color: showFilters ? "#fff" : theme.colors.text,
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
                      color: !genre ? "#fff" : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    {t("media.search.allGenres")}
                  </Text>
                </TouchableOpacity>
                {COMMON_GENRES.map((g) => (
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
                          genre === g ? "#fff" : theme.colors.text,
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
                            ? "#fff"
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

      {isError && debouncedQuery.length > 0 ? (
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
      ) : isLoading && hasQuery ? (
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
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>
            {t("media.search.searchLibrary")}
          </Text>
        </View>
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
  info: { flex: 1 },
  title: { fontSize: 15, fontWeight: "600" },
  meta: { fontSize: 12, marginTop: 2 },
  separator: { height: 8 },
});
