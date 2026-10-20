import { useState } from "react";
import { View, Text, Pressable, TextInput, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { episodeLabel, formatTimeMs, sessionTrackLines, type V1ActiveSession } from "@evey/api";
import { router } from "expo-router";
import { useTheme } from "../../providers/ThemeProvider";
import { TrickplayPreview } from "../TrickplayPreview";

// SessionCard is the admin "mini control player" for one active playback: what
// is being watched, by whom, on what device, how it's being served - plus live
// controls (play/pause, stop, message). Shared by the Sessions screen and the
// admin Dashboard's "Now playing" so both show the SAME rich card as the web
// (web: features/admin/DashboardNowPlaying.tsx). Presentational: the parent
// owns the kill/command mutations and passes them as onKill/onCommand.

// The session thumbnail's width on a phone: the title keeps the rest.
const THUMB_MAX_WIDTH = 112;

type Tr = (k: string, d?: string | Record<string, unknown>) => string;

export function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

export function formatElapsed(startedAt: string): string {
  if (!startedAt) return "";
  const diff = Date.now() - new Date(startedAt).getTime();
  return formatTimeMs(diff);
}

export function playMethodLabel(method: string, t: (k: string) => string): string {
  switch (method) {
    case "direct_play": return t("admin.sessions.directPlay");
    case "direct_stream": return t("admin.sessions.directStream");
    case "transcode": return t("admin.sessions.transcode");
    default: return method || "Direct";
  }
}

// TrackBadges renders the per-track serving detail (copy vs transcode, served
// codec, engine and backend) so an operator can tell a copy rung from a real
// transcode. The lines come from @evey/api's sessionTrackLines, the same
// helper the web card uses: the phone used to guess the engine from the
// decision alone and could name mkvgo for a track ffmpeg was serving.
function TrackBadges({ session, theme, t }: { session: V1ActiveSession; theme: ReturnType<typeof useTheme>; t: Tr }) {
  const lines = sessionTrackLines(session);
  if (lines.length === 0) return null;
  // A key the catalogue does not carry shows the raw server word, never the key.
  const named = (key: string, raw: string) => {
    const label = t(key);
    return label === key ? raw : label;
  };

  return (
    <View style={{ gap: 2, marginTop: 2 }}>
      {lines.map((line) => {
        const isTranscode = line.decision === "transcode";
        const decLabel = t(`admin.sessions.track.${line.id}${isTranscode ? "Transcoded" : "Copied"}`);
        const detail = [line.codec, line.res].filter(Boolean).join(", ");
        const engine = named(`admin.sessions.track.engine.${line.engine}`, line.engine);
        const backend = line.backend ? named(`admin.sessions.track.backend.${line.backend}`, line.backend) : "";
        return (
          <View key={line.id} style={{ flexDirection: "row", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <Ionicons name={line.id === "video" ? "film-outline" : "volume-high-outline"} size={11} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.text, fontSize: 10 }}>{t(`admin.sessions.track.${line.id}`)}:</Text>
            <View style={[styles.badge, { backgroundColor: isTranscode ? `${theme.colors.warning}20` : `${theme.colors.success}20` }]}>
              <Text style={{ fontSize: 9, fontWeight: "600", color: isTranscode ? theme.colors.warning : theme.colors.success }}>
                {decLabel}
              </Text>
            </View>
            {detail ? <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>({detail})</Text> : null}
            <Text style={{ color: theme.colors.textMuted, fontSize: 10, opacity: 0.7 }}>
              · {engine}{backend ? ` (${backend})` : ""}
            </Text>
          </View>
        );
      })}
    </View>
  );
}

export function SessionCard({ session, theme, t, onKill, onCommand }: {
  session: V1ActiveSession;
  theme: ReturnType<typeof useTheme>;
  t: Tr;
  onKill: (s: V1ActiveSession) => void;
  onCommand: (sessionId: string, command: string) => void;
}) {
  const [showMessage, setShowMessage] = useState(false);
  const [message, setMessage] = useState("");

  const sessionId = session.sessionId ?? "";
  const progress = Math.min(100, Math.max(0, session.progress ?? 0));
  const durationMs = parseInt(session.durationMs ?? "0");
  const positionMs = parseInt(session.positionMs ?? "0");
  const bandwidthBps = parseInt(session.bandwidthBps ?? "0");
  const remaining = durationMs > 0 ? formatTimeMs(Math.max(0, durationMs - positionMs)) : null;

  const mediaLabel = session.seriesTitle
    ? episodeLabel({
        seriesTitle: session.seriesTitle,
        seasonNumber: session.seasonNumber,
        episodeNumber: session.episodeNumber,
        title: session.mediaTitle,
      })
    : session.mediaTitle;

  // A track has no media page of its own; everything else opens its page.
  const mediaLink = session.mediaItemId && session.mediaType !== "track" ? `/media/${session.mediaItemId}` : null;

  const handleSendMessage = () => {
    if (!message.trim()) return;
    onCommand(sessionId, `message:${message.trim()}`);
    setMessage("");
    setShowMessage(false);
  };

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
        <View style={[styles.progressFill, { width: `${progress}%`, backgroundColor: theme.colors.accent }]} />
      </View>
      <View style={styles.progressLabels}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{formatTimeMs(positionMs)}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
          {remaining ? `-${remaining}` : `${formatElapsed(session.startedAt ?? "")} ${t("admin.sessions.elapsed")}`}
        </Text>
      </View>

      <View style={{ flexDirection: "row", gap: 10, alignItems: "center" }}>
        <TrickplayPreview mediaId={session.mediaItemId ?? ""} currentTime={positionMs / 1000} visible maxWidth={THUMB_MAX_WIDTH} />
        <Pressable
          style={{ flex: 1, minWidth: 0 }}
          disabled={!mediaLink}
          onPress={() => { if (mediaLink) router.push(mediaLink as never); }}
          accessibilityRole={mediaLink ? "link" : undefined}
          testID="session-media-link"
        >
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600", textDecorationLine: mediaLink ? "underline" : "none" }} numberOfLines={2}>
            {mediaLabel || t("common.unknown", "Unknown")}
          </Text>
        </Pressable>
      </View>

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
        {session.resolution ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.resolution}</Text> : null}
        {session.videoCodec ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.videoCodec}</Text> : null}
        {session.audioCodec ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.audioCodec}</Text> : null}
        {bandwidthBps > 0 ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{formatBandwidth(bandwidthBps)}</Text> : null}
        {session.transcodeInfo ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{session.transcodeInfo}</Text> : null}
      </View>

      <TrackBadges session={session} theme={theme} t={t} />

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

        <View style={styles.controls}>
          <Pressable
            onPress={() => onCommand(sessionId, session.isPaused ? "play" : "pause")}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel={session.isPaused ? t("common.play") : t("common.pause")}
          >
            <Ionicons name={session.isPaused ? "play-circle-outline" : "pause-circle-outline"} size={24} color={theme.colors.accent} />
          </Pressable>
          <Pressable
            onPress={() => setShowMessage(!showMessage)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel={t("admin.sessions.sendMessage")}
          >
            <Ionicons name="chatbubble-outline" size={20} color={theme.colors.textMuted} />
          </Pressable>
          <Pressable
            onPress={() => onKill(session)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel={t("admin.sessions.stopSession")}
          >
            <Ionicons name="stop-circle-outline" size={24} color={theme.colors.error} />
          </Pressable>
        </View>
      </View>

      {showMessage && (
        <View style={styles.messageRow}>
          <TextInput
            style={[styles.messageInput, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border }]}
            placeholder={t("admin.sessions.messagePlaceholder", "Send a message...")}
            placeholderTextColor={theme.colors.textMuted}
            value={message}
            onChangeText={setMessage}
            onSubmitEditing={handleSendMessage}
            returnKeyType="send"
          />
          <Pressable onPress={handleSendMessage} hitSlop={8} accessibilityRole="button" accessibilityLabel={t("admin.sessions.sendMessage")}>
            <Ionicons name="send" size={20} color={theme.colors.accent} />
          </Pressable>
        </View>
      )}
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
});
