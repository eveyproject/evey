import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Alert, Switch, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceListUsers,
  useUserServiceCreateUser,
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  getUserServiceListUsersQueryKey,
} from "@evey/api";
import type { V1User } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function AdminUsersScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newIsAdmin, setNewIsAdmin] = useState(false);

  const { data, isLoading, refetch } = useUserServiceListUsers();
  const users = data?.users ?? [];

  const createUser = useUserServiceCreateUser({
    mutation: {
      onSuccess: async () => {
        toast.success("User created");
        setShowCreate(false);
        setNewUsername("");
        setNewEmail("");
        setNewPassword("");
        setNewIsAdmin(false);
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error("Failed to create user"),
    },
  });

  const deleteUser = useUserServiceDeleteUser({
    mutation: {
      onSuccess: async () => {
        toast.success("User deleted");
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error("Failed to delete user"),
    },
  });

  const disableUser = useUserServiceDisableUser({
    mutation: {
      onSuccess: async () => {
        toast.success("User updated");
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
    },
  });

  const handleCreate = useCallback(async () => {
    if (!newUsername.trim() || !newPassword.trim()) return;
    try {
      await createUser.mutateAsync({
        data: { username: newUsername.trim(), email: newEmail.trim() || undefined, password: newPassword.trim(), isAdmin: newIsAdmin },
      });
    } catch {}
  }, [newUsername, newEmail, newPassword, newIsAdmin, createUser]);

  const handleDelete = useCallback((user: V1User) => {
    Alert.alert(t("common.delete"), `${t("common.delete")} ${user.username}?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.delete"), style: "destructive", onPress: () => user.id && deleteUser.mutate({ id: user.id }) },
    ]);
  }, [deleteUser, t]);

  const handleToggleDisable = useCallback((user: V1User) => {
    if (!user.id) return;
    disableUser.mutate({ id: user.id, params: { disabled: !(user as any).disabled } });
  }, [disableUser]);

  const renderUser = useCallback(({ item }: { item: V1User }) => (
    <View style={[styles.userCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
        <Text style={{ color: "#fff", fontWeight: "700" }}>{item.username?.[0]?.toUpperCase() ?? "?"}</Text>
      </View>
      <View style={{ flex: 1 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.username}</Text>
          {item.isAdmin && (
            <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
              <Text style={{ color: "#fff", fontSize: 9, fontWeight: "700" }}>ADMIN</Text>
            </View>
          )}
          {(item as any).disabled && (
            <View style={{ backgroundColor: theme.colors.textMuted, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
              <Text style={{ color: "#fff", fontSize: 9, fontWeight: "700" }}>DISABLED</Text>
            </View>
          )}
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{item.email || "—"}</Text>
      </View>
      <Pressable onPress={() => handleToggleDisable(item)} hitSlop={8} style={{ padding: 4 }}>
        <Ionicons name={(item as any).disabled ? "eye-off-outline" : "eye-outline"} size={18} color={theme.colors.textMuted} />
      </Pressable>
      <Pressable onPress={() => handleDelete(item)} hitSlop={8} style={{ padding: 4 }}>
        <Ionicons name="trash-outline" size={18} color="#ef4444" />
      </Pressable>
    </View>
  ), [theme, handleDelete, handleToggleDisable]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.users") || "Users",
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
          data={users}
          keyExtractor={(u) => u.id ?? ""}
          renderItem={renderUser}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>No users</Text>
            </View>
          }
        />
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={() => setShowCreate(false)}>
        <Pressable style={styles.backdrop} onPress={() => setShowCreate(false)} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>Create User</Text>
          <TextInput placeholder="Username *" placeholderTextColor={theme.colors.textMuted} value={newUsername} onChangeText={setNewUsername} autoCapitalize="none" style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
          <TextInput placeholder="Email" placeholderTextColor={theme.colors.textMuted} value={newEmail} onChangeText={setNewEmail} keyboardType="email-address" autoCapitalize="none" style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
          <TextInput placeholder="Password *" placeholderTextColor={theme.colors.textMuted} value={newPassword} onChangeText={setNewPassword} secureTextEntry style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
          <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 16 }}>
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>Admin</Text>
            <Switch value={newIsAdmin} onValueChange={setNewIsAdmin} trackColor={{ true: theme.colors.accent }} />
          </View>
          <Pressable onPress={handleCreate} disabled={createUser.isPending} style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createUser.isPending ? 0.5 : 1 }]}>
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>Create</Text>
          </Pressable>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  userCard: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  avatar: { width: 36, height: 36, borderRadius: 18, justifyContent: "center", alignItems: "center" },
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  createBtn: { paddingVertical: 14, alignItems: "center" },
});
