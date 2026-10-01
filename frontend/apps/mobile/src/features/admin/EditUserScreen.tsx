import { useEffect, useState } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  Switch,
  ScrollView,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceUpdateUser,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  getUserServiceListUsersQueryKey,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1User, UserServiceUpdateUserBody } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { ProfileManagerList } from "../profile/ProfileManagerList";
import { CreateProfileDialog } from "../profile/CreateProfileDialog";
import { FONT_SIZE } from "@evey/theme";

const USERNAME_REGEX = /^[a-zA-Z0-9_.-]{1,64}$/;

interface Props {
  user: V1User;
  onSaved?: () => void;
  onCancel?: () => void;
}

export function EditUserScreen({ user, onSaved, onCancel }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const [username, setUsername] = useState(user.username ?? "");
  const [email, setEmail] = useState(user.email ?? "");
  const [password, setPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  // An admin editing their OWN account re-enters their password, like on the
  // settings screen; editing someone else needs no proof (AUTH-3).
  const { data: me } = useUserServiceGetCurrentUser();
  const isSelf = !!me?.id && me.id === user.id;
  const [isAdmin, setIsAdmin] = useState(user.isAdmin ?? false);
  const [maxProfiles, setMaxProfiles] = useState<string>(String(user.maxProfiles ?? 0));
  const [maxDevicesPerProfile, setMaxDevicesPerProfile] = useState<string>(
    String(user.maxDevicesPerProfile ?? 0),
  );
  const [error, setError] = useState<string | null>(null);
  const [creatingProfile, setCreatingProfile] = useState(false);

  const { data: profilesData, isLoading: profilesLoading } = useUserServiceListProfiles(
    user.id ?? "",
    { query: { enabled: !!user.id } },
  );
  const profiles = profilesData?.profiles ?? [];

  useEffect(() => {
    setUsername(user.username ?? "");
    setEmail(user.email ?? "");
    setPassword("");
    setCurrentPassword("");
    setIsAdmin(user.isAdmin ?? false);
    setMaxProfiles(String(user.maxProfiles ?? 0));
    setMaxDevicesPerProfile(String(user.maxDevicesPerProfile ?? 0));
    setError(null);
  }, [user]);

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: async () => {
        await qc.invalidateQueries({ queryKey: getUserServiceListUsersQueryKey() });
        toast.success(t("admin.users.userUpdated", "User updated"));
        onSaved?.();
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("admin.users.failedUpdateUser", "Failed to update user"))),
    },
  });

  function validate(): boolean {
    if (!username.trim()) {
      setError(t("admin.users.usernameRequired", "Username is required"));
      return false;
    }
    if (!USERNAME_REGEX.test(username.trim())) {
      setError(
        t(
          "admin.users.usernameInvalid",
          "Username must be 1-64 chars: letters, digits, _ . -",
        ),
      );
      return false;
    }
    const mp = Number(maxProfiles);
    if (!Number.isFinite(mp) || mp < 0 || !Number.isInteger(mp)) {
      setError(t("admin.users.maxProfilesInvalid", "Max profiles must be a whole number >= 0"));
      return false;
    }
    const md = Number(maxDevicesPerProfile);
    if (!Number.isFinite(md) || md < 0 || !Number.isInteger(md)) {
      setError(
        t(
          "admin.users.maxDevicesInvalid",
          "Max devices must be a whole number >= 0",
        ),
      );
      return false;
    }
    return true;
  }

  function handleSubmit() {
    setError(null);
    if (!validate()) return;
    if (!user.id) return;

    const data: UserServiceUpdateUserBody = {};
    if (username.trim() !== (user.username ?? "")) data.username = username.trim();
    if (email.trim() !== (user.email ?? "")) data.email = email.trim();
    if (password) data.password = password;
    if (isAdmin !== (user.isAdmin ?? false)) data.isAdmin = isAdmin;

    const parsedMaxProfiles = Math.max(0, Math.trunc(Number(maxProfiles)));
    const parsedMaxDevices = Math.max(0, Math.trunc(Number(maxDevicesPerProfile)));
    if (parsedMaxProfiles !== (user.maxProfiles ?? 0)) data.maxProfiles = parsedMaxProfiles;
    if (parsedMaxDevices !== (user.maxDevicesPerProfile ?? 0))
      data.maxDevicesPerProfile = parsedMaxDevices;

    if (Object.keys(data).length === 0) {
      onSaved?.();
      return;
    }
    if (isSelf && (data.username !== undefined || data.email !== undefined || data.password !== undefined)) {
      if (!currentPassword) {
        setError(t("errors.user.current_password_required"));
        return;
      }
      data.currentPassword = currentPassword;
    }

    updateUser.mutate({ id: user.id, data });
  }

  const s = styles(theme);

  return (
    <View style={[s.root, { backgroundColor: theme.colors.bg }]}>
      <ScrollView
        style={s.scroll}
        contentContainerStyle={s.scrollContent}
        keyboardShouldPersistTaps="handled"
      >
        {/* Username */}
        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>
            {t("admin.users.username", "Username")}
          </Text>
          <TextInput
            value={username}
            onChangeText={setUsername}
            autoCapitalize="none"
            autoCorrect={false}
            placeholderTextColor={theme.colors.textMuted}
            style={[
              s.input,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            accessibilityLabel={t("admin.users.username", "Username")}
          />
        </View>

        {/* Email */}
        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>
            {t("admin.users.email", "Email")}
          </Text>
          <TextInput
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            keyboardType="email-address"
            placeholderTextColor={theme.colors.textMuted}
            style={[
              s.input,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            accessibilityLabel={t("admin.users.email", "Email")}
          />
        </View>

        {/* New password */}
        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>
            {t("admin.users.newPassword", "New password")}
          </Text>
          <TextInput
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            placeholder={t("admin.users.leaveBlank", "Leave blank to keep current")}
            placeholderTextColor={theme.colors.textMuted}
            style={[
              s.input,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            accessibilityLabel={t("admin.users.newPassword", "New password")}
          />
        </View>

        {isSelf && (
          <View style={s.field}>
            <Text style={[s.label, { color: theme.colors.textMuted }]}>
              {t("auth.settings.account.currentPassword")}
            </Text>
            <TextInput
              value={currentPassword}
              onChangeText={setCurrentPassword}
              secureTextEntry
              autoComplete="current-password"
              placeholder={t("auth.settings.account.currentPasswordHint")}
              placeholderTextColor={theme.colors.textMuted}
              style={[
                s.input,
                {
                  backgroundColor: theme.colors.surface,
                  color: theme.colors.text,
                  borderColor: theme.colors.border,
                  borderRadius: theme.radii.md,
                },
              ]}
              accessibilityLabel={t("auth.settings.account.currentPassword")}
            />
          </View>
        )}

        {/* isAdmin toggle */}
        <View
          style={[
            s.row,
            { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md },
          ]}
        >
          <View style={{ flex: 1 }}>
            <Text style={[s.rowLabel, { color: theme.colors.text }]}>
              {t("admin.users.admin", "Admin")}
            </Text>
          </View>
          <Switch
            value={isAdmin}
            onValueChange={setIsAdmin}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
            accessibilityLabel={t("admin.users.admin", "Admin")}
          />
        </View>

        {/* Max profiles */}
        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>
            {t("admin.users.maxProfiles", "Max profiles")}
          </Text>
          <TextInput
            value={maxProfiles}
            onChangeText={setMaxProfiles}
            keyboardType="number-pad"
            style={[
              s.input,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            accessibilityLabel={t("admin.users.maxProfiles", "Max profiles")}
          />
          <Text style={[s.help, { color: theme.colors.textMuted }]}>
            {t("admin.users.maxProfilesHelp", "0 = unlimited")}
          </Text>
        </View>

        {/* Max devices per profile */}
        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>
            {t("admin.users.maxDevicesPerProfile", "Max devices per profile")}
          </Text>
          <TextInput
            value={maxDevicesPerProfile}
            onChangeText={setMaxDevicesPerProfile}
            keyboardType="number-pad"
            style={[
              s.input,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            accessibilityLabel={t("admin.users.maxDevicesPerProfile", "Max devices per profile")}
          />
          <Text style={[s.help, { color: theme.colors.textMuted }]}>
            {t("admin.users.maxDevicesPerProfileHelp", "0 = unlimited")}
          </Text>
        </View>

        {/* Error */}
        {error && (
          <Text style={[s.error, { color: theme.colors.error }]}>{error}</Text>
        )}

        {/* Divider */}
        <View style={[s.divider, { borderTopColor: theme.colors.border }]} />

        {/* Profiles section */}
        <View style={s.sectionHeader}>
          <Text style={[s.sectionTitle, { color: theme.colors.text }]}>
            {t("admin.users.profilesTitle", "Profiles")}
          </Text>
          <Pressable
            onPress={() => setCreatingProfile(true)}
            disabled={!user.id}
            accessibilityRole="button"
            accessibilityLabel={t("auth.profile.manage.newProfile", "New profile")}
            style={{ opacity: user.id ? 1 : 0.4 }}
          >
            <Text style={[s.addLink, { color: theme.colors.accent }]}>
              {t("auth.profile.manage.newProfile", "New profile")}
            </Text>
          </Pressable>
        </View>

        {profilesLoading ? (
          <ActivityIndicator
            color={theme.colors.accent}
            style={{ paddingVertical: 16 }}
          />
        ) : profiles.length === 0 ? (
          <Text style={[s.help, { color: theme.colors.textMuted }]}>
            {t("admin.users.noProfiles", "This user has no profiles yet.")}
          </Text>
        ) : (
          <ProfileManagerList userId={user.id ?? ""} profiles={profiles} canSetDefault />
        )}
      </ScrollView>

      {/* Action buttons */}
      <View
        style={[
          s.footer,
          { borderTopColor: theme.colors.border, backgroundColor: theme.colors.bg },
        ]}
      >
        {onCancel && (
          <Pressable
            onPress={onCancel}
            style={[s.btn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
            accessibilityRole="button"
            accessibilityLabel={t("common.cancel", "Cancel")}
          >
            <Text style={[s.btnText, { color: theme.colors.text }]}>
              {t("common.cancel", "Cancel")}
            </Text>
          </Pressable>
        )}
        <Pressable
          onPress={handleSubmit}
          disabled={updateUser.isPending}
          style={[
            s.btn,
            s.btnPrimary,
            { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md },
            updateUser.isPending && { opacity: 0.6 },
          ]}
          accessibilityRole="button"
          accessibilityLabel={t("common.save", "Save")}
        >
          {updateUser.isPending ? (
            <ActivityIndicator color={theme.colors.onAccent} size="small" />
          ) : (
            <Text style={[s.btnText, { color: theme.colors.onAccent }]}>
              {t("common.save", "Save")}
            </Text>
          )}
        </Pressable>
      </View>

      {/* Create profile dialog */}
      {user.id && (
        <CreateProfileDialog
          open={creatingProfile}
          userId={user.id}
          hasKidsProfile={profiles.some((p: { isKids?: boolean }) => p.isKids === true)}
          onClose={() => {
            setCreatingProfile(false);
            void qc.invalidateQueries({
              queryKey: getUserServiceListProfilesQueryKey(user.id ?? ""),
            });
          }}
        />
      )}
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    root: { flex: 1 },
    scroll: { flex: 1 },
    scrollContent: {
      padding: t.spacing.lg,
      gap: t.spacing.md,
    },
    field: { gap: 6 },
    label: {
      fontSize: FONT_SIZE.xs,
      fontWeight: "600",
      textTransform: "uppercase",
    },
    input: {
      borderWidth: 1,
      paddingHorizontal: 12,
      paddingVertical: 10,
      fontSize: FONT_SIZE.sm,
    },
    help: { fontSize: FONT_SIZE.xs, marginTop: 2 },
    row: {
      flexDirection: "row",
      alignItems: "center",
      paddingHorizontal: 12,
      paddingVertical: 12,
      gap: t.spacing.md,
    },
    rowLabel: { fontSize: FONT_SIZE.sm, fontWeight: "500" },
    error: { fontSize: FONT_SIZE.xs },
    divider: {
      borderTopWidth: StyleSheet.hairlineWidth,
      marginVertical: t.spacing.sm,
    },
    sectionHeader: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
    },
    sectionTitle: { fontSize: FONT_SIZE.sm, fontWeight: "700" },
    addLink: { fontSize: FONT_SIZE.sm, fontWeight: "500" },
    footer: {
      flexDirection: "row",
      gap: t.spacing.sm,
      padding: t.spacing.md,
      borderTopWidth: StyleSheet.hairlineWidth,
    },
    btn: {
      flex: 1,
      paddingVertical: 13,
      alignItems: "center",
      justifyContent: "center",
    },
    btnPrimary: {},
    btnText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
  });
