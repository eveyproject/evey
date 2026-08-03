import { useEffect, useState, useCallback } from "react";
import { View, Text, ActivityIndicator, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, userServiceRefreshToken } from "@evey/api";
import type { V1Profile } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { ScreenState } from "../src/components/ScreenState";
import { useTheme } from "../src/providers/ThemeProvider";
import { FocusableCard } from "../src/components/FocusableCard";
import { ParentalPINModal } from "../src/components/ParentalPINModal";

type SwitchOutcome = "ok" | "pin" | "fail";

// Exported for unit testing: the server signals "this switch needs a
// parental PIN proof" via a bare 403 (RefreshToken, internal/app/user/auth.go
// RefreshToken) - a 403 received on a retry that ALREADY carried a pinToken
// means the proof itself was wrong/expired, which is a hard failure, not a
// second PIN prompt loop.
export function classifySwitchFailure(err: unknown, hadPinToken: boolean): "pin" | "fail" {
  const status = (err as { httpStatus?: number } | undefined)?.httpStatus;
  if (!hadPinToken && status === 403) return "pin";
  return "fail";
}

export default function ProfileSelect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const setProfile = useAuthStore((s) => s.setProfile);
  const activeProfileId = useAuthStore((s) => s.profileId);

  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id;

  const {
    data: profilesData,
    isLoading,
    isError,
    refetch,
  } = useUserServiceListProfiles(userId ?? "", {
    query: { enabled: !!userId },
  });

  const profiles = profilesData?.profiles ?? [];

  const [busy, setBusy] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);

  useEffect(() => {
    if (!isLoading && profiles.length === 1 && profiles[0].id) {
      setProfile(profiles[0].id);
      router.replace("/(auth)/(tabs)");
    }
  }, [isLoading, profiles, setProfile]);

  // Applies the switch only once the SERVER has confirmed it (RefreshToken).
  // The JWT carries the profile_id every parental gate keys off, so a local
  // setProfile() without a matching server-side grant would leave the app
  // showing a restricted-content screen for a profile the token doesn't
  // actually authorise - the historical bug (setProfile only, no round trip).
  const switchProfile = useCallback(async (target: V1Profile, pinToken?: string): Promise<SwitchOutcome> => {
    if (!target.id) return "fail";
    try {
      const resp = await userServiceRefreshToken({ profileId: target.id, pinToken });
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      await qc.invalidateQueries();
      return "ok";
    } catch (err) {
      return classifySwitchFailure(err, !!pinToken);
    }
  }, [qc]);

  async function handleSelect(profileId: string) {
    if (busy) return;
    const target = profiles.find((p) => p.id === profileId);
    if (!target?.id) return;
    setBusy(target.id);
    const res = await switchProfile(target);
    setBusy(null);
    if (res === "ok") router.replace("/(auth)/(tabs)");
    else if (res === "pin") setPinTarget(target);
    else toast.error(t("auth.profile.switchFailed", "Could not switch profile"));
  }

  async function handlePinVerified(pinToken?: string) {
    const target = pinTarget;
    if (!target?.id) return;
    setPinTarget(null);
    setBusy(target.id);
    const res = await switchProfile(target, pinToken);
    setBusy(null);
    if (res === "ok") router.replace("/(auth)/(tabs)");
    else toast.error(t("auth.profile.switchFailed", "Could not switch profile"));
  }

  // A failed list left `profiles` empty, which fell into the spinner below and
  // stayed there for ever - the exact "frozen app" this screen is least able to
  // afford, since it stands between the viewer and everything else. The
  // no-profile case lands here too: it should not happen, but spinning at a
  // dead end is not the way to say so.
  if (isError || (!isLoading && profiles.length === 0)) {
    return <ScreenState error onRetry={() => void refetch()} />;
  }

  // One profile is a transient: the effect above has already redirected.
  if (isLoading || profiles.length === 1) {
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
          // `??` lets an empty name through, and ""[0] is undefined: the
          // optional call is what keeps a nameless profile from taking the
          // whole picker down. Same guard as mobile.
          const initial = (profile.name ?? "?")[0]?.toUpperCase() ?? "?";
          return (
            <FocusableCard
              key={profile.id}
              onPress={() => profile.id && void handleSelect(profile.id)}
              hasTVPreferredFocus={index === 0}
              accessibilityLabel={profile.name}
              style={styles.card}
              focusedStyle={{ borderColor: theme.colors.accent }}
            >
              <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
                <Text style={[styles.avatarText, { color: theme.colors.onAccent }]}>{initial}</Text>
              </View>
              <Text style={[styles.profileName, { color: theme.colors.text }]} numberOfLines={1}>
                {profile.name}
              </Text>
              {busy === profile.id && <ActivityIndicator color={theme.colors.accent} size="small" />}
            </FocusableCard>
          );
        })}
      </View>
      {pinTarget?.id ? (
        <ParentalPINModal
          visible
          profileId={pinTarget.id}
          fallbackProfileId={activeProfileId ?? undefined}
          onClose={() => setPinTarget(null)}
          onVerified={(pinToken) => void handlePinVerified(pinToken)}
        />
      ) : null}
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
    fontSize: 48,
    fontWeight: "bold",
  },
  profileName: {
    fontSize: 22,
    textAlign: "center",
  },
});
