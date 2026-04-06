import { useEffect, useRef } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, StyleSheet, Alert } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import * as SecureStore from "expo-secure-store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrl } from "../src/hooks/useServerUrl";
import { supportedLanguages } from "../src/i18n";
import i18n from "../src/i18n";
import { useBiometricAvailable, useBiometricEnabled } from "../src/hooks/useBiometric";

export default function ProfileSelect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const setProfile = useAuthStore((s) => s.setProfile);
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();

  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id;

  const { data: profilesData, isLoading } = useUserServiceListProfiles(userId ?? "", {
    query: { enabled: !!userId },
  });

  const profiles = profilesData?.profiles ?? [];
  const defaultProfile = profiles.find((p) => p.isDefault) ?? profiles[0];

  // Task 2: sync locale from profile
  const localeSynced = useRef(false);
  useEffect(() => {
    if (localeSynced.current || !defaultProfile?.locale) return;
    const lang = defaultProfile.locale.split("-")[0];
    if (supportedLanguages.some((l) => l.code === lang) && !i18n.language.startsWith(lang)) {
      void i18n.changeLanguage(lang);
      useLocaleStore.getState().setLocale(lang);
    }
    localeSynced.current = true;
  }, [defaultProfile?.locale]);

  // Task 3: prompt biometric on first login
  const { available: biometricAvailable } = useBiometricAvailable();
  const { enabled: biometricEnabled, toggle: toggleBiometric } = useBiometricEnabled();
  const biometricPrompted = useRef(false);
  useEffect(() => {
    if (biometricPrompted.current || !biometricAvailable || biometricEnabled || isLoading) return;
    biometricPrompted.current = true;
    (async () => {
      const prompted = await SecureStore.getItemAsync("evey-biometric-prompted");
      if (prompted === "true") return;
      await SecureStore.setItemAsync("evey-biometric-prompted", "true");
      Alert.alert(
        t("mobile.auth.enableBiometric", "Enable biometric login?"),
        t("mobile.auth.enableBiometricDesc", "Use biometrics to unlock the app next time."),
        [
          { text: t("common.no", "No"), style: "cancel" },
          { text: t("common.yes", "Yes"), onPress: () => void toggleBiometric(true) },
        ],
      );
    })();
  }, [biometricAvailable, biometricEnabled, isLoading, t, toggleBiometric]);

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
              <View style={[s.avatar, { backgroundColor: theme.colors.accent, overflow: "hidden" }]}>
                {profile.avatarPath && serverUrl && accessToken ? (
                  <Image
                    source={{ uri: `${serverUrl}/api/v1/profiles/${profile.id}/avatar?token=${accessToken}` }}
                    style={{ width: 64, height: 64, borderRadius: 32 }}
                    contentFit="cover"
                  />
                ) : (
                  <Text style={s.avatarText}>{initial}</Text>
                )}
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
