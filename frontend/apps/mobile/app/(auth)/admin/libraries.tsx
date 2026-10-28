import {
  View, Text, FlatList, Pressable, Alert, ActivityIndicator,
  StyleSheet, Modal, TextInput, Switch, ScrollView,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useLibraryServiceCreateLibrary,
  useLibraryServiceUpdateLibrary,
  useLibraryServiceDeleteLibrary,
  deleteLibraryUnbounded,
  getLibraryServiceListLibrariesQueryKey,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import type { V1Library, V1LibraryType } from "@evey/api";
import { toast } from "@evey/store";
import { scanFailedPaths } from "@evey/client";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { DirectoryPickerModal, type ServerDir } from "../../../src/components/admin/DirectoryPickerModal";
import {
  emptyLibraryForm, formFromLibrary, libraryCreateBody, libraryUpdateBody, libraryFormValid,
  type LibraryFormData,
} from "../../../src/features/admin/libraryFormBody";

const TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "film-outline",
  LIBRARY_TYPE_SHOWS: "tv-outline",
  LIBRARY_TYPE_MUSIC: "musical-notes-outline",
  LIBRARY_TYPE_PHOTOS: "image-outline",
};

const LIBRARY_TYPES: { value: V1LibraryType; labelKey: string; fallback: string }[] = [
  { value: "LIBRARY_TYPE_MOVIES", labelKey: "admin.libraries.types.movies", fallback: "Movies" },
  { value: "LIBRARY_TYPE_SHOWS", labelKey: "admin.libraries.types.shows", fallback: "TV Shows" },
  { value: "LIBRARY_TYPE_MUSIC", labelKey: "admin.libraries.types.music", fallback: "Music" },
  { value: "LIBRARY_TYPE_PHOTOS", labelKey: "admin.libraries.types.photos", fallback: "Photos" },
];

interface LibraryStat {
  id: string;
  lastScannedAt?: string | null;
  itemCount?: number;
}

interface ScanSchedule {
  libraryId: string;
  cron: string;
  enabled: boolean;
  lastRunAt?: string | null;
  nextRunAt?: string | null;
}

function useLibraryStats() {
  return useQuery<LibraryStat[]>({
    queryKey: ["/api/v1/admin/library-stats"],
    queryFn: async () => {
      const json = await eveyFetch<{ stats: LibraryStat[] }>({ url: "/api/v1/admin/library-stats", method: "GET" });
      return json.stats ?? [];
    },
    staleTime: 30_000,
  });
}

function useSchedule(libraryId: string) {
  return useQuery<ScanSchedule | null>({
    queryKey: ["/api/v1/libraries", libraryId, "schedule"],
    queryFn: async () => {
      const data = await eveyFetch<{ schedule: ScanSchedule | null }>({
        url: `/api/v1/libraries/${libraryId}/schedule`,
        method: "GET",
      });
      return data.schedule;
    },
    enabled: !!libraryId,
    staleTime: 30_000,
  });
}

