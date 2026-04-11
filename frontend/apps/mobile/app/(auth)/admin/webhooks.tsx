import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Switch, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useWebhookServiceListWebhooks,
  useWebhookServiceCreateWebhook,
  useWebhookServiceDeleteWebhook,
  useWebhookServiceUpdateWebhook,
  getWebhookServiceListWebhooksQueryKey,
} from "@evey/api";
import type { V1WebhookConfig } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const QUERY_KEY = getWebhookServiceListWebhooksQueryKey();

export default function AdminWebhooksScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, refetch } = useWebhookServiceListWebhooks();
  const webhooks: V1WebhookConfig[] = data?.webhooks ?? [];

  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<V1WebhookConfig | null>(null);
  const [formName, setFormName] = useState("");
  const [formUrl, setFormUrl] = useState("");
  const [formEvents, setFormEvents] = useState("");
  const [formEnabled, setFormEnabled] = useState(true);

  const openCreate = useCallback(() => {
    setEditTarget(null);
    setFormName("");
    setFormUrl("");
    setFormEvents("");
    setFormEnabled(true);
    setShowForm(true);
  }, []);

  const openEdit = useCallback((wh: V1WebhookConfig) => {
    setEditTarget(wh);
    setFormName(wh.name ?? "");
    setFormUrl(wh.url ?? "");
    setFormEvents((wh.events ?? []).join(", "));
    setFormEnabled(wh.enabled ?? true);
    setShowForm(true);
  }, []);

  const closeForm = useCallback(() => {
    setShowForm(false);
    setEditTarget(null);
  }, []);

  const createWebhook = useWebhookServiceCreateWebhook({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.webhooks.webhookCreated", "Webhook created"));
        await qc.invalidateQueries({ queryKey: QUERY_KEY });
        closeForm();
      },
      onError: () => toast.error(t("admin.webhooks.failedCreate", "Failed to create webhook")),
    },
  });

  const updateWebhook = useWebhookServiceUpdateWebhook({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.webhooks.webhookUpdated", "Webhook updated"));
        await qc.invalidateQueries({ queryKey: QUERY_KEY });
        closeForm();
      },
      onError: () => toast.error(t("admin.webhooks.failedUpdate", "Failed to update webhook")),
    },
  });

  const deleteWebhook = useWebhookServiceDeleteWebhook({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.webhooks.webhookDeleted", "Webhook deleted"));
        await qc.invalidateQueries({ queryKey: QUERY_KEY });
      },
    },
  });

  const toggleEnabled = useWebhookServiceUpdateWebhook({
    mutation: {
      onSuccess: () => qc.invalidateQueries({ queryKey: QUERY_KEY }),
      onError: () => toast.error(t("admin.webhooks.failedUpdate", "Failed to update webhook")),
    },
  });

  const handleSubmit = useCallback(() => {
    if (!formUrl.trim()) return;
    const events = formEvents.split(",").map((e) => e.trim()).filter(Boolean);
    if (editTarget?.id) {
      updateWebhook.mutate({ id: editTarget.id, data: { name: formName, url: formUrl, events, enabled: formEnabled } });
    } else {
      createWebhook.mutate({ data: { name: formName, url: formUrl, events, enabled: formEnabled } });
    }
  }, [formName, formUrl, formEvents, formEnabled, editTarget, createWebhook, updateWebhook]);

  const handleDelete = useCallback((wh: V1WebhookConfig) => {
    Alert.alert(t("common.delete"), `Delete "${wh.name || wh.url}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.delete"), style: "destructive", onPress: () => wh.id && deleteWebhook.mutate({ id: wh.id }) },
    ]);
  }, [deleteWebhook, t]);

  const handleToggle = useCallback((wh: V1WebhookConfig) => {
    if (!wh.id) return;
    toggleEnabled.mutate({
      id: wh.id,
      data: { name: wh.name, url: wh.url, events: wh.events, enabled: !wh.enabled },
    });
  }, [toggleEnabled]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.webhooks.title", "Webhooks"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable onPress={openCreate} style={{ marginRight: 16 }}>
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
          data={webhooks}
          keyExtractor={(wh) => wh.id ?? wh.url ?? ""}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }) => (
            <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <Pressable onPress={() => openEdit(item)} style={{ flex: 1, gap: 2 }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
                  {item.name || item.url}
                </Text>
                {item.name ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{item.url}</Text>
                ) : null}
                {(item.events?.length ?? 0) > 0 && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 10 }} numberOfLines={1}>
                    {item.events!.join(", ")}
                  </Text>
                )}
              </Pressable>
              <Switch
                value={item.enabled}
                onValueChange={() => handleToggle(item)}
                trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
              />
              <Pressable onPress={() => handleDelete(item)} hitSlop={8}>
                <Ionicons name="trash-outline" size={18} color=theme.colors.error />
              </Pressable>
            </View>
          )}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="link-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>
                {t("admin.webhooks.noWebhooks", "No webhooks")}
              </Text>
            </View>
          }
        />
      )}

      <Modal visible={showForm} transparent animationType="slide" onRequestClose={closeForm}>
        <Pressable style={styles.backdrop} onPress={closeForm} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>
            {editTarget ? t("admin.webhooks.editWebhook", "Edit Webhook") : t("admin.webhooks.createWebhook", "Create Webhook")}
          </Text>
          <TextInput
            placeholder="Name"
            placeholderTextColor={theme.colors.textMuted}
            value={formName}
            onChangeText={setFormName}
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
          />
          <TextInput
            placeholder="URL"
            placeholderTextColor={theme.colors.textMuted}
            value={formUrl}
            onChangeText={setFormUrl}
            autoCapitalize="none"
            keyboardType="url"
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
          />
          <TextInput
            placeholder="Events (comma-separated)"
            placeholderTextColor={theme.colors.textMuted}
            value={formEvents}
            onChangeText={setFormEvents}
            autoCapitalize="none"
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
          />
          <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 16 }}>
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>{t("common.enabled", "Enabled")}</Text>
            <Switch
              value={formEnabled}
              onValueChange={setFormEnabled}
              trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
            />
          </View>
          <Pressable
            onPress={handleSubmit}
            disabled={createWebhook.isPending || updateWebhook.isPending}
            style={[styles.submitBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
          >
            <Text style={{ color: "#fff", fontWeight: "600" }}>
              {editTarget ? t("common.save", "Save") : t("common.create", "Create")}
            </Text>
          </Pressable>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  submitBtn: { paddingVertical: 14, alignItems: "center" },
});
