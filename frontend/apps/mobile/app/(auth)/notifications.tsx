import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, Alert, RefreshControl, StyleSheet } from "react-native";
import { router, Stack } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useNotificationServiceListNotifications,
  useNotificationServiceMarkRead,
  useNotificationServiceMarkAllRead,
  useNotificationServiceDeleteAllNotifications,
  getNotificationServiceListNotificationsQueryKey,
  getNotificationServiceGetUnreadCountQueryKey,
} from "@evey/api";
import type { V1Notification } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../src/providers/ThemeProvider";

const TITLE_KEYS: Record<string, string> = {
  new_movie: "common.notifications.newMovie",
  new_episode: "common.notifications.newEpisode",
  new_season: "common.notifications.newSeason",
  new_series: "common.notifications.newSeries",
  new_album: "common.notifications.newAlbum",
  backup_failed: "common.notifications.backupFailed",
};

function translateTitle(
  t: (k: string, opts?: Record<string, unknown>) => string,
  raw: string | undefined,
): string {
  if (!raw) return "";
  const colonIdx = raw.indexOf(":");
  if (colonIdx === -1) return raw;
  const key = raw.slice(0, colonIdx);
  const i18nKey = TITLE_KEYS[key];
  if (!i18nKey) return raw;
  const name = raw.slice(colonIdx + 1);
  return t(i18nKey, { name });
}

function relativeTime(
  t: (k: string, opts?: Record<string, unknown>) => string,
  iso: string | undefined,
): string {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return t("common.notifications.justNow");
  if (mins < 60) return t("common.notifications.minutesAgo", { count: mins });
  const hours = Math.floor(mins / 60);
  if (hours < 24) return t("common.notifications.hoursAgo", { count: hours });
  const days = Math.floor(hours / 24);
  if (days < 7) return t("common.notifications.daysAgo", { count: days });
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function iconForType(type: string | undefined): keyof typeof Ionicons.glyphMap {
  switch (type) {
    case "new_movie": return "film-outline";
    case "new_episode":
    case "new_season":
    case "new_series": return "tv-outline";
    case "new_album": return "musical-notes-outline";
    case "backup_failed": return "warning-outline";
    default: return "notifications-outline";
  }
}

function NotificationItem({ item, onPress }: { item: V1Notification; onPress: () => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const isUnread = !item.readAt;

  return (
    <Pressable
      onPress={onPress}
      style={{
        flexDirection: "row",
        alignItems: "center",
        padding: theme.spacing.md,
        gap: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
        backgroundColor: theme.colors.surface,
        opacity: isUnread ? 1 : 0.6,
      }}
      accessible
      accessibilityLabel={translateTitle(t, item.title)}
      accessibilityRole="button"
    >
      <View style={{
        width: 36,
        height: 36,
        borderRadius: 18,
        backgroundColor: isUnread ? theme.colors.accent + "20" : theme.colors.surfaceRaised,
        alignItems: "center",
        justifyContent: "center",
      }}>
        <Ionicons
          name={iconForType(item.type)}
          size={18}
          color={isUnread ? theme.colors.accent : theme.colors.textMuted}
        />
      </View>

      <View style={{ flex: 1, gap: 2 }}>
        <Text
          style={{
            color: theme.colors.text,
            fontSize: 14,
            fontWeight: isUnread ? "600" : "400",
          }}
          numberOfLines={1}
        >
          {translateTitle(t, item.title)}
        </Text>
        {item.message && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {item.message}
          </Text>
        )}
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {relativeTime(t, item.createdAt)}
        </Text>
      </View>

      {isUnread && (
        <View style={{
          width: 8,
          height: 8,
          borderRadius: 4,
          backgroundColor: theme.colors.accent,
        }} />
      )}
    </Pressable>
  );
}

function notifKeyExtractor(item: V1Notification) {
  return item.id ?? "";
}

export default function NotificationsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  // Notification types hidden from the user-facing list - average user
  // doesn't care about plugin failures (admins see them in the dedicated
  // plugin security dashboard). Mirror web fix `ebe634946`.
  const HIDDEN_TYPES = new Set(["plugin_error"]);

  const { data, isLoading, refetch } = useNotificationServiceListNotifications({ limit: 50 });
  const notifications = (data?.notifications ?? []).filter(
    (n) => !n.type || !HIDDEN_TYPES.has(n.type),
  );

  const invalidateAll = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const markRead = useNotificationServiceMarkRead({
    mutation: { onSuccess: invalidateAll },
  });

  const markAllRead = useNotificationServiceMarkAllRead({
    mutation: { onSuccess: invalidateAll },
  });

  const deleteAll = useNotificationServiceDeleteAllNotifications({
    mutation: {
      onSuccess: invalidateAll,
      onError: () => toast.error(t("common.notifications.clearFailed", "Failed to clear notifications")),
    },
  });

  const handleTap = useCallback((n: V1Notification) => {
    if (!n.readAt && n.id) {
      markRead.mutate({ id: n.id });
    }
    if (n.mediaItemId) {
      router.push(`/media/${n.mediaItemId}`);
    }
  }, [markRead, router]);

  const renderNotifItem = useCallback(
    ({ item }: { item: V1Notification }) => (
      <NotificationItem item={item} onPress={() => handleTap(item)} />
    ),
    [handleTap],
  );

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  }, [refetch]);

  function handleClearAll() {
    Alert.alert(
      t("common.notifications.clearTitle", "Clear Notifications"),
      t("common.notifications.clearConfirm", "Delete all notifications?"),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.delete"),
          style: "destructive",
          onPress: () => deleteAll.mutate({}),
        },
      ],
    );
  }

  const hasUnread = notifications.some((n) => !n.readAt);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("common.notifications.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <View style={{ flexDirection: "row", gap: theme.spacing.sm }}>
            {hasUnread && (
              <Pressable
                onPress={() => markAllRead.mutate({ data: {} })}
                disabled={markAllRead.isPending}
                accessible
                accessibilityLabel={t("common.markAllRead")}
                accessibilityRole="button"
              >
                <Ionicons name="checkmark-done-outline" size={22} color={theme.colors.accent} />
              </Pressable>
            )}
            {notifications.length > 0 && (
              <Pressable onPress={handleClearAll} accessible accessibilityLabel="Clear all" accessibilityRole="button">
                <Ionicons name="trash-outline" size={22} color={theme.colors.textMuted} />
              </Pressable>
            )}
          </View>
        ),
      }} />

      {!isLoading && notifications.length === 0 ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl }}>
          <Ionicons name="notifications-off-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md, textAlign: "center" }}>
            {t("common.noNotifications")}
          </Text>
        </View>
      ) : (
        <FlatList
          data={notifications}
          keyExtractor={notifKeyExtractor}
          renderItem={renderNotifItem}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />
          }
          contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
          initialNumToRender={10}
          maxToRenderPerBatch={10}
          windowSize={5}
          removeClippedSubviews={true}
        />
      )}
    </View>
  );
}
