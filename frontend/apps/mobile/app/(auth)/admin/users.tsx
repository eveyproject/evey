import { useState, useCallback, useEffect, useMemo } from "react";
import { parseInviteProfileCap } from "@evey/client";
import { View, Text, FlatList, Pressable, TextInput, Modal, Alert, Switch, ActivityIndicator, StyleSheet, ScrollView, Share } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import {
  useUserServiceListUsers,
  useUserServiceListAllProfiles,
  useUserServiceCreateUser,
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  useUserServiceEnableUser,
  useUserServiceImpersonateUser,
  getUserServiceListUsersQueryKey,
  useLibraryServiceListLibraries,
  useUserServiceListInviteLinks,
  useUserServiceCreateInviteLink,
  useUserServiceRevokeInviteLink,
  getUserServiceListInviteLinksQueryKey,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import type { V1User, V1InviteLink, V1Profile } from "@evey/api";
import { onColor } from "@evey/theme";
import { toast, useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { BitrateModal } from "./users/BitrateModal";

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
      <Pressable
        style={styles.backdrop}
        onPress={onClose}
        accessibilityElementsHidden={true}
        importantForAccessibility="no-hide-descendants"
      />
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
                accessible={true}
                accessibilityLabel={lib.name}
                accessibilityRole="checkbox"
                accessibilityState={{ checked: selected.has(lib.id) }}
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
          <Pressable
            onPress={() => setSelected(new Set())}
            style={{ paddingVertical: 8 }}
            accessible={true}
            accessibilityLabel={t("admin.users.clearAllUnrestricted")}
            accessibilityRole="button"
          >
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
          <Pressable
            onPress={onClose}
            style={[styles.createBtn, { flex: 1, backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
            accessible={true}
            accessibilityLabel={t("common.cancel")}
            accessibilityRole="button"
          >
            <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 15 }}>{t("common.cancel")}</Text>
          </Pressable>
          <Pressable
            onPress={handleSave}
            disabled={saving}
            style={[styles.createBtn, { flex: 1, backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: saving ? 0.5 : 1 }]}
            accessible={true}
            accessibilityLabel={t("common.save")}
            accessibilityRole="button"
          >
            {saving ? <ActivityIndicator color={theme.colors.onAccent} size="small" /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>}
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

interface UserAction {
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  destructive?: boolean;
  onPress: () => void;
}

// Small colored tag for a profile trait (Default / Kids / rating cap) - the
// same shape as the Admin/Disabled tags already drawn inline above, just
// reused for the profile list instead of duplicated per badge.
function ProfileBadge({ label, bg, fg }: { label: string; bg: string; fg: string }) {
  return (
    <View style={{ backgroundColor: bg, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
      <Text style={{ color: fg, fontSize: 9, fontWeight: "700" }}>{label.toUpperCase()}</Text>
    </View>
  );
}

// One labelled row per action, replacing the six 18px icon buttons that used
// to share each list row: full-width touch targets and explicit wording so
// destructive taps can't happen by fat-fingering the neighbour icon.
//
// The profiles list mirrors the web admin panel's expanded row (avatar,
// Default/Kids/rating badges, last active, history link) - the mobile screen
// has nowhere to expand a row into, so it lives here instead, right above the
// per-profile "Connect as" entries already in `actions`.
function UserActionsSheet({ user, actions, profiles, onClose, theme }: { user: V1User; actions: UserAction[]; profiles: V1Profile[]; onClose: () => void; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const accessToken = useAuthStore((s) => s.accessToken);
  return (
    <Modal visible transparent animationType="slide" onRequestClose={onClose}>
      <Pressable
        style={styles.backdrop}
        onPress={onClose}
        accessibilityElementsHidden={true}
        importantForAccessibility="no-hide-descendants"
      />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 12, marginBottom: 4 }}>
          <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
            <Text style={{ color: theme.colors.onAccent, fontWeight: "700" }}>{user.username?.[0]?.toUpperCase() ?? "?"}</Text>
          </View>
          <View style={{ flex: 1 }}>
            <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700" }} numberOfLines={1}>{user.username}</Text>
              {user.isAdmin && (
                <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
                  <Text style={{ color: theme.colors.onAccent, fontSize: 9, fontWeight: "700" }}>{t("admin.users.admin", "Admin").toUpperCase()}</Text>
                </View>
              )}
              {user.enabled === false && (
                <View style={{ backgroundColor: theme.colors.textMuted, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
                  <Text style={{ color: onColor(theme.colors.textMuted), fontSize: 9, fontWeight: "700" }}>{t("common.disabled", "Disabled").toUpperCase()}</Text>
                </View>
              )}
            </View>
            {!!user.email && (
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>{user.email}</Text>
            )}
          </View>
        </View>
        {profiles.length > 0 && (
          <View style={{ gap: 10, marginTop: 8, marginBottom: 4 }}>
            {profiles.map((p) => {
              const avatarUrl = p.avatarPath && p.id && serverUrl && accessToken
                ? `${serverUrl}/api/v1/profiles/${p.id}/avatar?token=${accessToken}`
                : null;
              return (
                <View key={p.id} style={{ flexDirection: "row", alignItems: "center", gap: 10 }}>
                  <View style={[styles.profileAvatar, { backgroundColor: theme.colors.surfaceRaised }]}>
                    {avatarUrl ? (
                      <Image source={{ uri: avatarUrl }} style={{ width: 28, height: 28, borderRadius: 14 }} />
                    ) : (
                      <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "700" }}>
                        {(p.name ?? "?").charAt(0).toUpperCase()}
                      </Text>
                    )}
                  </View>
                  <View style={{ flex: 1 }}>
                    <View style={{ flexDirection: "row", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
                      <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }} numberOfLines={1}>{p.name}</Text>
                      {p.isDefault && <ProfileBadge label={t("admin.users.defaultProfile", "Default")} bg={theme.colors.accent} fg={theme.colors.onAccent} />}
                      {p.isKids && <ProfileBadge label={t("admin.users.kidsProfile", "Kids")} bg={theme.colors.warning} fg={theme.colors.onWarning} />}
                      {!!p.maxRating && <ProfileBadge label={p.maxRating} bg={theme.colors.surfaceRaised} fg={theme.colors.text} />}
                    </View>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2 }}>
                      {t("admin.users.lastActive", "Last active")}: {p.lastActiveAt ? new Date(p.lastActiveAt).toLocaleString() : t("admin.users.never", "Never")}
                    </Text>
                  </View>
                  <Pressable
                    onPress={() => {
                      onClose();
                      // Pre-filtered on THIS profile, like the web's
                      // /admin/activity?profileId= - the screen reads both
                      // params as its initial watcher.
                      router.push({ pathname: "/admin/activity", params: { profileId: p.id } } as never);
                    }}
                    hitSlop={8}
                    style={{ padding: 6 }}
                    accessible={true}
                    accessibilityLabel={t("admin.users.viewHistory", "History")}
                    accessibilityRole="button"
                  >
                    <Ionicons name="time-outline" size={18} color={theme.colors.accent} />
                  </Pressable>
                </View>
              );
            })}
          </View>
        )}
        {actions.map((a) => (
          <Pressable
            key={a.label}
            onPress={a.onPress}
            style={[styles.actionRow, { borderRadius: theme.radii.md }]}
            accessible={true}
            accessibilityLabel={a.label}
            accessibilityRole="button"
          >
            <Ionicons name={a.icon} size={20} color={a.destructive ? theme.colors.error : theme.colors.text} />
            <Text style={{ color: a.destructive ? theme.colors.error : theme.colors.text, fontSize: 15, fontWeight: "500" }}>
              {a.label}
            </Text>
          </Pressable>
        ))}
      </View>
    </Modal>
  );
}

function InviteLinksSection({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const serverUrl = useServerUrl();
  const [showCreate, setShowCreate] = useState(false);
  const [maxUses, setMaxUses] = useState("");
  const [inviteMaxProfiles, setInviteMaxProfiles] = useState("");
  // Invite scoping (mirror web CreateInviteForm): expiry preset, admin flag,
  // and per-library access. Mobile uses expiry-day presets rather than a
  // datetime picker - same resulting expiresAt on the wire.
  const [expiryDays, setExpiryDays] = useState(0); // 0 = never
  const [inviteIsAdmin, setInviteIsAdmin] = useState(false);
  const [inviteLibs, setInviteLibs] = useState<Set<string>>(new Set());
  const { data: invLibData } = useLibraryServiceListLibraries();
  const invLibraries = invLibData?.libraries ?? [];

  const { data, isLoading, isError, refetch } = useUserServiceListInviteLinks();
  const invites = data?.inviteLinks ?? [];

  const createInvite = useUserServiceCreateInviteLink({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.users.inviteCreated", "Invite link created"));
        setShowCreate(false);
        setMaxUses("");
        setInviteMaxProfiles("");
        setExpiryDays(0);
        setInviteIsAdmin(false);
        setInviteLibs(new Set());
        await qc.invalidateQueries({ queryKey: getUserServiceListInviteLinksQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedCreateInvite", "Failed to create invite")),
    },
  });

  const revokeInvite = useUserServiceRevokeInviteLink({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.users.inviteRevoked", "Invite link revoked"));
        await qc.invalidateQueries({ queryKey: getUserServiceListInviteLinksQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedRevoke", "Failed to revoke invite")),
    },
  });

  const handleCreate = useCallback(async () => {
    const expiresAt = expiryDays > 0 ? new Date(Date.now() + expiryDays * 86_400_000).toISOString() : undefined;
    try {
      await createInvite.mutateAsync({
        data: {
          maxUses: maxUses ? parseInt(maxUses) : undefined,
          maxProfiles: parseInviteProfileCap(inviteMaxProfiles),
          expiresAt,
          isAdmin: inviteIsAdmin,
          libraryIds: inviteIsAdmin ? [] : Array.from(inviteLibs),
        },
      });
    } catch {}
  }, [maxUses, inviteMaxProfiles, expiryDays, inviteIsAdmin, inviteLibs, createInvite]);

  const handleShare = useCallback(async (link: V1InviteLink) => {
    const url = `${serverUrl}/register?invite=${link.code}`;
    try {
      await Share.share({ message: url });
    } catch {}
  }, [serverUrl]);

  const handleRevoke = useCallback((link: V1InviteLink) => {
    Alert.alert(
      t("admin.users.revokeInviteTitle", "Revoke Invite"),
      t("admin.users.revokeInviteConfirm", "Revoke this invite link? It will no longer be usable."),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        { text: t("admin.users.revoke", "Revoke"), style: "destructive", onPress: () => link.id && revokeInvite.mutate({ id: link.id }) },
      ],
    );
  }, [revokeInvite, t]);

  return (
    <View style={{ marginTop: 24 }}>
      <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 12, paddingHorizontal: 16 }}>
        <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700" }}>
          {t("admin.users.inviteLinks", "Invite Links")}
        </Text>
        <Pressable
          onPress={() => setShowCreate(true)}
          hitSlop={8}
          accessible={true}
          accessibilityLabel={t("admin.users.createInviteLink", "Create Invite Link")}
          accessibilityRole="button"
        >
          <Ionicons name="add-circle-outline" size={24} color={theme.colors.accent} />
        </Pressable>
      </View>

      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} />
      ) : isError ? (
        <LoadFailed inline onRetry={() => void refetch()} />
      ) : invites.length === 0 ? (
        <View style={{ alignItems: "center", paddingVertical: 24 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>
            {t("admin.users.noInvites", "No invite links")}
          </Text>
        </View>
      ) : (
        invites.map((link) => (
          <View key={link.id} style={[styles.userCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16, marginBottom: 8 }]}>
            <View style={{ flex: 1 }}>
              <Text style={{ color: theme.colors.text, fontSize: 14, fontFamily: "monospace" }} numberOfLines={1}>
                {link.code ?? "---"}
              </Text>
              <View style={{ flexDirection: "row", gap: 12, marginTop: 4 }}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                  {(link.timesUsed ?? 0)} / {link.maxUses ?? "\u221E"}
                </Text>
                {link.maxProfiles != null && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                    {t("admin.users.inviteMaxProfiles", { max: link.maxProfiles })}
                  </Text>
                )}
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                  {link.expiresAt ? new Date(link.expiresAt).toLocaleDateString() : t("admin.users.never", "Never")}
                </Text>
              </View>
            </View>
            <Pressable
              onPress={() => handleShare(link)}
              hitSlop={8}
              style={{ padding: 4 }}
              accessible={true}
              accessibilityLabel={t("admin.users.shareInviteNamed", "Share invite {{code}}", { code: link.code })}
              accessibilityRole="button"
            >
              <Ionicons name="share-outline" size={18} color={theme.colors.accent} />
            </Pressable>
            <Pressable
              onPress={() => handleRevoke(link)}
              hitSlop={8}
              style={{ padding: 4 }}
              accessible={true}
              accessibilityLabel={t("admin.users.revokeInviteNamed", "Revoke invite {{code}}", { code: link.code })}
              accessibilityRole="button"
            >
              <Ionicons name="trash-outline" size={18} color={theme.colors.danger} />
            </Pressable>
          </View>
        ))
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={() => setShowCreate(false)}>
        <Pressable
          style={styles.backdrop}
          onPress={() => setShowCreate(false)}
          accessibilityElementsHidden={true}
          importantForAccessibility="no-hide-descendants"
        />
        <ScrollView
          style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg, maxHeight: "85%" }]}
          contentContainerStyle={{ paddingBottom: 8 }}
          keyboardShouldPersistTaps="handled"
        >
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 16 }}>
            {t("admin.users.createInviteLink", "Create Invite Link")}
          </Text>
          <TextInput
            placeholder={t("admin.users.maxUsesLabel", "Max uses (empty = unlimited)")}
            placeholderTextColor={theme.colors.textMuted}
            value={maxUses}
            onChangeText={setMaxUses}
            keyboardType="number-pad"
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
            accessible={true}
            accessibilityLabel={t("admin.users.maxUsesLabel", "Max uses (empty = unlimited)")}
          />
          <TextInput
            placeholder={t("admin.users.inviteMaxProfilesLabel")}
            placeholderTextColor={theme.colors.textMuted}
            value={inviteMaxProfiles}
            onChangeText={setInviteMaxProfiles}
            keyboardType="number-pad"
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md, marginTop: 8 }]}
            accessible={true}
            accessibilityLabel={t("admin.users.inviteMaxProfilesLabel")}
            testID="invite-max-profiles"
          />

          {/* Expiry preset */}
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 12, marginBottom: 6 }}>
            {t("admin.users.expiresAtLabel", "Expiry")}
          </Text>
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
            {[
              { d: 0, label: t("admin.users.never", "Never") },
              { d: 1, label: t("admin.users.expiry1d", "1 day") },
              { d: 7, label: t("admin.users.expiry7d", "7 days") },
              { d: 30, label: t("admin.users.expiry30d", "30 days") },
            ].map((opt) => {
              const active = expiryDays === opt.d;
              return (
                <Pressable
                  key={opt.d}
                  onPress={() => setExpiryDays(opt.d)}
                  style={{ paddingHorizontal: 12, paddingVertical: 6, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}
                >
                  <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>{opt.label}</Text>
                </Pressable>
              );
            })}
          </View>

          {/* Admin invite */}
          <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginTop: 16 }}>
            <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
              {t("admin.users.inviteIsAdmin", "Invitee is an administrator")}
            </Text>
            <Switch
              value={inviteIsAdmin}
              onValueChange={setInviteIsAdmin}
              trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
            />
          </View>

          {/* Library scoping (only when not admin) */}
          {!inviteIsAdmin && invLibraries.length > 0 && (
            <View style={{ marginTop: 14 }}>
              <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
                {t("admin.users.inviteLibraries", "Library access")}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2, marginBottom: 8 }}>
                {t("admin.users.inviteLibrariesHelp", "Empty = access to all libraries.")}
              </Text>
              <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
                {invLibraries.map((lib) => {
                  const id = lib.id ?? "";
                  const active = inviteLibs.has(id);
                  return (
                    <Pressable
                      key={id}
                      onPress={() => setInviteLibs((prev) => { const n = new Set(prev); if (n.has(id)) n.delete(id); else n.add(id); return n; })}
                      style={{ paddingHorizontal: 12, paddingVertical: 6, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}
                    >
                      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{lib.name ?? id}</Text>
                    </Pressable>
                  );
                })}
              </View>
            </View>
          )}

          <Pressable
            onPress={handleCreate}
            disabled={createInvite.isPending}
            style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createInvite.isPending ? 0.5 : 1 }]}
            accessible={true}
            accessibilityLabel={t("common.create", "Create")}
            accessibilityRole="button"
          >
            {createInvite.isPending ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>
                {t("common.create", "Create")}
              </Text>
            )}
          </Pressable>
        </ScrollView>
      </Modal>
    </View>
  );
}

