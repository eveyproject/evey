import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Switch, Alert, ActivityIndicator, ScrollView, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useWebhookServiceListWebhooks,
  useWebhookServiceCreateWebhook,
  useWebhookServiceDeleteWebhook,
  useWebhookServiceUpdateWebhook,
  getWebhookServiceListWebhooksQueryKey,
  eveyFetch,
} from "@evey/api";
import type { V1WebhookConfig } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";

const QUERY_KEY = getWebhookServiceListWebhooksQueryKey();

const EVENT_TYPES = [
  "media.item.added",
  "media.item.updated",
  "media.item.removed",
  "library.created",
  "library.deleted",
  "library.scan.started",
  "library.scan.completed",
  "playback.started",
  "playback.stopped",
  "user.created",
  "user.deleted",
  "user.login.succeeded",
  "user.login.failed",
  "backup.created",
  "backup.failed",
] as const;

function WebhookRow({
  webhook,
  onEdit,
  onToggle,
  onDelete,
}: {
  webhook: V1WebhookConfig;
  onEdit: (wh: V1WebhookConfig) => void;
  onToggle: (wh: V1WebhookConfig) => void;
  onDelete: (wh: V1WebhookConfig) => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [testing, setTesting] = useState(false);

  async function handleTest() {
    if (!webhook.id) return;
    setTesting(true);
    try {
      await eveyFetch<void>({
        url: `/api/v1/admin/webhooks/${webhook.id}/test`,
        method: "POST",
      });
      toast.success(t("admin.webhooks.testPingSent", "Ping de test envoyé"));
    } catch {
      toast.error(t("admin.webhooks.testPingFailed", "Échec du ping de test"));
    } finally {
      setTesting(false);
    }
  }

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Pressable onPress={() => onEdit(webhook)} style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
          {webhook.name || webhook.url}
        </Text>
        {webhook.name ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{webhook.url}</Text>
        ) : null}
        {(webhook.events?.length ?? 0) > 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }} numberOfLines={1}>
            {webhook.events!.join(", ")}
          </Text>
        )}
      </Pressable>
      <Pressable
        onPress={handleTest}
        disabled={testing}
        hitSlop={8}
        accessibilityRole="button"
        accessibilityLabel={t("common.test", "Tester")}
        style={{ opacity: testing ? 0.5 : 1 }}
      >
        {testing ? (
          <ActivityIndicator size="small" color={theme.colors.accent} />
        ) : (
          <Ionicons name="paper-plane-outline" size={18} color={theme.colors.accent} />
        )}
      </Pressable>
      <Switch
        value={webhook.enabled}
        onValueChange={() => onToggle(webhook)}
        trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
      />
      <Pressable
        onPress={() => onDelete(webhook)}
        hitSlop={8}
        accessibilityRole="button"
        accessibilityLabel={t("common.delete")}
      >
        <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
      </Pressable>
    </View>
  );
}

