import { useState, useMemo, useCallback } from "react";
import { View, Text, TextInput, Pressable, Alert, ScrollView, Modal, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { setBaseURL } from "@evey/api";
import { useAuthStore, useServerListStore, toast } from "@evey/store";
import { setServerUrl, probeServer } from "../../src/lib/server-config";
import { initFetcher } from "../../src/lib/fetcher-setup";
import { useTheme } from "../../src/providers/ThemeProvider";

export default function ManageServersScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const servers = useServerListStore((s) => s.servers);
  const activeServerId = useServerListStore((s) => s.activeServerId);
  const setActiveServer = useServerListStore((s) => s.setActiveServer);
  const removeServer = useServerListStore((s) => s.removeServer);
  const renameServer = useServerListStore((s) => s.renameServer);

  const serverList = useMemo(
    () => Object.values(servers).sort((a, b) => b.lastUsedAt - a.lastUsedAt),
    [servers],
  );

  // Rename state
  const [renameId, setRenameId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");

  const openRename = useCallback((id: string) => {
    setRenameId(id);
    setRenameValue(servers[id]?.label ?? "");
  }, [servers]);

  const commitRename = useCallback(() => {
    if (!renameId) return;
    renameServer(renameId, renameValue.trim());
    setRenameId(null);
  }, [renameId, renameValue, renameServer]);

  const handleDelete = useCallback((id: string) => {
    const server = servers[id];
    if (!server) return;
    const isOnlyServer = serverList.length === 1 && activeServerId === id;
    if (isOnlyServer) {
      Alert.alert(t("mobile.servers.cantDeleteOnlyServer", "Can't delete the only saved server"));
      return;
    }
    const name = server.label ?? server.url;
    Alert.alert(
      t("mobile.servers.deleteConfirmTitle", "Delete server?"),
      t("mobile.servers.deleteConfirmMessage", "{{name}} will be removed from your saved servers. Sign-in tokens for this server will also be cleared.", { name }),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("mobile.servers.delete", "Delete"),
          style: "destructive",
          onPress: () => {
            removeServer(id);
          },
        },
      ],
    );
  }, [servers, serverList.length, activeServerId, removeServer, t]);

  const handleSwitch = useCallback(async (id: string) => {
    const server = servers[id];
    if (!server) return;
    // Reachability BEFORE switching: routing to /login against a server that
    // cannot answer produces failures the user cannot interpret, and nothing
    // reaches the real server's logs (2026-07-29). Same gate as
    // server-connect and the boot redirect - no path to /login skips it.
    const probe = await probeServer(server.url);
    if (probe !== "ok") {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed", "Connection failed"),
        probe === "not-evey"
          ? t("mobile.serverConnect.notEvey", "{{url}} answered, but it is not an Evey server.").replace("{{url}}", server.url)
          : t("mobile.serverConnect.unreachableSaved", "{{url}} did not answer. Check the address or your connection.").replace("{{url}}", server.url),
      );
      return;
    }
    setActiveServer(id);
    setBaseURL(server.url);
    await setServerUrl(server.url);
    await initFetcher();
    const name = server.label ?? server.url;
    toast.success(t("mobile.servers.switchedTo", "Switched to {{name}}", { name }));
    // Tokens auto-rehydrate via store; check access token now.
    const token = useAuthStore.getState().accessToken;
    if (token) {
      router.replace("/(auth)/(tabs)");
    } else {
      router.replace("/login");
    }
  }, [servers, setActiveServer, t]);

  const s = msStyles(theme);

  return (
    <View style={s.root}>
      <ScrollView style={s.scroll} contentContainerStyle={s.container}>
        <Text style={s.title}>{t("mobile.servers.manageTitle", "Manage Servers")}</Text>

        {serverList.length === 0 ? (
          <Text style={s.empty}>{t("mobile.servers.noServers", "No saved servers yet")}</Text>
        ) : (
          <View style={s.listSection}>
            {serverList.map((server) => {
              const isActive = server.id === activeServerId;
              return (
                <View key={server.id} style={[s.serverRow, { borderBottomColor: theme.colors.border }]}>
                  {/* Main tap area: switch server */}
                  <Pressable
                    style={s.serverMain}
                    onPress={() => isActive ? router.back() : handleSwitch(server.id)}
                    accessible={true}
                    accessibilityLabel={server.label ?? server.url}
                    accessibilityRole="button"
                  >
                    <Ionicons
                      name="server-outline"
                      size={20}
                      color={isActive ? theme.colors.accent : theme.colors.textMuted}
                      style={{ marginRight: theme.spacing.sm }}
                    />
                    <View style={{ flex: 1 }}>
                      <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                        <Text style={[s.serverLabel, { color: theme.colors.text }]} numberOfLines={1}>
                          {server.label ?? server.url}
                        </Text>
                        {isActive && (
                          <View style={[s.badge, { backgroundColor: theme.colors.accent }]}>
                            <Text style={s.badgeText}>{t("mobile.servers.active", "Active")}</Text>
                          </View>
                        )}
                      </View>
                      {server.label && (
                        <Text style={[s.serverSub, { color: theme.colors.textMuted }]} numberOfLines={1}>
                          {server.url}
                        </Text>
                      )}
                    </View>
                  </Pressable>

                  {/* Action buttons */}
                  <View style={s.actions}>
                    <Pressable
                      onPress={() => openRename(server.id)}
                      hitSlop={8}
                      accessible={true}
                      accessibilityLabel={t("mobile.servers.rename", "Rename")}
                      accessibilityRole="button"
                    >
                      <Ionicons name="pencil-outline" size={18} color={theme.colors.textMuted} />
                    </Pressable>
                    <Pressable
                      onPress={() => handleDelete(server.id)}
                      hitSlop={8}
                      accessible={true}
                      accessibilityLabel={t("mobile.servers.delete", "Delete")}
                      accessibilityRole="button"
                      style={{ marginLeft: theme.spacing.md }}
                    >
                      <Ionicons
                        name="trash-outline"
                        size={18}
                        color={serverList.length === 1 && isActive ? theme.colors.border : theme.colors.error}
                      />
                    </Pressable>
                  </View>
                </View>
              );
            })}
          </View>
        )}

        {/* Add another server */}
        <Pressable
          style={[s.addBtn, { borderColor: theme.colors.accent }]}
          onPress={() => router.push("/server-connect")}
          accessible={true}
          accessibilityLabel={t("mobile.servers.addAnother", "Add another server")}
          accessibilityRole="button"
        >
          <Ionicons name="add-circle-outline" size={18} color={theme.colors.accent} />
          <Text style={[s.addBtnText, { color: theme.colors.accent }]}>
            {t("mobile.servers.addAnother", "Add another server")}
          </Text>
        </Pressable>
      </ScrollView>

      {/* Rename modal */}
      <Modal
        visible={renameId !== null}
        transparent
        animationType="fade"
        onRequestClose={() => setRenameId(null)}
      >
        <Pressable style={s.modalBackdrop} onPress={() => setRenameId(null)}>
          <Pressable style={[s.modalBox, { backgroundColor: theme.colors.surface }]} onPress={() => {}}>
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
              <Pressable onPress={() => setRenameId(null)} style={[s.modalBtn, { borderColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.text }}>{t("common.cancel")}</Text>
              </Pressable>
              <Pressable onPress={commitRename} style={[s.modalBtn, { backgroundColor: theme.colors.accent, borderColor: theme.colors.accent }]}>
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.save", "Save")}</Text>
              </Pressable>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
    </View>
  );
}

