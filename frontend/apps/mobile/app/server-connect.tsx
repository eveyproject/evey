import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet, ScrollView, Linking } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { setBaseURL, isRemoteAccessDisabled, remoteAccessDisabledMessage } from "@evey/api";
import { useAuthStore, useServerListStore, serverDisplayName, isDefaultServerName } from "@evey/store";
import { setServerUrl, getServerUrl, probeServerHealth } from "../src/lib/server-config";
import { initFetcher } from "../src/lib/fetcher-setup";
import { applyServerLanguage, initI18n } from "../src/i18n";
import { parseEveyHealth } from "../src/lib/serverIdentity";
import { isCleartextToPublicHost, confirmInsecureServer } from "../src/lib/privateHost";
import { versionLabel } from "../src/lib/versionLabel";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../src/providers/ThemeProvider";
import { scanLocalNetwork } from "../src/lib/network-scan";

// describeConnectError turns the near-useless "Network request failed" that
// React Native's fetch throws into an actionable, cause-specific hint - the
// difference between a self-hoster giving up and fixing their URL/port/scheme.
function describeConnectError(e: unknown, target: string): string {
  const raw = e instanceof Error ? e.message : String(e);
  if (e instanceof Error && e.name === "AbortError") {
    return `No response from ${target} within 8s.\n\nThe server is probably down or not reachable - check it's running and that your phone is on the same network (or the server is exposed to the internet).`;
  }
  if (/network request failed|failed to fetch|typeerror/i.test(raw)) {
    const hint = /^https:/i.test(target)
      ? "This is an HTTPS address - for a LAN/self-signed server, try http:// instead."
      : "If your server uses HTTPS, switch the address to https://.";
    return `Couldn't reach ${target}.\n\n• Double-check the host and port.\n• ${hint}\n• Make sure the phone is on the same Wi-Fi as the server (or the server is reachable from outside).`;
  }
  if (/certificate|ssl|\btls\b|trust anchor/i.test(raw)) {
    return `TLS/certificate problem reaching ${target}.\n\nFor a self-signed or local server, connect over http:// instead of https://.`;
  }
  if (/^HTTP \d/.test(raw)) {
    return `${target} answered "${raw}" instead of a healthy Evey server.\n\nCheck the URL points to your Evey instance (right host, port and path - no trailing /api).`;
  }
  return `${raw}\n\n(${target})`;
}

// Where novices learn what a server address is and where to find theirs. Points
// at the wiki in the viewer's language when that language is published there
// (only the non-default locales need listing; everything else is the English
// root), so the help reads in the same language as the app.
const WIKI_LOCALES = new Set(["fr"]);
function serverHelpUrl(lang: string): string {
  const code = (lang || "en").split("-")[0];
  const prefix = WIKI_LOCALES.has(code) ? `/${code}` : "";
  return `https://eveyproject.org${prefix}/getting-started/apps/`;
}