export default function AdminWebhooksScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch } = useWebhookServiceListWebhooks();
  const webhooks: V1WebhookConfig[] = data?.webhooks ?? [];

  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<V1WebhookConfig | null>(null);
  const [formName, setFormName] = useState("");
  const [formUrl, setFormUrl] = useState("");
  const [formSecret, setFormSecret] = useState("");
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set(["*"]));
  const [allEvents, setAllEvents] = useState(true);
  const [formEnabled, setFormEnabled] = useState(true);

  const openCreate = useCallback(() => {
    setEditTarget(null);
    setFormName("");
    setFormUrl("");
    setFormSecret("");
    setSelectedEvents(new Set(["*"]));
    setAllEvents(true);
    setFormEnabled(true);
    setShowForm(true);
  }, []);

  const openEdit = useCallback((wh: V1WebhookConfig) => {
    setEditTarget(wh);
    setFormName(wh.name ?? "");
    setFormUrl(wh.url ?? "");
    setFormSecret(wh.secret ?? "");
    const evs = wh.events ?? [];
    const isAll = evs.length === 0 || (evs.length === 1 && evs[0] === "*");
    setAllEvents(isAll);
    setSelectedEvents(isAll ? new Set(["*"]) : new Set(evs));
    setFormEnabled(wh.enabled ?? true);
    setShowForm(true);
  }, []);

  const closeForm = useCallback(() => {
    setShowForm(false);
    setEditTarget(null);
  }, []);

  const toggleEvent = useCallback((evt: string) => {
    setSelectedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(evt)) next.delete(evt);
      else next.add(evt);
      return next;
    });
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
    const events = allEvents ? ["*"] : Array.from(selectedEvents);
    const payload = {
      name: formName,
      url: formUrl,
      events,
      secret: formSecret || undefined,
      enabled: formEnabled,
    };
    if (editTarget?.id) {
      updateWebhook.mutate({ id: editTarget.id, data: payload });
    } else {
      createWebhook.mutate({ data: payload });
    }
  }, [formName, formUrl, formSecret, allEvents, selectedEvents, formEnabled, editTarget, createWebhook, updateWebhook]);

  const handleDelete = useCallback((wh: V1WebhookConfig) => {
    Alert.alert(t("common.delete"), `Delete "${wh.name || wh.url}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.delete"), style: "destructive", onPress: () => wh.id && deleteWebhook.mutate({ id: wh.id }) },
    ]);
  }, [deleteWebhook, t]);

  const handleToggle = useCallback((wh: V1WebhookConfig) => {
    if (!wh.id) return;
    // UpdateWebhook is a full replace (proto3 scalar, no "field not sent"
    // semantics server-side) - omitting secret here would clear the signing
    // key on every enable/disable flip, so it must ride along unchanged.
    toggleEnabled.mutate({
      id: wh.id,
      data: { name: wh.name, url: wh.url, events: wh.events, secret: wh.secret, enabled: !wh.enabled },
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
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
      ) : (
        <FlatList
          data={webhooks}
          keyExtractor={(wh) => wh.id ?? wh.url ?? ""}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }) => (
            <WebhookRow
              webhook={item}
              onEdit={openEdit}
              onToggle={handleToggle}
              onDelete={handleDelete}
            />
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
          <ScrollView style={{ maxHeight: 480 }} keyboardShouldPersistTaps="handled">
            <TextInput
              placeholder={t("admin.webhooks.form.name", "Name")}
              placeholderTextColor={theme.colors.textMuted}
              value={formName}
              onChangeText={setFormName}
              style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
            />
            <TextInput
              placeholder={t("admin.webhooks.form.url", "URL")}
              placeholderTextColor={theme.colors.textMuted}
              value={formUrl}
              onChangeText={setFormUrl}
              autoCapitalize="none"
              keyboardType="url"
              style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
            />
            <TextInput
              placeholder={t("admin.webhooks.form.secret", "Secret")}
              placeholderTextColor={theme.colors.textMuted}
              value={formSecret}
              onChangeText={setFormSecret}
              autoCapitalize="none"
              style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
            />

            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginBottom: 8 }}>
              {t("admin.webhooks.form.events", "Events")}
            </Text>
            <Pressable
              onPress={() => setAllEvents((v) => !v)}
              style={[styles.scopeRow, { borderColor: theme.colors.border }]}
              accessibilityRole="checkbox"
              accessibilityState={{ checked: allEvents }}
            >
              <Ionicons
                name={allEvents ? "checkbox" : "square-outline"}
                size={20}
                color={allEvents ? theme.colors.accent : theme.colors.textMuted}
              />
              <Text style={{ color: theme.colors.text, fontSize: 13 }}>
                {t("admin.webhooks.form.allEvents", "All events")}
              </Text>
            </Pressable>

            {!allEvents && (
              <View style={{ marginTop: 4 }}>
                {EVENT_TYPES.map((evt) => {
                  const checked = selectedEvents.has(evt);
                  return (
                    <Pressable
                      key={evt}
                      onPress={() => toggleEvent(evt)}
                      style={[styles.scopeRow, { borderColor: theme.colors.border }]}
                      accessibilityRole="checkbox"
                      accessibilityState={{ checked }}
                    >
                      <Ionicons
                        name={checked ? "checkbox" : "square-outline"}
                        size={20}
                        color={checked ? theme.colors.accent : theme.colors.textMuted}
                      />
                      <Text style={{ color: theme.colors.text, fontSize: 12, fontFamily: "monospace" }}>{evt}</Text>
                    </Pressable>
                  );
                })}
              </View>
            )}

            <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginTop: 16, marginBottom: 16 }}>
              <Text style={{ color: theme.colors.text, fontSize: 15 }}>{t("common.enabled", "Enabled")}</Text>
              <Switch
                value={formEnabled}
                onValueChange={setFormEnabled}
                trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
              />
            </View>
          </ScrollView>

          <Pressable
            onPress={handleSubmit}
            disabled={createWebhook.isPending || updateWebhook.isPending || !formUrl.trim()}
            style={[styles.submitBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: !formUrl.trim() ? 0.5 : 1 }]}
          >
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>
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
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32, maxHeight: "90%" },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  submitBtn: { paddingVertical: 14, alignItems: "center", marginTop: 8 },
  scopeRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 6, paddingHorizontal: 4 },
});
