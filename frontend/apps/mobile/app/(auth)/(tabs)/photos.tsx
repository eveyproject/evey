import { useState, useCallback, useEffect, useMemo } from "react";
import { formatBytes } from "@evey/api";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Modal,
  Pressable,
  ActivityIndicator,
  Dimensions,
  StyleSheet,
  ScrollView,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { eveyFetch, hasMoreFromCursor } from "@evey/api";
import { Skeleton } from "../../../src/components/Skeleton";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { prevLightboxIndex, nextLightboxIndex, rotateClockwise, rotateCounterClockwise } from "../../../src/lib/lightboxNav";

// How close to the end of the loaded photos the viewer may get before the next
// page is requested. Same margin as the web lightbox.
const LIGHTBOX_PREFETCH_MARGIN = 5;

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

// Field names are lowerCamelCase because /api/v1/photos* is served by the
// grpc-gateway (protojson), not a custom PascalCase HTTP handler. Reading
// PascalCase here previously yielded `undefined` for every field - the
// grid 404'd on /photos/undefined/serve and albums showed blank.
interface Album {
  name: string;
  libraryId: string;
  photoCount: number;
  coverPhotoId: string;
}

interface Photo {
  id: string;
  title: string;
  album: string;
  // Empty for files ingested before migration 000096 or undecodable codecs.
  blurhash?: string;
}

interface PhotoDetail {
  id: string;
  title: string;
  filePath: string;
  fileSize: number;
  mimeType: string;
  width: number;
  height: number;
  takenAt: string;
  cameraModel: string;
  album: string;
}

const NUM_PHOTO_COLS = 3;
const NUM_ALBUM_COLS = 2;
const SPACING = 2;
const screenWidth = Dimensions.get("window").width;
const PHOTO_SIZE = (screenWidth - SPACING * (NUM_PHOTO_COLS + 1)) / NUM_PHOTO_COLS;
const ALBUM_SIZE = (screenWidth - SPACING * (NUM_ALBUM_COLS + 1) - 32) / NUM_ALBUM_COLS;

function usePhotoLibraries() {
  return useQuery({
    queryKey: ["libraries", "photos"],
    queryFn: () => eveyFetch<{ libraries: LibraryItem[] }>({ url: "/api/v1/libraries", method: "GET" }),
    select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_PHOTOS"),
    staleTime: 60_000,
  });
}

function useAlbums(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["photos", "albums", libraryID],
    queryFn: () => eveyFetch<{ albums: Album[] }>({ url: `/api/v1/photos/albums?library_id=${libraryID}`, method: "GET" }),
    enabled: !!libraryID,
    select: (d) => d.albums ?? [],
    staleTime: 30_000,
  });
}

function usePhotoDetail(photoId: string | undefined) {
  return useQuery({
    queryKey: ["photos", "detail", photoId],
    queryFn: () => eveyFetch<PhotoDetail>({ url: `/api/v1/photos/${photoId}`, method: "GET" }),
    enabled: !!photoId,
    staleTime: 60_000,
  });
}

// The grid is keyset-paginated, one page at a time. It used to ask for 5000 in
// a single call - the comment here even claimed that matched the web client and
// that `total` let users tell when the library exceeded the cap. Neither was
// true: the server replaces any limit above maxPageSize (500) with the photo
// handler's own default of 100 (clampLimitOffset, grpcapi/helpers.go), so a
// 7199-photo library rendered exactly 100 tiles under a footer announcing 7199.
// Page 1 takes the offset path (the only one that computes an exact total);
// every page after it carries the previous response's cursor.
const PHOTOS_PAGE_SIZE = 100;

interface PhotoPage {
  photos?: Photo[];
  total?: number;
  nextCursor?: string;
}

