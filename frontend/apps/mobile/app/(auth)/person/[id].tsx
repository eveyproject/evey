import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, Pressable, Dimensions, ActivityIndicator } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, router, Stack } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch, personDepartmentKey, useLibraryServiceListMediaItems, hasMoreFromCursor } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { LoadFailed } from "../../../src/components/LoadFailed";

const { width: SCREEN_WIDTH } = Dimensions.get("window");
const NUM_COLUMNS = 3;
const GAP = 8;
// Keyset page: 30 posters per fetch, streamed in as the grid scrolls. A
// prolific actor used to mount up to 500 posters at once (the old fetch cap),
// freezing mid-range phones before first paint; now the LegendList only mounts
// the ~visible rows and pages the DB by opaque cursor (constant per-page cost,
// stable while a scan inserts rows underneath).
const PAGE_SIZE = 30;

const CELL_WIDTH = SCREEN_WIDTH / NUM_COLUMNS;
const POSTER_WIDTH = CELL_WIDTH - GAP;
const POSTER_HEIGHT = POSTER_WIDTH * 1.5;
// Cell = poster + a 2-line label (~34) + the bottom gap. Feeds LegendList's
// estimatedItemSize so it can place rows without measuring every cell first.
const CELL_HEIGHT = POSTER_HEIGHT + 34 + GAP;

function personKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

interface PersonInfo {
  id?: string;
  name?: string;
  thumbUrl?: string;
  biography?: string;
  birthday?: string;
  deathday?: string;
  placeOfBirth?: string;
  knownFor?: string;
}

function useGetPerson(id: string) {
  return useQuery<PersonInfo>({
    queryKey: ["person", id],
    queryFn: () => eveyFetch<PersonInfo>({ url: `/api/v1/people/${id}`, method: "GET" }),
    enabled: !!id,
  });
}

export default function PersonDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [bioExpanded, setBioExpanded] = useState(false);
  const personId = id ?? "";

  // Keyset infinite scroll, same contract as library/[libraryId].tsx: accumulate
  // pages under an opaque cursor, stop when the server stops handing one back.
  const [cursor, setCursor] = useState("");
  const [items, setItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [total, setTotal] = useState<number | null>(null);
  const prevDataRef = useRef<unknown>(null);

  useEffect(() => {
    setCursor("");
    setItems([]);
    setHasMore(true);
    setTotal(null);
    prevDataRef.current = null;
  }, [id]);

  const {
    data: person,
    isLoading: personLoading,
    isError: personError,
    refetch: refetchPerson,
  } = useGetPerson(personId);
  const {
    data: mediaData,
    isLoading: mediaLoading,
    isFetching: mediaFetching,
    isError: mediaError,
    refetch: refetchMedia,
  } = useLibraryServiceListMediaItems(
    {
      personId,
      "pagination.pageSize": PAGE_SIZE,
      ...(cursor ? { "pagination.cursor": cursor } : {}),
    },
    { query: { enabled: !!personId } },
  );

  useEffect(() => {
    if (!mediaData || mediaData === prevDataRef.current) return;
    prevDataRef.current = mediaData;
    const newItems = mediaData.items ?? [];
    setItems((prev) => (cursor ? [...prev, ...newItems] : newItems));
    setHasMore(hasMoreFromCursor(mediaData.pagination));
    // totalItems is only exact on page 1 (the offset path); keep the first
    // non-zero value so the heading count stays honest as pages stream in.
    const tot = mediaData.pagination?.totalItems;
    if (tot != null && tot > 0) setTotal((prev) => prev ?? tot);
  }, [mediaData, cursor]);

  const handleEndReached = useCallback(() => {
    // Guard on isFetching so a fast scroll can't fire the same cursor twice
    // before the page resolves (LegendList may call onEndReached again).
    const next = mediaData?.pagination?.nextCursor;
    if (!mediaFetching && hasMore && next && next !== cursor) setCursor(next);
  }, [mediaData, mediaFetching, hasMore, cursor]);

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => (
      <PosterCard item={item} serverUrl={serverUrl} authHeaders={authHeaders} theme={theme} />
    ),
    [serverUrl, authHeaders, theme],
  );

  const name = person?.name ?? "Unknown";
  const filmographyCount = total ?? items.length;

  const header = useMemo(
    () => (
      <PersonHeader
        person={person}
        name={name}
        theme={theme}
        t={t}
        bioExpanded={bioExpanded}
        onToggleBio={() => setBioExpanded((p) => !p)}
        filmographyCount={filmographyCount}
        showCount={items.length > 0}
      />
    ),
    [person, name, theme, t, bioExpanded, filmographyCount, items.length],
  );

  const initialLoading = personLoading || (mediaLoading && items.length === 0);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: name,
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {initialLoading ? (
        <>
          {header}
          <View style={{ paddingVertical: 40, alignItems: "center" }}>
            <ActivityIndicator size="large" color={theme.colors.accent} />
          </View>
        </>
      ) : personError && !person ? (
        <LoadFailed onRetry={() => void refetchPerson()} />
      ) : (
        <LegendList
          data={items}
          renderItem={renderItem}
          keyExtractor={personKeyExtractor}
          numColumns={NUM_COLUMNS}
          recycleItems
          estimatedItemSize={CELL_HEIGHT}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          ListHeaderComponent={header}
          ListFooterComponent={
            mediaFetching && items.length > 0 ? (
              <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 16 }} />
            ) : null
          }
          ListEmptyComponent={
            mediaError ? (
              <LoadFailed inline onRetry={() => void refetchMedia()} />
            ) : (
              <Text
                style={{ color: theme.colors.textMuted, textAlign: "center", paddingVertical: 40, fontSize: 14 }}
              >
                {t("media.person.noFilmography")}
              </Text>
            )
          }
          contentContainerStyle={{ paddingBottom: theme.spacing.xl * 2 }}
          style={{ flex: 1 }}
        />
      )}
    </View>
  );
}

