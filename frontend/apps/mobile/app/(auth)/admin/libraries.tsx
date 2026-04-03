import { View, Text, FlatList, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  getLibraryServiceListLibrariesQueryKey,
} from "@evey/api";
import type { V1Library } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "film-outline",
  LIBRARY_TYPE_SHOWS: "tv-outline",
  LIBRARY_TYPE_MUSIC: "musical-notes-outline",
  LIBRARY_TYPE_PHOTOS: "image-outline",
  LIBRARY_TYPE_BOOKS: "book-outline",
};

export default function AdminLibrariesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];

  const scanLibrary = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => toast.success("Scan started"),
      onError: () => toast.error("Failed to start scan"),
    },
  });

  const handleScan = useCallback((lib: V1Library) => {
    if (!lib.id) return;
    scanLibrary.mutate({ id: lib.id });
  }, [scanLibrary]);

  const renderLibrary = useCallback(({ item }: { item: V1Library }) => {
    const icon = TYPE_ICONS[item.type ?? ""] ?? "folder-outline";
    const lastScan = item.lastScannedAt
      ? new Date(item.lastScannedAt).toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })
      : "Never";

    return (
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
        <Ionicons name={icon as never} size={28} color={theme.colors.accent} />
        <View style={{ flex: 1, gap: 2 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.name}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {item.type?.replace("LIBRARY_TYPE_", "")} · Last scan: {lastScan}
          </Text>
        </View>
        <Pressable
          onPress={() => handleScan(item)}
          disabled={scanLibrary.isPending}
          hitSlop={8}
          style={{ padding: 4 }}
        >
          <Ionicons name="refresh-outline" size={20} color={theme.colors.accent} />
        </Pressable>
      </View>
    );
  }, [theme, handleScan, scanLibrary.isPending]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.libraries") || "Libraries",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
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
              <Text style={{ color: theme.colors.textMuted }}>No libraries</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
});