function usePhotos(libraryID: string | undefined, album?: string) {
  const query = useInfiniteQuery({
    queryKey: ["photos", "list", libraryID, album],
    enabled: !!libraryID,
    initialPageParam: "" as string,
    // The URL is spelled out rather than assembled through URLSearchParams so
    // contractguard can still read which query parameters this call sends and
    // check them against ListPhotosRequest - a builder hides them from it.
    queryFn: ({ pageParam }) =>
      eveyFetch<PhotoPage>({
        url:
          `/api/v1/photos?libraryId=${libraryID}&limit=${PHOTOS_PAGE_SIZE}` +
          (pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : "&offset=0") +
          (album ? `&album=${encodeURIComponent(album)}` : ""),
        method: "GET",
      }),
    getNextPageParam: (last) => (hasMoreFromCursor(last) ? last.nextCursor : undefined),
    staleTime: 30_000,
  });

  const photos = useMemo(
    () => (query.data?.pages ?? []).flatMap((p) => p.photos ?? []),
    [query.data],
  );

  // Only the first page carries an exact total - a keyset page reports 0 by
  // design, that COUNT(*) OVER() being the cost the cursor path exists to avoid.
  const total = query.data?.pages[0]?.total ?? 0;

  return { ...query, photos, total };
}

export default function PhotosScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [selectedAlbum, setSelectedAlbum] = useState<string | undefined>(undefined);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [rotation, setRotation] = useState(0);

  const { data: libraries, isLoading: libLoading, isError: libError, refetch: refetchLibs } = usePhotoLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const { data: albums, isLoading: albumsLoading } = useAlbums(activeLibraryID);
  const {
    photos,
    total: photosTotal,
    isLoading: photosLoading,
    isError: photosError,
    refetch: refetchPhotos,
    fetchNextPage: fetchMorePhotos,
    hasNextPage: hasMorePhotos,
    isFetchingNextPage: loadingMorePhotos,
  } = usePhotos(activeLibraryID, selectedAlbum);
  const lightboxPhoto = lightboxIndex !== null ? photos[lightboxIndex] : undefined;
  const { data: photoDetail } = usePhotoDetail(lightboxPhoto?.id);

  // Stepping through the lightbox must not stop at the last loaded page.
  // nextLightboxIndex clamps at photos.length - 1, and photos only holds the
  // pages the GRID has pulled in, so opening the viewer without having scrolled
  // to the bottom made the Next arrow vanish partway through the album and the
  // "n / total" counter report the loaded count as the total. The web lightbox
  // already prefetches this way (LIGHTBOX_PREFETCH_MARGIN in
  // routes/_auth/photos/index.lazy.tsx); the mobile one never did.
  useEffect(() => {
    if (lightboxIndex === null) return;
    if (!hasMorePhotos || loadingMorePhotos) return;
    if (lightboxIndex >= photos.length - LIGHTBOX_PREFETCH_MARGIN) void fetchMorePhotos();
  }, [lightboxIndex, photos.length, hasMorePhotos, loadingMorePhotos, fetchMorePhotos]);

  // A stale index would otherwise index into a freshly filtered list when
  // switching library/album - showing the wrong photo, or auto-opening the
  // lightbox on a photo the user never clicked.
  useEffect(() => {
    setLightboxIndex(null);
  }, [selectedAlbum, activeLibraryID]);

  useEffect(() => {
    setRotation(0);
  }, [lightboxIndex]);

  const photoUrl = useCallback(
    (id: string) => (serverUrl ? `${serverUrl}/api/v1/photos/${id}/serve` : undefined),
    [serverUrl],
  );

  const photoThumbUrl = useCallback(
    (id: string) => (serverUrl ? `${serverUrl}/api/v1/photos/${id}/serve?w=400&fmt=webp` : undefined),
    [serverUrl],
  );

  const closeLightbox = useCallback(() => setLightboxIndex(null), []);
  const prevPhoto = useCallback(
    () => setLightboxIndex((i) => (i !== null ? prevLightboxIndex(i, photos.length) : i)),
    [photos.length],
  );
  const nextPhoto = useCallback(
    () => setLightboxIndex((i) => (i !== null ? nextLightboxIndex(i, photos.length) : i)),
    [photos.length],
  );

  const renderPhoto = useCallback(
    ({ item, index }: { item: Photo; index: number }) => {
      const uri = photoThumbUrl(item.id);
      return (
        <TouchableOpacity
          activeOpacity={0.85}
          onPress={() => setLightboxIndex(index)}
          style={{ width: PHOTO_SIZE, height: PHOTO_SIZE, margin: SPACING / 2 }}
          accessibilityRole="imagebutton"
          // Without this a screen reader announces a bare "button" for every
          // tile in the grid, with nothing to tell them apart - while the
          // lightbox chrome in this same file is fully labelled.
          accessibilityLabel={t("media.photos.openPhoto", "Photo {{n}}", { n: index + 1 })}
        >
          {uri ? (
            <Image
              source={{ uri, headers: authHeaders }}
              style={{ width: PHOTO_SIZE, height: PHOTO_SIZE }}
              contentFit="cover"
              transition={150}
              placeholder={item.blurhash ? { blurhash: item.blurhash } : undefined}
              placeholderContentFit="cover"
            />
          ) : (
            <View style={[styles.placeholder, { backgroundColor: theme.colors.surfaceRaised, width: PHOTO_SIZE, height: PHOTO_SIZE }]} />
          )}
        </TouchableOpacity>
      );
    },
    [photoThumbUrl, authHeaders, theme, t],
  );

  const renderAlbum = useCallback(
    ({ item }: { item: Album }) => {
      const coverUri = item.coverPhotoId ? photoThumbUrl(item.coverPhotoId) : undefined;
      return (
        <TouchableOpacity
          activeOpacity={0.85}
          onPress={() => setSelectedAlbum(item.name)}
          style={[styles.albumCard, { width: ALBUM_SIZE, backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityRole="button"
          accessibilityLabel={item.name}
        >
          {coverUri ? (
            <Image
              source={{ uri: coverUri, headers: authHeaders }}
              style={{ width: ALBUM_SIZE, height: ALBUM_SIZE, borderRadius: theme.radii.md }}
              contentFit="cover"
              transition={150}
            />
          ) : (
            <View style={[styles.placeholder, { width: ALBUM_SIZE, height: ALBUM_SIZE, borderRadius: theme.radii.md, backgroundColor: theme.colors.surfaceRaised }]}>
              <Text style={{ fontSize: 32 }}>📷</Text>
            </View>
          )}
          <Text style={[styles.albumTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.name}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.photoCount} photo{item.photoCount !== 1 ? "s" : ""}</Text>
        </TouchableOpacity>
      );
    },
    [photoThumbUrl, authHeaders, theme],
  );

  if (libLoading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>{t("media.photos.title")}</Text>
        <View style={styles.skeletonGrid}>
          {Array.from({ length: 9 }).map((_, i) => (
            <Skeleton key={i} width={PHOTO_SIZE} height={PHOTO_SIZE} borderRadius={0} style={{ margin: SPACING / 2 }} />
          ))}
        </View>
      </View>
    );
  }

  // Before this the failed library call fell through to "no photo libraries",
  // which reads as "you have not set one up" rather than "the server did not
  // answer" - and sends the user configuring something that is already fine.
  if (libError) {
    return <LoadFailed onRetry={() => void refetchLibs()} />;
  }

  if (!libraries || libraries.length === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ fontSize: 32, marginBottom: 12 }}>📷</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.photos.noPhotoLibraries")}</Text>
      </View>
    );
  }

  // Everything above the photo grid (title, library picker, albums, section
  // title / breadcrumb, and the loading/empty states for the grid itself)
  // rides in as the FlatList header rather than a wrapping ScrollView, so
  // the grid below stays a single real scroll container and can virtualize
  // (windowed rendering) instead of mounting every tile up front.
  const listHeader = (
    <>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>{t("media.photos.title")}</Text>

      {/* Library picker */}
      {libraries.length > 1 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.sm }}>
          {libraries.map((lib) => (
            <TouchableOpacity
              key={lib.id}
              activeOpacity={0.7}
              onPress={() => setSelectedLibraryID(lib.id)}
              style={[
                styles.libChip,
                {
                  backgroundColor: activeLibraryID === lib.id ? theme.colors.accent : theme.colors.surface,
                  borderRadius: theme.radii.sm,
                  marginRight: theme.spacing.sm,
                },
              ]}
            >
              <Text style={{ color: activeLibraryID === lib.id ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                {lib.name}
              </Text>
            </TouchableOpacity>
          ))}
        </ScrollView>
      )}

      {/* Albums section (hidden while drilled into one album). Small, bounded
          dataset (one row of covers) so a non-virtualized inner FlatList
          here is fine - it lives inside the header, not the scrolling grid. */}
      {!selectedAlbum && (
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>{t("media.photos.albums")}</Text>
          {albumsLoading ? (
            <View style={styles.albumGrid}>
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} width={ALBUM_SIZE} height={ALBUM_SIZE} borderRadius={theme.radii.md} style={{ marginBottom: theme.spacing.sm }} />
              ))}
            </View>
          ) : !albums || albums.length === 0 ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: theme.spacing.md }}>{t("media.photos.noAlbums")}</Text>
          ) : (
            <FlatList
              data={albums}
              keyExtractor={(item) => item.name}
              renderItem={renderAlbum}
              numColumns={NUM_ALBUM_COLS}
              scrollEnabled={false}
              columnWrapperStyle={{ gap: theme.spacing.sm, marginBottom: theme.spacing.sm }}
            />
          )}
        </View>
      )}

      {/* Photos section heading - all photos, or the selected album's breadcrumb */}
      <View style={{ marginTop: theme.spacing.md }}>
        {selectedAlbum ? (
          <View style={{ flexDirection: "row", alignItems: "center", gap: 4, paddingHorizontal: theme.spacing.md, marginBottom: theme.spacing.sm }}>
            <TouchableOpacity
              onPress={() => setSelectedAlbum(undefined)}
              activeOpacity={0.7}
              style={{ flexDirection: "row", alignItems: "center" }}
            >
              <Ionicons name="chevron-back" size={16} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("media.photos.albums")}</Text>
            </TouchableOpacity>
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>/</Text>
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", flexShrink: 1 }} numberOfLines={1}>
              {selectedAlbum.split("/").pop()}
            </Text>
          </View>
        ) : (
          <Text style={[styles.sectionTitle, { color: theme.colors.text, paddingHorizontal: theme.spacing.md }]}>{t("media.photos.allPhotos")}</Text>
        )}
        {photosLoading ? (
          <View style={styles.skeletonGrid}>
            {Array.from({ length: 12 }).map((_, i) => (
              <Skeleton key={i} width={PHOTO_SIZE} height={PHOTO_SIZE} borderRadius={0} style={{ margin: SPACING / 2 }} />
            ))}
          </View>
        ) : photosError ? (
          <LoadFailed inline onRetry={() => void refetchPhotos()} />
        ) : photos.length === 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, paddingHorizontal: theme.spacing.md, marginBottom: theme.spacing.lg }}>{t("media.photos.noPhotos")}</Text>
        ) : null}
      </View>
    </>
  );

  const listFooter =
    !photosLoading && photosTotal > 0 ? (
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, textAlign: "right", paddingHorizontal: theme.spacing.md, marginTop: 4, marginBottom: theme.spacing.lg }}>
        {t("media.photos.photoCount", { count: photosTotal })}
      </Text>
    ) : null;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <FlatList
        data={photosLoading ? [] : photos}
        keyExtractor={(item) => item.id}
        renderItem={renderPhoto}
        numColumns={NUM_PHOTO_COLS}
        ListHeaderComponent={listHeader}
        ListFooterComponent={listFooter}
        contentContainerStyle={{ paddingHorizontal: SPACING / 2 }}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (hasMorePhotos && !loadingMorePhotos) void fetchMorePhotos();
        }}
      />

      {/* Lightbox */}
      <Modal visible={lightboxIndex !== null} transparent animationType="fade" onRequestClose={closeLightbox}>
        <Pressable style={styles.lightboxBg} onPress={closeLightbox}>
          <View style={styles.lightboxTopBar}>
            <TouchableOpacity
              onPress={(e) => { e.stopPropagation(); setRotation(rotateCounterClockwise); }}
              hitSlop={10}
              accessibilityLabel={t("common.rotateLeft")}
            >
              <Ionicons name="arrow-undo-outline" size={24} color="#fff" />
            </TouchableOpacity>
            <TouchableOpacity
              onPress={(e) => { e.stopPropagation(); setRotation(rotateClockwise); }}
              hitSlop={10}
              accessibilityLabel={t("common.rotateRight")}
            >
              <Ionicons name="arrow-redo-outline" size={24} color="#fff" />
            </TouchableOpacity>
            <TouchableOpacity onPress={closeLightbox} hitSlop={10} accessibilityLabel={t("common.aria.close")}>
              <Ionicons name="close" size={28} color="#fff" />
            </TouchableOpacity>
          </View>

          {lightboxIndex !== null && lightboxIndex > 0 && (
            <TouchableOpacity
              onPress={(e) => { e.stopPropagation(); prevPhoto(); }}
              style={[styles.lightboxNav, { left: 8 }]}
              hitSlop={10}
              accessibilityLabel={t("common.previous")}
            >
              <Ionicons name="chevron-back" size={28} color="#fff" />
            </TouchableOpacity>
          )}
          {lightboxIndex !== null && lightboxIndex < photos.length - 1 && (
            <TouchableOpacity
              onPress={(e) => { e.stopPropagation(); nextPhoto(); }}
              style={[styles.lightboxNav, { right: 8 }]}
              hitSlop={10}
              accessibilityLabel={t("common.next")}
            >
              <Ionicons name="chevron-forward" size={28} color="#fff" />
            </TouchableOpacity>
          )}

          {lightboxPhoto && serverUrl && (
            <Image
              source={{ uri: photoUrl(lightboxPhoto.id), headers: authHeaders }}
              style={[styles.lightboxImage, { transform: [{ rotate: `${rotation}deg` }] }]}
              contentFit="contain"
            />
          )}
          {lightboxPhoto && (
            <View style={styles.lightboxMeta}>
              <Text style={styles.lightboxTitle} numberOfLines={1}>{lightboxPhoto.title || lightboxPhoto.album || ""}</Text>
              <Text style={styles.lightboxInfo}>{(lightboxIndex ?? 0) + 1} / {photos.length}</Text>
              {photoDetail && (
                <View style={{ gap: 2, marginTop: 6, alignItems: "center" }}>
                  {photoDetail.width > 0 && photoDetail.height > 0 && (
                    <Text style={styles.lightboxInfo}>{photoDetail.width} x {photoDetail.height}</Text>
                  )}
                  {photoDetail.fileSize > 0 && (
                    <Text style={styles.lightboxInfo}>{formatBytes(photoDetail.fileSize)}</Text>
                  )}
                  {photoDetail.takenAt && (
                    <Text style={styles.lightboxInfo}>{new Date(photoDetail.takenAt).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" })}</Text>
                  )}
                  {photoDetail.cameraModel && (
                    <Text style={styles.lightboxInfo}>{photoDetail.cameraModel}</Text>
                  )}
                </View>
              )}
            </View>
          )}
        </Pressable>
      </Modal>

      {/* Fullscreen loading overlay for initial photo load */}
      {(albumsLoading || photosLoading) && !libLoading && (
        <ActivityIndicator
          color={theme.colors.accent}
          style={{ position: "absolute", top: 80, alignSelf: "center" }}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 28, fontWeight: "700", marginBottom: 8 },
  sectionTitle: { fontSize: 18, fontWeight: "600", marginBottom: 10 },
  skeletonGrid: { flexDirection: "row", flexWrap: "wrap", paddingHorizontal: SPACING / 2 },
  albumGrid: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  albumCard: { alignItems: "center", overflow: "hidden", paddingBottom: 6 },
  albumTitle: { fontSize: 13, fontWeight: "600", marginTop: 6, paddingHorizontal: 6, alignSelf: "stretch" },
  placeholder: { justifyContent: "center", alignItems: "center" },
  libChip: { paddingHorizontal: 14, paddingVertical: 7 },
  lightboxBg: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.92)",
    justifyContent: "center",
    alignItems: "center",
  },
  lightboxTopBar: {
    position: "absolute",
    top: 48,
    right: 16,
    flexDirection: "row",
    alignItems: "center",
    gap: 20,
    zIndex: 1,
  },
  lightboxNav: {
    position: "absolute",
    top: "50%",
    marginTop: -20,
    padding: 8,
    borderRadius: 999,
    backgroundColor: "rgba(0,0,0,0.4)",
    zIndex: 1,
  },
  lightboxImage: {
    width: screenWidth,
    height: screenWidth,
  },
  lightboxMeta: {
    position: "absolute",
    bottom: 40,
    left: 16,
    right: 16,
    alignItems: "center",
  },
  lightboxTitle: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "500",
  },
  lightboxInfo: {
    color: "rgba(255,255,255,0.6)",
    fontSize: 12,
  },
});
