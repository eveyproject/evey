import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceListTrustedPublishers,
  useAdminPluginServiceAddTrustedPublisher,
  useAdminPluginServiceRemoveTrustedPublisher,
  getAdminPluginServiceListTrustedPublishersQueryKey,
  getAdminPluginServiceListPluginsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast, useDemoStore, demoLocks } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { shortFingerprint } from "./pluginSignature";

// The instance's keyring (mirror web TrustedPublishers): plugins signed by one
// of these keys show as trusted. The key pasted here is a PEM public key.
export function TrustedPublishersCard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const s = styles(theme);
  const addLocked = useDemoStore((st) => demoLocks(st, "/evey.v1.AdminPluginService/AddTrustedPublisher"));
  const removeLocked = useDemoStore((st) => demoLocks(st, "/evey.v1.AdminPluginService/RemoveTrustedPublisher"));
  const { data, isLoading } = useAdminPluginServiceListTrustedPublishers();
  const publishers = data?.publishers ?? [];
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [pem, setPem] = useState("");

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: getAdminPluginServiceListTrustedPublishersQueryKey() });
    void qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
  };

  const add = useAdminPluginServiceAddTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.publishers.added", "Publisher added"));
        setAdding(false);
        setName("");
        setPem("");
        refresh();
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.publishers.addFailed", "Failed to add the publisher"))),
    },
  });
  const remove = useAdminPluginServiceRemoveTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.publishers.removed", "Publisher removed"));
        refresh();
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.publishers.removeFailed", "Failed to remove the publisher"))),
    },
  });

  function confirmRemove(fingerprint: string, publisherName: string) {
    Alert.alert(
      t("admin.plugin.publishers.remove", "Remove"),
      t("admin.plugin.publishers.removeConfirm", { name: publisherName, defaultValue: "Remove the publisher {{name}}?" }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("admin.plugin.publishers.remove", "Remove"), style: "destructive", onPress: () => remove.mutate({ fingerprint }) },
      ],
    );
  }

  return (
    <View style={[s.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.plugin.publishers.title", "Trusted publishers")}</Text>
      <Text style={s.hint}>{t("admin.plugin.publishers.description", "Plugins signed by one of these keys show as trusted.")}</Text>

      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} />
      ) : publishers.length === 0 ? (
        <Text style={[s.hint, { marginTop: 8 }]}>{t("admin.plugin.publishers.empty", "No trusted publisher yet.")}</Text>
      ) : (
        publishers.map((p) => (
          <View key={p.fingerprint} style={s.row}>
            <View style={{ flex: 1, minWidth: 0 }}>
              <View style={{ flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{p.name}</Text>
                {p.seeded ? (
                  <View style={[s.badge, { borderColor: theme.colors.accent }]}>
                    <Text style={{ fontSize: 10, fontWeight: "700", color: theme.colors.accent }}>{t("admin.plugin.publishers.seeded", "Server key")}</Text>
                  </View>
                ) : null}
              </View>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }}>{shortFingerprint(p.fingerprint)}</Text>
              {p.addedAt ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{new Date(p.addedAt).toLocaleDateString()}</Text> : null}
            </View>
            {!p.seeded ? (
              <Pressable
                onPress={() => confirmRemove(p.fingerprint ?? "", p.name ?? "")}
                disabled={removeLocked}
                accessibilityRole="button"
                style={{ opacity: removeLocked ? 0.5 : 1, padding: 6 }}
              >
                <Text style={{ color: theme.colors.error, fontSize: 12, fontWeight: "600" }}>{t("admin.plugin.publishers.remove", "Remove")}</Text>
              </Pressable>
            ) : null}
          </View>
        ))
      )}

      {adding ? (
        <View style={{ marginTop: 10, gap: 8 }}>
          <Text style={s.label}>{t("admin.plugin.publishers.nameLabel", "Name")}</Text>
          <TextInput value={name} onChangeText={setName} style={s.input} placeholderTextColor={theme.colors.textMuted} />
          <Text style={s.label}>{t("admin.plugin.publishers.pemLabel", "Public key (PEM)")}</Text>
          <TextInput
            value={pem}
            onChangeText={setPem}
            multiline
            autoCapitalize="none"
            autoCorrect={false}
            placeholder={t("admin.plugin.publishers.pemPlaceholder", "-----BEGIN PUBLIC KEY-----")}
            placeholderTextColor={theme.colors.textMuted}
            style={[s.input, { minHeight: 90, fontFamily: "monospace", fontSize: 11 }]}
          />
          <View style={{ flexDirection: "row", gap: 8 }}>
            <Pressable onPress={() => setAdding(false)} style={[s.btn, { flex: 1, borderWidth: 1, borderColor: theme.colors.border }]}>
              <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
            </Pressable>
            <Pressable
              onPress={() => add.mutate({ data: { name: name.trim(), publicKeyPem: pem.trim() } })}
              disabled={!name.trim() || !pem.trim() || add.isPending}
              style={[s.btn, { flex: 1, backgroundColor: theme.colors.accent, opacity: !name.trim() || !pem.trim() || add.isPending ? 0.5 : 1 }]}
            >
              {add.isPending ? <ActivityIndicator size="small" color={theme.colors.onAccent} /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugin.publishers.add", "Add a publisher")}</Text>}
            </Pressable>
          </View>
        </View>
      ) : (
        <Pressable
          onPress={() => setAdding(true)}
          disabled={addLocked}
          style={[s.btn, { backgroundColor: theme.colors.accent, marginTop: 12, opacity: addLocked ? 0.5 : 1 }]}
        >
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugin.publishers.add", "Add a publisher")}</Text>
        </Pressable>
      )}
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    card: { padding: 14 },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 2 },
    label: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase" },
    input: { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md, paddingHorizontal: 12, paddingVertical: 10, fontSize: 14, borderWidth: StyleSheet.hairlineWidth, borderColor: theme.colors.border },
    row: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 8, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, marginTop: 4 },
    badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4, borderWidth: 1 },
    btn: { borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center" },
  });
