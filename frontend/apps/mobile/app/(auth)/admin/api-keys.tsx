import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
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

export default function AdminAPIKeysScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading, refetch } = useUserServiceListAPIKeys();
  const keys = (data as any)?.apiKeys ?? [];

  const [showCreate, setShowCreate] = useState(false);
  const [keyName, setKeyName] = useState("");
  const [newKey, setNewKey] = useState<string | null>(null);

  const createKey = useUserServiceCreateAPIKey({
    mutation: {
      onSuccess: async (resp: unknown) => {
        const key = (resp as any)?.key ?? (resp as any)?.apiKey?.key;
        if (key) setNewKey(key);
        toast.success("API key created");
        await qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
      },
      onError: () => toast.error("Failed to create key"),
    },
  });

  const revokeKey = useUserServiceRevokeAPIKey({
    mutation: {
      onSuccess: async () => {
        toast.success("Key revoked");
        await qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
      },
    },
  });

  const handleCreate = useCallback(async () => {
    if (!keyName.trim()) return;
    try {
      await createKey.mutateAsync({ data: { name: keyName.trim() } });
      setKeyName("");
      setShowCreate(false);
    } catch {}
  }, [keyName, createKey]);

  const handleRevoke = useCallback((keyId: string, name: string) => {
    Alert.alert("Revoke Key", `Revoke "${name}"?`, [
      { text: "Cancel", style: "cancel" },
      { text: "Revoke", style: "destructive", onPress: () => revokeKey.mutate({ id: keyId }) },
    ]);
  }, [revokeKey]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: "API Keys",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable onPress={() => setShowCreate(true)} style={{ marginRight: 16 }}>
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
          renderItem={({ item }: { item: any }) => (
            <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <View style={{ flex: 1, gap: 2 }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{item.name ?? "Unnamed"}</Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                  {item.lastUsedAt ? `Last used: ${new Date(item.lastUsedAt).toLocaleDateString()}` : "Never used"}
                </Text>
              </View>
              <Pressable onPress={() => handleRevoke(item.id, item.name)} hitSlop={8}>
                <Ionicons name="trash-outline" size={18} color="#ef4444" />
              </Pressable>
            </View>
          )}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>No API keys</Text>
            </View>
          }
        />
      )}

      {newKey && (
        <Modal visible transparent animationType="fade" onRequestClose={() => setNewKey(null)}>
          <View style={styles.backdrop}>
            <View style={[styles.keyModal, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", marginBottom: 8 }}>New API Key</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>Copy this key now. It won't be shown again.</Text>
              <View style={{ backgroundColor: theme.colors.bg, padding: 12, borderRadius: theme.radii.sm }}>
                <Text style={{ color: theme.colors.text, fontFamily: "monospace", fontSize: 12 }} selectable>{newKey}</Text>
              </View>
              <Pressable onPress={() => setNewKey(null)} style={{ marginTop: 16, alignSelf: "center" }}>
                <Text style={{ color: theme.colors.accent, fontWeight: "600" }}>Done</Text>
              </Pressable>
            </View>
          </View>
        </Modal>
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={() => setShowCreate(false)}>
        <Pressable style={styles.backdrop} onPress={() => setShowCreate(false)} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>Create API Key</Text>
          <TextInput
            placeholder="Key name"
            placeholderTextColor={theme.colors.textMuted}
            value={keyName}
            onChangeText={setKeyName}
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
          />
          <Pressable onPress={handleCreate} disabled={createKey.isPending} style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}>
            <Text style={{ color: "#fff", fontWeight: "600" }}>Create</Text>
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
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  createBtn: { paddingVertical: 14, alignItems: "center" },
});
