import { View, Text, FlatList, StyleSheet } from "react-native";
import { useGridColumns, DEEP_SCREEN_GRID_INSET } from "../../../../src/hooks/useGridColumns";
import { Image } from "expo-image";
import { router, useLocalSearchParams, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMusicServiceGetAlbums } from "@evey/api";
import type { V1MusicAlbum } from "@evey/api";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../../src/components/FocusableCard";
import { ScreenState } from "../../../../src/components/ScreenState";
import { musicAlbumHref } from "../../../../src/lib/musicRoutes";
import { CARD_WIDTH_FEATURED, GRID_GAP } from "../../../../src/lib/tvLayout";
import { coverArtUri } from "../../../../src/lib/artwork";

const CARD_WIDTH = CARD_WIDTH_FEATURED;
const ART_SIZE = CARD_WIDTH;
// Taller label block than LABEL_BLOCK_HEIGHT (40): this grid also shows an
// optional year line below the title, so it needs more reserved height.
const ROW_HEIGHT = ART_SIZE + 60 + GRID_GAP;

function albumKeyExtractor(item: V1MusicAlbum) {
  return item.id ?? "";
}

// With columns, the list indexes its ROWS: the offset is the row's.
const albumGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * index,
  index,
});

export default function ArtistDetailScreen() {
  const numColumns = useGridColumns(CARD_WIDTH, DEEP_SCREEN_GRID_INSET);
  const { id, libraryId } = useLocalSearchParams<{ id: string; libraryId?: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const artistId = id ?? "";
  const libId = libraryId ?? "";

  // The albums endpoint has no artist filter - album identity/artist are
  // resolved server-side and artist.id IS the display artist name (see
  // GetArtists), so filtering the library's album list by that name matches
  // what the server itself groups under this artist.
  const { data: albums, isLoading, isError, refetch } = useMusicServiceGetAlbums(
    { libraryId: libId },
    {
      query: {
        enabled: !!libId,
        select: (d) => (d.albums ?? []).filter((a) => a.artist === artistId),
      },
    },
  );

  const items = albums ?? [];

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: artistId,
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isLoading || isError ? (
        // Checked before the list, or a failed fetch renders as "no albums"
        // via ListEmptyComponent - a lie the viewer has no way to see through.
        <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />
      ) : (
        <FlatList
          data={items}
          keyExtractor={albumKeyExtractor}
          key={numColumns}
          numColumns={numColumns}
          contentContainerStyle={{ padding: 24 }}
          columnWrapperStyle={{ gap: 12, marginBottom: 16 }}
          getItemLayout={albumGetItemLayout}
          renderItem={({ item, index }) => {
            const uri = coverArtUri(serverUrl, item.coverArtUrl, ART_SIZE * 2);
            return (
              <FocusableCard
                onPress={() => item.id && router.push(musicAlbumHref(item.id, libId))}
                hasTVPreferredFocus={index === 0}
                style={{ width: CARD_WIDTH }}
                accessibilityLabel={item.title}
              >
                <View style={[styles.art, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
                  {uri ? (
                    <Image
                      cachePolicy="memory-disk"
                      source={{ uri, headers: authHeaders }}
                      style={{ width: ART_SIZE, height: ART_SIZE, borderRadius: theme.radii.sm }}
                      contentFit="cover"
                      recyclingKey={item.id}
                    />
                  ) : (
                    <Text style={{ fontSize: 40 }}>{"♪"}</Text>
                  )}
                </View>
                <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
                  {item.title || "-"}
                </Text>
                {item.year ? (
                  <Text style={[styles.subtitle, { color: theme.colors.textMuted }]}>{item.year}</Text>
                ) : null}
              </FocusableCard>
            );
          }}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("media.music.noAlbums")}</Text>
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
  art: { width: ART_SIZE, height: ART_SIZE, justifyContent: "center", alignItems: "center", overflow: "hidden" },
  title: { fontSize: 15, fontWeight: "600", marginTop: 8 },
  subtitle: { fontSize: 13, marginTop: 2 },
});
