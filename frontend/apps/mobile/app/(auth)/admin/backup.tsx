import { View, Text, FlatList, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface Backup {
  id: string;
  createdAt: string;
  version: string;
  size: number;
}

const BACKUPS_KEY = ["admin", "backups"];

function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function useBackups() {
  return useQuery({
    queryKey: BACKUPS_KEY,
    queryFn: () => eveyFetch<{ backups: Backup[] }>({ url: "/api/v1/admin/backups", method: "GET" }),
    select: (d) => d.backups ?? [],
  });
}

export default function AdminBackupScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: backups, isLoading, refetch } = useBackups();

  const createBackup = useMutation({
    mutationFn: () => eveyFetch<Backup>({ url: "/api/v1/admin/backups", method: "POST" }),
    onSuccess: async () => {
      toast.success(t("admin.backup.backupCreated"));
      await qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
    onError: (err: Error) => toast.error(err.message || t("admin.backup.failedCreate")),
  });

  const deleteBackup = useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/admin/backups/${id}`, method: "DELETE" }),
    onSuccess: async () => {
      toast.success(t("admin.backup.backupDeleted"));
      await qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
    onError: (err: Error) => toast.error(err.message || t("admin.backup.failedDeleteBackup")),
  });

  const handleDelete = useCallback((backup: Backup) => {
    Alert.alert(
      t("admin.backup.deleteBackupTitle"),
      t("admin.backup.deleteConfirm", { date: new Date(backup.createdAt).toLocaleString() }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("common.delete"), style: "destructive", onPress: () => deleteBackup.mutate(backup.id) },
      ],
    );
  }, [deleteBackup, t]);

  const renderBackup = useCallback(({ item }: { item: Backup }) => (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", fontFamily: "monospace" }} numberOfLines={1}>
          {item.id}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {new Date(item.createdAt).toLocaleString()} · {formatSize(item.size)}
        </Text>
        {item.version && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>v{item.version}</Text>
        )}
      </View>
      <Pressable onPress={() => handleDelete(item)} hitSlop={8}>
        <Ionicons name="trash-outline" size={20} color={theme.colors.error} />
      </Pressable>
    </View>
  ), [theme, handleDelete]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.backup.title") || "Database Backup",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <Pressable
        onPress={() => createBackup.mutate()}
        disabled={createBackup.isPending}
        style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createBackup.isPending ? 0.6 : 1 }]}
      >
        {createBackup.isPending ? (
          <ActivityIndicator size="small" color="#fff" />
        ) : (
          <Ionicons name="add-circle-outline" size={20} color="#fff" />
        )}
        <Text style={{ color: "#fff", fontSize: 15, fontWeight: "600" }}>
          {t("admin.backup.createBackup")}
        </Text>
      </Pressable>

      <FlatList
        data={backups ?? []}
        keyExtractor={(b) => b.id}
        renderItem={renderBackup}
        contentContainerStyle={{ paddingHorizontal: 16, gap: 8, paddingBottom: 32 }}
        refreshing={isLoading}
        onRefresh={refetch}
        ListEmptyComponent={
          isLoading ? null : (
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="cloud-download-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>
                {t("admin.backup.noBackups")}
              </Text>
            </View>
          )
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  createBtn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, marginHorizontal: 16, marginVertical: 12, paddingVertical: 12 },
});
