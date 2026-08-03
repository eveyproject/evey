import { View, Text, FlatList, Pressable, Alert, TextInput, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useCallback, useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { TrickplayPreview } from "../../../src/components/TrickplayPreview";

interface ActiveSession {
  sessionId: string;
  profileName: string;
  mediaTitle: string;
  mediaType: string;
  mediaItemId: string;
  progress: number;
  durationMs: string;
  positionMs: string;
  startedAt: string;
  deviceName: string;
  deviceType: string;
  clientIp: string;
  playMethod: string;
  videoCodec: string;
  audioCodec: string;
  resolution: string;
  bandwidthBps: string;
  transcodeInfo: string;
  seriesTitle: string;
  seasonNumber: number;
  episodeNumber: number;
  isPaused: boolean;
  videoDecision: string;
  audioDecision: string;
  castDeviceName: string;
}

const REFRESH_INTERVAL = 5_000;
const QUERY_KEY = ["admin", "sessions", "active"];

function useSessions() {
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => eveyFetch<{ sessions: ActiveSession[] }>({ url: "/api/v1/admin/sessions/active", method: "GET" }),
    select: (d) => d.sessions ?? [],
    refetchInterval: REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
  });
}

function formatMs(ms: number): string {
  if (!ms) return "0:00";
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function formatElapsed(startedAt: string): string {
  const diff = Date.now() - new Date(startedAt).getTime();
  return formatMs(diff);
}

function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

// trackEngine derives the serving tool per track: the copy rung is served by
// mkvgo's on-demand packager (default), any re-encode by ffmpeg.
function trackEngine(decision: string): string {
  return decision === "transcode" ? "ffmpeg" : "mkvgo";
}

function playMethodLabel(method: string, t: (k: string) => string): string {
  switch (method) {
    case "direct_play": return t("admin.sessions.directPlay");
    case "direct_stream": return t("admin.sessions.directStream");
    case "transcode": return t("admin.sessions.transcode");
    default: return method || "Direct";
  }
}

// TrackBadges renders the per-track serving detail (copy vs transcode, served
// codec, engine) so an operator can tell a copy rung from a real transcode.
function TrackBadges({ session, theme, t }: {
  session: ActiveSession;
  theme: ReturnType<typeof useTheme>;
  t: (k: string, d?: string | Record<string, unknown>) => string;
}) {
  const vDec = session.videoDecision ?? "";
  const aDec = session.audioDecision ?? "";
  if (!vDec && !aDec) return null;

  const rows: Array<{ id: string; icon: "film-outline" | "volume-high-outline"; kind: string; decision: string; detail: string }> = [];
  if (vDec) rows.push({
    id: "video",
    icon: "film-outline",
    kind: t("admin.sessions.track.video"),
    decision: vDec,
    detail: [session.videoCodec, session.resolution].filter(Boolean).join(", "),
  });
  if (aDec) rows.push({
    id: "audio",
    icon: "volume-high-outline",
    kind: t("admin.sessions.track.audio"),
    decision: aDec,
    detail: session.audioCodec ?? "",
  });

  return (
    <View style={{ gap: 2, marginTop: 2 }}>
      {rows.map((r) => {
        const isTranscode = r.decision === "transcode";
        const decLabel = t(`admin.sessions.track.${r.id}${isTranscode ? "Transcoded" : "Copied"}`);
        return (
          <View key={r.id} style={{ flexDirection: "row", alignItems: "center", gap: 4 }}>
            <Ionicons name={r.icon} size={11} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.text, fontSize: 10 }}>{r.kind}:</Text>
            <View style={[styles.badge, { backgroundColor: isTranscode ? `${theme.colors.warning}20` : `${theme.colors.success}20` }]}>
              <Text style={{ fontSize: 9, fontWeight: "600", color: isTranscode ? theme.colors.warning : theme.colors.success }}>
                {decLabel}
              </Text>
            </View>
            {r.detail ? <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>({r.detail})</Text> : null}
            <Text style={{ color: theme.colors.textMuted, fontSize: 10, opacity: 0.7 }}>· {trackEngine(r.decision)}</Text>
          </View>
        );
      })}
    </View>
  );
}

