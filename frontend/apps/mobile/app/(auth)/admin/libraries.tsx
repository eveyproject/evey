import {
  View, Text, FlatList, Pressable, Alert, ActivityIndicator,
  StyleSheet, Modal, TextInput, Switch, ScrollView,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useLibraryServiceCreateLibrary,
  useLibraryServiceUpdateLibrary,
  useLibraryServiceDeleteLibrary,
  getLibraryServiceListLibrariesQueryKey,
} from "@evey/api";
import type { V1Library, V1LibraryType } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "film-outline",
  LIBRARY_TYPE_SHOWS: "tv-outline",
  LIBRARY_TYPE_MUSIC: "musical-notes-outline",
  LIBRARY_TYPE_PHOTOS: "image-outline",
  LIBRARY_TYPE_BOOKS: "book-outline",
};

const LIBRARY_TYPES: { value: V1LibraryType; labelKey: string; fallback: string }[] = [
  { value: "LIBRARY_TYPE_MOVIES", labelKey: "admin.libraries.types.movies", fallback: "Movies" },
  { value: "LIBRARY_TYPE_SHOWS", labelKey: "admin.libraries.types.shows", fallback: "TV Shows" },
  { value: "LIBRARY_TYPE_MUSIC", labelKey: "admin.libraries.types.music", fallback: "Music" },
  { value: "LIBRARY_TYPE_PHOTOS", labelKey: "admin.libraries.types.photos", fallback: "Photos" },
];

interface LibraryFormData {
  name: string;
  type: V1LibraryType;
  paths: string[];
  scanOnStartup: boolean;
  realtimeMonitoring: boolean;
}

const EMPTY_FORM: LibraryFormData = {
  name: "",
  type: "LIBRARY_TYPE_MOVIES",
  paths: [""],
  scanOnStartup: true,
  realtimeMonitoring: false,
};

function formFromLibrary(lib: V1Library): LibraryFormData {
  return {
    name: lib.name ?? "",
    type: (lib.type as V1LibraryType) ?? "LIBRARY_TYPE_MOVIES",
    paths: lib.paths?.length ? [...lib.paths] : [""],
    scanOnStartup: lib.scanOnStartup ?? true,
    realtimeMonitoring: lib.realtimeMonitoring ?? false,
  };
}

