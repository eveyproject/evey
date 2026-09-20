import { useState, useMemo, useCallback } from "react";
import { View, Text, TextInput, Pressable, Alert, ScrollView, Modal, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { setBaseURL } from "@evey/api";
import { useAuthStore, useServerListStore, serverDisplayName } from "@evey/store";
import { setServerUrl } from "../../src/lib/server-config";
import { initFetcher } from "../../src/lib/fetcher-setup";
import { interpolateFallback } from "../../src/i18n/interpolateFallback";
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

  const [backFocused, setBackFocused] = useState(false);

  // Rename state
  const [renameId, setRenameId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameSaveFocused, setRenameSaveFocused] = useState(false);
  const [renameCancelFocused, setRenameCancelFocused] = useState(false);

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
    if (serverList.length === 1 && activeServerId === id) {
      Alert.alert(t("mobile.servers.cantDeleteOnlyServer", "Can't delete the only saved server"));
      return;
    }
    const name = serverDisplayName(server);
    Alert.alert(
      t("mobile.servers.deleteConfirmTitle", "Delete server?"),
      interpolateFallback(
        t("mobile.servers.deleteConfirmMessage", "{{name}} will be removed from your saved servers. Sign-in tokens for this server will also be cleared.", { name }),
        { name },
      ),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("mobile.servers.delete", "Delete"),
          style: "destructive",
          onPress: () => removeServer(id),
        },
      ],
    );
  }, [servers, serverList.length, activeServerId, removeServer, t]);

  const handleSwitch = useCallback(async (id: string) => {
    const server = servers[id];
    if (!server) return;
    setActiveServer(id);
    setBaseURL(server.url);
    await setServerUrl(server.url);
    await initFetcher();
    const token = useAuthStore.getState().accessToken;
    if (token) {
      router.replace("/(auth)/(tabs)");
    } else {
      router.replace("/login");
    }
  }, [servers, setActiveServer]);

  const s = msStyles(theme);

  return (
    <View style={s.root}>
      <ScrollView style={s.scroll} contentContainerStyle={s.container}>
        {/* Back button - always the first focusable element */}
        <Pressable
          onPress={() => router.back()}
          onFocus={() => setBackFocused(true)}
          onBlur={() => setBackFocused(false)}
          hasTVPreferredFocus
          accessible={true}
          accessibilityLabel={t("common.back", "Back")}
          accessibilityRole="button"
          style={[s.backBtn, { borderColor: backFocused ? theme.colors.accent : "transparent" }]}
        >
          <Text style={[s.backBtnText, { color: theme.colors.accent }]}>{"< "}{t("common.back", "Back")}</Text>
        </Pressable>

        <Text style={s.title}>{t("mobile.servers.manageTitle", "Manage Servers")}</Text>

        {serverList.length === 0 ? (
          <Text style={s.empty}>{t("mobile.servers.noServers", "No saved servers yet")}</Text>
        ) : (
          <View style={s.listSection}>
            {serverList.map((server, index) => {
              const isActive = server.id === activeServerId;
              return (
                <ManageRow
                  key={server.id}
                  server={server}
                  isActive={isActive}
                  isFirst={index === 0}
                  canDelete={!(serverList.length === 1 && isActive)}
                  onSwitch={() => isActive ? router.back() : handleSwitch(server.id)}
                  onRename={() => openRename(server.id)}
                  onDelete={() => handleDelete(server.id)}
                  theme={theme}
                  t={t}
                />
              );
            })}
          </View>
        )}
      </ScrollView>

      {/* Rename modal - centered focus-trap, TV-friendly */}
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
    </View>
  );
}

