import { View, Text, FlatList, Pressable, Alert, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface Session {
  id: string;
  userId: string;
  username: string;
  mediaTitle: string;
  mediaType: string;
  playMethod: string;
  deviceName: string;
  clientIp: string;
  videoCodec: string;
  audioCodec: string;
  resolution: string;
  progress: number;
  duration: number;
}

function useSessions() {
  return useQuery({
    queryKey: ["admin", "sessions"],
    queryFn: () => eveyFetch<{ sessions: Session[] }>({ url: "/api/v1/admin/sessions", method: "GET" }),
    select: (d) => d.sessions ?? [],
    refetchInterval: 10_000,
  });
}

function formatProgress(s: number): string {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export default function AdminSessionsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: sessions, isLoading, refetch } = useSessions();

  const killSession = useMutation({
    mutationFn: (id: string) => eveyFetch({ url: `/api/v1/admin/sessions/${id}`, method: "DELETE" }),
    onSuccess: async () => {
      toast.success("Session killed");
      await qc.invalidateQueries({ queryKey: ["admin", "sessions"] });
    },
  });

  const handleKill = useCallback((session: Session) => {
    Alert.alert("Kill Session", `Kill ${session.username}'s session?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: "Kill", style: "destructive", onPress: () => killSession.mutate(session.id) },
    ]);
  }, [killSession, t]);

  const renderSession = useCallback(({ item }: { item: Session }) => (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>{item.mediaTitle || "Unknown"}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{item.username} · {item.deviceName || item.clientIp}</Text>
        <View style={{ flexDirection: "row", gap: 8, marginTop: 2 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.playMethod || "Direct"}</Text>
          {item.videoCodec && <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.videoCodec}</Text>}
          {item.resolution && <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.resolution}</Text>}
        </View>
      </View>
      <Pressable onPress={() => handleKill(item)} hitSlop={8}>
        <Ionicons name="stop-circle-outline" size={22} color="#ef4444" />
      </Pressable>
    </View>
  ), [theme, handleKill]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.sessions") || "Sessions",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      <FlatList
        data={sessions ?? []}
        keyExtractor={(s) => s.id}
        renderItem={renderSession}
        contentContainerStyle={{ padding: 16, gap: 8 }}
        refreshing={isLoading}
        onRefresh={refetch}
        ListEmptyComponent={
          <View style={{ alignItems: "center", paddingTop: 60 }}>
            <Ionicons name="play-circle-outline" size={48} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>No active sessions</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
});
