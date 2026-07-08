import { useCallback, useEffect, useState } from "react";
import { View, Text, FlatList, Pressable, Alert, StyleSheet } from "react-native";
import { router, useFocusEffect } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../src/providers/ThemeProvider";
import {
  getDownloads,
  deleteDownload,
  getTotalDownloadSize,
  subscribeToProgress,
  getInFlightProgress,
  type DownloadedMedia,
} from "../../src/lib/downloads";

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

interface DownloadRowProps {
  item: DownloadedMedia;
  onDelete: (mediaItemId: string) => void;
  deleting: boolean;
}

function DownloadRow({ item, onDelete, deleting }: DownloadRowProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  // Live progress for a download that's still running (e.g. resumed from
  // rehydrateBackgroundDownloads after the app was killed). Once the item is
  // in the persisted index it's already complete, so this only ever shows a
  // non-null fraction in the brief window right after a retry.
  const [progress, setProgress] = useState<number | null>(getInFlightProgress(item.mediaItemId));

  useEffect(() => {
    return subscribeToProgress(item.mediaItemId, setProgress);
  }, [item.mediaItemId]);

  return (
    <Pressable
      onPress={() => router.push(`/media/${item.mediaItemId}`)}
      style={{
        flexDirection: "row",
        alignItems: "center",
        padding: theme.spacing.md,
        gap: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
        backgroundColor: theme.colors.surface,
      }}
      accessible
      accessibilityLabel={item.title}
      accessibilityRole="button"
    >
      <View style={{
        width: 40,
        height: 40,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surfaceRaised,
        alignItems: "center",
        justifyContent: "center",
      }}>
        <Ionicons name="download" size={18} color={theme.colors.accent} />
      </View>

      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
          {item.title}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {formatSize(item.fileSize)} · {formatDate(item.downloadedAt)}
        </Text>
        {progress != null && Number.isFinite(progress) && (
          <Text style={{ color: theme.colors.accent, fontSize: 11 }}>
            {t("downloads.inProgress", "{{percent}}%", { percent: Math.round(progress * 100) })}
          </Text>
        )}
      </View>

      <Pressable
        onPress={() => onDelete(item.mediaItemId)}
        disabled={deleting}
        hitSlop={8}
        accessible
        accessibilityLabel={t("downloads.delete", "Delete download")}
        accessibilityRole="button"
        style={{ opacity: deleting ? 0.5 : 1 }}
      >
        <Ionicons name="trash-outline" size={20} color={theme.colors.textMuted} />
      </Pressable>
    </Pressable>
  );
}

function downloadKeyExtractor(item: DownloadedMedia) {
  return item.mediaItemId;
}

export default function DownloadsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [items, setItems] = useState<DownloadedMedia[]>([]);
  const [totalSize, setTotalSize] = useState(0);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const [downloads, total] = await Promise.all([getDownloads(), getTotalDownloadSize()]);
    setItems(downloads);
    setTotalSize(total);
    setLoading(false);
  }, []);

  // Re-list on every focus - a download started elsewhere (media detail
  // screen) or completed in the background should show up without a
  // manual pull-to-refresh.
  useFocusEffect(
    useCallback(() => {
      void refresh();
    }, [refresh]),
  );

  const handleDelete = useCallback((mediaItemId: string) => {
    const item = items.find((i) => i.mediaItemId === mediaItemId);
    Alert.alert(
      t("downloads.deleteTitle", "Delete download"),
      t("downloads.deleteConfirm", "Remove \"{{title}}\" from this device?", { title: item?.title ?? "" }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            setDeletingId(mediaItemId);
            try {
              await deleteDownload(mediaItemId);
              await refresh();
            } finally {
              setDeletingId(null);
            }
          },
        },
      ],
    );
  }, [items, refresh, t]);

  const renderItem = useCallback(
    ({ item }: { item: DownloadedMedia }) => (
      <DownloadRow item={item} onDelete={handleDelete} deleting={deletingId === item.mediaItemId} />
    ),
    [handleDelete, deletingId],
  );

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <View style={{
        flexDirection: "row",
        alignItems: "center",
        justifyContent: "space-between",
        paddingHorizontal: theme.spacing.md,
        paddingTop: theme.spacing.lg,
        paddingBottom: theme.spacing.sm,
      }}>
        <Pressable onPress={() => router.back()} accessible accessibilityLabel={t("common.back", "Back")} accessibilityRole="button">
          <Ionicons name="chevron-back" size={24} color={theme.colors.text} />
        </Pressable>
        <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "600" }}>
          {t("downloads.title", "Downloads")}
        </Text>
        <View style={{ width: 24 }} />
      </View>

      {items.length > 0 && (
        <Text style={{
          color: theme.colors.textMuted,
          fontSize: 12,
          paddingHorizontal: theme.spacing.md,
          paddingBottom: theme.spacing.sm,
        }}>
          {t("downloads.totalSize", "{{size}} used on this device", { size: formatSize(totalSize) })}
        </Text>
      )}

      {!loading && items.length === 0 ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl }}>
          <Ionicons name="download-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md, textAlign: "center" }}>
            {t("downloads.empty", "No downloads yet")}
          </Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={downloadKeyExtractor}
          renderItem={renderItem}
          contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
        />
      )}
    </View>
  );
}
