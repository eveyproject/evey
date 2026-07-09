import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router, useLocalSearchParams, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMusicServiceGetAlbums } from "@evey/api";
import type { V1MusicAlbum } from "@evey/api";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../../src/components/FocusableCard";
import { musicAlbumHref } from "../../../../src/lib/musicRoutes";

const NUM_COLUMNS = 5;
const CARD_WIDTH = 220;
const ART_SIZE = CARD_WIDTH;
const ROW_HEIGHT = ART_SIZE + 60 + 16;

function coverUri(serverUrl: string, coverArtUrl?: string): string | undefined {
  if (!coverArtUrl) return undefined;
  return /^https?:/i.test(coverArtUrl) ? coverArtUrl : `${serverUrl}${coverArtUrl}`;
}

function albumKeyExtractor(item: V1MusicAlbum) {
  return item.id ?? "";
}

const albumGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

export default function ArtistDetailScreen() {
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
  const { data: albums, isLoading } = useMusicServiceGetAlbums(
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
      {isLoading ? (
        <View style={styles.center}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={albumKeyExtractor}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ padding: 24 }}
          columnWrapperStyle={{ gap: 16, marginBottom: 16 }}
          getItemLayout={albumGetItemLayout}
          renderItem={({ item, index }) => {
            const uri = coverUri(serverUrl, item.coverArtUrl);
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
                      source={{ uri, headers: authHeaders }}
                      style={{ width: ART_SIZE, height: ART_SIZE, borderRadius: theme.radii.sm }}
                      contentFit="cover"
                      transition={200}
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
              <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.music.noAlbums")}</Text>
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
  title: { fontSize: 16, fontWeight: "600", marginTop: 8 },
  subtitle: { fontSize: 14, marginTop: 2 },
});
