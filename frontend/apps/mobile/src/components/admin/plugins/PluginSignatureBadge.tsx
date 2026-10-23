import { useState } from "react";
import { View, Text, Pressable, Modal, TextInput, ActivityIndicator, StyleSheet } from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceAddTrustedPublisher,
  getAdminPluginServiceListTrustedPublishersQueryKey,
  getAdminPluginServiceListPluginsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast, useDemoStore, demoLocks } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { type PluginSignatureInfo, shortFingerprint, signatureKind } from "./pluginSignature";

// The signature verdict on a plugin row, with the "Trust this key" action for a
// valid signature whose key is not in the trusted-publisher list yet.
export function PluginSignatureBadge({ signature, canTrust = true }: { signature?: PluginSignatureInfo; canTrust?: boolean }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const locked = useDemoStore((st) => demoLocks(st, "/evey.v1.AdminPluginService/AddTrustedPublisher"));
  const [asking, setAsking] = useState(false);
  const [name, setName] = useState("");
  const kind = signatureKind(signature);
  const fp = signature?.keyFingerprint ?? "";

  const add = useAdminPluginServiceAddTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.signature.trustedToast", "Key trusted"));
        setAsking(false);
        setName("");
        void qc.invalidateQueries({ queryKey: getAdminPluginServiceListTrustedPublishersQueryKey() });
        void qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.signature.trustFailed", "Failed to trust the key"))),
    },
  });

  const [label, color] =
    kind === "trusted"
      ? [t("admin.plugin.signature.signedBy", { name: signature?.publisher, defaultValue: "Signed by {{name}}" }), theme.colors.success]
      : kind === "untrusted"
        ? [t("admin.plugin.signature.signedUnknown", { fp: shortFingerprint(fp), defaultValue: "Signed - unknown key {{fp}}" }), theme.colors.warning]
        : kind === "invalid"
          ? [t("admin.plugin.signature.invalid", "Invalid signature"), theme.colors.error]
          : [t("admin.plugin.signature.unsigned", "Unsigned"), theme.colors.textMuted];

  return (
    <>
      <View style={[styles.badge, { borderColor: color }]} accessibilityLabel={fp || undefined}>
        <Text style={{ fontSize: 10, fontWeight: "700", color }}>{label}</Text>
      </View>
      {kind === "untrusted" && canTrust ? (
        <Pressable
          onPress={() => setAsking(true)}
          disabled={locked}
          accessibilityRole="button"
          style={[styles.badge, { borderColor: theme.colors.accent, opacity: locked ? 0.5 : 1 }]}
        >
          <Text style={{ fontSize: 10, fontWeight: "700", color: theme.colors.accent }}>{t("admin.plugin.signature.trustKey", "Trust this key")}</Text>
        </Pressable>
      ) : null}

      <Modal visible={asking} transparent animationType="fade" onRequestClose={() => setAsking(false)}>
        <View style={styles.backdrop}>
          <View style={{ backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, padding: theme.spacing.md, gap: theme.spacing.sm }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700" }}>{t("admin.plugin.signature.trustTitle", "Trust this key")}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }}>{fp}</Text>
            <TextInput
              value={name}
              onChangeText={setName}
              placeholder={t("admin.plugin.signature.trustNameLabel", "Publisher name")}
              placeholderTextColor={theme.colors.textMuted}
              style={{ backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md, padding: theme.spacing.sm + 2, borderWidth: 1, borderColor: theme.colors.border, fontSize: 13 }}
            />
            <View style={{ flexDirection: "row", gap: theme.spacing.sm }}>
              <Pressable onPress={() => setAsking(false)} style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}>
                <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
              </Pressable>
              <Pressable
                onPress={() => add.mutate({ data: { name: name.trim(), fingerprint: fp } })}
                disabled={!name.trim() || add.isPending}
                style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, backgroundColor: theme.colors.accent, opacity: !name.trim() || add.isPending ? 0.5 : 1 }}
              >
                {add.isPending ? (
                  <ActivityIndicator color={theme.colors.onAccent} size="small" />
                ) : (
                  <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugin.signature.trustConfirm", "Trust")}</Text>
                )}
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4, borderWidth: 1 },
  backdrop: { flex: 1, backgroundColor: "rgba(0,0,0,0.5)", justifyContent: "center", padding: 24 },
});
