import { useState, useCallback } from "react";
import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useAdminServiceGetAuditLog } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function AdminAuditScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const { data, isLoading, refetch } = useAdminServiceGetAuditLog({ limit: 100 });
  const entries = data?.entries ?? [];

  const renderEntry = useCallback(({ item }: { item: any }) => (
    <View style={[styles.row, { borderBottomColor: theme.colors.border }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "500" }}>
          {item.action ?? "Unknown action"}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {item.username ?? "System"} · {item.resourceType ?? ""} {item.resourceId ? `#${item.resourceId.slice(0, 8)}` : ""}
        </Text>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
        {item.createdAt ? new Date(item.createdAt).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }) : ""}
      </Text>
    </View>
  ), [theme]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.audit") || "Audit Log",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={entries}
          keyExtractor={(item, i) => item.id ?? `${i}`}
          renderItem={renderEntry}
          refreshing={false}
          onRefresh={refetch}
          contentContainerStyle={{ paddingBottom: 32 }}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>No audit entries</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth },
});