export default function ServerConnect() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [discovered, setDiscovered] = useState<{ url: string; version?: string; name?: string }[]>([]);
  const [scanned, setScanned] = useState(false);

  const servers = useServerListStore((s) => s.servers);
  const activeServerId = useServerListStore((s) => s.activeServerId);
  const addServer = useServerListStore((s) => s.addServer);
  const setActiveServer = useServerListStore((s) => s.setActiveServer);
  const setServerName = useServerListStore((s) => s.setServerName);
  const removeServer = useServerListStore((s) => s.removeServer);

  const serverList = useMemo(
    () => Object.values(servers).sort((a, b) => b.lastUsedAt - a.lastUsedAt),
    [servers],
  );
  const hasServers = serverList.length > 0;

  useEffect(() => {
    // Pre-fill URL input from legacy storage (first-run or migration fallback).
    if (!hasServers) {
      getServerUrl()
        .then((saved) => { if (saved) setUrl(saved); })
        .catch((err) => console.warn("server-connect legacy url", err));
    }
  }, [hasServers]);

  // Leaving the screen aborts the subnet sweep (~3000 probes over tens of
  // seconds): without this the scan kept probing and reporting into an
  // unmounted screen for its full duration.
  const scanAbortRef = useRef<AbortController | null>(null);
  useEffect(() => () => scanAbortRef.current?.abort(), []);

  const handleScan = useCallback(async () => {
    scanAbortRef.current?.abort();
    const controller = new AbortController();
    scanAbortRef.current = controller;
    setScanning(true);
    setScanned(false);
    setDiscovered([]);
    const found = new Set<string>();
    await scanLocalNetwork((server) => {
      if (!found.has(server.url)) {
        found.add(server.url);
        setDiscovered((prev) => [...prev, server]);
      }
    }, { signal: controller.signal });
    if (!controller.signal.aborted) {
      setScanning(false);
      // A scan that finds nothing used to leave the button reverting in
      // silence, indistinguishable from a scan that never ran. `scanned`
      // separates "not tried yet" from "tried, found nothing".
      setScanned(true);
    }
  }, []);

  // Switch to an existing saved server.
  const handleSelectServer = useCallback(async (serverId: string) => {
    const server = servers[serverId];
    if (!server) return;
    // Reachability FIRST, exactly like handleConnect and the boot redirect: a
    // saved server that has since moved/died used to send the user straight to
    // /login, where every attempt failed with no explanation because nothing
    // ever reached a server (2026-07-29). Never offer a login we know cannot
    // be delivered.
    setLoading(true);
    const { probe, health } = await probeServerHealth(server.url);
    setLoading(false);
    if (probe !== "ok") {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed", "Connection failed"),
        probe === "not-evey"
          ? t("mobile.serverConnect.notEvey", "{{url}} answered, but it is not an Evey server.").replace("{{url}}", server.url)
          : t("mobile.serverConnect.unreachableSaved", "{{url}} did not answer. Check the address or your connection.").replace("{{url}}", server.url),
      );
      return;
    }
    // The probe just read the server's own name: keep the list current.
    setServerName(serverId, health?.name);
    setActiveServer(serverId);
    setBaseURL(server.url);
    await setServerUrl(server.url);
    await initFetcher();
    // Picking a SAVED/discovered server used to skip i18n entirely: on a first
    // launch (boot found no server, so nothing initialised it) the user landed
    // on /login with raw keys (2026-07-29). Same three calls as handleConnect.
    await initI18n(server.url);
    await applyServerLanguage(server.url);
    // Tokens for this server are rehydrated by the store; check now.
    const token = useAuthStore.getState().accessToken;
    if (token) {
      router.replace("/(auth)/(tabs)");
    } else {
      router.replace("/login");
    }
  }, [servers, setActiveServer, setServerName, t]);

  // Delete a saved server from the connect screen, so a stale address (the
  // server moved, or the URL was a typo) can be removed without signing in
  // first. Mirrors Manage Servers: confirm, and never strand the user by
  // deleting the only server they are currently on.
  const handleDeleteServer = useCallback((id: string) => {
    const server = servers[id];
    if (!server) return;
    if (serverList.length === 1 && activeServerId === id) {
      Alert.alert(t("mobile.servers.cantDeleteOnlyServer", "Can't delete the only saved server"));
      return;
    }
    const name = serverDisplayName(server);
    Alert.alert(
      t("mobile.servers.deleteConfirmTitle", "Delete server?"),
      t("mobile.servers.deleteConfirmMessage", "{{name}} will be removed from your saved servers. Sign-in tokens for this server will also be cleared.", { name }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("mobile.servers.delete", "Delete"), style: "destructive", onPress: () => removeServer(id) },
      ],
    );
  }, [servers, serverList.length, activeServerId, removeServer, t]);

  // Submit a new server URL.
  async function handleConnect() {
    const trimmed = url.trim().replace(/\/+$/, "");
    if (!trimmed) return;
    if (!/^https?:\/\//i.test(trimmed)) {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed", "Connection failed"),
        t("mobile.serverConnect.missingScheme", "Start the address with http:// or https://."),
      );
      return;
    }
    // HTTP to a public host means the password typed on the next screen, and
    // every frame afterwards, crosses the internet readable by anyone on the
    // path. HTTP to the LAN is the normal self-hosted setup and says nothing.
    // Android cannot make this distinction for us - network_security_config
    // knows exact hosts, not address ranges - so the app makes it here.
    if (isCleartextToPublicHost(trimmed) && !(await confirmInsecureServer(t, trimmed))) {
      return;
    }

    setLoading(true);
    // fetch() has no default timeout - abort after 8s so an unreachable host
    // reports a clear timeout instead of hanging on the spinner forever.
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 8000);
    try {
      const res = await fetch(`${trimmed}/healthz`, { signal: ctrl.signal });
      if (!res.ok) {
        // /healthz is exempt from the gate on stock Evey, but a reverse proxy
        // in front can (and, for one beta tester, did) apply its own
        // remote-access rule to every path - a server refusing outsiders is
        // not the same failure as no server being there at all.
        const refused = res.status === 403 && (
          res.headers.get("X-Evey-Remote-Access") === "disabled"
          || isRemoteAccessDisabled(await res.json().catch(() => null))
        );
        if (refused) {
          const { title, body } = remoteAccessDisabledMessage(t);
          Alert.alert(title, body);
          return;
        }
        throw new Error(`HTTP ${res.status}${res.statusText ? ` ${res.statusText}` : ""}`);
      }
      // 200 is not proof: require Evey's own health payload, or the app
      // "connects" to a captive portal and every later call fails silently.
      const health = parseEveyHealth(await res.json().catch(() => null));
      if (!health) {
        Alert.alert(
          t("mobile.serverConnect.connectionFailed", "Connection failed"),
          t("mobile.serverConnect.notEvey", "{{url}} answered, but it is not an Evey server.").replace("{{url}}", trimmed),
        );
        return;
      }
      await setServerUrl(trimmed);
      await initFetcher();
      await initI18n(trimmed);
      await applyServerLanguage(trimmed);
      const saved = addServer(trimmed);
      setServerName(saved.id, health.name);
      setActiveServer(saved.id);
      setBaseURL(trimmed);
      router.replace("/login");
    } catch (e) {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed", "Connection failed"),
        describeConnectError(e, trimmed),
      );
    } finally {
      clearTimeout(timer);
      setLoading(false);
    }
  }

  const inputStyle = useMemo(() => ({
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
  }), [theme]);

  const s = scStyles(theme);

  return (
    <ScrollView style={s.scroll} contentContainerStyle={s.container} keyboardShouldPersistTaps="handled">
      <Text style={s.brand}>Evey</Text>
      <Text style={s.heading}>{t("mobile.serverConnect.title", "Connect to your server")}</Text>

      {/* Saved servers list */}
      {hasServers && (
        <>
          <Text style={[s.sectionLabel, { color: theme.colors.textMuted }]}>
            {t("mobile.servers.yourServers", "Your servers")}
          </Text>
          <View style={s.listSection}>
          {serverList.map((server) => {
            const isActive = server.id === activeServerId;
            const canDelete = !(serverList.length === 1 && isActive);
            return (
              <View key={server.id} style={[s.serverRow, { backgroundColor: theme.colors.surface }]}>
                <Pressable
                  onPress={() => handleSelectServer(server.id)}
                  style={s.serverMain}
                  accessible={true}
                  accessibilityLabel={serverDisplayName(server)}
                  accessibilityRole="button"
                >
                  <Ionicons
                    name="server-outline"
                    size={20}
                    color={isActive ? theme.colors.accent : theme.colors.textMuted}
                    style={{ marginRight: theme.spacing.sm }}
                  />
                  <View style={{ flex: 1 }}>
                    <Text style={[s.serverLabel, { color: theme.colors.text }]} numberOfLines={1}>
                      {serverDisplayName(server)}
                    </Text>
                    {serverDisplayName(server) !== server.url && (
                      <Text style={[s.serverSub, { color: theme.colors.textMuted }]} numberOfLines={1}>
                        {server.url}
                      </Text>
                    )}
                  </View>
                  {isActive && (
                    <View style={[s.badge, { backgroundColor: theme.colors.accent }]}>
                      <Text style={s.badgeText}>{t("mobile.servers.current", "Current")}</Text>
                    </View>
                  )}
                </Pressable>
                <Pressable
                  onPress={() => handleDeleteServer(server.id)}
                  hitSlop={8}
                  style={s.deleteBtn}
                  accessible={true}
                  accessibilityLabel={t("mobile.servers.delete", "Delete")}
                  accessibilityRole="button"
                >
                  <Ionicons name="trash-outline" size={18} color={canDelete ? theme.colors.error : theme.colors.border} />
                </Pressable>
              </View>
            );
          })}
          </View>
        </>
      )}

      {/* Add new server section */}
      <Text style={[s.sectionLabel, { color: theme.colors.textMuted, marginTop: hasServers ? theme.spacing.lg : 0 }]}>
        {hasServers
          ? t("mobile.servers.addAnother", "Add another server")
          : t("mobile.serverConnect.title", "Connect to your server")}
      </Text>
      {!hasServers && (
        <>
          <Text style={[s.hint, { color: theme.colors.textMuted }]}>
            {t("mobile.serverConnect.hint", "Evey runs on a server someone set up for you. Enter its web address - the same one you open in a browser.")}
          </Text>
          <Pressable
            onPress={() => { Linking.openURL(serverHelpUrl(i18n.language)).catch(() => {}); }}
            hitSlop={8}
            accessible={true}
            accessibilityRole="link"
            accessibilityLabel={t("mobile.serverConnect.whereToFind", "Where do I find my server's address?")}
          >
            <Text style={[s.helpLink, { color: theme.colors.accent }]}>
              {t("mobile.serverConnect.whereToFind", "Where do I find my server's address?")}
            </Text>
          </Pressable>
        </>
      )}
      <TextInput
        placeholder="https://your-server.com"
        placeholderTextColor={theme.colors.textMuted}
        value={url}
        onChangeText={setUrl}
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        accessible={true}
        accessibilityLabel={t("mobile.serverConnect.urlLabel", "Server URL")}
        style={inputStyle}
      />
      <Pressable
        onPress={handleConnect}
        disabled={loading}
        accessible={true}
        accessibilityLabel={t("mobile.serverConnect.connect", "Connect")}
        accessibilityRole="button"
        style={[s.connectBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
      >
        {loading ? (
          <ActivityIndicator color={theme.colors.text} />
        ) : (
          <Text style={s.connectBtnText}>{t("mobile.serverConnect.connect", "Connect")}</Text>
        )}
      </Pressable>

      <Pressable
        onPress={handleScan}
        disabled={scanning}
        style={s.scanBtn}
        accessible={true}
        accessibilityLabel={
          scanning
            ? t("admin.scanning", "Scanning...")
            : t("mobile.serverConnect.scanLocalNetwork", "Scan local network")
        }
        accessibilityRole="button"
      >
        {scanning ? (
          <ActivityIndicator color={theme.colors.accent} size="small" />
        ) : (
          <Ionicons name="wifi-outline" size={16} color={theme.colors.accent} />
        )}
        <Text style={[s.scanText, { color: theme.colors.accent }]}>
          {scanning
            ? t("admin.scanning", "Scanning...")
            : t("mobile.serverConnect.scanLocalNetwork", "Scan local network")}
        </Text>
      </Pressable>

      {scanned && !scanning && discovered.length === 0 && (
        <Text
          style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm, marginTop: theme.spacing.sm, textAlign: "center" }}
          accessibilityLiveRegion="polite"
        >
          {t("mobile.serverConnect.noServersFound", "No server found on this network - enter the address below")}
        </Text>
      )}

      {discovered.length > 0 && (
        <View style={{ marginTop: theme.spacing.md }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: theme.spacing.sm }}>
            {t("mobile.serverConnect.serversFound", "{{count}} server(s) found", { count: discovered.length })}
          </Text>
          {discovered.map((sv) => (
            <Pressable
              key={sv.url}
              onPress={() => setUrl(sv.url)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                gap: 8,
                padding: theme.spacing.sm,
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.sm,
                marginBottom: 4,
              }}
              accessible={true}
              accessibilityLabel={t("mobile.serverConnect.useDiscovered", "Use discovered server {{url}}", { url: sv.url })}
              accessibilityRole="button"
            >
              <Ionicons name="server-outline" size={16} color={theme.colors.accent} />
              <View style={{ flex: 1 }}>
                {!isDefaultServerName(sv.name) && (
                  <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>{sv.name}</Text>
                )}
                <Text style={{ color: isDefaultServerName(sv.name) ? theme.colors.text : theme.colors.textMuted, fontSize: isDefaultServerName(sv.name) ? 14 : 12 }}>{sv.url}</Text>
              </View>
              {versionLabel(sv.version) !== "" && (
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{versionLabel(sv.version)}</Text>
              )}
            </Pressable>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const scStyles = (theme: ReturnType<typeof useTheme>) => StyleSheet.create({
  scroll: { flex: 1, backgroundColor: theme.colors.bg },
  container: {
    flexGrow: 1,
    justifyContent: "center",
    padding: theme.spacing.xl,
    paddingBottom: theme.spacing.xl * 2,
  },
  brand: { fontSize: 36, fontWeight: "800", color: theme.colors.accent, marginBottom: 8, letterSpacing: -1 },
  heading: { fontSize: 18, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.lg },
  hint: { fontSize: 13, marginBottom: theme.spacing.lg },
  listSection: {
    borderRadius: theme.radii.md,
    overflow: "hidden",
    marginBottom: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
  },
  serverRow: {
    flexDirection: "row",
    alignItems: "center",
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: theme.colors.border,
  },
  serverMain: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    padding: theme.spacing.md,
  },
  deleteBtn: {
    padding: theme.spacing.md,
  },
  serverLabel: { fontSize: 15, fontWeight: "600" },
  serverSub: { fontSize: 12, marginTop: 2 },
  badge: {
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
    marginLeft: theme.spacing.sm,
  },
  badgeText: { color: theme.colors.onAccent, fontSize: 10, fontWeight: "700" },
  sectionLabel: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginBottom: theme.spacing.sm },
  helpLink: { fontSize: 14, fontWeight: "600", marginBottom: theme.spacing.md, textDecorationLine: "underline" },
  connectBtn: { padding: theme.spacing.md, alignItems: "center" },
  connectBtnText: { color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 },
  scanBtn: { marginTop: theme.spacing.md, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6 },
  scanText: { fontSize: 14 },
});
