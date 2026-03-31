import { useEffect } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ProfileSelect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const setProfile = useAuthStore((s) => s.setProfile);

  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id;

  const { data: profilesData, isLoading } = useUserServiceListProfiles(userId ?? "", {
    query: { enabled: !!userId },
  });

  const profiles = profilesData?.profiles ?? [];

  useEffect(() => {
    if (!isLoading && profiles.length === 1 && profiles[0].id) {
      setProfile(profiles[0].id);
      router.replace("/(auth)/(tabs)");
    }
  }, [isLoading, profiles, setProfile]);

  function handleSelect(profileId: string) {
    setProfile(profileId);
    router.replace("/(auth)/(tabs)");
  }

  if (isLoading || profiles.length <= 1) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Text style={s.title}>{t("mobile.profileSelect.title", "Who's watching?")}</Text>
      <View style={s.grid}>
        {profiles.map((profile) => {
          const initial = (profile.name ?? "?")[0].toUpperCase();
          return (
            <Pressable
              key={profile.id}
              style={s.card}
              onPress={() => profile.id && handleSelect(profile.id)}
              accessible={true}
              accessibilityLabel={profile.name}
              accessibilityRole="button"
            >
              <View style={[s.avatar, { backgroundColor: theme.colors.accent }]}>
                <Text style={s.avatarText}>{initial}</Text>
              </View>
              <Text style={s.profileName} numberOfLines={1}>{profile.name}</Text>
            </Pressable>
          );
        })}
      </View>
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
      flex: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.xl,
    },
    title: {
      fontSize: 24,
      fontWeight: "bold",
      color: t.colors.text,
      marginBottom: t.spacing.xl,
    },
    grid: {
      flexDirection: "row",
      flexWrap: "wrap",
      justifyContent: "center",
      gap: t.spacing.lg,
    },
    card: {
      alignItems: "center",
      width: 100,
    },
    avatar: {
      width: 80,
      height: 80,
      borderRadius: 40,
      justifyContent: "center",
      alignItems: "center",
      marginBottom: t.spacing.sm,
    },
    avatarText: {
      color: "#fff",
      fontSize: 32,
      fontWeight: "bold",
    },
    profileName: {
      color: t.colors.text,
      fontSize: 14,
      textAlign: "center",
    },
  });
