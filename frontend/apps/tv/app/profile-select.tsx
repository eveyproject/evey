import { useEffect } from "react";
import { View, Text, ActivityIndicator, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { FocusableCard } from "../src/components/FocusableCard";

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
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("mobile.profileSelect.title", "Who's watching?")}
      </Text>
      <View style={styles.grid}>
        {profiles.map((profile, index) => {
          const initial = (profile.name ?? "?")[0].toUpperCase();
          return (
            <FocusableCard
              key={profile.id}
              onPress={() => profile.id && handleSelect(profile.id)}
              hasTVPreferredFocus={index === 0}
              style={styles.card}
              focusedStyle={{ borderColor: theme.colors.accent }}
            >
              <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
                <Text style={styles.avatarText}>{initial}</Text>
              </View>
              <Text style={[styles.profileName, { color: theme.colors.text }]} numberOfLines={1}>
                {profile.name}
              </Text>
            </FocusableCard>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 80,
  },
  center: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  title: {
    fontSize: 42,
    fontWeight: "bold",
    marginBottom: 48,
  },
  grid: {
    flexDirection: "row",
    flexWrap: "wrap",
    justifyContent: "center",
    gap: 40,
  },
  card: {
    alignItems: "center",
    width: 180,
    padding: 24,
  },
  avatar: {
    width: 120,
    height: 120,
    borderRadius: 60,
    justifyContent: "center",
    alignItems: "center",
    marginBottom: 16,
  },
  avatarText: {
    color: "#fff",
    fontSize: 48,
    fontWeight: "bold",
  },
  profileName: {
    fontSize: 22,
    textAlign: "center",
  },
});
