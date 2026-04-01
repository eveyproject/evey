import { useState } from "react";
import { View, Text, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

type Device = {
  token: string;
  platform: string;
  createdAt: string;
};

function platformLabel(platform: string): string {
  switch (platform.toLowerCase()) {
    case "ios": return "iOS";
    case "android": return "Android";
    case "web": return "Web";
    default: return platform;
  }
}

function truncateToken(token: string): string {
  if (token.length <= 12) return token;
  return `${token.slice(0, 6)}\u2026${token.slice(-6)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

const DEVICES_QUERY_KEY = ["push-devices"] as const;

export default function DevicesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: DEVICES_QUERY_KEY,
    queryFn: () =>
      eveyFetch<{ devices: Device[] }>({
        url: "/api/v1/push/devices",
        method: "GET",
      }),
  });

  const devices = data?.devices ?? [];

  const revokeMutation = useMutation({
    mutationFn: (token: string) =>
      eveyFetch({
        url: `/api/v1/push/devices/${encodeURIComponent(token)}`,
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
      Alert.alert(t("auth.devices.deviceRevoked"));
    },
    onError: (err: Error) => Alert.alert(t("auth.devices.revokeError"), err.message),
  });

  function handleRevoke(device: Device) {
    Alert.alert(
      t("auth.devices.revokeTitle"),
      t("auth.devices.revokeConfirm", { platform: platformLabel(device.platform) }),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.revoke"),
          style: "destructive",
          onPress: () => revokeMutation.mutate(device.token),
        },
      ],
    );
  }

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: 40 }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("auth.devices.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginHorizontal: theme.spacing.md, marginTop: theme.spacing.lg, marginBottom: theme.spacing.xs }}>
        <Text style={s.sectionTitle}>{t("auth.devices.description")}</Text>
        <Pressable
          onPress={() => void refetch()}
          disabled={isFetching}
          style={{
            backgroundColor: theme.colors.surface,
            borderRadius: theme.radii.md,
            paddingVertical: 6,
            paddingHorizontal: 12,
          }}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
            {t("common.refresh")}
          </Text>
        </Pressable>
      </View>

      {isLoading ? (
        <View style={{ padding: theme.spacing.xl, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      ) : devices.length === 0 ? (
        <View style={[s.section, { padding: theme.spacing.lg, alignItems: "center" }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>
            {t("auth.devices.noDevices")}
          </Text>
        </View>
      ) : (
        <View style={s.section}>
          {devices.map((d) => (
            <View key={d.token} style={s.row}>
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "500" }}>
                  {platformLabel(d.platform)}
                </Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontFamily: "monospace", marginTop: 2 }}>
                  {truncateToken(d.token)}
                </Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
                  {formatDate(d.createdAt)}
                </Text>
              </View>
              <Pressable
                onPress={() => handleRevoke(d)}
                disabled={revokeMutation.isPending}
                style={{
                  backgroundColor: "#ef444420",
                  borderRadius: theme.radii.sm,
                  paddingVertical: 6,
                  paddingHorizontal: 12,
                }}
              >
                <Text style={{ color: "#ef4444", fontSize: 13, fontWeight: "600" }}>
                  {t("common.revoke")}
                </Text>
              </Pressable>
            </View>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    sectionTitle: {
      color: t.colors.textMuted,
      fontSize: 13,
      flex: 1,
    },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      overflow: "hidden",
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      paddingHorizontal: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
  });