export default function AdminUsersScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const router = useRouter();
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newIsAdmin, setNewIsAdmin] = useState(false);
  const [newMaxProfiles, setNewMaxProfiles] = useState("0");
  const [newMaxDevicesPerProfile, setNewMaxDevicesPerProfile] = useState("0");
  const [newSelectedLibs, setNewSelectedLibs] = useState<Set<string>>(new Set());
  const [libAccessUser, setLibAccessUser] = useState<V1User | null>(null);
  const [bitrateUser, setBitrateUser] = useState<V1User | null>(null);
  const [actionUser, setActionUser] = useState<V1User | null>(null);
  const [query, setQuery] = useState("");

  const doImpersonate = useAuthStore((s) => s.impersonate);
  const impersonateUser = useUserServiceImpersonateUser({
    mutation: {
      onSuccess: async (data) => {
        if (!data.accessToken) return;
        // Stash the admin token, swap to the target's, land on the home tabs.
        // Clear the query cache so admin-session data (which sees every library
        // unfiltered) is not served stale under the target's identity - every
        // query refetches with the impersonation token.
        doImpersonate(data.accessToken);
        qc.clear();
        router.replace("/(auth)/(tabs)");
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedImpersonate", "Failed to impersonate user"))),
    },
  });

  const { data, isLoading, isError, refetch } = useUserServiceListUsers();
  // One read for every profile on the server, grouped by owner. A user card
  // without its profiles cannot say who is actually behind the account, and a
  // ListProfiles per row would be N+1 requests.
  const { data: profileData } = useUserServiceListAllProfiles();
  const profilesByUser = useMemo(() => {
    const map = new Map<string, V1Profile[]>();
    for (const p of profileData?.profiles ?? []) {
      if (!p.userId) continue;
      const list = map.get(p.userId);
      if (list) list.push(p);
      else map.set(p.userId, [p]);
    }
    return map;
  }, [profileData]);
  const users = data?.users ?? [];
  const trimmedQuery = query.trim().toLowerCase();
  const filteredUsers = trimmedQuery
    ? users.filter((u) =>
        (u.username ?? "").toLowerCase().includes(trimmedQuery) ||
        (u.email ?? "").toLowerCase().includes(trimmedQuery))
    : users;

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
        let libraryAccessFailed = false;
        if (userId && !newIsAdmin && newSelectedLibs.size > 0) {
          try {
            await eveyFetch<void>({
              url: `/api/v1/admin/users/${userId}/library-access`,
              method: "PUT",
              data: { libraryIds: Array.from(newSelectedLibs) },
            });
          } catch {
            libraryAccessFailed = true;
          }
        }
        // The user row always exists at this point - what may have failed is
        // the follow-up library-access grant. Reporting a plain "created"
        // success here would hide that the admin still has no libraries.
        if (libraryAccessFailed) {
          toast.error(t("admin.users.userCreatedAccessFailed", "User created, but library access could not be applied"));
        } else {
          toast.success(t("admin.users.userCreated"));
        }
        setShowCreate(false);
        setNewUsername("");
        setNewEmail("");
        setNewPassword("");
        setNewIsAdmin(false);
        setNewMaxProfiles("0");
        setNewMaxDevicesPerProfile("0");
        setNewSelectedLibs(new Set());
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedCreateUser")),
    },
  });

  const deleteUser = useUserServiceDeleteUser({
    mutation: {
      // The success toast is per call (handleDelete): only it knows the name.
      onSuccess: async () => {
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
      onError: () => toast.error(t("admin.users.failedDeleteUser")),
    },
  });

  const toggleOpts = {
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.users.userUpdated"));
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
      },
    },
  };
  // Two routes: /disable always disables, /enable re-enables.
  const disableUser = useUserServiceDisableUser(toggleOpts);
  const enableUser = useUserServiceEnableUser(toggleOpts);

  const handleCreate = useCallback(async () => {
    if (!newUsername.trim() || !newPassword.trim()) return;
    const parsedMaxProfiles = Number.isFinite(Number(newMaxProfiles)) ? Math.max(0, Math.trunc(Number(newMaxProfiles))) : 0;
    const parsedMaxDevices = Number.isFinite(Number(newMaxDevicesPerProfile)) ? Math.max(0, Math.trunc(Number(newMaxDevicesPerProfile))) : 0;
    try {
      await createUser.mutateAsync({
        data: {
          username: newUsername.trim(),
          email: newEmail.trim() || undefined,
          password: newPassword.trim(),
          isAdmin: newIsAdmin,
          maxProfiles: parsedMaxProfiles,
          maxDevicesPerProfile: parsedMaxDevices,
        },
      });
    } catch {}
  }, [newUsername, newEmail, newPassword, newIsAdmin, newMaxProfiles, newMaxDevicesPerProfile, createUser]);

  const handleDelete = useCallback((user: V1User) => {
    Alert.alert(t("common.delete"), `${t("common.delete")} ${user.username}?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.delete"), style: "destructive", onPress: () => user.id && deleteUser.mutate({ id: user.id }, {
        onSuccess: () => toast.success(t("admin.users.userDeleted", 'User "{{name}}" deleted', { name: user.username ?? "" })),
      }) },
    ]);
  }, [deleteUser, t]);

  const handleToggleDisable = useCallback((user: V1User) => {
    if (!user.id) return;
    if (user.enabled !== false) disableUser.mutate({ id: user.id, params: { disabled: true } });
    else enableUser.mutate({ id: user.id });
  }, [disableUser, enableUser]);

  // profileId empty = the target's default profile, which is all the user-level
  // button could ever do. Passing one is what lets an admin land on the profile
  // they actually mean.
  const handleImpersonate = useCallback((user: V1User, profileId?: string) => {
    if (!user.id) return;
    impersonateUser.mutate({ userId: user.id, data: profileId ? { profileId } : {} });
  }, [impersonateUser]);

  // Close the action sheet, then open the follow-up modal on the next tick:
  // mounting a second RN Modal in the same commit as one dismisses races the
  // iOS presentation controller and can swallow the new modal.
  const afterSheet = useCallback((fn: () => void) => {
    setActionUser(null);
    setTimeout(fn, 250);
  }, []);

  const sheetActions = useCallback((user: V1User): UserAction[] => {
    const actions: UserAction[] = [
      { icon: "pencil-outline", label: t("common.edit", "Edit"), onPress: () => { setActionUser(null); if (user.id) router.push(`/admin/users/${user.id}` as never); } },
    ];
    if (!user.isAdmin) {
      actions.push({ icon: "library-outline", label: t("admin.users.libraryAccess", "Library Access"), onPress: () => afterSheet(() => setLibAccessUser(user)) });
    }
    actions.push({ icon: "speedometer-outline", label: t("admin.users.bitrate", "Bitrate"), onPress: () => afterSheet(() => setBitrateUser(user)) });
    if (user.enabled !== false) {
      const profiles = profilesByUser.get(user.id ?? "") ?? [];
      if (profiles.length === 0) {
        actions.push({ icon: "log-in-outline", label: t("admin.users.impersonate", "Connect as"), onPress: () => { setActionUser(null); handleImpersonate(user); } });
      } else {
        for (const p of profiles) {
          actions.push({
            icon: "log-in-outline",
            label: t("admin.users.impersonateProfile", "Connect as {{name}}", { name: p.name }),
            onPress: () => { setActionUser(null); handleImpersonate(user, p.id); },
          });
        }
      }
    }
    actions.push({
      icon: user.enabled === false ? "person-add-outline" : "person-remove-outline",
      label: user.enabled === false ? t("common.enable", "Enable") : t("common.disable", "Disable"),
      onPress: () => { setActionUser(null); handleToggleDisable(user); },
    });
    actions.push({ icon: "trash-outline", label: t("common.delete", "Delete"), destructive: true, onPress: () => { setActionUser(null); handleDelete(user); } });
    return actions;
  }, [afterSheet, handleDelete, handleToggleDisable, handleImpersonate, profilesByUser, router, t]);

  const renderUser = useCallback(({ item }: { item: V1User }) => {
    const disabled = item.enabled === false;
    return (
      <Pressable
        onPress={() => setActionUser(item)}
        style={[styles.userCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        accessible={true}
        accessibilityRole="button"
        accessibilityLabel={t("admin.users.actionsFor", "Actions for {{name}}", { name: item.username })}
      >
        <View style={[styles.avatar, { backgroundColor: disabled ? theme.colors.surfaceRaised : theme.colors.accent }]}>
          <Text style={{ color: disabled ? theme.colors.textMuted : theme.colors.onAccent, fontWeight: "700" }}>{item.username?.[0]?.toUpperCase() ?? "?"}</Text>
        </View>
        <View style={{ flex: 1, opacity: disabled ? 0.6 : 1 }}>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
            <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }} numberOfLines={1}>{item.username}</Text>
            {item.isAdmin && (
              <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
                <Text style={{ color: theme.colors.onAccent, fontSize: 9, fontWeight: "700" }}>{t("admin.users.admin", "Admin").toUpperCase()}</Text>
              </View>
            )}
            {disabled && (
              <View style={{ backgroundColor: theme.colors.textMuted, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
                <Text style={{ color: onColor(theme.colors.textMuted), fontSize: 9, fontWeight: "700" }}>{t("common.disabled", "Disabled").toUpperCase()}</Text>
              </View>
            )}
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>{item.email || "-"}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2 }} numberOfLines={1}>
            {(() => {
              const profiles = profilesByUser.get(item.id ?? "") ?? [];
              if (profiles.length === 0) return t("admin.users.noProfiles", "This user has no profiles yet.");
              return `${t("admin.users.profileCount", { count: profiles.length })}: ${profiles.map((p) => p.name).join(", ")}`;
            })()}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10, marginTop: 2 }}>
            {t("admin.users.registered", "Registered")}: {item.createdAt ? new Date(item.createdAt).toLocaleString() : "-"}
            {"  ·  "}
            {t("admin.users.lastLogin", "Last login")}: {item.lastLogin ? new Date(item.lastLogin).toLocaleString() : "-"}
          </Text>
        </View>
        <Ionicons name="ellipsis-vertical" size={18} color={theme.colors.textMuted} />
      </Pressable>
    );
  }, [theme, t, profilesByUser]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.nav.users"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable
            onPress={() => setShowCreate(true)}
            style={{ marginRight: 16 }}
            accessible={true}
            accessibilityLabel={t("admin.users.createUser")}
            accessibilityRole="button"
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
          data={filteredUsers}
          keyExtractor={(u) => u.id ?? ""}
          renderItem={renderUser}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListHeaderComponent={
            users.length >= 8 ? (
              <View style={[styles.searchBox, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                <Ionicons name="search" size={16} color={theme.colors.textMuted} />
                <TextInput
                  placeholder={t("admin.users.searchUsers", "Search users")}
                  placeholderTextColor={theme.colors.textMuted}
                  value={query}
                  onChangeText={setQuery}
                  autoCapitalize="none"
                  autoCorrect={false}
                  style={{ flex: 1, color: theme.colors.text, fontSize: 15, paddingVertical: 10 }}
                  accessible={true}
                  accessibilityLabel={t("admin.users.searchUsers", "Search users")}
                />
                {query.length > 0 && (
                  <Pressable
                    onPress={() => setQuery("")}
                    hitSlop={8}
                    accessible={true}
                    accessibilityLabel={t("common.clear", "Clear")}
                    accessibilityRole="button"
                  >
                    <Ionicons name="close-circle" size={18} color={theme.colors.textMuted} />
                  </Pressable>
                )}
              </View>
            ) : null
          }
          ListEmptyComponent={
            isError ? (
              <LoadFailed onRetry={() => void refetch()} />
            ) : (
              <View style={{ alignItems: "center", paddingTop: 60 }}>
                <Text style={{ color: theme.colors.textMuted }}>{t("admin.users.noUsers")}</Text>
              </View>
            )
          }
          ListFooterComponent={<InviteLinksSection theme={theme} />}
        />
      )}

      <Modal visible={showCreate} transparent animationType="slide" onRequestClose={() => setShowCreate(false)}>
        <Pressable
          style={styles.backdrop}
          onPress={() => setShowCreate(false)}
          accessibilityElementsHidden={true}
          importantForAccessibility="no-hide-descendants"
        />
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
            <View style={{ marginBottom: 4 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: 4 }}>
                {t("admin.users.maxProfiles", "Max profiles")}
              </Text>
              <TextInput
                placeholder="0"
                placeholderTextColor={theme.colors.textMuted}
                value={newMaxProfiles}
                onChangeText={setNewMaxProfiles}
                keyboardType="number-pad"
                style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
              />
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: -8, marginBottom: 12 }}>
                {t("admin.users.maxProfilesHelp", "0 = unlimited")}
              </Text>
            </View>
            <View style={{ marginBottom: 4 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: 4 }}>
                {t("admin.users.maxDevicesPerProfile", "Max devices per profile")}
              </Text>
              <TextInput
                placeholder="0"
                placeholderTextColor={theme.colors.textMuted}
                value={newMaxDevicesPerProfile}
                onChangeText={setNewMaxDevicesPerProfile}
                keyboardType="number-pad"
                style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md }]}
              />
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: -8, marginBottom: 12 }}>
                {t("admin.users.maxDevicesPerProfileHelp", "0 = unlimited")}
              </Text>
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
                    accessible={true}
                    accessibilityLabel={lib.name ?? lib.id}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked: newSelectedLibs.has(lib.id!) }}
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
                  <Pressable
                    onPress={() => setNewSelectedLibs(new Set())}
                    style={{ paddingVertical: 6 }}
                    accessible={true}
                    accessibilityLabel={t("admin.users.clearAllUnrestricted")}
                    accessibilityRole="button"
                  >
                    <Text style={{ color: theme.colors.accent, fontSize: 13, textAlign: "center" }}>
                      {t("admin.users.clearAllUnrestricted")}
                    </Text>
                  </Pressable>
                )}
              </View>
            )}
          </ScrollView>
          <Pressable
            onPress={handleCreate}
            disabled={createUser.isPending}
            style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createUser.isPending ? 0.5 : 1 }]}
            accessible={true}
            accessibilityLabel={t("common.create")}
            accessibilityRole="button"
          >
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("common.create")}</Text>
          </Pressable>
        </View>
      </Modal>

      {actionUser && (
        <UserActionsSheet
          user={actionUser}
          actions={sheetActions(actionUser)}
          profiles={profilesByUser.get(actionUser.id ?? "") ?? []}
          onClose={() => setActionUser(null)}
          theme={theme}
        />
      )}

      {libAccessUser && !libAccessUser.isAdmin && (
        <LibraryAccessModal
          user={libAccessUser}
          visible={!!libAccessUser}
          onClose={() => setLibAccessUser(null)}
          theme={theme}
        />
      )}

      {bitrateUser && (
        <BitrateModal
          user={bitrateUser}
          visible={!!bitrateUser}
          onClose={() => setBitrateUser(null)}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  userCard: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  avatar: { width: 36, height: 36, borderRadius: 18, justifyContent: "center", alignItems: "center" },
  profileAvatar: { width: 28, height: 28, borderRadius: 14, justifyContent: "center", alignItems: "center", overflow: "hidden" },
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  input: { paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginBottom: 12 },
  createBtn: { paddingVertical: 14, alignItems: "center" },
  libRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 12, paddingVertical: 10, marginBottom: 6 },
  actionRow: { flexDirection: "row", alignItems: "center", gap: 14, paddingHorizontal: 12, paddingVertical: 14, marginTop: 4 },
  searchBox: { flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 12, marginBottom: 8 },
});
