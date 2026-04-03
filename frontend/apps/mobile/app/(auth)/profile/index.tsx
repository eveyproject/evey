import { useState, useCallback } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function ProfileScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const isAdmin = useAuthStore((s) => s.isAdmin);

  const { data: user, isLoading } = useUserServiceGetCurrentUser();
  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: async () => {
        try {
          await qc.invalidateQueries({ queryKey: ["/api/v1/users/me"] });
        } catch {}
        setEditing(false);
        toast.success("Profile updated");
      },
      onError: () => toast.error("Failed to update profile"),
    },
  });

  const startEdit = useCallback(() => {
    setUsername(user?.username ?? "");
    setEmail(user?.email ?? "");
    setEditing(true);
  }, [user]);

  const cancelEdit = useCallback(() => {
    setEditing(false);
  }, []);

  const handleSave = useCallback(async () => {
    if (!user?.id) return;
    try {
      await updateUser.mutateAsync({
        id: user.id,
        data: { username: username.trim(), email: email.trim() },
      });
    } catch {}
  }, [user, username, email, updateUser]);

  const initial = user?.username?.charAt(0).toUpperCase() ?? "U";
  const memberSince = user?.createdAt
    ? new Date(user.createdAt).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })
    : null;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("common.nav.profile"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerRight: () =>
            !editing ? (
              <Pressable onPress={startEdit} style={{ marginRight: 16 }}>
                <Ionicons name="pencil" size={20} color={theme.colors.accent} />
              </Pressable>
            ) : null,
        }}
      />

      {isLoading ? (
        <View style={styles.center}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <View style={{ padding: theme.spacing.lg }}>
          {/* Avatar */}
          <View style={{ alignItems: "center", marginBottom: theme.spacing.lg }}>
            <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
              <Text style={styles.avatarText}>{initial}</Text>
            </View>
            {isAdmin && (
              <View style={[styles.adminBadge, { backgroundColor: theme.colors.accent }]}>
                <Text style={styles.adminText}>Admin</Text>
              </View>
            )}
          </View>

          {editing ? (
            <>
              <Text style={[styles.label, { color: theme.colors.textMuted }]}>
                {t("auth.settings.account.username") || "Username"}
              </Text>
              <TextInput
                style={[styles.input, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderRadius: theme.radii.md }]}
                value={username}
                onChangeText={setUsername}
                autoCapitalize="none"
                autoCorrect={false}
              />

              <Text style={[styles.label, { color: theme.colors.textMuted, marginTop: theme.spacing.md }]}>
                {t("auth.settings.account.email") || "Email"}
              </Text>
              <TextInput
                style={[styles.input, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderRadius: theme.radii.md }]}
                value={email}
                onChangeText={setEmail}
                keyboardType="email-address"
                autoCapitalize="none"
                autoCorrect={false}
              />

              <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.lg }}>
                <Pressable
                  onPress={handleSave}
                  disabled={updateUser.isPending}
                  style={[styles.saveBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, flex: 1, opacity: updateUser.isPending ? 0.5 : 1 }]}
                >
                  {updateUser.isPending ? (
                    <ActivityIndicator size={16} color="#fff" />
                  ) : (
                    <Text style={styles.saveBtnText}>{t("common.save") || "Save"}</Text>
                  )}
                </Pressable>
                <Pressable
                  onPress={cancelEdit}
                  style={[styles.cancelBtn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, flex: 1 }]}
                >
                  <Text style={[styles.cancelBtnText, { color: theme.colors.text }]}>{t("common.cancel") || "Cancel"}</Text>
                </Pressable>
              </View>
            </>
          ) : (
            <>
              <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                <InfoRow label={t("auth.settings.account.username") || "Username"} value={user?.username} theme={theme} />
                <View style={[styles.divider, { backgroundColor: theme.colors.border }]} />
                <InfoRow label={t("auth.settings.account.email") || "Email"} value={user?.email || "—"} theme={theme} />
                {memberSince && (
                  <>
                    <View style={[styles.divider, { backgroundColor: theme.colors.border }]} />
                    <InfoRow label={t("auth.settings.account.memberSince") || "Member since"} value={memberSince} theme={theme} />
                  </>
                )}
              </View>
            </>
          )}
        </View>
      )}
    </View>
  );
}

function InfoRow({ label, value, theme }: { label: string; value?: string; theme: ReturnType<typeof useTheme> }) {
  return (
    <View style={styles.infoRow}>
      <Text style={[styles.infoLabel, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.infoValue, { color: theme.colors.text }]}>{value ?? "—"}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  avatar: { width: 80, height: 80, borderRadius: 40, justifyContent: "center", alignItems: "center" },
  avatarText: { color: "#fff", fontSize: 32, fontWeight: "700" },
  adminBadge: { marginTop: 8, paddingHorizontal: 10, paddingVertical: 3, borderRadius: 12 },
  adminText: { color: "#fff", fontSize: 11, fontWeight: "700" },
  label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5, marginBottom: 6 },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 16 },
  saveBtn: { paddingVertical: 12, alignItems: "center" },
  saveBtnText: { color: "#fff", fontSize: 16, fontWeight: "600" },
  cancelBtn: { paddingVertical: 12, alignItems: "center" },
  cancelBtnText: { fontSize: 16, fontWeight: "500" },
  infoCard: { padding: 16 },
  infoRow: { paddingVertical: 12 },
  infoLabel: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5, marginBottom: 4 },
  infoValue: { fontSize: 16 },
  divider: { height: StyleSheet.hairlineWidth },
});
