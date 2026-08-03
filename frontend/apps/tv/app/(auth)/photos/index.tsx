import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, FlatList, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { router, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  usePhotoServiceListPhotoAlbums,
  usePhotoServiceListPhotos,
} from "@evey/api";
import type { V1Library } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../src/components/FocusableCard";
import {
  photoServeUrl,
  buildPhotoListParams,
  hasMorePhotos,
  PHOTO_GRID_THUMB_WIDTH,
  type TvPhoto,
  type TvPhotoAlbum,
} from "../../../src/lib/photoGrid";
import { photoSlideshowHref } from "../../../src/lib/photoRoutes";

// Square tiles, deliberately NOT the 2:3 poster ratio used by every other TV
// grid in this app - photos are photos, not movie posters, and a landing
// screen built for browsing a photo library at a distance reads better as an
// even grid of squares than as a wall of tall rectangles.
const NUM_COLUMNS = 5;
const TILE_SIZE = 220;
const GRID_GAP = 16;
const LABEL_HEIGHT = 36;
const ROW_HEIGHT = TILE_SIZE + LABEL_HEIGHT + GRID_GAP;
// Bounded page size: this is a metadata list (id/title/blurhash), not image
// bytes, but a NAS-class server still has to round-trip a DB page for it, and
// a small page keeps a very large photo library from being pulled in one
// shot just because someone scrolled to the top of the screen.
const PAGE_SIZE = 60;

function usePhotoLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_PHOTOS"),
      staleTime: 60_000,
    },
  });
}

function photoKeyExtractor(item: TvPhoto) {
  return item.id ?? "";
}

function getItemLayout(_: unknown, index: number) {
  return { length: ROW_HEIGHT, offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS), index };
}

function LibraryChip({ library, active, onPress }: { library: V1Library; active: boolean; onPress: () => void }) {
  const theme = useTheme();
  return (
    <FocusableCard
      onPress={onPress}
      accessibilityLabel={library.name ?? ""}
      style={[styles.chip, { backgroundColor: active ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.sm }]}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 16, fontWeight: "600" }}>{library.name}</Text>
    </FocusableCard>
  );
}