function formatScheduleDate(iso?: string | null): string {
  if (!iso) return "-";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function ScheduleModal({ library, onClose }: { library: V1Library; onClose: () => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: schedule, isLoading } = useSchedule(library.id!);
  const [preset, setPreset] = useState("");
  const [customCron, setCustomCron] = useState("");
  const [enabled, setEnabled] = useState(true);

  const PRESET_SCHEDULES = [
    { value: "0 */6 * * *", label: t("admin.libraries.presets.every6h", "Every 6 hours") },
    { value: "0 */12 * * *", label: t("admin.libraries.presets.every12h", "Every 12 hours") },
    { value: "0 3 * * *", label: t("admin.libraries.presets.daily3am", "Daily at 3am") },
    { value: "0 3 * * 0", label: t("admin.libraries.presets.weekly3am", "Weekly (Sun 3am)") },
    { value: "custom", label: t("admin.libraries.presets.customCron", "Custom cron") },
  ];

  useEffect(() => {
    if (schedule) {
      setEnabled(schedule.enabled);
      const match = PRESET_SCHEDULES.find((p) => p.value === schedule.cron);
      if (match) {
        setPreset(schedule.cron);
      } else {
        setPreset("custom");
        setCustomCron(schedule.cron);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- PRESET_SCHEDULES uses t() so its identity changes every render; only sync when the schedule prop changes
  }, [schedule]);

  const cronValue = preset === "custom" ? customCron : preset;

  const saveMutation = useMutation({
    mutationFn: async () => {
      await eveyFetch<{ schedule: ScanSchedule }>({
        url: `/api/v1/libraries/${library.id}/schedule`,
        method: "PUT",
        data: { cron: cronValue, enabled },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["/api/v1/libraries", library.id, "schedule"] });
      toast.success(t("admin.libraries.scheduleSaved", "Scan schedule saved"));
      onClose();
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedSchedule", "Failed to save schedule"))),
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      await eveyFetch<void>({
        url: `/api/v1/libraries/${library.id}/schedule`,
        method: "DELETE",
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["/api/v1/libraries", library.id, "schedule"] });
      toast.success(t("admin.libraries.scheduleRemoved", "Schedule removed"));
      onClose();
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedRemoveSchedule", "Failed to remove schedule"))),
  });

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <View style={[styles.modalHeader, { backgroundColor: theme.colors.surface, borderBottomColor: theme.colors.border }]}>
        <Pressable
          onPress={onClose}
          hitSlop={8}
          accessible={true}
          accessibilityLabel={t("common.cancel", "Cancel")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.accent, fontSize: 15 }}>{t("common.cancel", "Cancel")}</Text>
        </Pressable>
        <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }} numberOfLines={1}>
          {t("admin.libraries.scanScheduleTitle", { name: library.name, defaultValue: "Scan schedule - {{name}}" })}
        </Text>
        <Pressable
          onPress={() => saveMutation.mutate()}
          disabled={!cronValue || saveMutation.isPending}
          hitSlop={8}
          accessible={true}
          accessibilityLabel={t("common.save", "Save")}
          accessibilityRole="button"
        >
          <Text style={{ color: cronValue ? theme.colors.accent : theme.colors.textMuted, fontSize: 15, fontWeight: "600" }}>
            {saveMutation.isPending ? (t("common.loading", "...")) : t("common.save", "Save")}
          </Text>
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={{ padding: 16, gap: 20 }}>
        {isLoading ? (
          <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 24 }} />
        ) : (
          <>
            <View style={{ gap: 8 }}>
              <Text style={[styles.label, { color: theme.colors.text }]}>
                {t("admin.libraries.scanFrequency", "Scan frequency")}
              </Text>
              <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
                {PRESET_SCHEDULES.map((p) => {
                  const selected = preset === p.value;
                  return (
                    <Pressable
                      key={p.value}
                      onPress={() => {
                        setPreset(p.value);
                        if (p.value !== "custom") setCustomCron("");
                      }}
                      style={[
                        styles.typeChip,
                        {
                          backgroundColor: selected ? theme.colors.accent : theme.colors.surface,
                          borderColor: selected ? theme.colors.accent : theme.colors.border,
                        },
                      ]}
                      accessible={true}
                      accessibilityLabel={p.label}
                      accessibilityRole="radio"
                      accessibilityState={{ selected }}
                    >
                      <Text style={{ color: selected ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{p.label}</Text>
                    </Pressable>
                  );
                })}
              </View>
            </View>

            {preset === "custom" && (
              <View style={{ gap: 6 }}>
                <Text style={[styles.label, { color: theme.colors.text }]}>
                  {t("admin.libraries.cronExpression", "Cron expression")}
                </Text>
                <TextInput
                  style={[styles.input, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border }]}
                  value={customCron}
                  onChangeText={setCustomCron}
                  placeholder="0 */6 * * *"
                  placeholderTextColor={theme.colors.textMuted}
                  autoCapitalize="none"
                  autoCorrect={false}
                />
              </View>
            )}

            <View style={styles.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>
                {t("common.enabled", "Enabled")}
              </Text>
              <Switch
                value={enabled}
                onValueChange={setEnabled}
                trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
              />
            </View>

            {schedule && (schedule.lastRunAt || schedule.nextRunAt) && (
              <View style={{ gap: 2 }}>
                {schedule.lastRunAt && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                    {t("admin.libraries.lastRun", { date: formatScheduleDate(schedule.lastRunAt), defaultValue: "Last run: {{date}}" })}
                  </Text>
                )}
                {schedule.nextRunAt && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                    {t("admin.libraries.nextRun", { date: formatScheduleDate(schedule.nextRunAt), defaultValue: "Next run: {{date}}" })}
                  </Text>
                )}
              </View>
            )}

            {schedule && (
              <Pressable
                onPress={() => deleteMutation.mutate()}
                disabled={deleteMutation.isPending}
                style={{ paddingVertical: 8 }}
                accessible={true}
                accessibilityLabel={t("common.remove", "Remove")}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.error, fontSize: 14, textAlign: "center" }}>
                  {deleteMutation.isPending ? t("common.loading", "...") : t("common.remove", "Remove")}
                </Text>
              </Pressable>
            )}
          </>
        )}
      </ScrollView>
    </View>
  );
}

