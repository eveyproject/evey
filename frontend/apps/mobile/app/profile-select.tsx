import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, StyleSheet, Alert } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import * as SecureStore from "expo-secure-store";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  userServiceRefreshToken,
  apiErrorMessage,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { useAuthStore, useLocaleStore, toast } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrl } from "../src/hooks/useServerUrl";
import { supportedLanguages } from "../src/i18n";
import i18n from "../src/i18n";
import { useBiometricAvailable, useBiometricEnabled } from "../src/hooks/useBiometric";
import { PinPromptDialog } from "../src/features/profile/PinPromptDialog";
import { loadProfileCache, type CachedProfile } from "@evey/store";

const LAST_PROFILE_KEY = "evey:last_profile_id";

export default function ProfileSelect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const setProfile = useAuthStore((s) => s.setProfile);
  const accessToken = useAuthStore((s) => s.accessToken);
  const activeProfileId = useAuthStore((s) => s.profileId);
  const serverUrl = useServerUrl();

  const { data: user, isLoading: userLoading } = useUserServiceGetCurrentUser();
  const userId = user?.id;

  const { data: profilesData, isLoading: profilesLoading } = useUserServiceListProfiles(userId ?? "", {
    query: { enabled: !!userId },
  });

  // Seeded from the on-device cache so the picker paints on the first frame
  // instead of a spinner: at cold start this screen is the first thing between
  // the user and their library, and the list it draws changes about once a
  // year. The live answer replaces it as soon as it lands - and it is the live
  // one every decision is taken on, since only it carries isDefault and the
  // rest of V1Profile.
  const [cached, setCached] = useState<CachedProfile[] | null>(null);
  useEffect(() => {
    let cancelled = false;
    void loadProfileCache().then((c) => { if (!cancelled) setCached(c); });
    return () => { cancelled = true; };
  }, []);

  const profiles = useMemo(() => profilesData?.profiles ?? [], [profilesData]);
  // Rows to draw: the live list once it exists, the cache until then.
  const rows: { id?: string; name?: string; avatarPath?: string; isKids?: boolean }[] =
    profiles.length > 0 ? profiles : (cached ?? []);
  // Two different questions, and conflating them is how the cache would break
  // this screen. "Do I have anything to draw?" is answered by rows - the cache
  // counts. "Has the server answered?" is liveSettled, and only that may drive
  // the auto-skip below, which needs isDefault and the rest of V1Profile.
  const liveSettled = !userLoading && !profilesLoading;
  const loading = !liveSettled && rows.length === 0;
  const hasKidsProfile = rows.some((p) => p.isKids === true);
  const defaultProfile = profiles.find((p) => p.isDefault) ?? profiles[0];

  const currentProfile = profiles.find((p) => p.id === activeProfileId);
  const currentIsKids = currentProfile?.isKids === true;

  const [busy, setBusy] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);
  const autoRedirectedRef = useRef(false);

  // Task: sync locale from default profile (preserved from previous behavior)
  const localeSynced = useRef(false);
  useEffect(() => {
    if (localeSynced.current || !defaultProfile?.locale) return;
    const lang = defaultProfile.locale.split("-")[0];
    if (supportedLanguages.some((l) => l.code === lang) && !i18n.language.startsWith(lang)) {
      void i18n.changeLanguage(lang);
      useLocaleStore.getState().setLocale(lang, "profile");
    }
    localeSynced.current = true;
  }, [defaultProfile?.locale]);

  // Task: prompt biometric on first profile-select (preserved)
  const { available: biometricAvailable } = useBiometricAvailable();
  const { enabled: biometricEnabled, toggle: toggleBiometric } = useBiometricEnabled();
  const biometricPrompted = useRef(false);
  useEffect(() => {
    if (biometricPrompted.current || !biometricAvailable || biometricEnabled || loading) return;
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
  }, [biometricAvailable, biometricEnabled, loading, t, toggleBiometric]);

  const switchProfile = useCallback(async (target: V1Profile, pinToken?: string): Promise<"ok" | "pin" | "fail"> => {
    if (!target.id) return "fail";
    try {
      const resp = await userServiceRefreshToken({ profileId: target.id, pinToken });
      if (resp.accessToken) {
        useAuthStore.getState().setTokens(resp.accessToken);
      } else {
        // No new token returned - at least pin the local profileId so AuthLayout
        // doesn't bounce back to /profile-select.
        setProfile(target.id);
      }
      try {
        await SecureStore.setItemAsync(LAST_PROFILE_KEY, target.id);
      } catch { /* ignore */ }
      await qc.invalidateQueries();
      return "ok";
    } catch (err) {
      // 403 = the server wants a parental PIN proof (gated source or
      // PIN-carrying target). The refresh token is not consumed, so a retry
      // with a pinToken works.
      if (!pinToken && (err as { httpStatus?: number }).httpStatus === 403) return "pin";
      console.warn("profile switch failed", err);
      toast.error(apiErrorMessage(err as Error, t, t("auth.profile.switchFailed", "Could not switch profile")));
      return "fail";
    }
  }, [qc, setProfile, t]);

  const handleSelect = useCallback(async (p: V1Profile) => {
    if (!p.id || busy) return;
    const targetIsKids = p.isKids === true;
    const needsPin = currentIsKids && !targetIsKids;
    if (needsPin) {
      setPinTarget(p);
      return;
    }
    setBusy(p.id);
    const res = await switchProfile(p);
    setBusy(null);
    if (res === "ok") router.replace("/(auth)/(tabs)");
    else if (res === "pin") setPinTarget(p);
  }, [busy, currentIsKids, switchProfile]);

  // Auto-skip when no kids profile exists.
  useEffect(() => {
    // liveSettled, not loading: the fallback below picks the default profile,
    // and isDefault exists only on the server's answer - never in the cache.
    if (!liveSettled || autoRedirectedRef.current) return;
    if (profiles.length === 0) {
      autoRedirectedRef.current = true;
      router.replace("/(auth)/(tabs)");
      return;
    }
    // No kids profile, or only one profile: nothing anyone could pick wrong,
    // so the screen has no reason to exist. Straight through.
    if (!hasKidsProfile || profiles.length === 1) {
      autoRedirectedRef.current = true;
      void (async () => {
        let lastId: string | null = null;
        try {
          lastId = await SecureStore.getItemAsync(LAST_PROFILE_KEY);
        } catch { /* ignore */ }
        const target = profiles.find((p) => p.id === lastId)
          ?? profiles.find((p) => p.isDefault)
          ?? profiles[0];
        if (target?.id && target.id !== activeProfileId) {
          await switchProfile(target);
        } else if (target?.id) {
          setProfile(target.id);
        }
        router.replace("/(auth)/(tabs)");
      })();
    }
  }, [liveSettled, hasKidsProfile, profiles, activeProfileId, switchProfile, setProfile]);

  if (loading || (liveSettled && !hasKidsProfile)) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Text style={s.title}>{t("auth.profile.select.title", "Who's watching?")}</Text>
      <Text style={s.subtitle}>{t("auth.profile.select.description", "Select a profile to continue.")}</Text>
      <View style={s.grid}>
        {rows.map((p) => {
          const initial = (p.name ?? "?")[0]?.toUpperCase() ?? "?";
          const avatarUrl = p.avatarPath && p.id && serverUrl && accessToken
            ? `${serverUrl}/api/v1/profiles/${p.id}/avatar?token=${accessToken}`
            : null;
          const isBusy = busy === p.id;
          return (
            <Pressable
              key={p.id}
              style={[s.card, busy ? { opacity: 0.5 } : null]}
              onPress={() => void handleSelect(p as V1Profile)}
              disabled={busy !== null}
              accessible
              accessibilityLabel={p.name}
              accessibilityRole="button"
            >
              <View style={[s.avatar, { backgroundColor: theme.colors.accent, overflow: "hidden" }]}>
                {avatarUrl ? (
                  // Decorative: the parent Pressable already carries the
                  // profile name as its accessibilityLabel.
                  <Image
                    source={{ uri: avatarUrl }}
                    style={{ width: 96, height: 96, borderRadius: 48 }}
                    contentFit="cover"
                    accessible={false}
                    accessibilityElementsHidden={true}
                    importantForAccessibility="no-hide-descendants"
                  />
                ) : (
                  <Text style={s.avatarText}>{initial}</Text>
                )}
              </View>
              <View style={s.nameRow}>
                <Text style={s.profileName} numberOfLines={1}>{p.name}</Text>
                {p.isKids && (
                  <Text style={s.kidsBadge}>
                    {t("auth.profile.select.kidsBadge", "Kids")}
                  </Text>
                )}
              </View>
              {isBusy && <ActivityIndicator color={theme.colors.accent} size="small" />}
            </Pressable>
          );
        })}
      </View>
      <Pressable
        onPress={() => router.push("/(auth)/profile/manage")}
        style={s.manageBtn}
        accessibilityRole="button"
        accessibilityLabel={t("auth.profile.select.manage", "Manage Profiles")}
      >
        <Text style={[s.manageText, { color: theme.colors.textMuted }]}>
          {t("auth.profile.select.manage", "Manage Profiles")}
        </Text>
      </Pressable>

      {pinTarget && (
        <PinPromptDialog
          target={pinTarget}
          fallback={currentProfile}
          onCancel={() => setPinTarget(null)}
          onVerified={async (target, pinToken) => {
            setPinTarget(null);
            setBusy(target.id ?? null);
            const res = await switchProfile(target, pinToken);
            setBusy(null);
            if (res === "ok") router.replace("/(auth)/(tabs)");
          }}
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
      flexGrow: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.xl,
    },
    title: {
      fontSize: 26,
      fontWeight: "700",
      color: t.colors.text,
      marginBottom: 4,
      textAlign: "center",
    },
    subtitle: {
      fontSize: 14,
      color: t.colors.textMuted,
      marginBottom: t.spacing.xl,
      textAlign: "center",
    },
    grid: {
      flexDirection: "row",
      flexWrap: "wrap",
      justifyContent: "center",
      gap: t.spacing.lg,
    },
    card: {
      alignItems: "center",
      width: 120,
      gap: 8,
    },
    avatar: {
      width: 96,
      height: 96,
      borderRadius: 48,
      justifyContent: "center",
      alignItems: "center",
    },
    avatarText: {
      color: t.colors.onAccent,
      fontSize: 38,
      fontWeight: "700",
    },
    nameRow: {
      flexDirection: "row",
      alignItems: "center",
      gap: 6,
      flexWrap: "wrap",
      justifyContent: "center",
    },
    profileName: {
      color: t.colors.text,
      fontSize: 15,
      textAlign: "center",
    },
    kidsBadge: {
      fontSize: 11,
      color: t.colors.textMuted,
    },
    manageBtn: {
      marginTop: t.spacing.xl,
      paddingVertical: 8,
      paddingHorizontal: 16,
    },
    manageText: {
      fontSize: 14,
      fontWeight: "500",
    },
  });
