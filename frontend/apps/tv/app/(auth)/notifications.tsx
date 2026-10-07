import { useCallback } from "react";
import { View, Text, FlatList, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { notificationLabel, notificationMessage, type NotificationLabelInput } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import {
  useNotificationServiceListNotifications,
  useNotificationServiceMarkRead,
  useNotificationServiceMarkAllRead,
  getNotificationServiceListNotificationsQueryKey,
  getNotificationServiceGetUnreadCountQueryKey,
} from "@evey/api";
import type { V1Notification } from "@evey/api";
import { toast, useDemoStore } from "@evey/store";
import { useTheme } from "../../src/providers/ThemeProvider";
import { ScreenState } from "../../src/components/ScreenState";
import { FocusableCard } from "../../src/components/FocusableCard";
import { DEEP_SCREEN_GUTTER } from "../../src/lib/tvLayout";

// Wording lives in @evey/api: the three apps had three copies of this, each
// with its own TITLE_KEYS map, and none of them could render the counts in the
// viewer's language.
const translateTitle = (
  t: (k: string, opts?: Record<string, unknown>) => string,
  n: NotificationLabelInput,
): string => notificationLabel(t, n);

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

function tvNotifKeyExtractor(item: V1Notification) {
  return item.id ?? "";
}

function TVNotifSeparator() {
  return <View style={{ height: 4 }} />;
}

export default function NotificationsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  // No limit on purpose: the server's own default is 200 and the web asks for
  // nothing, so the number lives in one place. Asking for 50 here made these
  // two screens hide notifications 51..200 that the server was already willing
  // to send, with no "load more" to reach them.
  const { data, isLoading, isError, refetch } = useNotificationServiceListNotifications();
  const notifications = data?.notifications ?? [];

  const invalidateAll = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const markRead = useNotificationServiceMarkRead({
    mutation: {
      onSuccess: invalidateAll,
      onError: () => toast.error(t("common.notifications.markReadFailed", "Could not mark as read")),
    },
  });

  const markAllRead = useNotificationServiceMarkAllRead({
    mutation: {
      onSuccess: invalidateAll,
      onError: () => toast.error(t("common.notifications.markAllReadFailed", "Could not mark all as read")),
    },
  });

  // Demo: the unread list is the shared account's and the server refuses the
  // read marks - opening one must not raise an error every time.
  const demo = useDemoStore((st) => st.demoMode);
  const hasUnread = !demo && notifications.some((n) => !n.readAt);

  const handleTap = useCallback((n: V1Notification) => {
    if (!n.readAt && n.id && !demo) {
      markRead.mutate({ id: n.id });
    }
    if (n.mediaItemId) {
      router.push(`/media/${n.mediaItemId}`);
    }
  }, [markRead, demo]);

  const renderNotifItem = useCallback(
    ({ item, index }: { item: V1Notification; index: number }) => (
      <NotificationItem
        item={item}
        t={t}
        theme={theme}
        isFirst={index === 0}
        onPress={() => handleTap(item)}
      />
    ),
    [handleTap, t, theme],
  );

  // "Empty" and "the server did not answer" are different sentences, and this
  // screen used to say the first for both.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <View style={styles.headerRow}>
        <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.notifications.title")}</Text>
        {hasUnread && (
          <FocusableCard
            onPress={() => markAllRead.mutate({ data: {} })}
            style={[styles.markAllBtn, { backgroundColor: theme.colors.surface }]}
            accessibilityLabel={t("common.notifications.markAllRead")}
          >
            <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>{t("common.notifications.markAllRead")}</Text>
          </FocusableCard>
        )}
      </View>

      {notifications.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("common.noNotifications")}</Text>
        </View>
      ) : (
        <FlatList
          data={notifications}
          keyExtractor={tvNotifKeyExtractor}
          contentContainerStyle={{ paddingHorizontal: DEEP_SCREEN_GUTTER, paddingBottom: 64 }}
          ItemSeparatorComponent={TVNotifSeparator}
          renderItem={renderNotifItem}
          initialNumToRender={8}
          maxToRenderPerBatch={10}
          windowSize={5}
          removeClippedSubviews={true}
        />
      )}
    </View>
  );
}

function NotificationItem({
  item,
  t,
  theme,
  isFirst,
  onPress,
}: {
  item: V1Notification;
  t: (k: string, opts?: Record<string, unknown>) => string;
  theme: ReturnType<typeof useTheme>;
  isFirst?: boolean;
  onPress: () => void;
}) {
  const isUnread = !item.readAt;

  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[
        styles.row,
        {
          backgroundColor: theme.colors.surface,
          borderRadius: theme.radii.md,
          opacity: isUnread ? 1 : 0.6,
        },
      ]}
      accessibilityLabel={translateTitle(t, item)}
    >
      <View
        style={[
          styles.iconCircle,
          { backgroundColor: isUnread ? theme.colors.accent + "20" : theme.colors.surfaceRaised },
        ]}
      >
        <Text style={{ color: isUnread ? theme.colors.accent : theme.colors.textMuted, fontSize: 18 }}>
          {"\u25CF"}
        </Text>
      </View>
      <View style={{ flex: 1, gap: 4 }}>
        <Text
          style={{
            color: theme.colors.text,
            fontSize: 18,
            fontWeight: isUnread ? "600" : "400",
          }}
          numberOfLines={1}
        >
          {translateTitle(t, item)}
        </Text>
        {item.message && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 15 }} numberOfLines={1}>
            {notificationMessage(t, item)}
          </Text>
        )}
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
          {relativeTime(t, item.createdAt)}
        </Text>
      </View>
      {isUnread && (
        <View style={[styles.unreadDot, { backgroundColor: theme.colors.accent }]} />
      )}
    </FocusableCard>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  headerRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: DEEP_SCREEN_GUTTER, paddingTop: 32, paddingBottom: 16 },
  header: { fontSize: 30, fontWeight: "700" },
  markAllBtn: { paddingHorizontal: 20, paddingVertical: 9, borderRadius: 8 },
  row: { flexDirection: "row", alignItems: "center", padding: 20, gap: 12 },
  iconCircle: { width: 48, height: 48, borderRadius: 24, justifyContent: "center", alignItems: "center" },
  unreadDot: { width: 12, height: 12, borderRadius: 6 },
});
