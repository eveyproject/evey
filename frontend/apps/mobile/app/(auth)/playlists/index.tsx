import { useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
  Modal,
  TextInput,
  Alert,
  Pressable,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { Stack, useRouter } from "expo-router";
import { Skeleton } from "../../../src/components/Skeleton";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { useTheme } from "../../../src/providers/ThemeProvider";
import {
  usePlaylistList,
  useCreatePlaylist,
  useDeletePlaylist,
  type Playlist,
} from "../../../src/api/playlists";

function PlaylistCard({
  playlist,
  theme,
  onPress,
  onLongPress,
}: {
  playlist: Playlist;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
  onLongPress: () => void;
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
      onLongPress={onLongPress}
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
        {playlist.type?.includes("SMART") ? (
          <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1, alignSelf: "flex-start", marginTop: 2 }}>
            <Text style={{ color: theme.colors.onAccent, fontSize: 10, fontWeight: "700" }}>SMART</Text>
          </View>
        ) : null}
      </View>
      <Ionicons name="chevron-forward" size={18} color={theme.colors.textMuted} />
    </TouchableOpacity>
  );
}

type PlaylistType = "manual" | "smart";

export default function PlaylistsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data: playlists, isLoading, isError, refetch } = usePlaylistList();
  const createMutation = useCreatePlaylist();
  const deleteMutation = useDeletePlaylist();

  const [modalVisible, setModalVisible] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [newType, setNewType] = useState<PlaylistType>("manual");

  const resetForm = () => {
    setNewName("");
    setNewDesc("");
    setNewType("manual");
  };

  const handleCreate = async () => {
    if (!newName.trim()) return;
    try {
      const res = await createMutation.mutateAsync({
        name: newName.trim(),
        description: newDesc.trim() || undefined,
        type: newType,
      });
      setModalVisible(false);
      resetForm();
      if (res?.playlist?.id) {
        router.push(`/playlists/${res.playlist.id}`);
      }
    } catch {
      Alert.alert(t("common.error"), t("media.playlist.createError"));
    }
  };

  const confirmDelete = (playlist: Playlist) => {
    Alert.alert(
      t("common.delete"),
      t("media.playlist.confirmDelete", { name: playlist.name }),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.delete"),
          style: "destructive",
          onPress: async () => {
            try {
              await deleteMutation.mutateAsync(playlist.id);
            } catch {
              Alert.alert(t("common.error"), t("media.playlist.deleteError"));
            }
          },
        },
      ],
    );
  };

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("common.playlists"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerRight: () => (
            <TouchableOpacity
              onPress={() => setModalVisible(true)}
              style={{ marginRight: 8 }}
              hitSlop={8}
            >
              <Ionicons name="add" size={26} color={theme.colors.accent} />
            </TouchableOpacity>
          ),
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
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
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
              onLongPress={() => confirmDelete(item)}
            />
          )}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>{t("common.noPlaylists")}</Text>
            </View>
          }
        />
      )}

      {/* Create playlist modal */}
      <Modal
        visible={modalVisible}
        transparent
        animationType="fade"
        onRequestClose={() => setModalVisible(false)}
      >
        <Pressable style={styles.overlay} onPress={() => setModalVisible(false)}>
          <Pressable
            style={[styles.modal, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}
            onPress={() => {}}
          >
            <Text style={[styles.modalTitle, { color: theme.colors.text }]}>
              {t("media.playlist.create")}
            </Text>

            <TextInput
              style={[
                styles.input,
                {
                  backgroundColor: theme.colors.bg,
                  color: theme.colors.text,
                  borderColor: theme.colors.border,
                  borderRadius: theme.radii.sm,
                },
              ]}
              placeholder={t("media.playlist.namePlaceholder")}
              placeholderTextColor={theme.colors.textMuted}
              value={newName}
              onChangeText={setNewName}
              autoFocus
            />

            <TextInput
              style={[
                styles.input,
                {
                  backgroundColor: theme.colors.bg,
                  color: theme.colors.text,
                  borderColor: theme.colors.border,
                  borderRadius: theme.radii.sm,
                },
              ]}
              placeholder={t("media.playlist.descriptionPlaceholder")}
              placeholderTextColor={theme.colors.textMuted}
              value={newDesc}
              onChangeText={setNewDesc}
            />

            <View style={styles.typePicker}>
              {(["manual", "smart"] as PlaylistType[]).map((type) => (
                <TouchableOpacity
                  key={type}
                  onPress={() => setNewType(type)}
                  style={[
                    styles.typeBtn,
                    {
                      backgroundColor: newType === type ? theme.colors.accent : theme.colors.bg,
                      borderColor: theme.colors.border,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                >
                  <Text
                    style={{
                      color: newType === type ? theme.colors.onAccent : theme.colors.text,
                      fontWeight: "600",
                      fontSize: 13,
                    }}
                  >
                    {type === "manual" ? t("media.playlist.typeManual") : t("media.playlist.typeSmart")}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <View style={styles.modalActions}>
              <TouchableOpacity
                onPress={() => { setModalVisible(false); resetForm(); }}
                style={[styles.actionBtn, { borderColor: theme.colors.border, borderRadius: theme.radii.sm }]}
              >
                <Text style={{ color: theme.colors.textMuted }}>{t("common.cancel")}</Text>
              </TouchableOpacity>
              <TouchableOpacity
                onPress={handleCreate}
                disabled={!newName.trim() || createMutation.isPending}
                style={[
                  styles.actionBtn,
                  {
                    backgroundColor: newName.trim() ? theme.colors.accent : theme.colors.border,
                    borderRadius: theme.radii.sm,
                  },
                ]}
              >
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>
                  {createMutation.isPending ? "..." : t("common.create")}
                </Text>
              </TouchableOpacity>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
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
  overlay: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.5)",
    justifyContent: "center",
    alignItems: "center",
    padding: 24,
  },
  modal: { width: "100%", maxWidth: 400, padding: 20 },
  modalTitle: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  input: { borderWidth: 1, padding: 12, fontSize: 15, marginBottom: 12 },
  typePicker: { flexDirection: "row", gap: 8, marginBottom: 16 },
  typeBtn: { flex: 1, paddingVertical: 10, alignItems: "center", borderWidth: 1 },
  modalActions: { flexDirection: "row", gap: 8, justifyContent: "flex-end" },
  actionBtn: { paddingHorizontal: 20, paddingVertical: 10, borderWidth: 1, borderColor: "transparent" },
});
