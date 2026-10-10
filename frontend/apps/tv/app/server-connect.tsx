import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet, ScrollView, Modal } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { setBaseURL } from "@evey/api";
import { useAuthStore, useServerListStore, serverDisplayName, isDefaultServerName } from "@evey/store";
import { setServerUrl, getServerUrl, healthName } from "../src/lib/server-config";
import { initFetcher } from "../src/lib/fetcher-setup";
import { applyServerLanguage, initI18n } from "../src/i18n";
import { scanLocalNetwork, type DiscoveredServer } from "../src/lib/network-scan";
import { onResultPicked, onScanStarted, rowClaimsFocus, scanFocusIdle } from "../src/lib/scanFocus";
import { interpolateFallback } from "../src/i18n/interpolateFallback";
import { useTheme } from "../src/providers/ThemeProvider";

// Wiki help for novices, in the viewer's language when it is published there
// (only non-default locales need listing; everything else is the English root).
const WIKI_LOCALES = new Set(["fr"]);
function serverHelpUrl(lang: string): string {
  const code = (lang || "en").split("-")[0];
  const prefix = WIKI_LOCALES.has(code) ? `/${code}` : "";
  return `https://eveyproject.org${prefix}/getting-started/apps/`;
}

export default function ServerConnect() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const { unreachable } = useLocalSearchParams<{ unreachable?: string }>();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [inputFocused, setInputFocused] = useState(false);
  const [connectFocused, setConnectFocused] = useState(false);
  const [manageFocused, setManageFocused] = useState(false);
  const [scanFocused, setScanFocused] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoveredServer[]>([]);
  // Focus handoffs the D-pad needs (rules + tests in lib/scanFocus).
  const [scanFocus, setScanFocus] = useState(scanFocusIdle);
  const urlInputRef = useRef<TextInput>(null);

  // Rename modal state
  const [renameId, setRenameId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameSaveFocused, setRenameSaveFocused] = useState(false);
  const [renameCancelFocused, setRenameCancelFocused] = useState(false);

  const servers = useServerListStore((s) => s.servers);
  const activeServerId = useServerListStore((s) => s.activeServerId);
  const addServer = useServerListStore((s) => s.addServer);
  const setActiveServer = useServerListStore((s) => s.setActiveServer);
  const removeServer = useServerListStore((s) => s.removeServer);
  const renameServer = useServerListStore((s) => s.renameServer);
  const setServerName = useServerListStore((s) => s.setServerName);

  const serverList = useMemo(
    () => Object.values(servers).sort((a, b) => b.lastUsedAt - a.lastUsedAt),
    [servers],
  );
  const hasServers = serverList.length > 0;

  useEffect(() => {
    if (!hasServers) {
      getServerUrl().then((saved) => { if (saved) setUrl(saved); });
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
    setScanFocus(onScanStarted());
    setDiscovered([]);
    const found = new Set<string>();
    await scanLocalNetwork((server) => {
      if (!found.has(server.url)) {
        found.add(server.url);
        setDiscovered((prev) => [...prev, server]);
      }
    }, { signal: controller.signal });
    if (!controller.signal.aborted) setScanning(false);
  }, []);

  // Switch to a saved server (non-destructive: tokens rehydrate from store).
  const handleSelectServer = useCallback(async (serverId: string) => {
    const server = servers[serverId];
    if (!server) return;
    setActiveServer(serverId);
    setBaseURL(server.url);
    await setServerUrl(server.url);
    await initFetcher();
    // Same three calls as handleConnect: picking a saved server used to skip
    // i18n entirely, so a cold start through this path rendered raw keys
    // (auth.login.signIn) - seen on the first release build, 2026-08-02.
    await initI18n(server.url);
    await applyServerLanguage(server.url);
    const token = useAuthStore.getState().accessToken;
    if (token) {
      router.replace("/(auth)/(tabs)");
    } else {
      router.replace("/login");
    }
  }, [servers, setActiveServer]);

  // Connect to a new server URL.
  async function handleConnect() {
    const trimmed = url.trim().replace(/\/+$/, "");
    if (!trimmed) return;

    setLoading(true);
    try {
      const res = await fetch(`${trimmed}/healthz`);
      if (!res.ok) throw new Error(`Server returned ${res.status}`);
      const advertisedName = await healthName(res);
      await setServerUrl(trimmed);
      await initFetcher();
      await initI18n(trimmed);
      await applyServerLanguage(trimmed);
      const saved = addServer(trimmed);
      setServerName(saved.id, advertisedName);
      setActiveServer(saved.id);
      setBaseURL(trimmed);
      router.replace("/login");
    } catch (e) {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed", "Connection failed"),
        e instanceof Error ? e.message : t("mobile.serverConnect.unreachable", "Unable to reach server"),
      );
    } finally {
      setLoading(false);
    }
  }

  const handleDelete = useCallback((id: string) => {
    const server = servers[id];
    if (!server) return;
    if (serverList.length === 1 && activeServerId === id) {
      Alert.alert(t("mobile.servers.cantDeleteOnlyServer", "Can't delete the only saved server"));
      return;
    }
    const name = serverDisplayName(server);
    Alert.alert(
      t("mobile.servers.deleteConfirmTitle", "Delete server?"),
      interpolateFallback(t("mobile.servers.deleteConfirmMessage", "{{name}} will be removed.", { name }), { name }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("mobile.servers.delete", "Delete"), style: "destructive", onPress: () => removeServer(id) },
      ],
    );
  }, [servers, serverList.length, activeServerId, removeServer, t]);

  const openRename = useCallback((id: string) => {
    setRenameId(id);
    setRenameValue(servers[id]?.label ?? "");
  }, [servers]);

  const commitRename = useCallback(() => {
    if (!renameId) return;
    renameServer(renameId, renameValue.trim());
    setRenameId(null);
  }, [renameId, renameValue, renameServer]);

  const s = styles(theme);

  return (
    <ScrollView style={s.scroll} contentContainerStyle={s.container}>
      <Text style={s.brand}>Evey</Text>
      <Text style={s.heading}>{t("mobile.serverConnect.title", "Connect to your server")}</Text>

      {unreachable === "1" ? (
        <Text style={[s.unreachable, { color: theme.colors.textMuted, borderColor: theme.colors.border }]}>
          {t("mobile.serverConnect.serverUnreachable", "Your server did not answer. It may be asleep or off the network.")}
        </Text>
      ) : null}

      {/* Saved servers list */}
      {hasServers && (
        <>
          <Text style={s.sectionLabel}>{t("mobile.servers.yourServers", "Your servers")}</Text>
          <View style={s.listSection}>
          {serverList.map((server, index) => {
            const isActive = server.id === activeServerId;
            return (
              <ServerRow
                key={server.id}
                server={server}
                isActive={isActive}
                isFirst={index === 0}
                onSelect={() => handleSelectServer(server.id)}
                onRename={() => openRename(server.id)}
                onDelete={() => handleDelete(server.id)}
                canDelete={!(serverList.length === 1 && isActive)}
                theme={theme}
                t={t}
              />
            );
          })}
          </View>
        </>
      )}

      {/* Add new server section */}
      <Text style={[s.sectionLabel, { marginTop: hasServers ? 32 : 0 }]}>
        {hasServers
          ? t("mobile.servers.addAnother", "Add another server")
          : t("mobile.serverConnect.title", "Connect to your server")}
      </Text>

      {!hasServers && (
        <>
          <Text style={{ color: theme.colors.textMuted, fontSize: 18, lineHeight: 26, marginBottom: 12 }}>
            {t("mobile.serverConnect.hint", "Evey runs on a server someone set up for you. Enter its web address - the same one you open in a browser.")}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginBottom: 20 }}>
            {t("mobile.serverConnect.whereToFind", "Where do I find my server's address?")} {serverHelpUrl(i18n.language)}
          </Text>
        </>
      )}

      <TextInput
        ref={urlInputRef}
        placeholder="https://your-server.com"
        placeholderTextColor={theme.colors.textMuted}
        value={url}
        onChangeText={setUrl}
        autoCapitalize="none"
        autoCorrect={false}
        onFocus={() => setInputFocused(true)}
        onBlur={() => setInputFocused(false)}
        onSubmitEditing={handleConnect}
        accessible={true}
        accessibilityLabel={t("mobile.serverConnect.urlLabel", "Server URL")}
        style={[
          s.input,
          {
            borderColor: inputFocused ? theme.colors.accent : theme.colors.border,
          },
        ]}
      />

      <Pressable
        onPress={handleConnect}
        disabled={loading}
        onFocus={() => setConnectFocused(true)}
        onBlur={() => setConnectFocused(false)}
        accessible={true}
        accessibilityLabel={t("mobile.serverConnect.connect", "Connect")}
        accessibilityRole="button"
        style={[
          s.connectBtn,
          {
            borderColor: connectFocused ? "#fff" : "transparent",
          },
        ]}
      >
        {loading ? (
          <ActivityIndicator color={theme.colors.onAccent} />
        ) : (
          <Text style={s.connectBtnText}>{t("mobile.serverConnect.connect", "Connect")}</Text>
        )}
      </Pressable>

      {/* Local network discovery - same probe as mobile, D-pad friendly */}
      <Pressable
        onPress={handleScan}
        disabled={scanning}
        onFocus={() => setScanFocused(true)}
        onBlur={() => setScanFocused(false)}
        accessible={true}
        accessibilityLabel={
          scanning
            ? t("admin.scanning", "Scanning...")
            : t("mobile.serverConnect.scanLocalNetwork", "Scan local network")
        }
        accessibilityRole="button"
        style={[
          s.scanBtn,
          {
            borderColor: scanFocused ? theme.colors.accent : theme.colors.border,
          },
        ]}
      >
        {scanning && <ActivityIndicator color={theme.colors.accent} size="small" />}
        <Text style={[s.scanBtnText, { color: scanning ? theme.colors.textMuted : theme.colors.accent }]}>
          {scanning
            ? t("admin.scanning", "Scanning...")
            : t("mobile.serverConnect.scanLocalNetwork", "Scan local network")}
        </Text>
      </Pressable>

      {discovered.length > 0 && (
        <View style={s.discoveredSection}>
          <Text style={s.sectionLabel}>
            {interpolateFallback(
              t("mobile.serverConnect.serversFound", "{{count}} server(s) found", { count: discovered.length }),
              { count: discovered.length },
            )}
          </Text>
          {discovered.map((sv, index) => (
            <DiscoveredRow
              key={sv.url}
              server={sv}
              claimFocus={rowClaimsFocus(scanFocus, index)}
              onSelect={() => {
                setUrl(sv.url);
                setScanFocus(onResultPicked());
                // The address is in the field now; the viewer still has to
                // press Connect, so put the cursor where the change happened.
                urlInputRef.current?.focus();
              }}
              theme={theme}
              t={t}
            />
          ))}
        </View>
      )}

      {/* Manage saved servers button (only when list is non-empty) */}
      {hasServers && (
        <Pressable
          onPress={() => router.push("/(auth)/manage-servers")}
          onFocus={() => setManageFocused(true)}
          onBlur={() => setManageFocused(false)}
          accessible={true}
          accessibilityLabel={t("mobile.servers.manageTitle", "Manage Servers")}
          accessibilityRole="button"
          style={[
            s.manageBtn,
            {
              borderColor: manageFocused ? theme.colors.accent : theme.colors.border,
            },
          ]}
        >
          <Text style={[s.manageBtnText, { color: theme.colors.textMuted }]}>
            {t("mobile.servers.manageTitle", "Manage Servers")}
          </Text>
        </Pressable>
      )}

      {/* Rename modal - centered, TV-friendly (no swipe) */}
      <Modal
        visible={renameId !== null}
        transparent
        animationType="fade"
        onRequestClose={() => setRenameId(null)}
      >
        <View style={s.modalBackdrop}>
          <View style={[s.modalBox, { backgroundColor: theme.colors.surface }]}>
            <Text style={[s.modalTitle, { color: theme.colors.text }]}>
              {t("mobile.servers.rename", "Rename")}
            </Text>
            <TextInput
              value={renameValue}
              onChangeText={setRenameValue}
              placeholder={t("mobile.servers.renamePlaceholder", "Server name")}
              placeholderTextColor={theme.colors.textMuted}
              autoFocus
              style={[s.renameInput, { color: theme.colors.text, borderColor: theme.colors.border, backgroundColor: theme.colors.bg }]}
            />
            <View style={s.modalActions}>
              <Pressable
                onPress={() => setRenameId(null)}
                onFocus={() => setRenameCancelFocused(true)}
                onBlur={() => setRenameCancelFocused(false)}
                accessible={true}
                accessibilityLabel={t("common.cancel")}
                accessibilityRole="button"
                style={[s.modalBtn, { borderColor: renameCancelFocused ? theme.colors.accent : theme.colors.border }]}
              >
                <Text style={[s.modalBtnText, { color: theme.colors.text }]}>{t("common.cancel")}</Text>
              </Pressable>
              <Pressable
                onPress={commitRename}
                onFocus={() => setRenameSaveFocused(true)}
                onBlur={() => setRenameSaveFocused(false)}
                accessible={true}
                accessibilityLabel={t("common.save", "Save")}
                accessibilityRole="button"
                style={[s.modalBtn, s.modalBtnPrimary, { borderColor: renameSaveFocused ? "#fff" : theme.colors.accent }]}
              >
                <Text style={[s.modalBtnText, { color: theme.colors.onAccent, fontWeight: "700" }]}>{t("common.save", "Save")}</Text>
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
    </ScrollView>
  );
}

