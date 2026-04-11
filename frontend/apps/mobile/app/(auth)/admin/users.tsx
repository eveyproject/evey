import { useState, useCallback, useEffect } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Alert, Switch, ActivityIndicator, StyleSheet, ScrollView } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import {
  useUserServiceListUsers,
  useUserServiceCreateUser,
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  getUserServiceListUsersQueryKey,
  useLibraryServiceListLibraries,
  eveyFetch,
} from "@evey/api";
import type { V1User } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface LibraryInfo {
  id: string;
  name: string;
}

interface LibraryAccessData {
  libraryIds: string[];
  libraries: LibraryInfo[];
}

function LibraryAccessModal({ user, visible, onClose, theme }: { user: V1User; visible: boolean; onClose: () => void; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);

  const { data, isLoading } = useQuery<LibraryAccessData>({
    queryKey: ["/api/v1/admin/users", user.id, "library-access"],
    queryFn: () => eveyFetch<LibraryAccessData>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "GET" }),
    enabled: visible,
  });

  useEffect(() => {
    if (data) setSelected(new Set(data.libraryIds));
  }, [data]);

  const handleToggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      if (selected.size === 0) {
        await eveyFetch<void>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "DELETE" });
      } else {
        await eveyFetch<void>({
          url: `/api/v1/admin/users/${user.id}/library-access`,
          method: "PUT",
          data: { libraryIds: Array.from(selected) },
        });
      }
      toast.success(t("admin.users.libraryAccessUpdated"));
      onClose();
    } catch {
      toast.error(t("admin.users.failedLibraryAccess"));
    } finally {
      setSaving(false);
    }
  }, [selected, user.id, onClose, t]);

  const libraries = data?.libraries ?? [];

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 8 }}>
          {t("admin.users.libraryAccessTitle", { name: user.username })}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: 12 }}>
          {selected.size === 0
            ? t("admin.users.unrestrictedDesc")
            : t("admin.users.restrictedDesc", { count: selected.size, libraryWord: selected.size === 1 ? "library" : "libraries" })}
        </Text>
        {isLoading ? (
          <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 24 }} />
        ) : (
          <ScrollView style={{ maxHeight: 300 }}>
            {libraries.map((lib) => (
              <Pressable
                key={lib.id}
                onPress={() => handleToggle(lib.id)}
                style={[styles.libRow, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
              >
                <Ionicons
                  name={selected.has(lib.id) ? "checkbox" : "square-outline"}
                  size={22}
                  color={selected.has(lib.id) ? theme.colors.accent : theme.colors.textMuted}
                />
                <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{lib.name}</Text>
              </Pressable>
            ))}
          </ScrollView>
        )}
        {selected.size > 0 && (
          <Pressable onPress={() => setSelected(new Set())} style={{ paddingVertical: 8 }}>
            <Text style={{ color: theme.colors.accent, fontSize: 13, textAlign: "center" }}>
              {t("admin.users.clearAllUnrestricted")}
            </Text>
          </Pressable>
        )}
        {selected.size === 0 && !isLoading && (
          <View style={{ backgroundColor: theme.colors.accent + "22", borderRadius: theme.radii.md, paddingHorizontal: 10, paddingVertical: 4, alignSelf: "flex-start", marginBottom: 8 }}>
            <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>{t("admin.users.unrestricted")}</Text>
          </View>
        )}
        <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
          <Pressable onPress={onClose} style={[styles.createBtn, { flex: 1, backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}>
            <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 15 }}>{t("common.cancel")}</Text>
          </Pressable>
          <Pressable onPress={handleSave} disabled={saving} style={[styles.createBtn, { flex: 1, backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: saving ? 0.5 : 1 }]}>
            {saving ? <ActivityIndicator color="#fff" size="small" /> : <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>}
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

export default function AdminUsersScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newIsAdmin, setNewIsAdmin] = useState(false);
  const [newSelectedLibs, setNewSelectedLibs] = useState<Set<string>>(new Set());
  const [libAccessUser, setLibAccessUser] = useState<V1User | null>(null);

  const { data, isLoading, refetch } = useUserServiceListUsers();
  const users = data?.users ?? [];

  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];

  const handleToggleNewLib = useCallback((id: string) => {
    setNewSelectedLibs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  const createUser = useUserServiceCreateUser({
    mutation: {
      onSuccess: async (data) => {
        const userId = data?.id;
        if (userId && !newIsAdmin && newSelectedLibs.size > 0) {
          try {
            await eveyFetch<void>({
              url: `/api/v1/admin/users/${userId}/library-access`,
              method: "PUT",
              data: { libraryIds: Array.from(newSelectedLibs) },
            });
          } catch {
            toast.error(t("admin.users.failedLibraryAccess"));
          }
        }
        toast.success(t("admin.users.userCreated"));
        setShowCreate(false);
        setNewUsername("");
        setNewEmail("");
        setNewPassword("");
        setNewIsAdmin(false);
        setNewSelectedLibs(new Set());
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedCreateUser")),
    },
  });

  const deleteUser = useUserServiceDeleteUser({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.users.userDeleted", { name: "" }));
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedDeleteUser")),
    },
  });

  const disableUser = useUserServiceDisableUser({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.users.userUpdated"));
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
  }, [newUsername, newEmail, newPassword, newIsAdmin, newSelectedLibs, createUser]);

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
      {!item.isAdmin && (
        <Pressable onPress={() => setLibAccessUser(item)} hitSlop={8} style={{ padding: 4 }}>
          <Ionicons name="library-outline" size={18} color={theme.colors.textMuted} />
        </Pressable>
      )}
      <Pressable onPress={() => handleToggleDisable(item)} hitSlop={8} style={{ padding: 4 }}>
        <Ionicons name={(item as any).disabled ? "eye-off-outline" : "eye-outline"} size={18} color={theme.colors.textMuted} />
      </Pressable>
      <Pressable onPress={() => handleDelete(item)} hitSlop={8} style={{ padding: 4 }}>
        <Ionicons name="trash-outline" size={18} color=theme.colors.error />
      </Pressable>
    </View>
  ), [theme, handleDelete, handleToggleDisable, setLibAccessUser]);

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
              <Text style={{ color: theme.colors.textMuted }}>{t("admin.users.noUsers")}</Text>
            </View>
          }
        />
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={() => setShowCreate(false)}>
        <Pressable style={styles.backdrop} onPress={() => setShowCreate(false)} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>{t("admin.users.createUser")}</Text>
          <ScrollView style={{ maxHeight: 420 }} showsVerticalScrollIndicator={false}>
            <TextInput placeholder={`${t("admin.users.username")} *`} placeholderTextColor={theme.colors.textMuted} value={newUsername} onChangeText={setNewUsername} autoCapitalize="none" style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
            <TextInput placeholder={t("admin.users.email")} placeholderTextColor={theme.colors.textMuted} value={newEmail} onChangeText={setNewEmail} keyboardType="email-address" autoCapitalize="none" style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
            <TextInput placeholder={`${t("admin.users.password")} *`} placeholderTextColor={theme.colors.textMuted} value={newPassword} onChangeText={setNewPassword} secureTextEntry style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]} />
            <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 16 }}>
              <Text style={{ color: theme.colors.text, fontSize: 15 }}>{t("admin.users.admin")}</Text>
              <Switch value={newIsAdmin} onValueChange={setNewIsAdmin} trackColor={{ true: theme.colors.accent }} />
            </View>
            {!newIsAdmin && libraries.length > 0 && (
              <View style={{ marginBottom: 16 }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600", marginBottom: 4 }}>
                  {t("admin.users.libraryAccessTitle", { name: "" }).replace(/\s*$/, "")}
                </Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 8 }}>
                  {newSelectedLibs.size === 0
                    ? t("admin.users.unrestrictedDesc")
                    : t("admin.users.restrictedDesc", { count: newSelectedLibs.size, libraryWord: newSelectedLibs.size === 1 ? "library" : "libraries" })}
                </Text>
                {newSelectedLibs.size === 0 && (
                  <View style={{ backgroundColor: theme.colors.accent + "22", borderRadius: theme.radii.md, paddingHorizontal: 10, paddingVertical: 4, alignSelf: "flex-start", marginBottom: 8 }}>
                    <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>{t("admin.users.unrestricted")}</Text>
                  </View>
                )}
                {libraries.map((lib) => (
                  <Pressable
                    key={lib.id}
                    onPress={() => handleToggleNewLib(lib.id!)}
                    style={[styles.libRow, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
                  >
                    <Ionicons
                      name={newSelectedLibs.has(lib.id!) ? "checkbox" : "square-outline"}
                      size={22}
                      color={newSelectedLibs.has(lib.id!) ? theme.colors.accent : theme.colors.textMuted}
                    />
                    <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{lib.name ?? lib.id}</Text>
                  </Pressable>
                ))}
                {newSelectedLibs.size > 0 && (
                  <Pressable onPress={() => setNewSelectedLibs(new Set())} style={{ paddingVertical: 6 }}>
                    <Text style={{ color: theme.colors.accent, fontSize: 13, textAlign: "center" }}>
                      {t("admin.users.clearAllUnrestricted")}
                    </Text>
                  </Pressable>
                )}
              </View>
            )}
          </ScrollView>
          <Pressable onPress={handleCreate} disabled={createUser.isPending} style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createUser.isPending ? 0.5 : 1 }]}>
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>{t("common.create")}</Text>
          </Pressable>
        </View>
      </Modal>

      {libAccessUser && !libAccessUser.isAdmin && (
        <LibraryAccessModal
          user={libAccessUser}
          visible={!!libAccessUser}
          onClose={() => setLibAccessUser(null)}
          theme={theme}
        />
      )}
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
  libRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 12, paddingVertical: 10, marginBottom: 6 },
});
