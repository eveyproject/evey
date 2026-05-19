import { useState, useCallback } from "react";
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
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

// Field names are lowerCamelCase because /api/v1/photos* is served by the
// grpc-gateway (protojson), not a custom PascalCase HTTP handler. Reading
// PascalCase here previously yielded `undefined` for every field — the
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

function formatFileSize(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`;
  return `${(bytes / 1024).toFixed(0)} KB`;
}

function usePhotos(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["photos", "list", libraryID],
    queryFn: () => eveyFetch<{ photos: Photo[]; total: number }>({ url: `/api/v1/photos?library_id=${libraryID}&limit=200`, method: "GET" }),
    enabled: !!libraryID,
    select: (d) => d.photos ?? [],
    staleTime: 30_000,
  });
}

export default function PhotosScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [lightboxPhoto, setLightboxPhoto] = useState<Photo | null>(null);

  const { data: libraries, isLoading: libLoading } = usePhotoLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const { data: albums, isLoading: albumsLoading } = useAlbums(activeLibraryID);
  const { data: photos, isLoading: photosLoading } = usePhotos(activeLibraryID);
  const { data: photoDetail } = usePhotoDetail(lightboxPhoto?.id);

  const photoUrl = useCallback(
    (id: string) => (serverUrl ? `${serverUrl}/api/v1/photos/${id}/serve` : undefined),
    [serverUrl],
  );

  const renderPhoto = useCallback(
    ({ item }: { item: Photo }) => {
      const uri = photoUrl(item.id);
      return (
        <TouchableOpacity
          activeOpacity={0.85}
          onPress={() => setLightboxPhoto(item)}
          style={{ width: PHOTO_SIZE, height: PHOTO_SIZE, margin: SPACING / 2 }}
        >
          {uri ? (
            <Image
              source={{ uri, headers: authHeaders }}
              style={{ width: PHOTO_SIZE, height: PHOTO_SIZE }}
              contentFit="cover"
              transition={150}
            />
          ) : (
            <View style={[styles.placeholder, { backgroundColor: theme.colors.surfaceRaised, width: PHOTO_SIZE, height: PHOTO_SIZE }]} />
          )}
        </TouchableOpacity>
      );
    },
    [photoUrl, authHeaders, theme],
  );

  const renderAlbum = useCallback(
    ({ item }: { item: Album }) => {
      const coverUri = item.coverPhotoId ? photoUrl(item.coverPhotoId) : undefined;
      return (
        <View style={[styles.albumCard, { width: ALBUM_SIZE, backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
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
        </View>
      );
    },
    [photoUrl, authHeaders, theme],
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

  if (!libraries || libraries.length === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ fontSize: 32, marginBottom: 12 }}>📷</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.photos.noPhotoLibraries")}</Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <ScrollView>
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
                <Text style={{ color: activeLibraryID === lib.id ? "#fff" : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                  {lib.name}
                </Text>
              </TouchableOpacity>
            ))}
          </ScrollView>
        )}

        {/* Albums section */}
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

        {/* All photos section */}
        <View style={{ marginTop: theme.spacing.md }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text, paddingHorizontal: theme.spacing.md }]}>{t("media.photos.allPhotos")}</Text>
          {photosLoading ? (
            <View style={styles.skeletonGrid}>
              {Array.from({ length: 12 }).map((_, i) => (
                <Skeleton key={i} width={PHOTO_SIZE} height={PHOTO_SIZE} borderRadius={0} style={{ margin: SPACING / 2 }} />
              ))}
            </View>
          ) : !photos || photos.length === 0 ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 13, paddingHorizontal: theme.spacing.md, marginBottom: theme.spacing.lg }}>{t("media.photos.noPhotos")}</Text>
          ) : (
            <FlatList
              data={photos}
              keyExtractor={(item) => item.id}
              renderItem={renderPhoto}
              numColumns={NUM_PHOTO_COLS}
              scrollEnabled={false}
              contentContainerStyle={{ paddingHorizontal: SPACING / 2 }}
            />
          )}
        </View>
      </ScrollView>

      {/* Lightbox */}
      <Modal visible={!!lightboxPhoto} transparent animationType="fade" onRequestClose={() => setLightboxPhoto(null)}>
        <Pressable style={styles.lightboxBg} onPress={() => setLightboxPhoto(null)}>
          {lightboxPhoto && serverUrl && (
            <Image
              source={{ uri: photoUrl(lightboxPhoto.id), headers: authHeaders }}
              style={styles.lightboxImage}
              contentFit="contain"
            />
          )}
          {lightboxPhoto && (
            <View style={styles.lightboxMeta}>
              <Text style={styles.lightboxTitle} numberOfLines={1}>{lightboxPhoto.title || lightboxPhoto.album || ""}</Text>
              {photoDetail && (
                <View style={{ gap: 2, marginTop: 6, alignItems: "center" }}>
                  {photoDetail.width > 0 && photoDetail.height > 0 && (
                    <Text style={styles.lightboxInfo}>{photoDetail.width} x {photoDetail.height}</Text>
                  )}
                  {photoDetail.fileSize > 0 && (
                    <Text style={styles.lightboxInfo}>{formatFileSize(photoDetail.fileSize)}</Text>
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
