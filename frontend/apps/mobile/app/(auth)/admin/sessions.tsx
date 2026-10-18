import { View, Text, FlatList, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useCallback, useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, type V1ActiveSession } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { SessionCard } from "../../../src/components/admin/SessionCard";
import { confirmStopSession } from "../../../src/components/admin/confirmStopSession";
import { LoadFailed } from "../../../src/components/LoadFailed";

const REFRESH_INTERVAL = 5_000;
const QUERY_KEY = ["admin", "sessions", "active"];

function useSessions() {
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => eveyFetch<{ sessions: V1ActiveSession[] }>({ url: "/api/v1/admin/sessions/active", method: "GET" }),
    select: (d) => d.sessions ?? [],
    refetchInterval: REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
  });
}

function RefreshStatus({ dataUpdatedAt, theme, t }: {
  dataUpdatedAt: number;
  theme: ReturnType<typeof useTheme>;
  t: (k: string, d?: string | Record<string, unknown>) => string;
}) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);
  const updatedAt = dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : "";
  return (
    <Text style={{ color: theme.colors.textMuted, fontSize: 11, textAlign: "center", paddingVertical: 4 }}>
      {t("admin.sessions.refreshStatus", { time: updatedAt, seconds: Math.round(REFRESH_INTERVAL / 1000) })}
    </Text>
  );
}

export default function AdminSessionsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: sessions, isLoading, isError, refetch, dataUpdatedAt } = useSessions();

  const killSession = useMutation({
    mutationFn: (sessionId: string) => eveyFetch({ url: `/api/v1/admin/sessions/${sessionId}`, method: "DELETE" }),
    onSuccess: async () => {
      toast.success(t("admin.sessions.sessionStopped", "Session stopped"));
      await qc.invalidateQueries({ queryKey: QUERY_KEY });
    },
    onError: () => toast.error(t("admin.sessions.failedStop", "Failed to stop session")),
  });

  const sendCommand = useMutation({
    mutationFn: ({ sessionId, command }: { sessionId: string; command: string }) =>
      eveyFetch({
        url: `/api/v1/admin/sessions/${sessionId}/command`,
        method: "POST",
        headers: { "Content-Type": "application/json" },
        data: { command },
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: QUERY_KEY });
    },
    onError: () => toast.error(t("admin.sessions.commandFailed", "Failed to send command")),
  });

  const handleKill = useCallback((session: V1ActiveSession) => {
    confirmStopSession(t, session, () => killSession.mutate(session.sessionId ?? ""));
  }, [killSession, t]);

  const handleCommand = useCallback((sessionId: string, command: string) => {
    sendCommand.mutate({ sessionId, command });
  }, [sendCommand]);

  const renderSession = useCallback(({ item }: { item: V1ActiveSession }) => (
    <SessionCard
      session={item}
      theme={theme}
      t={t as never}
      onKill={handleKill}
      onCommand={handleCommand}
    />
  ), [theme, handleKill, handleCommand, t]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.sessions.title", "Sessions"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      <FlatList
        data={sessions ?? []}
        keyExtractor={(s) => s.sessionId ?? ""}
        renderItem={renderSession}
        contentContainerStyle={{ padding: 16, gap: 10 }}
        refreshing={isLoading}
        onRefresh={refetch}
        ListHeaderComponent={
          sessions && sessions.length > 0 ? (
            <View style={styles.header}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                {t("admin.sessions.activeCount", { count: sessions.length })}
              </Text>
              <RefreshStatus dataUpdatedAt={dataUpdatedAt} theme={theme} t={t as never} />
            </View>
          ) : null
        }
        ListEmptyComponent={
          isError ? (
            <LoadFailed onRetry={() => void refetch()} />
          ) : (
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="play-circle-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>
                {t("admin.sessions.noSessions", "No active sessions")}
              </Text>
            </View>
          )
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 4 },
});
