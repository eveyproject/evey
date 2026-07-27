import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistList, type Playlist } from "../../../src/api/playlists";
import { FocusableCard } from "../../../src/components/FocusableCard";

export default function PlaylistsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data: playlists, isLoading } = usePlaylistList();

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.playlists")}</Text>
      <FlatList
        data={playlists ?? []}
        keyExtractor={(item) => item.id}
        contentContainerStyle={{ padding: 24 }}
        ItemSeparatorComponent={() => <View style={{ height: 12 }} />}
        renderItem={({ item, index }) => (
          <PlaylistCard
            playlist={item}
            theme={theme}
            isFirst={index === 0}
            onPress={() => router.push(`/playlists/${item.id}`)}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("common.noPlaylists")}</Text>
            {/* Focusable target for the empty state - without it the remote has
                nowhere to go and gets stuck on this screen (no other control
                is rendered when the list is empty). */}
            <FocusableCard
              onPress={() => router.push("/(auth)/(tabs)/library")}
              hasTVPreferredFocus
              style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
            </FocusableCard>
          </View>
        }
      />
    </View>
  );
}

function PlaylistCard({
  playlist,
  theme,
  isFirst,
  onPress,
}: {
  playlist: Playlist;
  theme: ReturnType<typeof useTheme>;
  isFirst?: boolean;
  onPress: () => void;
}) {
  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <View style={[styles.iconBox, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
        <Text style={{ color: theme.colors.accent, fontSize: 24 }}>{"\u266A"}</Text>
      </View>
      <View style={{ flex: 1 }}>
        <Text style={[styles.name, { color: theme.colors.text }]} numberOfLines={1}>
          {playlist.name}
        </Text>
        {playlist.description ? (
          <Text style={[styles.desc, { color: theme.colors.textMuted }]} numberOfLines={1}>
            {playlist.description}
          </Text>
        ) : null}
        <Text style={[styles.meta, { color: theme.colors.textMuted }]}>{playlist.type}</Text>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>{"\u203A"}</Text>
    </FocusableCard>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  card: { flexDirection: "row", alignItems: "center", padding: 20 },
  iconBox: { width: 56, height: 56, justifyContent: "center", alignItems: "center", marginRight: 16 },
  name: { fontSize: 22, fontWeight: "600" },
  desc: { fontSize: 16, marginTop: 2 },
  meta: { fontSize: 14, marginTop: 2 },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 14 },
});
