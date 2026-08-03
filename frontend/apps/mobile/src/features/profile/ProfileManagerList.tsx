import { useState } from "react";
import { View, Text, Pressable, Modal, StyleSheet, ActivityIndicator } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceDeleteProfile,
  useUserServiceSetDefaultProfile,
  getUserServiceListProfilesQueryKey,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { EditProfileDialog } from "./EditProfileDialog";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface Props {
  userId: string;
  profiles: V1Profile[];
  /**
   * Reveals the "Set as default" affordance on non-default profiles. The
   * underlying RPC is admin-only on the server, so this stays gated to the
   * admin → user-edit flow rather than the regular user settings page.
   */
  canSetDefault?: boolean;
}

export function ProfileManagerList({ userId, profiles, canSetDefault = false }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const [editing, setEditing] = useState<V1Profile | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<V1Profile | null>(null);

  const hasKids = profiles.some((p) => p.isKids === true);

  const deleteProfile = useUserServiceDeleteProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.deleted", "Profile deleted"));
        setConfirmDelete(null);
      },
      onError: (err: Error) =>
        toast.error(err.message || t("auth.profile.failedToUpdate", "Failed to update profile")),
    },
  });

  const setDefault = useUserServiceSetDefaultProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.manage.defaultUpdated", "Default profile updated"));
      },
      onError: (err: Error) =>
        toast.error(err.message || t("auth.profile.failedToUpdate", "Failed to update profile")),
    },
  });

  const s = styles(theme);

  return (
    <>
      <View style={{ gap: theme.spacing.sm }}>
        {profiles.map((p) => {
          const initial = (p.name ?? "?").charAt(0).toUpperCase();
          const avatarUrl = p.avatarPath && p.id && serverUrl && accessToken
            ? `${serverUrl}/api/v1/profiles/${p.id}/avatar?token=${accessToken}`
            : null;
          const isDefault = p.isDefault === true;
          const setDefPending = setDefault.isPending && setDefault.variables?.id === p.id;
          return (
            <View
              key={p.id}
              style={[
                s.card,
                { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.md },
              ]}
            >
              <View style={[s.avatar, { backgroundColor: theme.colors.accent }]}>
                {avatarUrl ? (
                  <Image source={{ uri: avatarUrl }} style={{ width: 40, height: 40, borderRadius: 20 }} contentFit="cover" />
                ) : (
                  <Text style={[s.avatarText, { color: theme.colors.onAccent }]}>{initial}</Text>
                )}
              </View>
              <View style={{ flex: 1, gap: 4 }}>
                <Text style={[s.name, { color: theme.colors.text }]} numberOfLines={1}>
                  {p.name}
                </Text>
                <View style={s.badges}>
                  {isDefault && (
                    <View style={[s.badge, { backgroundColor: theme.colors.info }]}>
                      <Text style={[s.badgeText, { color: theme.colors.onAccent }]}>
                        {t("auth.profile.manage.defaultProfileBadge", "Default")}
                      </Text>
                    </View>
                  )}
                  {p.isKids && (
                    <View style={[s.badge, { backgroundColor: theme.colors.success }]}>
                      <Text style={[s.badgeText, { color: theme.colors.onAccent }]}>
                        {t("auth.profile.manage.kidsBadge", "Kids")}
                      </Text>
                    </View>
                  )}
                </View>
              </View>

              {canSetDefault && !isDefault && (
                <Pressable
                  onPress={() => p.id && setDefault.mutate({ id: p.id, data: {} })}
                  disabled={setDefPending}
                  style={s.iconBtn}
                  accessibilityRole="button"
                  accessibilityLabel={t("auth.profile.manage.setDefault", "Set as default")}
                >
                  {setDefPending ? (
                    <ActivityIndicator color={theme.colors.accent} size="small" />
                  ) : (
                    <Ionicons name="star-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                  )}
                </Pressable>
              )}

              <Pressable
                onPress={() => setEditing(p)}
                style={s.iconBtn}
                accessibilityRole="button"
                accessibilityLabel={t("common.edit", "Edit")}
              >
                <Ionicons name="pencil-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
              </Pressable>

              <Pressable
                onPress={() => !isDefault && setConfirmDelete(p)}
                disabled={isDefault}
                style={[s.iconBtn, { opacity: isDefault ? 0.4 : 1 }]}
                accessibilityRole="button"
                accessibilityLabel={t("common.delete", "Delete")}
              >
                <Ionicons name="trash-outline" size={ICON_SIZE.lg} color={theme.colors.error} />
              </Pressable>
            </View>
          );
        })}
      </View>

      {editing && (
        <EditProfileDialog
          open={!!editing}
          profile={editing}
          hasKidsProfile={hasKids}
          userId={userId}
          onClose={() => setEditing(null)}
        />
      )}

      <Modal
        visible={!!confirmDelete}
        transparent
        animationType="fade"
        onRequestClose={() => setConfirmDelete(null)}
      >
        <View style={[s.backdrop, { backgroundColor: theme.colors.overlay }]}>
          <View style={[s.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
            <Text style={[s.title, { color: theme.colors.text }]}>
              {t("common.delete", "Delete")}
            </Text>
            <Text style={[s.desc, { color: theme.colors.textMuted }]}>
              {t("auth.profile.manage.deleteConfirm", "Delete profile \"{{name}}\"? This cannot be undone.", { name: confirmDelete?.name ?? "" })}
            </Text>
            <View style={s.buttons}>
              <Pressable
                onPress={() => setConfirmDelete(null)}
                style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
                accessibilityRole="button"
                accessibilityLabel={t("common.cancel", "Cancel")}
              >
                <Text style={[s.btnText, { color: theme.colors.text }]}>{t("common.cancel", "Cancel")}</Text>
              </Pressable>
              <Pressable
                onPress={() => confirmDelete?.id && deleteProfile.mutate({ id: confirmDelete.id })}
                disabled={deleteProfile.isPending}
                style={[s.btn, { backgroundColor: theme.colors.danger, borderRadius: theme.radii.md, opacity: deleteProfile.isPending ? 0.6 : 1 }]}
                accessibilityRole="button"
                accessibilityLabel={t("common.delete", "Delete")}
              >
                {deleteProfile.isPending ? (
                  <ActivityIndicator color={theme.colors.onDanger} size="small" />
                ) : (
                  <Text style={[s.btnText, { color: theme.colors.onDanger }]}>{t("common.delete", "Delete")}</Text>
                )}
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    card: {
      flexDirection: "row",
      alignItems: "center",
      gap: t.spacing.sm + 4,
      padding: t.spacing.sm + 4,
      borderWidth: 1,
    },
    avatar: {
      width: 40,
      height: 40,
      borderRadius: 20,
      alignItems: "center",
      justifyContent: "center",
      overflow: "hidden",
    },
    avatarText: {
      fontSize: FONT_SIZE.base,
      fontWeight: "700",
    },
    name: {
      fontSize: FONT_SIZE.sm,
      fontWeight: "500",
    },
    badges: {
      flexDirection: "row",
      gap: 6,
      flexWrap: "wrap",
    },
    badge: {
      paddingHorizontal: 8,
      paddingVertical: 2,
      borderRadius: 4,
    },
    badgeText: {
      fontSize: FONT_SIZE.xs,
      fontWeight: "700",
      textTransform: "uppercase",
    },
    iconBtn: {
      padding: 8,
    },
    backdrop: {
      flex: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.lg,
    },
    dialog: {
      width: "100%",
      maxWidth: 360,
      padding: t.spacing.lg,
    },
    title: {
      fontSize: FONT_SIZE.lg,
      fontWeight: "700",
      marginBottom: t.spacing.sm,
    },
    desc: {
      fontSize: FONT_SIZE.sm,
      marginBottom: t.spacing.lg,
    },
    buttons: {
      flexDirection: "row",
      gap: t.spacing.sm,
    },
    btn: {
      flex: 1,
      paddingVertical: 12,
      alignItems: "center",
      justifyContent: "center",
    },
    btnText: {
      fontWeight: "600",
      fontSize: FONT_SIZE.sm,
    },
  });
