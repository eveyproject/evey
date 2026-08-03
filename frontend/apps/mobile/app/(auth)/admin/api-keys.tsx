import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Alert, ActivityIndicator, ScrollView, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceListAPIKeys,
  useUserServiceCreateAPIKey,
  useUserServiceRevokeAPIKey,
  getUserServiceListAPIKeysQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const SCOPE_OPTIONS = [
  "media:read",
  "media:write",
  "admin:read",
  "admin:write",
  "users:read",
  "users:write",
  "playback:read",
  "playback:write",
] as const;

export default function AdminAPIKeysScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, refetch } = useUserServiceListAPIKeys();
  const keys = data?.apiKeys ?? [];

  const [showCreate, setShowCreate] = useState(false);
  const [keyName, setKeyName] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(new Set());
  const [newKey, setNewKey] = useState<string | null>(null);

  const resetForm = useCallback(() => {
    setKeyName("");
    setSelectedScopes(new Set());
  }, []);

  const createKey = useUserServiceCreateAPIKey({
    mutation: {
      onSuccess: async (resp) => {
        if (resp?.key) setNewKey(resp.key);
        toast.success(t("auth.apiKeys.keyCreated"));
        await qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
      },
      onError: () => toast.error(t("auth.apiKeys.createError")),
    },
  });

  const revokeKey = useUserServiceRevokeAPIKey({
    mutation: {
      onSuccess: async () => {
        toast.success(t("auth.apiKeys.keyRevoked"));
        await qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
      },
      // handleRevoke fires this via mutate() (fire-and-forget) with no local
      // try/catch, so without onError a failed revoke left the key looking
      // still active with zero feedback to the admin.
      onError: () => toast.error(t("auth.apiKeys.revokeError", "Failed to revoke key")),
    },
  });

  const toggleScope = useCallback((sc: string) => {
    setSelectedScopes((prev) => {
      const next = new Set(prev);
      if (next.has(sc)) next.delete(sc);
      else next.add(sc);
      return next;
    });
  }, []);

  const handleCreate = useCallback(async () => {
    if (!keyName.trim()) return;
    try {
      await createKey.mutateAsync({ data: { name: keyName.trim(), scopes: Array.from(selectedScopes) } });
      resetForm();
      setShowCreate(false);
    } catch {}
  }, [keyName, selectedScopes, createKey, resetForm]);

  const handleRevoke = useCallback((keyId: string, name: string) => {
    Alert.alert(t("auth.apiKeys.revoke", "Revoke Key"), `${t("auth.apiKeys.revoke", "Revoke")} "${name}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("auth.apiKeys.revoke", "Revoke"), style: "destructive", onPress: () => revokeKey.mutate({ id: keyId }) },
    ]);
  }, [revokeKey, t]);

  const closeCreate = useCallback(() => {
    setShowCreate(false);
    resetForm();
  }, [resetForm]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.nav.apiKeys"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable
            onPress={() => setShowCreate(true)}
            style={{ marginRight: 16 }}
            accessibilityRole="button"
            accessibilityLabel={t("auth.apiKeys.newKeyTitle")}
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
          data={keys}
          keyExtractor={(k: any) => k.id ?? k.name}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }: { item: any }) => {
            const scopes: string[] = item.scopes ?? [];
            return (
              <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                <View style={{ flex: 1, gap: 4 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{item.name ?? t("common.unknown")}</Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {item.lastUsedAt ? `Last used: ${new Date(item.lastUsedAt).toLocaleDateString()}` : "Never used"}
                  </Text>
                  <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 4, marginTop: 2 }}>
                    {scopes.length === 0 ? (
                      <View style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised }]}>
                        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
                          {t("auth.apiKeys.scopesAll")}
                        </Text>
                      </View>
                    ) : (
                      scopes.map((sc) => (
                        <View key={sc} style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised }]}>
                          <Text style={{ color: theme.colors.textMuted, fontSize: 10, fontFamily: "monospace" }}>{sc}</Text>
                        </View>
                      ))
                    )}
                  </View>
                </View>
                <Pressable
                  onPress={() => handleRevoke(item.id, item.name)}
                  hitSlop={8}
                  accessibilityRole="button"
                  accessibilityLabel={t("auth.apiKeys.revokeTitle")}
                >
                  <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
                </Pressable>
              </View>
            );
          }}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>{t("auth.apiKeys.noKeys")}</Text>
            </View>
          }
        />
      )}

      {newKey && (
        <Modal visible transparent animationType="fade" onRequestClose={() => setNewKey(null)}>
          <View style={styles.backdrop}>
            <View style={[styles.keyModal, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", marginBottom: 8 }}>{t("auth.apiKeys.keyCreatedTitle")}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>{t("auth.apiKeys.saveWarning")}</Text>
              <View style={{ backgroundColor: theme.colors.bg, padding: 12, borderRadius: theme.radii.sm }}>
                <Text style={{ color: theme.colors.text, fontFamily: "monospace", fontSize: 12 }} selectable>{newKey}</Text>
              </View>
              <Pressable onPress={() => setNewKey(null)} style={{ marginTop: 16, alignSelf: "center" }}>
                <Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.done")}</Text>
              </Pressable>
            </View>
          </View>
        </Modal>
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={closeCreate}>
        <Pressable style={styles.backdrop} onPress={closeCreate} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>{t("auth.apiKeys.newKeyTitle")}</Text>
          <TextInput
            placeholder={t("auth.apiKeys.name")}
            placeholderTextColor={theme.colors.textMuted}
            value={keyName}
            onChangeText={setKeyName}
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
          />
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginBottom: 4 }}>
            {t("auth.apiKeys.scopes")}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginBottom: 8 }}>
            {t("auth.apiKeys.scopesHint")}
          </Text>
          <ScrollView style={{ maxHeight: 240 }} contentContainerStyle={{ gap: 6, paddingBottom: 4 }}>
            {SCOPE_OPTIONS.map((sc) => {
              const checked = selectedScopes.has(sc);
              return (
                <Pressable
                  key={sc}
                  onPress={() => toggleScope(sc)}
                  style={[styles.scopeRow, { borderColor: theme.colors.border }]}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked }}
                >
                  <Ionicons
                    name={checked ? "checkbox" : "square-outline"}
                    size={20}
                    color={checked ? theme.colors.accent : theme.colors.textMuted}
                  />
                  <View style={{ flex: 1 }}>
                    <Text style={{ color: theme.colors.text, fontSize: 12, fontFamily: "monospace" }}>{sc}</Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
                      {t(`auth.apiKeys.scopeLabels.${sc}`)}
                    </Text>
                  </View>
                </Pressable>
              );
            })}
          </ScrollView>
          <Pressable
            onPress={handleCreate}
            disabled={createKey.isPending || !keyName.trim()}
            style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: !keyName.trim() ? 0.5 : 1 }]}
          >
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.create")}</Text>
          </Pressable>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)", justifyContent: "center", alignItems: "center" },
  keyModal: { padding: 24, marginHorizontal: 24, width: "85%" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32, maxHeight: "85%" },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  createBtn: { paddingVertical: 14, alignItems: "center", marginTop: 12 },
  chip: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  scopeRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 6, paddingHorizontal: 4 },
});
