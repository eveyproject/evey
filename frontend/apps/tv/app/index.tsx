import { useEffect, useState } from "react";
import { ActivityIndicator, View } from "react-native";
import { Redirect } from "expo-router";
import { useAuthStore, useServerListStore, loadProfileCache, shouldPickProfileOnColdStart } from "@evey/store";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { probeServerHealth } from "../src/lib/server-config";
import { seedOfflineStrings } from "../src/i18n";
import { useTheme } from "../src/providers/ThemeProvider";

export default function EntryRedirect() {
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const serverUrl = useServerUrlNullable();
  const [serverOk, setServerOk] = useState<boolean | undefined>(undefined);

  useEffect(() => {
    if (!serverUrl) return;
    probeServerHealth(serverUrl).then(({ ok, name }) => {
      // Translations live on the server. Knowing here that it is unreachable is
      // what lets the next screen speak the viewer's language instead of
      // waiting for i18next's own fetch to time out.
      if (!ok) seedOfflineStrings();
      // Every launch re-reads the name the server advertises, so an admin
      // rename reaches the server list without the viewer re-adding it.
      const { activeServerId, setServerName } = useServerListStore.getState();
      if (ok && activeServerId) setServerName(activeServerId, name);
      setServerOk(ok);
    });
  }, [serverUrl]);

  // Read once, at launch, from the same cache the phone uses. A television is
  // mains-powered and wired, so there is no offline branch here: unlike the
  // phone it holds no downloads to fall back on.
  const hasSession = !!accessToken && !!profileId;
  const [pickProfile, setPickProfile] = useState<boolean | undefined>(undefined);
  useEffect(() => {
    if (!hasSession) { setPickProfile(false); return; }
    let cancelled = false;
    void loadProfileCache().then((cached) => {
      if (!cancelled) setPickProfile(shouldPickProfileOnColdStart(cached));
    });
    return () => { cancelled = true; };
  }, [hasSession]);

  if (serverUrl === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (!serverUrl) return <Redirect href="/server-connect" />;

  if (serverOk === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  // Say WHY: an unreachable server used to drop the viewer on a screen that
  // looks exactly like a first-time setup, with no hint that the server they
  // already saved is simply asleep.
  if (!serverOk) return <Redirect href="/server-connect?unreachable=1" />;
  if (!accessToken) return <Redirect href="/login" />;
  if (!profileId) return <Redirect href="/profile-select" />;
  // "Who is watching", on a cold start only. This screen is the initial route,
  // so it mounts once per launch and never on a resume. Asked only where
  // somebody could pick wrong: a household with a kids profile AND more than
  // one profile. Same rule and same cache as the phone - it lives in
  // @evey/store precisely so the two cannot drift apart.
  if (pickProfile === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }
  if (pickProfile) return <Redirect href="/profile-select" />;
  return <Redirect href="/(auth)/(tabs)" />;
}