const PosterCard = memo(function PosterCard({
  item,
  serverUrl,
  authHeaders,
  theme,
}: {
  item: V1MediaItem;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  theme: ReturnType<typeof useTheme>;
}) {
  const uri = item.id ? artworkUrl(serverUrl, item.id, "poster", 200, item) : undefined;
  return (
    <Pressable
      onPress={() => router.push(`/(auth)/media/${item.id}`)}
      style={{ width: CELL_WIDTH, paddingHorizontal: GAP / 2, paddingBottom: GAP }}
    >
      <Image
        cachePolicy="memory-disk"
        // recyclingKey tells expo-image to drop the previous cell's bitmap when
        // LegendList reuses this view for a different item - without it a fast
        // scroll shows the wrong poster until the new one decodes.
        recyclingKey={item.id}
        source={uri ? { uri, headers: authHeaders } : undefined}
        style={{
          width: POSTER_WIDTH,
          height: POSTER_HEIGHT,
          borderRadius: theme.radii.sm,
          backgroundColor: theme.colors.surface,
        }}
        contentFit="cover"
        transition={200}
      />
      <Text style={{ color: theme.colors.text, fontSize: 12, marginTop: 4, width: POSTER_WIDTH }} numberOfLines={1}>
        {item.title}
      </Text>
      {item.year ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11, width: POSTER_WIDTH }}>{item.year}</Text>
      ) : null}
    </Pressable>
  );
});

function PersonHeader({
  person,
  name,
  theme,
  t,
  bioExpanded,
  onToggleBio,
  filmographyCount,
  showCount,
}: {
  person: PersonInfo | undefined;
  name: string;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
  bioExpanded: boolean;
  onToggleBio: () => void;
  filmographyCount: number;
  showCount: boolean;
}) {
  const initial = name[0]?.toUpperCase() ?? "?";
  const padH = theme.spacing.md;
  return (
    <View>
      <View style={{ alignItems: "center", paddingTop: theme.spacing.xl, paddingHorizontal: padH }}>
        {person?.thumbUrl ? (
          <Image
            source={{ uri: person.thumbUrl }}
            style={{ width: 120, height: 120, borderRadius: 60, backgroundColor: theme.colors.surface }}
            contentFit="cover"
            transition={200}
          />
        ) : (
          <View
            style={{
              width: 120,
              height: 120,
              borderRadius: 60,
              backgroundColor: theme.colors.surface,
              justifyContent: "center",
              alignItems: "center",
            }}
          >
            <Text style={{ color: theme.colors.textMuted, fontSize: 40, fontWeight: "bold" }}>{initial}</Text>
          </View>
        )}
        <Text
          style={{ color: theme.colors.text, fontSize: 24, fontWeight: "bold", marginTop: theme.spacing.md, textAlign: "center" }}
        >
          {name}
        </Text>
        {person?.knownFor ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.xs, textAlign: "center" }}>
            {(() => {
              const key = personDepartmentKey(person.knownFor);
              return key ? t(key, person.knownFor) : person.knownFor;
            })()}
          </Text>
        ) : null}
        {person?.birthday ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.xs, textAlign: "center" }}>
            {new Date(person.birthday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}
            {person.deathday
              ? ` - ${new Date(person.deathday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
              : ` (${Math.floor((Date.now() - new Date(person.birthday).getTime()) / 31557600000)} years old)`}
          </Text>
        ) : null}
        {person?.placeOfBirth ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2, textAlign: "center" }}>
            {person.placeOfBirth}
          </Text>
        ) : null}
      </View>
      {person?.biography ? (
        <Pressable onPress={onToggleBio} style={{ paddingHorizontal: padH, marginTop: theme.spacing.md }}>
          <Text style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20 }} numberOfLines={bioExpanded ? undefined : 4}>
            {person.biography}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 13, marginTop: 4 }}>
            {bioExpanded ? t("common.showLess") : t("common.readMore")}
          </Text>
        </Pressable>
      ) : null}
      {showCount ? (
        <Text
          style={{
            color: theme.colors.text,
            fontSize: 16,
            fontWeight: "600",
            paddingHorizontal: padH,
            marginTop: theme.spacing.lg,
            marginBottom: theme.spacing.sm,
          }}
        >
          {t("media.person.filmography")} ({filmographyCount})
        </Text>
      ) : null}
    </View>
  );
}
