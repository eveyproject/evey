import { View, Text, FlatList, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMusicServiceListTracks, formatTime } from "@evey/api";
import type { V1Track } from "@evey/api";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../../src/components/FocusableCard";
import { ScreenState } from "../../../../src/components/ScreenState";
import { useMusicQueueStore, type QueueTrack } from "../../../../src/stores/musicQueue";
import { coverArtUri } from "../../../../src/lib/artwork";

const ALBUM_ART_SIZE = 160;

function trackToQueueTrack(t: V1Track): QueueTrack {
  return {
    id: t.id ?? "",
    title: t.title ?? "",
    artist: t.artist ?? "",
    album: t.album ?? "",
    duration: t.duration ?? 0,
  };
}

function trackKeyExtractor(item: V1Track) {
  return item.id ?? "";
}

export default function AlbumDetailScreen() {
  const { id, libraryId } = useLocalSearchParams<{ id: string; libraryId?: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const albumId = id ?? "";
  const libId = libraryId ?? "";

  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const queue = useMusicQueueStore((s) => s.queue);
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const currentTrackId = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex].id : undefined;

  const { data: tracks, isLoading, isError, refetch } = useMusicServiceListTracks(
    { libraryId: libId, albumId },
    { query: { enabled: !!libId && !!albumId, select: (d) => d.tracks ?? [] } },
  );

  const items = tracks ?? [];
  const albumTitle = items[0]?.album || albumId;
  const albumArtist = items[0]?.artist || "";
  const albumArt = coverArtUri(serverUrl, items[0]?.coverArtUrl, ALBUM_ART_SIZE * 2);

  function handlePlayTrack(index: number) {
    setQueue(items.map(trackToQueueTrack), index);
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: albumTitle,
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isLoading || isError ? (
        // Checked before the list, or a failed fetch renders as "no tracks"
        // via ListEmptyComponent - a lie the viewer has no way to see through.
        <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />
      ) : (
        <FlatList
          data={items}
          keyExtractor={trackKeyExtractor}
          contentContainerStyle={{ padding: 24 }}
          ListHeaderComponent={
            <View style={styles.albumHeader}>
              <View style={[styles.albumArt, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
                {albumArt ? (
                  <Image
                    source={{ uri: albumArt, headers: authHeaders }}
                    style={{ width: ALBUM_ART_SIZE, height: ALBUM_ART_SIZE, borderRadius: theme.radii.sm }}
                    contentFit="cover"
                    transition={200}
                  />
                ) : (
                  <Text style={{ fontSize: 48 }}>{"♪"}</Text>
                )}
              </View>
              <View style={{ marginLeft: 24, flex: 1 }}>
                <Text style={[styles.albumTitle, { color: theme.colors.text }]} numberOfLines={2}>
                  {albumTitle}
                </Text>
                <Text style={[styles.albumArtist, { color: theme.colors.textMuted }]} numberOfLines={1}>
                  {albumArtist}
                </Text>
              </View>
            </View>
          }
          renderItem={({ item, index }) => {
            const isPlaying = !!item.id && item.id === currentTrackId;
            return (
              <FocusableCard
                onPress={() => handlePlayTrack(index)}
                hasTVPreferredFocus={index === 0}
                style={styles.trackRow}
                accessibilityLabel={item.title}
              >
                <Text style={[styles.trackIndex, { color: theme.colors.textMuted }]}>
                  {item.trackNumber || index + 1}
                </Text>
                <View style={{ flex: 1, marginLeft: 16 }}>
                  <Text
                    style={[styles.trackTitle, { color: isPlaying ? theme.colors.accent : theme.colors.text }]}
                    numberOfLines={1}
                  >
                    {item.title || "-"}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 14 }} numberOfLines={1}>
                    {item.artist || "-"}
                  </Text>
                </View>
                {item.duration ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>
                    {formatTime(item.duration)}
                  </Text>
                ) : null}
              </FocusableCard>
            );
          }}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.music.noTracks")}</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  albumHeader: { flexDirection: "row", alignItems: "center", marginBottom: 24 },
  albumArt: { width: ALBUM_ART_SIZE, height: ALBUM_ART_SIZE, justifyContent: "center", alignItems: "center", overflow: "hidden" },
  albumTitle: { fontSize: 28, fontWeight: "700" },
  albumArtist: { fontSize: 18, marginTop: 6 },
  trackRow: { flexDirection: "row", alignItems: "center", paddingVertical: 14, paddingHorizontal: 16 },
  trackIndex: { width: 32, fontSize: 16, textAlign: "center" },
  trackTitle: { fontSize: 18, fontWeight: "600" },
});