// A discovered server fills the URL field instead of connecting outright, so
// the user can read and correct the address before committing - same
// behaviour as mobile.
function DiscoveredRow({
  server,
  onSelect,
  claimFocus,
  theme,
  t,
}: {
  server: DiscoveredServer;
  onSelect: () => void;
  // The first row of a fresh scan takes the remote's focus as it appears.
  claimFocus?: boolean;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const [focused, setFocused] = useState(false);
  const s = styles(theme);

  return (
    <Pressable
      onPress={onSelect}
      hasTVPreferredFocus={claimFocus}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      accessible={true}
      accessibilityLabel={interpolateFallback(
        t("mobile.serverConnect.useDiscovered", "Use discovered server {{url}}", { url: server.url }),
        { url: server.url },
      )}
      accessibilityRole="button"
      style={[
        s.discoveredRow,
        {
          backgroundColor: focused ? theme.colors.accent + "22" : theme.colors.surface,
          borderColor: focused ? theme.colors.accent : "transparent",
        },
      ]}
    >
      {!isDefaultServerName(server.name) && (
        <Text style={[s.discoveredUrl, { color: theme.colors.text }]} numberOfLines={1}>
          {server.name}
        </Text>
      )}
      <Text style={[s.discoveredUrl, !isDefaultServerName(server.name) && s.discoveredVersion, { color: isDefaultServerName(server.name) ? theme.colors.text : theme.colors.textMuted }]} numberOfLines={1}>
        {server.url}
      </Text>
      {!!server.version && (
        <Text style={[s.discoveredVersion, { color: theme.colors.textMuted }]}>{server.version}</Text>
      )}
    </Pressable>
  );
}

// Extracted row component to keep focus state isolated per row.
function ServerRow({
  server,
  isActive,
  isFirst,
  onSelect,
  onRename,
  onDelete,
  canDelete,
  theme,
  t,
}: {
  server: { id: string; url: string; label?: string };
  isActive: boolean;
  isFirst: boolean;
  onSelect: () => void;
  onRename: () => void;
  onDelete: () => void;
  canDelete: boolean;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const [rowFocused, setRowFocused] = useState(false);
  const [renameFocused, setRenameFocused] = useState(false);
  const [deleteFocused, setDeleteFocused] = useState(false);
  const s = styles(theme);

  return (
    <View style={[s.serverRow, !isFirst && { borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border }]}>
      {/* Main tap area: switch to this server */}
      <Pressable
        onPress={onSelect}
        onFocus={() => setRowFocused(true)}
        onBlur={() => setRowFocused(false)}
        hasTVPreferredFocus={isFirst && isActive}
        accessible={true}
        accessibilityLabel={serverDisplayName(server)}
        accessibilityRole="button"
        style={[
          s.serverMain,
          rowFocused && { backgroundColor: theme.colors.accent + "22" },
          rowFocused && s.serverMainFocused,
        ]}
      >
        <View style={{ flex: 1 }}>
          <Text style={[s.serverLabel, { color: isActive ? theme.colors.accent : theme.colors.text }]} numberOfLines={1}>
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
            <Text style={s.badgeText}>{t("mobile.servers.active", "Active")}</Text>
          </View>
        )}
      </Pressable>

      {/* Action buttons - individually focusable for D-pad */}
      <View style={s.actions}>
        <Pressable
          onPress={onRename}
          onFocus={() => setRenameFocused(true)}
          onBlur={() => setRenameFocused(false)}
          accessible={true}
          accessibilityLabel={t("mobile.servers.rename", "Rename")}
          accessibilityRole="button"
          style={[s.actionBtn, { borderColor: renameFocused ? theme.colors.accent : "transparent" }]}
        >
          <Text style={[s.actionBtnText, { color: theme.colors.textMuted }]}>
            {t("mobile.servers.rename", "Rename")}
          </Text>
        </Pressable>
        <Pressable
          onPress={onDelete}
          onFocus={() => setDeleteFocused(true)}
          onBlur={() => setDeleteFocused(false)}
          accessible={true}
          accessibilityLabel={t("mobile.servers.delete", "Delete")}
          accessibilityRole="button"
          style={[s.actionBtn, { borderColor: deleteFocused ? (canDelete ? theme.colors.error : theme.colors.border) : "transparent" }]}
        >
          <Text style={[s.actionBtnText, { color: canDelete ? theme.colors.error : theme.colors.border }]}>
            {t("mobile.servers.delete", "Delete")}
          </Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) => StyleSheet.create({
  scroll: { flex: 1, backgroundColor: theme.colors.bg },
  container: {
    flexGrow: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 80,
    paddingBottom: 120,
  },
  brand: {
    fontSize: 54,
    fontWeight: "800",
    color: theme.colors.accent,
    marginBottom: 12,
    letterSpacing: -1,
  },
  unreachable: {
    fontSize: 20,
    lineHeight: 30,
    textAlign: "center",
    borderWidth: 1,
    borderRadius: 12,
    paddingVertical: 11,
    paddingHorizontal: 24,
    marginBottom: 24,
  },
  heading: {
    fontSize: 22,
    fontWeight: "bold",
    color: theme.colors.text,
    marginBottom: 40,
  },
  listSection: {
    width: 700,
    borderRadius: theme.radii.md,
    overflow: "hidden",
    marginBottom: 8,
    borderWidth: 2,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.surface,
  },
  serverRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  serverMain: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    padding: 24,
    borderWidth: 3,
    borderColor: "transparent",
    borderRadius: theme.radii.md,
  },
  serverMainFocused: {
    borderColor: theme.colors.accent,
  },
  serverLabel: {
    fontSize: 20,
    fontWeight: "600",
  },
  serverSub: {
    fontSize: 15,
    marginTop: 4,
  },
  badge: {
    borderRadius: 4,
    paddingHorizontal: 10,
    paddingVertical: 4,
    marginLeft: 12,
  },
  badgeText: { color: theme.colors.onAccent, fontSize: 12, fontWeight: "700" },
  actions: {
    flexDirection: "row",
    alignItems: "center",
    paddingRight: 16,
    gap: 8,
  },
  actionBtn: {
    paddingVertical: 10,
    paddingHorizontal: 16,
    borderRadius: theme.radii.sm,
    borderWidth: 2,
  },
  actionBtnText: {
    fontSize: 17,
    fontWeight: "600",
  },
  sectionLabel: {
    width: 700,
    fontSize: 15,
    fontWeight: "600",
    textTransform: "uppercase",
    color: theme.colors.textMuted,
    marginBottom: 16,
  },
  input: {
    width: 700,
    fontSize: 21,
    padding: 20,
    borderWidth: 3,
    marginBottom: 20,
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
  },
  connectBtn: {
    width: 700,
    padding: 20,
    alignItems: "center",
    backgroundColor: theme.colors.accent,
    borderRadius: theme.radii.md,
    borderWidth: 3,
  },
  connectBtnText: {
    color: theme.colors.onAccent,
    fontWeight: "700",
    fontSize: 21,
  },
  scanBtn: {
    width: 700,
    marginTop: 24,
    padding: 20,
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 10,
    borderWidth: 3,
    borderRadius: theme.radii.md,
  },
  scanBtnText: {
    fontSize: 18,
    fontWeight: "600",
  },
  discoveredSection: {
    width: 700,
    marginTop: 24,
  },
  discoveredRow: {
    flexDirection: "row",
    alignItems: "center",
    padding: 20,
    marginBottom: 8,
    borderWidth: 3,
    borderRadius: theme.radii.md,
  },
  discoveredUrl: {
    flex: 1,
    fontSize: 18,
    fontWeight: "600",
  },
  discoveredVersion: {
    fontSize: 15,
    marginLeft: 12,
  },
  manageBtn: {
    width: 700,
    marginTop: 24,
    padding: 20,
    alignItems: "center",
    borderWidth: 3,
    borderRadius: theme.radii.md,
  },
  manageBtnText: {
    fontSize: 18,
    fontWeight: "600",
  },
  // Rename modal
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.7)",
    justifyContent: "center",
    alignItems: "center",
    padding: 80,
  },
  modalBox: {
    width: 600,
    borderRadius: theme.radii.md,
    padding: 40,
    gap: 14,
  },
  modalTitle: {
    fontSize: 24,
    fontWeight: "700",
  },
  renameInput: {
    borderWidth: 2,
    borderRadius: theme.radii.md,
    padding: 16,
    fontSize: 20,
  },
  modalActions: {
    flexDirection: "row",
    gap: 12,
    justifyContent: "flex-end",
    marginTop: 8,
  },
  modalBtn: {
    paddingVertical: 10,
    paddingHorizontal: 28,
    borderRadius: theme.radii.md,
    borderWidth: 3,
    alignItems: "center",
  },
  modalBtnPrimary: {
    backgroundColor: theme.colors.accent,
  },
  modalBtnText: {
    fontSize: 18,
  },
});