const msStyles = (theme: ReturnType<typeof useTheme>) => StyleSheet.create({
  root: { flex: 1, backgroundColor: theme.colors.bg },
  scroll: { flex: 1 },
  container: { padding: theme.spacing.md, paddingBottom: theme.spacing.xl * 2 },
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: theme.spacing.lg,
    marginTop: theme.spacing.md,
  },
  empty: { color: theme.colors.textMuted, fontSize: 15, textAlign: "center", marginTop: theme.spacing.xl },
  listSection: {
    borderRadius: theme.radii.md,
    overflow: "hidden",
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
    backgroundColor: theme.colors.surface,
  },
  serverRow: {
    flexDirection: "row",
    alignItems: "center",
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  serverMain: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    padding: theme.spacing.md,
  },
  serverLabel: { fontSize: 15, fontWeight: "600" },
  serverSub: { fontSize: 12, marginTop: 2 },
  badge: {
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  badgeText: { color: theme.colors.onAccent, fontSize: 10, fontWeight: "700" },
  actions: {
    flexDirection: "row",
    alignItems: "center",
    paddingRight: theme.spacing.md,
  },
  addBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderRadius: theme.radii.md,
    marginTop: theme.spacing.md,
  },
  addBtnText: { fontSize: 15, fontWeight: "600" },
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.5)",
    justifyContent: "center",
    alignItems: "center",
    padding: theme.spacing.xl,
  },
  modalBox: {
    width: "100%",
    borderRadius: theme.radii.md,
    padding: theme.spacing.lg,
    gap: theme.spacing.md,
  },
  modalTitle: { fontSize: 17, fontWeight: "700" },
  renameInput: {
    borderWidth: 1,
    borderRadius: theme.radii.md,
    padding: theme.spacing.sm + 4,
    fontSize: 15,
  },
  modalActions: { flexDirection: "row", gap: theme.spacing.sm, justifyContent: "flex-end" },
  modalBtn: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    alignItems: "center",
  },
});
