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
  getFeedServiceGetFeedConfigQueryKey,
  useLibraryServiceListLibraries,
  useAdminPluginServiceListPlugins,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";

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

// A plugin home.section row: admin can force-disable it (propagates to every
// user as a hard override) but not reorder it (each user keeps their own drag
// order), so its max-items is "-" and its move arrows are disabled.
function isPluginRow(feedType: string): boolean {
  return feedType.startsWith("plugin:");
}
function pluginLabel(feedType: string, pluginNames: Map<string, string>): string {
  const id = feedType.slice("plugin:".length);
  return pluginNames.get(id) ?? id;
}

export default function AdminFeedsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, isError, refetch } = useFeedServiceGetAdminFeedConfigs();
  const { data: librariesData } = useLibraryServiceListLibraries();
  const { data: pluginsData } = useAdminPluginServiceListPlugins();
  const libraryNames = new Map<string, string>(
    (librariesData?.libraries ?? []).map((l: { id?: string; name?: string }) => [l.id!, l.name ?? l.id!]),
  );
  // Only plugins that expose a `home.section` frontend slot belong on the home
  // page (mirror web FeedConfigCard.homePlugins).
  const homePlugins = (pluginsData?.plugins ?? []).filter(
    (p) => p.enabled && (p.frontend ?? []).some((mod) => mod.slot === "home.section"),
  );
  const pluginNames = new Map<string, string>(homePlugins.map((p) => [p.id ?? "", p.name ?? p.id ?? ""]));

  const [rows, setRows] = useState<FeedRow[]>([]);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    // Don't overwrite unsaved local edits when a background refetch arrives.
    if (dirty) return;
    if (data?.configs) {
      const persisted = data.configs.map((c: { feedType?: string; enabled?: boolean; displayOrder?: number; maxItems?: number }) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
        maxItems: c.maxItems ?? 20,
      }));
      // Synthesise rows for plugins with no admin row yet, so a freshly
      // installed plugin shows up at once - enabled, parked at the end.
      const persistedTypes = new Set(persisted.map((r: FeedRow) => r.feedType));
      const baseOrder = persisted.length;
      const virtualPluginRows: FeedRow[] = homePlugins
        .map((p) => `plugin:${p.id ?? ""}`)
        .filter((ft) => !persistedTypes.has(ft))
        .map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: baseOrder + i, maxItems: 20 }));
      setRows([...persisted, ...virtualPluginRows].sort((a, b) => a.displayOrder - b.displayOrder));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- homePlugins derived from pluginsData; re-run on configs/plugins/dirty only
  }, [data, pluginsData, dirty]);

  const batchUpdate = useFeedServiceBatchUpdateAdminFeedConfigs({
    mutation: {
      onSuccess: () => {
        toast.success(t("common.feeds.saved"));
        qc.invalidateQueries({ queryKey: getFeedServiceGetAdminFeedConfigsQueryKey() });
        // The home page reads the MERGED config (admin defaults + profile
        // overrides) via GetFeedConfig - invalidate it too or open home pages
        // keep showing the stale order/sections until staleTime expires.
        qc.invalidateQueries({ queryKey: getFeedServiceGetFeedConfigQueryKey() });
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
    // Plugin rows keep each user's own order - admin can't reorder them.
    if (isPluginRow(rows[i].feedType) || isPluginRow(rows[i - 1].feedType)) return;
    setRows((prev) => {
      const next = [...prev];
      [next[i - 1], next[i]] = [next[i], next[i - 1]];
      return next.map((r, idx) => ({ ...r, displayOrder: idx }));
    });
    setDirty(true);
  };

  const moveDown = (i: number) => {
    if (i >= rows.length - 1) return;
    if (isPluginRow(rows[i].feedType) || isPluginRow(rows[i + 1].feedType)) return;
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
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
      ) : rows.length === 0 ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <Ionicons name="grid-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.feeds.noSections", "No feed sections configured")}</Text>
        </View>
      ) : (
        <View style={{ padding: 16, gap: 8 }}>
          {rows.map((row, i) => {
            const plugin = isPluginRow(row.feedType);
            const label = plugin ? pluginLabel(row.feedType, pluginNames) : feedLabel(t, row.feedType, libraryNames);
            return (
            <View key={row.feedType} style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, borderLeftWidth: plugin ? 3 : 0, borderLeftColor: theme.colors.accent }]}>
              <View style={{ gap: 4, flex: 1 }}>
                <View style={{ flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                  <Text style={{ color: row.enabled ? theme.colors.text : theme.colors.textMuted, fontSize: 15, fontWeight: "500" }}>
                    {label}
                  </Text>
                  {plugin && (
                    <View style={{ paddingHorizontal: 6, paddingVertical: 2, borderRadius: theme.radii.sm, backgroundColor: theme.colors.accent + "22" }}>
                      <Text style={{ color: theme.colors.accent, fontSize: 9, fontWeight: "700", textTransform: "uppercase" }}>
                        {t("admin.serverConfig.feedConfig.pluginBadge", "Plugin")}
                      </Text>
                    </View>
                  )}
                </View>
                {plugin ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>-</Text>
                ) : (
                  <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                      {t("common.feeds.maxItems", "Éléments max")}
                    </Text>
                    <TextInput
                      value={String(row.maxItems)}
                      onChangeText={(text) => {
                        // Guard NaN (empty / non-numeric input): keep the old value.
                        const v = Number(text);
                        setMaxItems(i, Number.isFinite(v) && text !== "" ? Math.min(100, Math.max(1, v)) : row.maxItems);
                      }}
                      keyboardType="numeric"
                      maxLength={3}
                      accessibilityLabel={`${t("common.feeds.maxItems", "Éléments max")}: ${label}`}
                      style={[styles.maxItemsInput, { color: theme.colors.text, borderColor: theme.colors.border, backgroundColor: theme.colors.bg }]}
                    />
                  </View>
                )}
              </View>

              <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                <View style={{ gap: 2 }}>
                  <Pressable onPress={() => moveUp(i)} disabled={i === 0 || plugin} hitSlop={12} accessibilityRole="button" accessibilityLabel={t("common.feeds.moveUp")}>
                    <Ionicons name="chevron-up" size={16} color={i === 0 || plugin ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                  <Pressable onPress={() => moveDown(i)} disabled={i >= rows.length - 1 || plugin} hitSlop={12} accessibilityRole="button" accessibilityLabel={t("common.feeds.moveDown")}>
                    <Ionicons name="chevron-down" size={16} color={i >= rows.length - 1 || plugin ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                </View>
                <Switch
                  value={row.enabled}
                  onValueChange={() => toggle(i)}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>
            </View>
            );
          })}
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

