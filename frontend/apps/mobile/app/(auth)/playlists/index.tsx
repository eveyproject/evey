import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack, useRouter } from "expo-router";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistList, type Playlist } from "../../../src/api/playlists";

function PlaylistCard({
  playlist,
  theme,
  onPress,
}: {
  playlist: Playlist;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      style={[
        styles.card,
        {
          backgroundColor: theme.colors.surface,
          borderRadius: theme.radii.md,
        },
      ]}
      activeOpacity={0.7}
      onPress={onPress}
    >
      <View
        style={[
          styles.iconBox,
          {
            backgroundColor: theme.colors.surfaceRaised,
            borderRadius: theme.radii.sm,
          },
        ]}
      >
        <Ionicons name="list" size={20} color={theme.colors.accent} />
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
        <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
          {playlist.type}
        </Text>
      </View>
      <Ionicons name="chevron-forward" size={18} color={theme.colors.textMuted} />
    </TouchableOpacity>
  );
}

export default function PlaylistsScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { data: playlists, isLoading, refetch } = usePlaylistList();

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: "Playlists",
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isLoading ? (
        <View style={{ padding: theme.spacing.md }}>
          {Array.from({ length: 5 }).map((_, i) => (
            <View key={i} style={{ marginBottom: theme.spacing.sm }}>
              <Skeleton width="100%" height={64} borderRadius={theme.radii.md} />
            </View>
          ))}
        </View>
      ) : (
        <FlatList
          data={playlists ?? []}
          keyExtractor={(item) => item.id}
          contentContainerStyle={{ padding: theme.spacing.md }}
          ItemSeparatorComponent={() => <View style={{ height: theme.spacing.sm }} />}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }) => (
            <PlaylistCard
              playlist={item}
              theme={theme}
              onPress={() => router.push(`/playlists/${item.id}`)}
            />
          )}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>No playlists</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 64 },
  card: { flexDirection: "row", alignItems: "center", padding: 14 },
  iconBox: { width: 40, height: 40, justifyContent: "center", alignItems: "center", marginRight: 12 },
  name: { fontSize: 16, fontWeight: "600" },
  desc: { fontSize: 12, marginTop: 2 },
  meta: { fontSize: 11, marginTop: 2 },
});
