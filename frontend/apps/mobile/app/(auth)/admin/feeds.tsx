import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, Switch, Pressable, ActivityIndicator, StyleSheet, RefreshControl, TextInput } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useFeedServiceGetAdminFeedConfigs,
  useFeedServiceBatchUpdateAdminFeedConfigs,
  getFeedServiceGetAdminFeedConfigsQueryKey,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface FeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
  maxItems: number;
}

const FEED_LABEL_KEYS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  next_up: "common.feeds.labels.next_up",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
  collections: "common.feeds.labels.collections",
};

function feedLabel(t: (k: string) => string, feedType: string, libraryNames: Map<string, string>): string {
  if (feedType.startsWith("library:")) {
    const libId = feedType.slice("library:".length);
    return libraryNames.get(libId) ?? feedType;
  }
  return FEED_LABEL_KEYS[feedType] ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

export default function AdminFeedsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useFeedServiceGetAdminFeedConfigs();
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryNames = new Map<string, string>(
    (librariesData?.libraries ?? []).map((l: { id?: string; name?: string }) => [l.id!, l.name ?? l.id!]),
  );

  const [rows, setRows] = useState<FeedRow[]>([]);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data?.configs) {
      setRows(
        data.configs
          .map((c: { feedType?: string; enabled?: boolean; displayOrder?: number; maxItems?: number }) => ({
            feedType: c.feedType ?? "",
            enabled: c.enabled ?? true,
            displayOrder: c.displayOrder ?? 0,
            maxItems: c.maxItems ?? 20,
          }))
          .sort((a: FeedRow, b: FeedRow) => a.displayOrder - b.displayOrder),
      );
      setDirty(false);
    }
  }, [data]);

  const batchUpdate = useFeedServiceBatchUpdateAdminFeedConfigs({
    mutation: {
      onSuccess: () => {
        toast.success(t("common.feeds.saved"));
        qc.invalidateQueries({ queryKey: getFeedServiceGetAdminFeedConfigsQueryKey() });
        setDirty(false);
      },
      onError: () => toast.error(t("common.error")),
    },
  });

  const toggle = (i: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, enabled: !r.enabled } : r)));
    setDirty(true);
  };

  const moveUp = (i: number) => {
    if (i === 0) return;
    setRows((prev) => {
      const next = [...prev];
      [next[i - 1], next[i]] = [next[i], next[i - 1]];
      return next.map((r, idx) => ({ ...r, displayOrder: idx }));
    });
    setDirty(true);
  };

  const moveDown = (i: number) => {
    if (i >= rows.length - 1) return;
    setRows((prev) => {
      const next = [...prev];
      [next[i], next[i + 1]] = [next[i + 1], next[i]];
      return next.map((r, idx) => ({ ...r, displayOrder: idx }));
    });
    setDirty(true);
  };

  const setMaxItems = (i: number, v: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, maxItems: v } : r)));
    setDirty(true);
  };

  const handleSave = useCallback(async () => {
    try {
      await batchUpdate.mutateAsync({ data: { configs: rows } });
    } catch {
      // handled by onError
    }
  }, [rows, batchUpdate]);

  const onRefresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={false} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("common.feeds.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: dirty ? () => (
          <Pressable onPress={handleSave} disabled={batchUpdate.isPending} style={{ marginRight: 8 }}>
            <Text style={{ color: theme.colors.accent, fontWeight: "600", fontSize: 15, opacity: batchUpdate.isPending ? 0.5 : 1 }}>
              {batchUpdate.isPending ? t("common.loading") : t("common.save")}
            </Text>
          </Pressable>
        ) : undefined,
      }} />

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : rows.length === 0 ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <Ionicons name="grid-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.feeds.noSections", "No feed sections configured")}</Text>
        </View>
      ) : (
        <View style={{ padding: 16, gap: 8 }}>
          {rows.map((row, i) => (
            <View key={row.feedType} style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <View style={{ gap: 4, flex: 1 }}>
                <Text style={{ color: row.enabled ? theme.colors.text : theme.colors.textMuted, fontSize: 15, fontWeight: "500" }}>
                  {feedLabel(t, row.feedType, libraryNames)}
                </Text>
                <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {t("common.feeds.maxItems", "Éléments max")}
                  </Text>
                  <TextInput
                    value={String(row.maxItems)}
                    onChangeText={(text) => {
                      // Guard NaN (empty / non-numeric input): keep the old value
                      // rather than persisting NaN to the backend on Save.
                      const v = Number(text);
                      setMaxItems(i, Number.isFinite(v) && text !== "" ? Math.min(100, Math.max(1, v)) : row.maxItems);
                    }}
                    keyboardType="numeric"
                    maxLength={3}
                    accessibilityLabel={`${t("common.feeds.maxItems", "Éléments max")}: ${feedLabel(t, row.feedType, libraryNames)}`}
                    style={[styles.maxItemsInput, {
                      color: theme.colors.text,
                      borderColor: theme.colors.border,
                      backgroundColor: theme.colors.bg,
                    }]}
                  />
                </View>
              </View>

              <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                <View style={{ gap: 2 }}>
                  <Pressable onPress={() => moveUp(i)} disabled={i === 0} hitSlop={4}>
                    <Ionicons name="chevron-up" size={16} color={i === 0 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                  <Pressable onPress={() => moveDown(i)} disabled={i >= rows.length - 1} hitSlop={4}>
                    <Ionicons name="chevron-down" size={16} color={i >= rows.length - 1 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                </View>
                <Switch
                  value={row.enabled}
                  onValueChange={() => toggle(i)}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>
            </View>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  maxItemsInput: {
    width: 44,
    fontSize: 12,
    textAlign: "center",
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 6,
    paddingVertical: 2,
  },
});

