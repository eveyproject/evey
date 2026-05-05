import { useCallback, useState } from "react";
import { View, Text, FlatList, Pressable, Switch, Alert, ActivityIndicator, Modal, TextInput, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminPluginServiceListPlugins,
  useAdminPluginServiceEnablePlugin,
  useAdminPluginServiceDisablePlugin,
  useAdminPluginServiceUninstallPlugin,
  useAdminPluginServiceInstallPlugin,
  getAdminPluginServiceListPluginsQueryKey,
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

  const { data, isLoading, refetch } = useAdminPluginServiceListPlugins();
  const plugins: Plugin[] = (data?.plugins ?? []) as Plugin[];

  const enablePlugin = useAdminPluginServiceEnablePlugin({
    mutation: { onSuccess: () => { toast.success(t("admin.plugins.pluginEnabled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); } },
  });
  const disablePlugin = useAdminPluginServiceDisablePlugin({
    mutation: { onSuccess: () => { toast.success(t("admin.plugins.pluginDisabled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); } },
  });
  const uninstallPlugin = useAdminPluginServiceUninstallPlugin({
    mutation: { onSuccess: () => { toast.success(t("admin.plugins.pluginUninstalled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); } },
  });
  const installPlugin = useAdminPluginServiceInstallPlugin({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugins.pluginInstalled", "Plugin installed"));
        qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
        setInstallOpen(false);
        setInstallSource("");
      },
      onError: (err: Error) => toast.error(err.message || t("admin.plugins.installFailed", "Install failed")),
    },
  });

  const [installOpen, setInstallOpen] = useState(false);
  const [installSource, setInstallSource] = useState("");

  const handleToggle = useCallback((plugin: Plugin) => {
    if (plugin.enabled) {
      disablePlugin.mutate({ id: plugin.id });
    } else {
      enablePlugin.mutate({ id: plugin.id });
    }
  }, [enablePlugin, disablePlugin]);

  const handleUninstall = useCallback((plugin: Plugin) => {
    Alert.alert(t("admin.plugin.uninstall"), `${t("admin.plugin.uninstall")} "${plugin.name}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("admin.plugin.uninstall"), style: "destructive", onPress: () => uninstallPlugin.mutate({ id: plugin.id }) },
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
        <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
      </Pressable>
    </View>
  ), [theme, handleToggle, handleUninstall]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.plugins.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable
            onPress={() => setInstallOpen(true)}
            style={{ paddingHorizontal: 12, paddingVertical: 6 }}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("admin.plugins.installFromUrl", "Install from URL")}
          >
            <Ionicons name="add" size={24} color={theme.colors.accent} />
          </Pressable>
        ),
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
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.plugins.noPlugins", "No plugins installed")}</Text>
            </View>
          }
        />
      )}

      <Modal visible={installOpen} transparent animationType="fade" onRequestClose={() => setInstallOpen(false)}>
        <Pressable style={{ flex: 1, backgroundColor: "rgba(0,0,0,0.5)" }} onPress={() => setInstallOpen(false)} />
        <View style={{
          position: "absolute",
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: theme.colors.surface,
          borderTopLeftRadius: theme.radii.lg,
          borderTopRightRadius: theme.radii.lg,
          padding: theme.spacing.md,
          paddingBottom: theme.spacing.xl,
          gap: theme.spacing.sm,
        }}>
          <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700" }}>
            {t("admin.plugins.installFromUrl", "Install from URL")}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
            {t("admin.plugins.installFromUrlHint", "Paste a plugin .zip URL — optionally append #sha256:<hash> to verify the download.")}
          </Text>
          <TextInput
            value={installSource}
            onChangeText={setInstallSource}
            placeholder="https://example.com/plugin.zip"
            placeholderTextColor={theme.colors.textMuted}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            style={{
              backgroundColor: theme.colors.bg,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              padding: theme.spacing.sm + 2,
              borderWidth: 1,
              borderColor: theme.colors.border,
              fontSize: 14,
            }}
          />
          <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.sm }}>
            <Pressable
              onPress={() => setInstallOpen(false)}
              style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}
            >
              <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
            </Pressable>
            <Pressable
              onPress={() => installSource.trim() && installPlugin.mutate({ data: { source: installSource.trim() } })}
              disabled={!installSource.trim() || installPlugin.isPending}
              style={{
                flex: 1,
                alignItems: "center",
                paddingVertical: theme.spacing.sm + 2,
                borderRadius: theme.radii.md,
                backgroundColor: theme.colors.accent,
                opacity: !installSource.trim() || installPlugin.isPending ? 0.5 : 1,
              }}
            >
              {installPlugin.isPending ? (
                <ActivityIndicator color={theme.colors.onAccent} size="small" />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugins.install", "Install")}</Text>
              )}
            </Pressable>
          </View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
});
