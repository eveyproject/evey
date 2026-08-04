import { useState } from "react";
import { View, Text, ScrollView, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { Stack } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { ProfileManagerList } from "../../../src/features/profile/ProfileManagerList";
import { CreateProfileDialog } from "../../../src/features/profile/CreateProfileDialog";
import { LoadFailed } from "../../../src/components/LoadFailed";

export default function ProfileManageScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [creating, setCreating] = useState(false);
  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id ?? "";
  const { data: profilesResp, isLoading, isError, refetch } = useUserServiceListProfiles(userId, {
    query: { enabled: !!userId },
  });
  const profiles = profilesResp?.profiles ?? [];
  const hasKidsProfile = profiles.some((p) => p.isKids === true);

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("auth.profile.manage.title", "Manage Profiles"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      <Text style={[s.description, { color: theme.colors.textMuted }]}>
        {t("auth.profile.manage.description", "Add, edit, or remove profiles. The default profile cannot be deleted.")}
      </Text>

      <Pressable
        onPress={() => setCreating(true)}
        style={[s.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
        accessibilityRole="button"
        accessibilityLabel={t("auth.profile.manage.createProfile", "Create Profile")}
      >
        <Ionicons name="add" size={18} color={theme.colors.onAccent} />
        <Text style={[s.createBtnText, { color: theme.colors.onAccent }]}>
          {t("auth.profile.manage.createProfile", "Create Profile")}
        </Text>
      </Pressable>

      {isLoading && (
        <View style={{ padding: theme.spacing.lg, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      )}

      {!isLoading && isError && (
        <LoadFailed inline onRetry={() => void refetch()} />
      )}

      {!isLoading && !isError && userId && profiles.length > 0 && (
        <ProfileManagerList userId={userId} profiles={profiles} />
      )}

      {!isLoading && !isError && profiles.length === 0 && (
        <Text style={[s.empty, { color: theme.colors.textMuted }]}>
          {t("auth.profile.manage.noProfiles", "No profiles yet.")}
        </Text>
      )}

      {userId && (
        <CreateProfileDialog
          open={creating}
          onClose={() => setCreating(false)}
          userId={userId}
          hasKidsProfile={hasKidsProfile}
        />
      )}
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: {
      flex: 1,
      backgroundColor: t.colors.bg,
    },
    content: {
      padding: t.spacing.md,
      gap: t.spacing.md,
      paddingBottom: t.spacing.xl,
    },
    description: {
      fontSize: 13,
    },
    createBtn: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "center",
      gap: 8,
      paddingVertical: 12,
      paddingHorizontal: 16,
    },
    createBtnText: {
      fontSize: 15,
      fontWeight: "600",
    },
    empty: {
      fontSize: 14,
      textAlign: "center",
      paddingVertical: t.spacing.lg,
    },
  });