// Each row is its own component so focus state doesn't leak between rows.
function ManageRow({
  server,
  isActive,
  isFirst,
  canDelete,
  onSwitch,
  onRename,
  onDelete,
  theme,
  t,
}: {
  server: { id: string; url: string; label?: string };
  isActive: boolean;
  isFirst: boolean;
  canDelete: boolean;
  onSwitch: () => void;
  onRename: () => void;
  onDelete: () => void;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const [mainFocused, setMainFocused] = useState(false);
  const [renameFocused, setRenameFocused] = useState(false);
  const [deleteFocused, setDeleteFocused] = useState(false);
  const s = msStyles(theme);

  return (
    <View style={[s.serverRow, !isFirst && { borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border }]}>
      {/* Main tap area: switch to server or go back if already active */}
      <Pressable
        onPress={onSwitch}
        onFocus={() => setMainFocused(true)}
        onBlur={() => setMainFocused(false)}
        accessible={true}
        accessibilityLabel={serverDisplayName(server)}
        accessibilityRole="button"
        style={[
          s.serverMain,
          mainFocused && { backgroundColor: theme.colors.accent + "22" },
          mainFocused && s.serverMainFocused,
        ]}
      >
        <View style={{ flex: 1 }}>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 10 }}>
            <Text style={[s.serverLabel, { color: isActive ? theme.colors.accent : theme.colors.text }]} numberOfLines={1}>
              {serverDisplayName(server)}
            </Text>
            {isActive && (
              <View style={[s.badge, { backgroundColor: theme.colors.accent }]}>
                <Text style={s.badgeText}>{t("mobile.servers.active", "Active")}</Text>
              </View>
            )}
          </View>
          {serverDisplayName(server) !== server.url && (
            <Text style={[s.serverSub, { color: theme.colors.textMuted }]} numberOfLines={1}>
              {server.url}
            </Text>
          )}
        </View>
      </Pressable>

      {/* Action buttons */}
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

const msStyles = (theme: ReturnType<typeof useTheme>) => StyleSheet.create({
  root: { flex: 1, backgroundColor: theme.colors.bg },
  scroll: { flex: 1 },
  container: {
    padding: 80,
    paddingBottom: 120,
  },
  backBtn: {
    alignSelf: "flex-start",
    marginBottom: 24,
    paddingVertical: 10,
    paddingHorizontal: 20,
    borderWidth: 3,
    borderRadius: theme.radii.md,
  },
  backBtnText: {
    fontSize: 18,
    fontWeight: "600",
  },
  title: {
    fontSize: 42,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: 32,
  },
  empty: {
    color: theme.colors.textMuted,
    fontSize: 20,
    textAlign: "center",
    marginTop: 40,
  },
  listSection: {
    borderRadius: theme.radii.md,
    overflow: "hidden",
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
    padding: 28,
    borderWidth: 3,
    borderColor: "transparent",
    borderRadius: theme.radii.md,
  },
  serverMainFocused: {
    borderColor: theme.colors.accent,
  },
  serverLabel: {
    fontSize: 21,
    fontWeight: "600",
  },
  serverSub: {
    fontSize: 17,
    marginTop: 4,
  },
  badge: {
    borderRadius: 4,
    paddingHorizontal: 10,
    paddingVertical: 4,
  },
  badgeText: { color: theme.colors.onAccent, fontSize: 12, fontWeight: "700" },
  actions: {
    flexDirection: "row",
    alignItems: "center",
    paddingRight: 20,
    gap: 10,
  },
  actionBtn: {
    paddingVertical: 9,
    paddingHorizontal: 20,
    borderRadius: theme.radii.sm,
    borderWidth: 3,
  },
  actionBtnText: {
    fontSize: 17,
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
    width: 700,
    borderRadius: theme.radii.md,
    padding: 48,
    gap: 16,
  },
  modalTitle: {
    fontSize: 27,
    fontWeight: "700",
  },
  renameInput: {
    borderWidth: 2,
    borderRadius: theme.radii.md,
    padding: 18,
    fontSize: 21,
  },
  modalActions: {
    flexDirection: "row",
    gap: 12,
    justifyContent: "flex-end",
    marginTop: 8,
  },
  modalBtn: {
    paddingVertical: 10,
    paddingHorizontal: 32,
    borderRadius: theme.radii.md,
    borderWidth: 3,
    alignItems: "center",
  },
  modalBtnPrimary: {
    backgroundColor: theme.colors.accent,
  },
  modalBtnText: {
    fontSize: 20,
  },
});