function SessionCard({ session, theme, t, onKill, onCommand }: {
  session: ActiveSession;
  theme: ReturnType<typeof useTheme>;
  t: (k: string, d?: string | Record<string, unknown>) => string;
  onKill: (s: ActiveSession) => void;
  onCommand: (sessionId: string, command: string) => void;
}) {
  const [showMessage, setShowMessage] = useState(false);
  const [message, setMessage] = useState("");

  const progress = Math.min(100, Math.max(0, session.progress ?? 0));
  const durationMs = parseInt(session.durationMs ?? "0");
  const positionMs = parseInt(session.positionMs ?? "0");
  const bandwidthBps = parseInt(session.bandwidthBps ?? "0");
  const remaining = durationMs > 0 ? formatMs(Math.max(0, durationMs - positionMs)) : null;

  const hasSeriesInfo = session.seriesTitle && session.seasonNumber && session.episodeNumber;
  const seriesLabel = hasSeriesInfo
    ? `S${String(session.seasonNumber).padStart(2, "0")}E${String(session.episodeNumber).padStart(2, "0")}`
    : null;

  const handleSendMessage = () => {
    if (!message.trim()) return;
    onCommand(session.sessionId, `message:${message.trim()}`);
    setMessage("");
    setShowMessage(false);
  };

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      {/* Progress bar */}
      <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
        <View style={[styles.progressFill, { width: `${progress}%`, backgroundColor: theme.colors.accent }]} />
      </View>
      <View style={styles.progressLabels}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{formatMs(positionMs)}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
          {remaining
            ? `-${remaining}`
            : `${formatElapsed(session.startedAt ?? "")} ${t("admin.sessions.elapsed")}`}
        </Text>
      </View>

      {/* Title + series info + trickplay thumbnail */}
      <View style={{ flexDirection: "row", gap: 10, alignItems: "center" }}>
        <TrickplayPreview mediaId={session.mediaItemId ?? ""} currentTime={positionMs / 1000} visible />
        <View style={{ flex: 1, minWidth: 0 }}>
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
            {session.mediaTitle || t("common.unknown", "Unknown")}
          </Text>
          {hasSeriesInfo && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 1 }} numberOfLines={1}>
              {session.seriesTitle} - {seriesLabel}
            </Text>
          )}
        </View>
      </View>

      {/* Play method + codec badges */}
      <View style={styles.badgeRow}>
        <View style={[
          styles.badge,
          { backgroundColor: session.playMethod === "transcode" ? `${theme.colors.warning}20` : `${theme.colors.success}20` },
        ]}>
          <Text style={{
            fontSize: 10,
            fontWeight: "600",
            color: session.playMethod === "transcode" ? theme.colors.warning : theme.colors.success,
          }}>
            {playMethodLabel(session.playMethod ?? "", t)}
          </Text>
        </View>
        {session.resolution ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.resolution}</Text>
        ) : null}
        {session.videoCodec ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.videoCodec}</Text>
        ) : null}
        {session.audioCodec ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.audioCodec}</Text>
        ) : null}
        {bandwidthBps > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{formatBandwidth(bandwidthBps)}</Text>
        ) : null}
        {session.transcodeInfo ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.transcodeInfo}</Text>
        ) : null}
      </View>

      {/* Per-track (video/audio) decision + engine detail */}
      <TrackBadges session={session} theme={theme} t={t} />

      {/* User + device + controls */}
      <View style={styles.userRow}>
        <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
          <Text style={{ color: theme.colors.onAccent, fontWeight: "700", fontSize: 13 }}>
            {(session.profileName ?? "?").charAt(0).toUpperCase()}
          </Text>
        </View>
        <View style={{ flex: 1, minWidth: 0 }}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }} numberOfLines={1}>
            {session.profileName}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>
            {session.deviceName || session.clientIp}
            {session.deviceType ? ` (${session.deviceType})` : ""}
          </Text>
          {session.castDeviceName ? (
            <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginTop: 1 }}>
              <Ionicons name="tv-outline" size={11} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.accent, fontSize: 11 }} numberOfLines={1}>
                {t("admin.sessions.castingTo", { device: session.castDeviceName })}
              </Text>
            </View>
          ) : null}
        </View>

        {/* Controls */}
        <View style={styles.controls}>
          <Pressable
            onPress={() => onCommand(session.sessionId, session.isPaused ? "play" : "pause")}
            hitSlop={8}
          >
            <Ionicons
              name={session.isPaused ? "play-circle-outline" : "pause-circle-outline"}
              size={24}
              color={theme.colors.accent}
            />
          </Pressable>
          <Pressable onPress={() => setShowMessage(!showMessage)} hitSlop={8}>
            <Ionicons name="chatbubble-outline" size={20} color={theme.colors.textMuted} />
          </Pressable>
          <Pressable onPress={() => onKill(session)} hitSlop={8}>
            <Ionicons name="stop-circle-outline" size={24} color={theme.colors.error} />
          </Pressable>
        </View>
      </View>

      {/* Send message input */}
      {showMessage && (
        <View style={styles.messageRow}>
          <TextInput
            style={[styles.messageInput, {
              backgroundColor: theme.colors.bg,
              color: theme.colors.text,
              borderColor: theme.colors.border,
            }]}
            placeholder={t("admin.sessions.messagePlaceholder", "Send a message...")}
            placeholderTextColor={theme.colors.textMuted}
            value={message}
            onChangeText={setMessage}
            onSubmitEditing={handleSendMessage}
            returnKeyType="send"
          />
          <Pressable onPress={handleSendMessage} hitSlop={8}>
            <Ionicons name="send" size={20} color={theme.colors.accent} />
          </Pressable>
        </View>
      )}
    </View>
  );
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

  const { data: sessions, isLoading, refetch, dataUpdatedAt } = useSessions();

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

  const handleKill = useCallback((session: ActiveSession) => {
    Alert.alert(
      t("admin.sessions.stopSession", "Stop Session"),
      `${t("admin.sessions.stop", "Stop")} ${session.profileName}?`,
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("admin.sessions.stop", "Stop"), style: "destructive", onPress: () => killSession.mutate(session.sessionId) },
      ],
    );
  }, [killSession, t]);

  const handleCommand = useCallback((sessionId: string, command: string) => {
    sendCommand.mutate({ sessionId, command });
  }, [sendCommand]);

  const renderSession = useCallback(({ item }: { item: ActiveSession }) => (
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
        keyExtractor={(s) => s.sessionId}
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
          <View style={{ alignItems: "center", paddingTop: 60 }}>
            <Ionicons name="play-circle-outline" size={48} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>
              {t("admin.sessions.noSessions", "No active sessions")}
            </Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 14, gap: 6 },
  progressTrack: { height: 4, borderRadius: 2, overflow: "hidden" },
  progressFill: { height: 4, borderRadius: 2 },
  progressLabels: { flexDirection: "row", justifyContent: "space-between", marginTop: 2 },
  badgeRow: { flexDirection: "row", gap: 8, alignItems: "center", flexWrap: "wrap", marginTop: 2 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  userRow: { flexDirection: "row", alignItems: "center", gap: 10, marginTop: 4 },
  avatar: { width: 32, height: 32, borderRadius: 16, alignItems: "center", justifyContent: "center" },
  controls: { flexDirection: "row", alignItems: "center", gap: 12 },
  messageRow: { flexDirection: "row", alignItems: "center", gap: 8, marginTop: 4 },
  messageInput: { flex: 1, height: 36, borderRadius: 8, borderWidth: 1, paddingHorizontal: 10, fontSize: 13 },
  header: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 4 },
});
