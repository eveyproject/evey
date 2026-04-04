import { useCallback } from "react";
import { View, Text, FlatList, Pressable, Switch, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminServiceListPlugins,
  useAdminServiceEnablePlugin,
  useAdminServiceDisablePlugin,
  useAdminServiceUninstallPlugin,
  getAdminServiceListPluginsQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface Plugin {
  id: string;
  name: string;
  version: string;
  description?: string;
  enabled: boolean;
  author?: string;
}

export default function AdminPluginsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useAdminServiceListPlugins();
  const plugins: Plugin[] = (data?.plugins ?? []) as Plugin[];

  const enablePlugin = useAdminServiceEnablePlugin({
    mutation: { onSuccess: () => { toast.success("Plugin enabled"); qc.invalidateQueries({ queryKey: getAdminServiceListPluginsQueryKey() }); } },
  });
  const disablePlugin = useAdminServiceDisablePlugin({
    mutation: { onSuccess: () => { toast.success("Plugin disabled"); qc.invalidateQueries({ queryKey: getAdminServiceListPluginsQueryKey() }); } },
  });
  const uninstallPlugin = useAdminServiceUninstallPlugin({
    mutation: { onSuccess: () => { toast.success("Plugin uninstalled"); qc.invalidateQueries({ queryKey: getAdminServiceListPluginsQueryKey() }); } },
  });

  const handleToggle = useCallback((plugin: Plugin) => {
    if (plugin.enabled) {
      disablePlugin.mutate({ id: plugin.id });
    } else {
      enablePlugin.mutate({ id: plugin.id });
    }
  }, [enablePlugin, disablePlugin]);

  const handleUninstall = useCallback((plugin: Plugin) => {
    Alert.alert("Uninstall Plugin", `Uninstall "${plugin.name}"?`, [
      { text: "Cancel", style: "cancel" },
      { text: "Uninstall", style: "destructive", onPress: () => uninstallPlugin.mutate({ id: plugin.id }) },
    ]);
  }, [uninstallPlugin]);

  const renderPlugin = useCallback(({ item }: { item: Plugin }) => (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.name}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>v{item.version}</Text>
        </View>
        {item.description ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={2}>{item.description}</Text>
        ) : null}
        {item.author ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>by {item.author}</Text>
        ) : null}
      </View>
      <Switch
        value={item.enabled}
        onValueChange={() => handleToggle(item)}
        trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
      />
      <Pressable onPress={() => handleUninstall(item)} hitSlop={8} style={{ padding: 4 }}>
        <Ionicons name="trash-outline" size={18} color="#ef4444" />
      </Pressable>
    </View>
  ), [theme, handleToggle, handleUninstall]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: "Plugins",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={plugins}
          keyExtractor={(p) => p.id}
          renderItem={renderPlugin}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="extension-puzzle-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>No plugins installed</Text>
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