function LibraryRow({
  library,
  stats,
  onEdit,
  onScan,
  onDelete,
  scanPending,
}: {
  library: V1Library;
  stats: LibraryStat[];
  onEdit: (lib: V1Library) => void;
  onScan: (lib: V1Library) => void;
  onDelete: (lib: V1Library) => void;
  scanPending: boolean;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [showSchedule, setShowSchedule] = useState(false);
  const { data: schedule } = useSchedule(library.id ?? "");

  const refreshMetadata = useMutation({
    mutationFn: () =>
      eveyFetch({
        url: "/api/v1/admin/media/refresh-metadata",
        method: "POST",
        data: { libraryId: library.id },
      }),
    onSuccess: () => toast.success(t("admin.libraries.refreshAllMetadataStarted", "Metadata refresh started")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const matchOrphans = useMutation<{ queued?: number }, Error>({
    mutationFn: () =>
      eveyFetch({
        url: "/api/v1/admin/media/match-orphans",
        method: "POST",
        data: { libraryId: library.id },
      }),
    onSuccess: (resp) => {
      const queued = resp?.queued ?? 0;
      if (queued === 0) {
        toast.info(t("admin.libraries.matchOrphansEmpty", "No orphan files found"));
      } else {
        toast.success(t("admin.libraries.matchOrphansStarted", { count: queued, defaultValue: "Matching {{count}} orphan files" }));
      }
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const icon = TYPE_ICONS[library.type ?? ""] ?? "folder-outline";
  // Provider matching (match orphans / refresh metadata) only applies to
  // movie/show libraries - music and photos have no metadata provider, so the
  // actions would be no-ops (mirror web supportsProviderMatching).
  const supportsMatch = library.type === "LIBRARY_TYPE_MOVIES" || library.type === "LIBRARY_TYPE_SHOWS";
  const libStat = stats.find((s) => s.id === library.id);
  const lastScan = libStat?.lastScannedAt
    ? new Date(libStat.lastScannedAt).toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })
    : t("common.never", "Never");
  const totalItems = libStat?.itemCount;

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Ionicons name={icon as never} size={28} color={theme.colors.accent} />
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{library.name}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {library.type?.replace("LIBRARY_TYPE_", "")}
          {" · "}
          {totalItems != null
            ? t("admin.libraries.items", { count: totalItems, defaultValue: "{{count}} items" })
            : "-"}
          {" · "}
          {t("admin.libraries.lastScan", "Last scan")}: {lastScan}
        </Text>
        {library.paths && library.paths.length > 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>
            {library.paths.join(", ")}
          </Text>
        )}
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {schedule?.enabled
            ? t("admin.libraries.tableHeaders.nextScan", "Next scan") + ": " + (schedule.nextRunAt ? formatScheduleDate(schedule.nextRunAt) : t("common.enabled", "Enabled"))
            : t("admin.libraries.off", "Scan schedule off")}
        </Text>
      </View>
      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 4, justifyContent: "flex-end", maxWidth: 96 }}>
        <Pressable
          onPress={() => onEdit(library)}
          hitSlop={8}
          style={{ padding: 4 }}
          accessible={true}
          accessibilityLabel={t("admin.libraries.editLibraryNamed", "Edit {{name}}", { name: library.name })}
          accessibilityRole="button"
        >
          <Ionicons name="pencil-outline" size={18} color={theme.colors.accent} />
        </Pressable>
        <Pressable
          onPress={() => onScan(library)}
          disabled={scanPending}
          hitSlop={8}
          style={{ padding: 4 }}
          accessible={true}
          accessibilityLabel={t("admin.libraries.scanLibraryNamed", "Scan {{name}}", { name: library.name })}
          accessibilityRole="button"
        >
          <Ionicons name="refresh-outline" size={18} color={theme.colors.accent} />
        </Pressable>
        <Pressable
          onPress={() => setShowSchedule(true)}
          hitSlop={8}
          style={{ padding: 4 }}
          accessible={true}
          accessibilityLabel={t("admin.libraries.scheduleLibraryNamed", "Scan schedule for {{name}}", { name: library.name })}
          accessibilityRole="button"
        >
          <Ionicons name="calendar-outline" size={18} color={theme.colors.accent} />
        </Pressable>
        {supportsMatch && (
          <>
            <Pressable
              onPress={() => matchOrphans.mutate()}
              disabled={matchOrphans.isPending}
              hitSlop={8}
              style={{ padding: 4 }}
              accessible={true}
              accessibilityLabel={t("admin.libraries.matchOrphansNamed", "Match orphans for {{name}}", { name: library.name })}
              accessibilityRole="button"
            >
              <Ionicons name="link-outline" size={18} color={theme.colors.accent} />
            </Pressable>
            <Pressable
              onPress={() => refreshMetadata.mutate()}
              disabled={refreshMetadata.isPending}
              hitSlop={8}
              style={{ padding: 4 }}
              accessible={true}
              accessibilityLabel={t("admin.libraries.refreshMetadataNamed", "Refresh metadata for {{name}}", { name: library.name })}
              accessibilityRole="button"
            >
              <Ionicons name="cloud-download-outline" size={18} color={theme.colors.accent} />
            </Pressable>
          </>
        )}
        <Pressable
          onPress={() => onDelete(library)}
          hitSlop={8}
          style={{ padding: 4 }}
          accessible={true}
          accessibilityLabel={t("admin.libraries.deleteLibraryNamed", "Delete {{name}}", { name: library.name })}
          accessibilityRole="button"
        >
          <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
        </Pressable>
      </View>

      <Modal visible={showSchedule} animationType="slide" presentationStyle="pageSheet" onRequestClose={() => setShowSchedule(false)}>
        <ScheduleModal library={library} onClose={() => setShowSchedule(false)} />
      </Modal>
    </View>
  );
}

export default function AdminLibrariesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, isError, refetch } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];
  const { data: stats = [] } = useLibraryStats();

  const [modalVisible, setModalVisible] = useState(false);
  const [editingLibrary, setEditingLibrary] = useState<V1Library | null>(null);
  const [form, setForm] = useState<LibraryFormData>(emptyLibraryForm);
  // Index of the path row whose directory picker is open, null = closed.
  const [pickerFor, setPickerFor] = useState<number | null>(null);

  // Server-side directory suggestions, same source as the web's LibraryForm
  // select. Best-effort: an error just leaves the free-text inputs alone.
  const { data: serverDirs = [] } = useQuery<ServerDir[]>({
    queryKey: ["/api/v1/admin/fs/browse"],
    queryFn: () => eveyFetch<ServerDir[]>({ url: "/api/v1/admin/fs/browse", method: "GET" }),
    enabled: modalVisible,
    staleTime: 5 * 60_000,
    retry: false,
  });

  const scanLibrary = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: (data) => {
        const failedPaths = scanFailedPaths(data);
        if (failedPaths.length > 0) {
          toast.warning(t("admin.libraries.scanPartial", { paths: failedPaths.join(", ") }));
          return;
        }
        toast.success(t("admin.libraries.scanStarted", "Scan started"));
      },
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
      // The cascade runs inside the request: no fetcher timeout on it.
      mutationFn: ({ id }) => deleteLibraryUnbounded(id),
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getLibraryServiceListLibrariesQueryKey() });
        qc.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
        toast.success(t("admin.libraries.libraryDeleted") || "Library deleted");
      },
      onError: () => toast.error(t("common.error") || "Failed to delete library"),
    },
  });

  const openCreate = useCallback(() => {
    setEditingLibrary(null);
    setForm(emptyLibraryForm());
    setModalVisible(true);
  }, []);

  // Server-wide collection-membership rebuild (mirror web libraries "Relink
  // collections" header action).
  const relinkCollections = useMutation({
    mutationFn: () => eveyFetch({ url: "/api/v1/admin/collections/relink", method: "POST", data: {} }),
    onSuccess: () => toast.success(t("admin.libraries.relinkCollectionsStarted", "Relinking collections…")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const openEdit = useCallback((lib: V1Library) => {
    setEditingLibrary(lib);
    setForm(formFromLibrary(lib));
    setModalVisible(true);
  }, []);

  const closeModal = useCallback(() => {
    setModalVisible(false);
    setEditingLibrary(null);
    setForm(emptyLibraryForm());
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
    if (!libraryFormValid(form)) return;
    try {
      if (editingLibrary?.id) {
        updateLib.mutate({ id: editingLibrary.id, data: libraryUpdateBody(form) });
      } else {
        createLib.mutate({ data: libraryCreateBody(form) });
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

  const renderLibrary = useCallback(({ item }: { item: V1Library }) => (
    <LibraryRow
      library={item}
      stats={stats}
      onEdit={openEdit}
      onScan={handleScan}
      onDelete={handleDelete}
      scanPending={scanLibrary.isPending}
    />
  ), [stats, openEdit, handleScan, handleDelete, scanLibrary.isPending]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.nav.libraries"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <View style={{ flexDirection: "row", alignItems: "center", gap: 14, marginRight: 8 }}>
            <Pressable
              onPress={() => relinkCollections.mutate()}
              disabled={relinkCollections.isPending}
              hitSlop={10}
              accessible={true}
              accessibilityLabel={t("admin.libraries.relinkCollections", "Relink collections")}
              accessibilityRole="button"
            >
              {relinkCollections.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.accent} />
              ) : (
                <Ionicons name="git-network-outline" size={24} color={theme.colors.accent} />
              )}
            </Pressable>
            <Pressable
              onPress={openCreate}
              hitSlop={10}
              accessible={true}
              accessibilityLabel={t("admin.libraries.addLibrary", "Add Library")}
              accessibilityRole="button"
            >
              <Ionicons name="add-circle-outline" size={26} color={theme.colors.accent} />
            </Pressable>
          </View>
        ),
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
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
            <Pressable
              onPress={closeModal}
              hitSlop={8}
              accessible={true}
              accessibilityLabel={t("common.cancel") || "Cancel"}
              accessibilityRole="button"
            >
              <Text style={{ color: theme.colors.accent, fontSize: 15 }}>{t("common.cancel") || "Cancel"}</Text>
            </Pressable>
            <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }}>
              {isEdit
                ? (t("admin.libraries.editLibrary") || "Edit Library")
                : (t("admin.libraries.addLibrary") || "Add Library")}
            </Text>
            <Pressable
              onPress={handleSubmit}
              disabled={!canSubmit}
              hitSlop={8}
              accessible={true}
              accessibilityLabel={isEdit ? (t("common.save") || "Save") : (t("common.create") || "Create")}
              accessibilityRole="button"
            >
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
                        accessible={true}
                        accessibilityLabel={t(lt.labelKey) || lt.fallback}
                        accessibilityRole="radio"
                        accessibilityState={{ selected }}
                      >
                        <Ionicons
                          name={(TYPE_ICONS[lt.value] ?? "folder-outline") as never}
                          size={16}
                          color={selected ? theme.colors.onAccent : theme.colors.textMuted}
                        />
                        <Text style={{ color: selected ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>
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
                  {serverDirs.length > 0 && (
                    <Pressable
                      onPress={() => setPickerFor(i)}
                      hitSlop={8}
                      style={{ padding: 4 }}
                      testID={`browse-path-${i}`}
                      accessible={true}
                      accessibilityLabel={t("admin.libraries.selectDirectory", "Select a directory")}
                      accessibilityRole="button"
                    >
                      <Ionicons name="folder-open-outline" size={22} color={theme.colors.accent} />
                    </Pressable>
                  )}
                  {form.paths.length > 1 && (
                    <Pressable
                      onPress={() => removePath(i)}
                      hitSlop={8}
                      style={{ padding: 4 }}
                      accessible={true}
                      accessibilityLabel={t("admin.libraries.removePathNamed", "Remove path {{index}}", { index: i + 1 })}
                      accessibilityRole="button"
                    >
                      <Ionicons name="close-circle" size={22} color={theme.colors.error} />
                    </Pressable>
                  )}
                </View>
              ))}
              <Pressable
                onPress={addPath}
                style={{ flexDirection: "row", alignItems: "center", gap: 4, paddingTop: 4 }}
                accessible={true}
                accessibilityLabel={t("admin.libraries.addPath") || "Add path"}
                accessibilityRole="button"
              >
                <Ionicons name="add-circle-outline" size={18} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.accent, fontSize: 13 }}>
                  {t("admin.libraries.addPath") || "Add path"}
                </Text>
              </Pressable>
            </View>

            <View style={{ gap: 4 }}>
              <View style={styles.switchRow}>
                <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>
                  {t("admin.libraries.realtimeMonitoring")}
                </Text>
                <Switch
                  value={form.realtimeMonitoring}
                  onValueChange={(v) => setForm((prev) => ({ ...prev, realtimeMonitoring: v }))}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                {t("admin.libraries.realtimeMonitoringHint")}
              </Text>
            </View>
          </ScrollView>
        </View>
        <DirectoryPickerModal
          visible={pickerFor !== null}
          dirs={serverDirs}
          onSelect={(path) => {
            if (pickerFor !== null) updatePath(pickerFor, path);
          }}
          onClose={() => setPickerFor(null)}
        />
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
