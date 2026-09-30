import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, TextInput, Switch, Pressable, ActivityIndicator, StyleSheet, RefreshControl } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface RemoteAccessConfig {
  enabled: boolean;
  externalUrl: string;
  serverBaseUrl: string;
  lanNetworks: string[];
  maxRemoteBitrateKbps: number;
  trustedProxies?: string[];
  trustedProxiesFromPanel?: boolean;
  // The server derives these from the current request/config (mirror web).
  lockoutRisk?: boolean;
  published?: boolean;
}

interface NetworkInterface {
  name: string;
  address: string;
}

const CONFIG_KEY = ["/api/v1/admin/remote-access"];
const NETWORK_KEY = ["/api/v1/admin/network-info"];

export default function AdminRemoteAccessScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useQuery<RemoteAccessConfig>({
    queryKey: CONFIG_KEY,
    queryFn: () => eveyFetch<RemoteAccessConfig>({ url: "/api/v1/admin/remote-access", method: "GET" }),
  });

  const { data: netInfo } = useQuery<{ interfaces: NetworkInterface[] }>({
    queryKey: NETWORK_KEY,
    queryFn: () => eveyFetch<{ interfaces: NetworkInterface[] }>({ url: "/api/v1/admin/network-info", method: "GET" }),
  });

  const [enabled, setEnabled] = useState(false);
  const [externalUrl, setExternalUrl] = useState("");
  const [lanNetworks, setLanNetworks] = useState("");
  const [maxBitrate, setMaxBitrate] = useState("0");
  const [trustedProxies, setTrustedProxies] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (data) {
      setEnabled(data.enabled);
      setExternalUrl(data.externalUrl);
      setLanNetworks(data.lanNetworks.join(", "));
      setMaxBitrate(String(data.maxRemoteBitrateKbps));
      setTrustedProxies((data.trustedProxies ?? []).join(", "));
      setDirty(false);
    }
  }, [data]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const networks = lanNetworks.split(",").map((s) => s.trim()).filter(Boolean);
      const proxies = trustedProxies.split(",").map((s) => s.trim()).filter(Boolean);
      await eveyFetch<RemoteAccessConfig>({
        url: "/api/v1/admin/remote-access",
        method: "PUT",
        data: {
          enabled,
          externalUrl,
          lanNetworks: networks,
          maxRemoteBitrateKbps: parseInt(maxBitrate, 10) || 0,
          // An emptied field hands the answer back to EVEY_TRUSTED_PROXIES.
          trustedProxies: proxies,
          clearTrustedProxies: proxies.length === 0,
        },
      });
      toast.success(t("admin.remoteAccess.saved"));
      qc.invalidateQueries({ queryKey: CONFIG_KEY });
      setDirty(false);
    } catch {
      toast.error(t("common.error"));
    } finally {
      setSaving(false);
    }
  }, [enabled, externalUrl, lanNetworks, maxBitrate, trustedProxies, qc, t]);

  const onRefresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const s = formStyles(theme);

  return (
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={false} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.remoteAccess.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: dirty ? () => (
          <Pressable onPress={handleSave} disabled={saving} style={{ marginRight: 8 }}>
            <Text style={{ color: theme.colors.accent, fontWeight: "600", fontSize: 15, opacity: saving ? 0.5 : 1 }}>
              {saving ? t("common.loading") : t("common.save")}
            </Text>
          </Pressable>
        ) : undefined,
      }} />

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <View style={{ padding: 16 }}>
          {/* Lockout-risk warning: enabling/changing this can lock the admin out. */}
          {data?.lockoutRisk ? (
            <View style={{ flexDirection: "row", gap: 10, padding: 12, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.warning, backgroundColor: theme.colors.warning + "1A", marginBottom: 12 }}>
              <Ionicons name="warning-outline" size={20} color={theme.colors.warning} />
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.warning, fontWeight: "700", fontSize: 14 }}>{t("admin.remoteAccess.lockoutRiskTitle", "Lockout risk")}</Text>
                <Text style={{ color: theme.colors.text, fontSize: 13, marginTop: 2 }}>{t("admin.remoteAccess.lockoutRiskMessage", "This change could lock you out of the server.")}</Text>
              </View>
            </View>
          ) : data?.enabled && data?.published ? (
            <View style={{ flexDirection: "row", gap: 10, padding: 12, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border, backgroundColor: theme.colors.surfaceRaised, marginBottom: 12 }}>
              <Ionicons name="information-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }}>{t("admin.remoteAccess.publishedInfo", "Remote access is published and reachable.")}</Text>
            </View>
          ) : null}

          {/* Status badge */}
          <View style={[styles.statusBadge, { backgroundColor: enabled ? theme.colors.success + "20" : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}>
            <Ionicons name={enabled ? "globe-outline" : "lock-closed-outline"} size={20} color={enabled ? theme.colors.success : theme.colors.textMuted} />
            <Text style={{ color: enabled ? theme.colors.success : theme.colors.textMuted, fontWeight: "600", fontSize: 14 }}>
              {enabled ? t("common.enabled") : t("common.disabled")}
            </Text>
          </View>

          {/* Enable toggle */}
          <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 16 }]}>
            <View style={{ flex: 1 }}>
              <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "500" }}>
                {t("admin.remoteAccess.enableRemoteAccess")}
              </Text>
            </View>
            <Switch
              value={enabled}
              onValueChange={(v) => { setEnabled(v); if (!v) setExternalUrl(""); setDirty(true); }}
              trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
            />
          </View>

          {/* External URL */}
          <Text style={s.label}>{t("admin.remoteAccess.externalUrl")}</Text>
          <TextInput
            value={externalUrl}
            onChangeText={(v) => { setExternalUrl(v); setDirty(true); }}
            style={s.input}
            accessibilityLabel={t("admin.remoteAccess.externalUrl")}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={data?.serverBaseUrl || "https://evey.example.com"}
            autoCapitalize="none"
            keyboardType="url"
          />
          {!externalUrl && data?.serverBaseUrl && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 4 }}>
              Using default: {data.serverBaseUrl}
            </Text>
          )}
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 4 }}>
            {t("admin.remoteAccess.externalUrlHttpsHint")}
          </Text>

          {/* Trusted proxies */}
          <Text style={s.label}>{t("admin.remoteAccess.trustedProxies")}</Text>
          <TextInput
            value={trustedProxies}
            onChangeText={(v) => { setTrustedProxies(v); setDirty(true); }}
            style={s.input}
            accessibilityLabel={t("admin.remoteAccess.trustedProxies")}
            placeholderTextColor={theme.colors.textMuted}
            placeholder="172.16.0.0/12"
            autoCapitalize="none"
          />
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 4 }}>
            {t("admin.remoteAccess.trustedProxiesHint")}{" "}
            {data?.trustedProxiesFromPanel
              ? t("admin.remoteAccess.trustedProxiesFromPanel")
              : t("admin.remoteAccess.trustedProxiesFromEnv")}
          </Text>

          {/* LAN Networks */}
          <Text style={s.label}>{t("admin.remoteAccess.lanNetworks")}</Text>
          <TextInput
            value={lanNetworks}
            onChangeText={(v) => { setLanNetworks(v); setDirty(true); }}
            style={s.input}
            accessibilityLabel={t("admin.remoteAccess.lanNetworks")}
            placeholderTextColor={theme.colors.textMuted}
            placeholder="192.168.0.0/16, 10.0.0.0/8"
            autoCapitalize="none"
          />

          {/* Max Bitrate */}
          <Text style={s.label}>{t("admin.remoteAccess.maxRemoteBitrate")}</Text>
          <TextInput
            value={maxBitrate}
            onChangeText={(v) => { setMaxBitrate(v); setDirty(true); }}
            style={s.input}
            keyboardType="numeric"
            accessibilityLabel={t("admin.remoteAccess.maxRemoteBitrate")}
            placeholder="0"
            placeholderTextColor={theme.colors.textMuted}
          />
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 4 }}>0 = unlimited</Text>

          {/* Network Interfaces */}
          {netInfo?.interfaces && netInfo.interfaces.length > 0 && (
            <>
              <Text style={[s.sectionTitle, { marginTop: 24 }]}>{t("admin.remoteAccess.networkInterfaces")}</Text>
              <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                {netInfo.interfaces.map((iface, i) => (
                  <View
                    key={`${iface.name}-${i}`}
                    style={[styles.infoRow, { borderBottomColor: theme.colors.border, borderBottomWidth: i < netInfo.interfaces.length - 1 ? StyleSheet.hairlineWidth : 0 }]}
                  >
                    <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "500" }}>{iface.name}</Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontFamily: "monospace" }}>{iface.address}</Text>
                  </View>
                ))}
              </View>
            </>
          )}
        </View>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statusBadge: { flexDirection: "row", alignItems: "center", gap: 8, padding: 12, alignSelf: "flex-start" },
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  infoCard: { overflow: "hidden" },
  infoRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 12, paddingHorizontal: 16 },
});

const formStyles = (t: ReturnType<typeof useTheme>) => StyleSheet.create({
  label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 16, marginBottom: 6 },
  input: { backgroundColor: t.colors.surface, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
  sectionTitle: { color: t.colors.textMuted, fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginBottom: 8 },
});
