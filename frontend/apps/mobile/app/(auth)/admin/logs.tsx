import { useState, useEffect, useRef, useCallback } from "react";
import { View, Text, FlatList, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

type LogLevel = "all" | "debug" | "info" | "warn" | "error";

interface LogLine {
  id: string;
  timestamp: string;
  level: string;
  message: string;
  component?: string;
}

const LEVEL_COLORS: Record<string, string> = {
  debug: "#6b7280",
  info: "#3b82f6",
  warn: "#f59e0b",
  error: "#ef4444",
};

export default function AdminLogsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [level, setLevel] = useState<LogLevel>("all");
  const [live, setLive] = useState(true);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchLogs = useCallback(async () => {
    try {
      const data = await eveyFetch<{ logs: LogLine[] }>({
        url: `/api/v1/admin/logs?limit=100${level !== "all" ? `&level=${level}` : ""}`,
        method: "GET",
      });
      setLogs((data.logs ?? []).map((l, i) => ({ ...l, id: l.id || `${i}` })));
    } catch {}
  }, [level]);

  useEffect(() => {
    fetchLogs();
    if (live) {
      intervalRef.current = setInterval(fetchLogs, 5000);
      return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
    }
    return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
  }, [fetchLogs, live]);

  const filteredLogs = level === "all" ? logs : logs.filter((l) => l.level === level);

  const renderLog = useCallback(({ item }: { item: LogLine }) => {
    const color = LEVEL_COLORS[item.level] ?? theme.colors.textMuted;
    return (
      <View style={styles.logRow}>
        <View style={[styles.levelBadge, { backgroundColor: color }]}>
          <Text style={styles.levelText}>{item.level?.toUpperCase().slice(0, 4)}</Text>
        </View>
        <View style={{ flex: 1 }}>
          <Text style={{ color: theme.colors.text, fontSize: 12, fontFamily: "monospace" }} numberOfLines={3}>{item.message}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10, marginTop: 1 }}>
            {item.component ? `${item.component} · ` : ""}{item.timestamp ? new Date(item.timestamp).toLocaleTimeString() : ""}
          </Text>
        </View>
      </View>
    );
  }, [theme]);

  const levels: LogLevel[] = ["all", "debug", "info", "warn", "error"];

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.logs") || "Logs",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <View style={{ flexDirection: "row", alignItems: "center", paddingHorizontal: 12, paddingVertical: 8, gap: 6 }}>
        {levels.map((l) => (
          <Pressable
            key={l}
            onPress={() => setLevel(l)}
            style={[styles.chip, { backgroundColor: level === l ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.sm }]}
          >
            <Text style={{ color: level === l ? "#fff" : theme.colors.text, fontSize: 12, fontWeight: "600", textTransform: "capitalize" }}>{l}</Text>
          </Pressable>
        ))}
        <Pressable onPress={() => setLive(!live)} style={{ marginLeft: "auto", flexDirection: "row", alignItems: "center", gap: 4 }}>
          <View style={[styles.liveDot, { backgroundColor: live ? "#22c55e" : theme.colors.textMuted }]} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{live ? "Live" : "Paused"}</Text>
        </Pressable>
      </View>

      <FlatList
        data={filteredLogs}
        keyExtractor={(item) => item.id}
        renderItem={renderLog}
        contentContainerStyle={{ paddingBottom: 32 }}
        initialNumToRender={30}
        maxToRenderPerBatch={20}
        windowSize={5}
        ListEmptyComponent={
          <View style={{ alignItems: "center", paddingTop: 60 }}>
            <Text style={{ color: theme.colors.textMuted }}>No logs</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  chip: { paddingHorizontal: 10, paddingVertical: 5 },
  logRow: { flexDirection: "row", alignItems: "flex-start", paddingHorizontal: 12, paddingVertical: 8, gap: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: "rgba(255,255,255,0.05)" },
  levelBadge: { paddingHorizontal: 4, paddingVertical: 1, borderRadius: 3, minWidth: 36, alignItems: "center" },
  levelText: { color: "#fff", fontSize: 9, fontWeight: "700" },
  liveDot: { width: 8, height: 8, borderRadius: 4 },
});
