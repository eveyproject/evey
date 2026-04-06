import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMetadataServiceGetMetadataProviders, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface MetadataProvider {
  id?: string;
  name?: string;
  enabled?: boolean;
  priority?: number;
  supportedTypes?: string[];
}

export default function AdminMetadataScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data, isLoading, refetch } = useMetadataServiceGetMetadataProviders();
  const providers: MetadataProvider[] = (data?.providers ?? []) as MetadataProvider[];
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [listRefreshing, setListRefreshing] = useState(false);

  const handleRefreshAll = useCallback(() => {
    Alert.alert(
      t("admin.metadata.refreshAll", "Refresh All Metadata"),
      t("admin.metadata.refreshAllConfirm", "This will re-fetch metadata for all media items. Continue?"),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.confirm", "Confirm"),
          onPress: async () => {
            setRefreshingAll(true);
            try {
              await eveyFetch<void>({ url: "/api/v1/admin/metadata/refresh", method: "POST" });
              toast.success(t("admin.metadata.refreshStarted", "Metadata refresh started"));
            } catch {
              toast.error(t("admin.metadata.refreshFailed", "Failed to start metadata refresh"));
            } finally {
              setRefreshingAll(false);
            }
          },
        },
      ],
    );
  }, [t]);

  const onRefresh = useCallback(async () => {
    setListRefreshing(true);
    await refetch();
    setListRefreshing(false);
  }, [refetch]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.metadata.title", "Metadata"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={providers}
          keyExtractor={(p) => p.id ?? p.name ?? ""}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={listRefreshing}
          onRefresh={onRefresh}
          ListHeaderComponent={
            <Pressable
              onPress={handleRefreshAll}
              disabled={refreshingAll}
              style={[styles.actionBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, marginBottom: 16 }]}
            >
              {refreshingAll ? (
                <ActivityIndicator color="#fff" size="small" />
              ) : (
                <Ionicons name="refresh-outline" size={20} color="#fff" />
              )}
              <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>
                {t("admin.metadata.refreshAll", "Refresh All Metadata")}
              </Text>
            </Pressable>
          }
          renderItem={({ item }) => (
            <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <Ionicons name="cloud-outline" size={20} color={theme.colors.accent} />
              <View style={{ flex: 1, gap: 2 }}>
                <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{item.name ?? item.id}</Text>
                  {item.priority != null && (
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>#{item.priority}</Text>
                  )}
                </View>
                {(item.supportedTypes?.length ?? 0) > 0 && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>
                    {item.supportedTypes!.map((t) => t.replace("MEDIA_ITEM_TYPE_", "")).join(", ")}
                  </Text>
                )}
              </View>
              <View style={[styles.badge, { backgroundColor: item.enabled ? "#22c55e20" : "#ef444420" }]}>
                <Text style={{ color: item.enabled ? "#22c55e" : "#ef4444", fontSize: 11, fontWeight: "600" }}>
                  {item.enabled ? t("common.enabled", "Enabled") : t("common.disabled", "Disabled")}
                </Text>
              </View>
            </View>
          )}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 40 }}>
              <Ionicons name="cloud-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>No metadata providers</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  actionBtn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 14 },
  badge: { paddingHorizontal: 8, paddingVertical: 3, borderRadius: 6 },
});