export default function AdminLibrariesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];

  const [modalVisible, setModalVisible] = useState(false);
  const [editingLibrary, setEditingLibrary] = useState<V1Library | null>(null);
  const [form, setForm] = useState<LibraryFormData>(EMPTY_FORM);

  const scanLibrary = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => toast.success(t("admin.libraries.scanStarted", "Scan started")),
      onError: () => toast.error(t("admin.libraries.scanFailed", "Failed to start scan")),
    },
  });

  const createLib = useLibraryServiceCreateLibrary({
    mutation: {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getLibraryServiceListLibrariesQueryKey() });
        toast.success(t("admin.libraries.libraryCreated") || "Library created");
        closeModal();
      },
      onError: () => toast.error(t("common.error") || "Failed to create library"),
    },
  });

  const updateLib = useLibraryServiceUpdateLibrary({
    mutation: {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getLibraryServiceListLibrariesQueryKey() });
        toast.success(t("admin.libraries.libraryUpdated") || "Library updated");
        closeModal();
      },
      onError: () => toast.error(t("common.error") || "Failed to update library"),
    },
  });

  const deleteLib = useLibraryServiceDeleteLibrary({
    mutation: {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getLibraryServiceListLibrariesQueryKey() });
        toast.success(t("admin.libraries.libraryDeleted") || "Library deleted");
      },
      onError: () => toast.error(t("common.error") || "Failed to delete library"),
    },
  });

  const openCreate = useCallback(() => {
    setEditingLibrary(null);
    setForm(EMPTY_FORM);
    setModalVisible(true);
  }, []);

  const openEdit = useCallback((lib: V1Library) => {
    setEditingLibrary(lib);
    setForm(formFromLibrary(lib));
    setModalVisible(true);
  }, []);

  const closeModal = useCallback(() => {
    setModalVisible(false);
    setEditingLibrary(null);
    setForm(EMPTY_FORM);
  }, []);

  const handleScan = useCallback((lib: V1Library) => {
    if (!lib.id) return;
    scanLibrary.mutate({ id: lib.id });
  }, [scanLibrary]);

  const handleDelete = useCallback((lib: V1Library) => {
    if (!lib.id) return;
    Alert.alert(
      t("admin.libraries.deleteTitle") || "Delete Library",
      t("admin.libraries.deleteConfirm", { name: lib.name }) || `Delete "${lib.name}"? This cannot be undone.`,
      [
        { text: t("common.cancel") || "Cancel", style: "cancel" },
        {
          text: t("common.delete") || "Delete",
          style: "destructive",
          onPress: () => {
            try {
              deleteLib.mutate({ id: lib.id! });
            } catch {
              // handled by onError
            }
          },
        },
      ],
    );
  }, [t, deleteLib]);

  const handleSubmit = useCallback(() => {
    const cleanPaths = form.paths.filter((p) => p.trim() !== "");
    if (!form.name.trim() || cleanPaths.length === 0) return;
    try {
      if (editingLibrary?.id) {
        updateLib.mutate({
          id: editingLibrary.id,
          data: { name: form.name.trim(), paths: cleanPaths },
        });
      } else {
        createLib.mutate({
          data: {
            name: form.name.trim(),
            type: form.type,
            paths: cleanPaths,
            scanOnStartup: form.scanOnStartup,
            realtimeMonitoring: form.realtimeMonitoring,
          },
        });
      }
    } catch {
      // handled by onError
    }
  }, [form, editingLibrary, createLib, updateLib]);

  const updatePath = useCallback((index: number, value: string) => {
    setForm((prev) => ({
      ...prev,
      paths: prev.paths.map((p, i) => (i === index ? value : p)),
    }));
  }, []);

  const addPath = useCallback(() => {
    setForm((prev) => ({ ...prev, paths: [...prev.paths, ""] }));
  }, []);

  const removePath = useCallback((index: number) => {
    setForm((prev) => ({
      ...prev,
      paths: prev.paths.filter((_, i) => i !== index),
    }));
  }, []);

  const isEdit = !!editingLibrary;
  const isPending = isEdit ? updateLib.isPending : createLib.isPending;
  const canSubmit = form.name.trim() !== "" && form.paths.some((p) => p.trim() !== "") && !isPending;

  const renderLibrary = useCallback(({ item }: { item: V1Library }) => {
    const icon = TYPE_ICONS[item.type ?? ""] ?? "folder-outline";
    const lastScan = item.lastScannedAt
      ? new Date(item.lastScannedAt).toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })
      : t("common.never", "Never");

    return (
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
        <Ionicons name={icon as never} size={28} color={theme.colors.accent} />
        <View style={{ flex: 1, gap: 2 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.name}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {item.type?.replace("LIBRARY_TYPE_", "")} · {t("admin.libraries.lastScan", "Last scan")}: {lastScan}
          </Text>
          {item.paths && item.paths.length > 0 && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>
              {item.paths.join(", ")}
            </Text>
          )}
        </View>
        <View style={{ flexDirection: "row", gap: 4 }}>
          <Pressable onPress={() => openEdit(item)} hitSlop={8} style={{ padding: 4 }}>
            <Ionicons name="pencil-outline" size={18} color={theme.colors.accent} />
          </Pressable>
          <Pressable
            onPress={() => handleScan(item)}
            disabled={scanLibrary.isPending}
            hitSlop={8}
            style={{ padding: 4 }}
          >
            <Ionicons name="refresh-outline" size={18} color={theme.colors.accent} />
          </Pressable>
          <Pressable onPress={() => handleDelete(item)} hitSlop={8} style={{ padding: 4 }}>
            <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
          </Pressable>
        </View>
      </View>
    );
  }, [theme, handleScan, handleDelete, openEdit, scanLibrary.isPending, t]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.libraries") || "Libraries",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable onPress={openCreate} style={{ marginRight: 8 }}>
            <Ionicons name="add-circle-outline" size={26} color={theme.colors.accent} />
          </Pressable>
        ),
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={libraries}
          keyExtractor={(l) => l.id ?? ""}
          renderItem={renderLibrary}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>{t("admin.libraries.noLibraries") || "No libraries"}</Text>
            </View>
          }
        />
      )}

      <Modal visible={modalVisible} animationType="slide" presentationStyle="pageSheet" onRequestClose={closeModal}>
        <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
          <View style={[styles.modalHeader, { backgroundColor: theme.colors.surface, borderBottomColor: theme.colors.border }]}>
            <Pressable onPress={closeModal} hitSlop={8}>
              <Text style={{ color: theme.colors.accent, fontSize: 15 }}>{t("common.cancel") || "Cancel"}</Text>
            </Pressable>
            <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }}>
              {isEdit
                ? (t("admin.libraries.editLibrary") || "Edit Library")
                : (t("admin.libraries.addLibrary") || "Add Library")}
            </Text>
            <Pressable onPress={handleSubmit} disabled={!canSubmit} hitSlop={8}>
              <Text style={{ color: canSubmit ? theme.colors.accent : theme.colors.textMuted, fontSize: 15, fontWeight: "600" }}>
                {isPending
                  ? (t("common.loading") || "...")
                  : isEdit
                    ? (t("common.save") || "Save")
                    : (t("common.create") || "Create")}
              </Text>
            </Pressable>
          </View>

          <ScrollView contentContainerStyle={{ padding: 16, gap: 20 }}>
            {/* Name */}
            <View style={{ gap: 6 }}>
              <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.name") || "Name"}</Text>
              <TextInput
                style={[styles.input, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border }]}
                value={form.name}
                onChangeText={(v) => setForm((prev) => ({ ...prev, name: v }))}
                placeholder={t("admin.libraries.namePlaceholder") || "My Movies"}
                placeholderTextColor={theme.colors.textMuted}
              />
            </View>

            {/* Type (create only) */}
            {!isEdit && (
              <View style={{ gap: 6 }}>
                <Text style={[styles.label, { color: theme.colors.text }]}>
                  {t("admin.libraries.tableHeaders.type") || "Type"}
                </Text>
                <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
                  {LIBRARY_TYPES.map((lt) => {
                    const selected = form.type === lt.value;
                    return (
                      <Pressable
                        key={lt.value}
                        onPress={() => setForm((prev) => ({ ...prev, type: lt.value }))}
                        style={[
                          styles.typeChip,
                          {
                            backgroundColor: selected ? theme.colors.accent : theme.colors.surface,
                            borderColor: selected ? theme.colors.accent : theme.colors.border,
                          },
                        ]}
                      >
                        <Ionicons
                          name={(TYPE_ICONS[lt.value] ?? "folder-outline") as never}
                          size={16}
                          color={selected ? "#fff" : theme.colors.textMuted}
                        />
                        <Text style={{ color: selected ? "#fff" : theme.colors.text, fontSize: 13 }}>
                          {t(lt.labelKey) || lt.fallback}
                        </Text>
                      </Pressable>
                    );
                  })}
                </View>
              </View>
            )}

            {/* Paths */}
            <View style={{ gap: 6 }}>
              <Text style={[styles.label, { color: theme.colors.text }]}>
                {t("admin.libraries.paths") || "Paths"}
              </Text>
              {form.paths.map((p, i) => (
                <View key={i} style={{ flexDirection: "row", gap: 8, alignItems: "center" }}>
                  <TextInput
                    style={[styles.input, { flex: 1, backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border }]}
                    value={p}
                    onChangeText={(v) => updatePath(i, v)}
                    placeholder="/media/movies"
                    placeholderTextColor={theme.colors.textMuted}
                    autoCapitalize="none"
                    autoCorrect={false}
                  />
                  {form.paths.length > 1 && (
                    <Pressable onPress={() => removePath(i)} hitSlop={8} style={{ padding: 4 }}>
                      <Ionicons name="close-circle" size={22} color={theme.colors.error} />
                    </Pressable>
                  )}
                </View>
              ))}
              <Pressable onPress={addPath} style={{ flexDirection: "row", alignItems: "center", gap: 4, paddingTop: 4 }}>
                <Ionicons name="add-circle-outline" size={18} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.accent, fontSize: 13 }}>
                  {t("admin.libraries.addPath") || "Add path"}
                </Text>
              </Pressable>
            </View>

            {/* Toggles (create only) */}
            {!isEdit && (
              <View style={{ gap: 12 }}>
                <View style={styles.switchRow}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>
                    {t("admin.libraries.scanOnStartup") || "Scan on startup"}
                  </Text>
                  <Switch
                    value={form.scanOnStartup}
                    onValueChange={(v) => setForm((prev) => ({ ...prev, scanOnStartup: v }))}
                    trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                  />
                </View>
                <View style={styles.switchRow}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>
                    {t("admin.libraries.realtimeMonitoring") || "Realtime monitoring"}
                  </Text>
                  <Switch
                    value={form.realtimeMonitoring}
                    onValueChange={(v) => setForm((prev) => ({ ...prev, realtimeMonitoring: v }))}
                    trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                  />
                </View>
              </View>
            )}
          </ScrollView>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  modalHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: { fontSize: 13, fontWeight: "600" },
  input: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 15,
  },
  typeChip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderRadius: 8,
    borderWidth: 1,
  },
  switchRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
});
