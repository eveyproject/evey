import { useEffect, useState } from "react";
import { ActivityIndicator, View } from "react-native";
import { Redirect } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { pingServer } from "../src/lib/server-config";
import { useTheme } from "../src/providers/ThemeProvider";
import { useOnlineStatus } from "../src/hooks/useOnlineStatus";
import { loadProfileCache, shouldPickProfileOnColdStart } from "@evey/store";

export default function EntryRedirect() {
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const serverUrl = useServerUrlNullable();
  const [serverOk, setServerOk] = useState<boolean | undefined>(undefined);

  // A saved session skips the probe entirely (see the redirect below for why),
  // so don't spend a 5s round-trip - and a 5s spinner - on an answer nothing
  // reads.
  const hasSession = !!accessToken && !!profileId;
  const online = useOnlineStatus();

  useEffect(() => {
    if (!serverUrl || hasSession) return;
    void pingServer(serverUrl).then(setServerOk);
  }, [serverUrl, hasSession]);

  // "Who is watching" on a COLD START, and only there. This screen is the
  // initial route, so it mounts once per process launch and never on a resume
  // from the background - which is exactly the distinction we want. Coming
  // back to the app after reading a notification is the adult returning;
  // picking the phone up the next morning is when it may have changed hands.
  //
  // Asked only when somebody could pick wrong: a household with a kids profile
  // AND more than one profile. Everyone else walks straight in, which is the
  // whole reason this is not shown unconditionally.
  //
  // Skipped when offline: the picker cannot lead anywhere new there (switching
  // profiles is a server round-trip), and the session keeps the profile it was
  // left on - which sees only its own downloads. Turning the one screen a
  // signed-in user must cross into a network dependency is how the app stopped
  // working on a plane the last time.
  const [pickProfile, setPickProfile] = useState<boolean | undefined>(undefined);
  useEffect(() => {
    if (!hasSession) { setPickProfile(false); return; }
    if (!online) { setPickProfile(false); return; }
    let cancelled = false;
    void loadProfileCache().then((cached) => {
      if (!cancelled) setPickProfile(shouldPickProfileOnColdStart(cached));
    });
    return () => { cancelled = true; };
  }, [hasSession, online]);

  // Still loading URL from SecureStore
  if (serverUrl === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  // No saved URL → server-connect
  if (!serverUrl) return <Redirect href="/server-connect" />;

  // An already-authenticated session goes straight in, reachable server or not.
  // The probe is a diagnosis for the CONNECT flow; as a GATE on a returning
  // user it was a dead end: offline (plane, tunnel, no data) it always fails,
  // and /server-connect cannot be completed without a server, so the app was
  // unusable exactly when its downloads were the point. Inside, every screen
  // has its own error state, the Downloads screen and local playback need no
  // server at all, and a genuinely moved server is still fixable from
  // Settings → the server list.
  if (hasSession) {
    // Undefined = the cached list has not been read yet. It is a local read,
    // so this is a frame or two, not a spinner anyone sees.
    if (pickProfile === undefined) {
      return (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      );
    }
    if (pickProfile) return <Redirect href="/profile-select" />;
    return <Redirect href="/(auth)/(tabs)" />;
  }

  // URL saved, pinging server...
  if (serverOk === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  // Server unreachable → server-connect (will show saved URL)
  if (!serverOk) return <Redirect href="/server-connect" />;

  // Server OK but no auth → login
  if (!accessToken) return <Redirect href="/login" />;

  // Server OK + auth but no profile → profile select
  if (!profileId) return <Redirect href="/profile-select" />;

  // All good → home
  return <Redirect href="/(auth)/(tabs)" />;
}