function AllPhotosChip({ active, onPress, label }: { active: boolean; onPress: () => void; label: string }) {
  const theme = useTheme();
  return (
    <FocusableCard
      onPress={onPress}
      accessibilityLabel={label}
      style={[styles.albumChip, { backgroundColor: active ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <View style={[styles.albumChipArt, { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
        <Ionicons name="images-outline" size={28} color={active ? theme.colors.onAccent : theme.colors.textMuted} />
      </View>
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 15, fontWeight: "600" }} numberOfLines={1}>
        {label}
      </Text>
    </FocusableCard>
  );
}

function AlbumChip({
  album,
  active,
  onPress,
  serverUrl,
  authHeaders,
}: {
  album: TvPhotoAlbum;
  active: boolean;
  onPress: () => void;
  serverUrl: string;
  authHeaders?: { Authorization: string };
}) {
  const theme = useTheme();
  const coverUri = album.coverPhotoId ? photoServeUrl(serverUrl, album.coverPhotoId, { width: PHOTO_GRID_THUMB_WIDTH, format: "webp" }) : undefined;
  return (
    <FocusableCard
      onPress={onPress}
      accessibilityLabel={album.name}
      style={[styles.albumChip, { backgroundColor: active ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <View style={[styles.albumChipArt, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm, overflow: "hidden" }]}>
        {coverUri ? (
          <Image source={{ uri: coverUri, headers: authHeaders }} style={StyleSheet.absoluteFill} contentFit="cover" transition={150} />
        ) : (
          <Ionicons name="images-outline" size={28} color={theme.colors.textMuted} />
        )}
      </View>
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 15, fontWeight: "600" }} numberOfLines={1}>
        {album.name}
      </Text>
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 12 }}>{album.photoCount ?? 0}</Text>
    </FocusableCard>
  );
}

export default function PhotosScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [selectedAlbum, setSelectedAlbum] = useState<string | undefined>(undefined);
  // undefined = page 1 (offset(0) path, exact total). Every following page
  // is requested by the previous response's keyset cursor instead.
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [allPhotos, setAllPhotos] = useState<TvPhoto[]>([]);
  const prevDataRef = useRef<unknown>(null);

  const { data: libraries, isLoading: libLoading } = usePhotoLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;

  const { data: albums, isLoading: albumsLoading } = usePhotoServiceListPhotoAlbums(
    { libraryId: activeLibraryID },
    { query: { enabled: !!activeLibraryID, select: (d) => d.albums ?? [] } },
  );

  const {
    data,
    isLoading: photosLoading,
    isFetching,
    isError,
    refetch,
  } = usePhotoServiceListPhotos(
    buildPhotoListParams({ libraryId: activeLibraryID ?? "", album: selectedAlbum, limit: PAGE_SIZE, cursor }),
    { query: { enabled: !!activeLibraryID, gcTime: 30_000 } },
  );

  // Reset pagination whenever the library or album changes - declared before
  // the accumulate effect so a synchronously-cached page-1 result doesn't get
  // wiped right back out by this reset running second (see the library
  // screen's identical ordering note for the failure mode this avoids).
  useEffect(() => {
    setCursor(undefined);
    setAllPhotos([]);
    prevDataRef.current = null;
  }, [activeLibraryID, selectedAlbum]);

  useEffect(() => {
    if (!data || data === prevDataRef.current) return;
    prevDataRef.current = data;
    const newPhotos = data.photos ?? [];
    setAllPhotos((prev) => (cursor === undefined ? newPhotos : [...prev, ...newPhotos]));
  }, [data, cursor]);

  const nextCursor = data?.nextCursor;
  const lastPageCount = data?.photos?.length ?? 0;

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMorePhotos(nextCursor, lastPageCount, PAGE_SIZE)) setCursor(nextCursor);
  }, [isFetching, nextCursor, lastPageCount]);

  const openSlideshow = useCallback(
    (index: number) => {
      if (!activeLibraryID) return;
      router.push(photoSlideshowHref(activeLibraryID, index, selectedAlbum) as never);
    },
    [activeLibraryID, selectedAlbum],
  );

  const renderPhoto = useCallback(
    ({ item, index }: { item: TvPhoto; index: number }) => {
      const uri = item.id ? photoServeUrl(serverUrl, item.id, { width: PHOTO_GRID_THUMB_WIDTH, format: "webp" }) : undefined;
      return (
        <FocusableCard
          onPress={() => openSlideshow(index)}
          hasTVPreferredFocus={index === 0}
          style={styles.tile}
          // A photo with neither title nor album used to announce nothing at
          // all; on a television the focused element is the only thing a
          // screen reader reads, so its position is the honest fallback.
          accessibilityLabel={item.title || item.album || t("media.photos.openPhoto", "Photo {{n}}", { n: index + 1 })}
        >
          <View style={[styles.tileArt, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm, overflow: "hidden" }]}>
            {uri ? (
              <Image
                source={{ uri, headers: authHeaders }}
                style={StyleSheet.absoluteFill}
                contentFit="cover"
                transition={150}
                recyclingKey={item.id}
                placeholder={item.blurhash ? { blurhash: item.blurhash } : undefined}
                placeholderContentFit="cover"
              />
            ) : null}
          </View>
        </FocusableCard>
      );
    },
    [theme, serverUrl, authHeaders, openSlideshow, t],
  );

  if (libLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  // Nothing on this screen would ever claim TV focus otherwise, stranding
  // the remote (same pattern as history.tsx's empty state).
  if (!libraries || libraries.length === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("media.photos.noPhotoLibraries")}</Text>
        <FocusableCard
          onPress={() => router.push("/(auth)/(tabs)/library")}
          hasTVPreferredFocus
          style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={t("media.library.libraries")}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
        </FocusableCard>
      </View>
    );
  }

  const isFirstPage = cursor === undefined;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen options={{ headerShown: true, title: t("media.photos.title"), headerStyle: { backgroundColor: theme.colors.surface }, headerTintColor: theme.colors.text }} />

      {isError ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 12 }}>{t("common.loadError")}</Text>
          <FocusableCard onPress={() => refetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
            <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
          </FocusableCard>
        </View>
      ) : (
        <>
          {libraries.length > 1 && (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.libRow} contentContainerStyle={{ gap: 16 }}>
              {libraries.map((lib) => (
                <LibraryChip key={lib.id} library={lib} active={lib.id === activeLibraryID} onPress={() => setSelectedLibraryID(lib.id)} />
              ))}
            </ScrollView>
          )}

          {albumsLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 16, marginLeft: 24 }} />
          ) : (albums ?? []).length > 0 ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.albumRow} contentContainerStyle={{ gap: 16, paddingHorizontal: 24 }}>
              <AllPhotosChip active={!selectedAlbum} onPress={() => setSelectedAlbum(undefined)} label={t("media.photos.allPhotos")} />
              {(albums ?? []).map((a) => (
                <AlbumChip
                  key={a.name}
                  album={a}
                  active={selectedAlbum === a.name}
                  onPress={() => setSelectedAlbum(selectedAlbum === a.name ? undefined : a.name)}
                  serverUrl={serverUrl}
                  authHeaders={authHeaders}
                />
              ))}
            </ScrollView>
          ) : null}

          {photosLoading && isFirstPage ? (
            <View style={styles.center}>
              <ActivityIndicator color={theme.colors.accent} size="large" />
            </View>
          ) : (
            <FlatList
              data={allPhotos}
              keyExtractor={photoKeyExtractor}
              renderItem={renderPhoto}
              numColumns={NUM_COLUMNS}
              contentContainerStyle={{ padding: 24 }}
              columnWrapperStyle={{ gap: GRID_GAP, marginBottom: GRID_GAP }}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              getItemLayout={getItemLayout}
              initialNumToRender={10}
              maxToRenderPerBatch={15}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={isFetching && !isFirstPage ? <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 24 }} /> : null}
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>
                    {t("media.photos.noPhotos")}
                  </Text>
                  <FocusableCard
                    onPress={() => router.push("/(auth)/(tabs)/library")}
                    hasTVPreferredFocus
                    style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
                    accessibilityLabel={t("media.library.libraries")}
                  >
                    <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
                  </FocusableCard>
                </View>
              }
            />
          )}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  libRow: { paddingHorizontal: 24, paddingTop: 16, flexGrow: 0 },
  albumRow: { marginTop: 16, flexGrow: 0 },
  chip: { paddingHorizontal: 20, paddingVertical: 10 },
  albumChip: { width: 160, alignItems: "center", padding: 10 },
  albumChipArt: { width: 120, height: 90, justifyContent: "center", alignItems: "center", marginBottom: 6 },
  tile: { width: TILE_SIZE, alignItems: "center" },
  tileArt: { width: TILE_SIZE, height: TILE_SIZE },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 14 },
});
