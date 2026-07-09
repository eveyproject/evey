import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { BottomSheet } from "./BottomSheet";
import type { V1StreamInfo, V1MediaStream } from "@evey/api";

interface StreamInfoPanelProps {
  visible: boolean;
  onClose: () => void;
  streamInfo: V1StreamInfo | null;
}

function formatBitrate(bitrate?: string): string {
  if (!bitrate) return "-";
  const bps = Number(bitrate);
  if (isNaN(bps) || bps <= 0) return bitrate;
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${Math.round(bps / 1_000)} Kbps`;
  return `${bps} bps`;
}

function formatPlayMethod(method?: string): string {
  switch (method) {
    case "PLAY_METHOD_DIRECT_PLAY": return "Direct Play";
    case "PLAY_METHOD_DIRECT_STREAM": return "Direct Stream";
    case "PLAY_METHOD_TRANSCODE": return "Transcode";
    default: return method ?? "-";
  }
}

function formatChannels(ch?: number): string {
  if (!ch) return "";
  if (ch === 1) return "Mono";
  if (ch === 2) return "Stereo";
  if (ch === 6) return "5.1";
  if (ch === 8) return "7.1";
  return `${ch}ch`;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  const theme = useTheme();
  return (
    <View style={[styles.row, { borderBottomColor: theme.colors.divider }]}>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.value, { color: theme.colors.text }]}>{value}</Text>
    </View>
  );
}

function SectionHeader({ title }: { title: string }) {
  const theme = useTheme();
  return (
    <Text style={[styles.section, { color: theme.colors.accent }]}>{title}</Text>
  );
}

export function StreamInfoPanel({ visible, onClose, streamInfo }: StreamInfoPanelProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  if (!streamInfo) return null;

  const videoStream = streamInfo.mediaStreams?.find(
    (s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO",
  );
  const audioStream = streamInfo.mediaStreams?.find(
    (s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_AUDIO",
  );

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.streamInfo.title", "Stream Info")}>
      <SectionHeader title={t("player.streamInfo.playback", "Playback")} />
      <InfoRow label={t("player.streamInfo.method", "Method")} value={formatPlayMethod(streamInfo.playMethod)} />
      {streamInfo.container && (
        <InfoRow label={t("player.streamInfo.container", "Container")} value={streamInfo.container.toUpperCase()} />
      )}
      {streamInfo.bitrate && (
        <InfoRow label={t("player.streamInfo.totalBitrate", "Total Bitrate")} value={formatBitrate(streamInfo.bitrate)} />
      )}

      {videoStream && (
        <>
          <SectionHeader title={t("player.streamInfo.video", "Video")} />
          <InfoRow label={t("player.streamInfo.codec", "Codec")} value={(videoStream.codec ?? "-").toUpperCase()} />
          {videoStream.width && videoStream.height && (
            <InfoRow label={t("player.streamInfo.resolution", "Resolution")} value={`${videoStream.width}×${videoStream.height}`} />
          )}
          {videoStream.bitrate && (
            <InfoRow label={t("player.streamInfo.bitrate", "Bitrate")} value={formatBitrate(videoStream.bitrate)} />
          )}
          {videoStream.frameRate && (
            <InfoRow label={t("player.streamInfo.framerate", "Frame Rate")} value={`${videoStream.frameRate} fps`} />
          )}
          {videoStream.profile && (
            <InfoRow label={t("player.streamInfo.profile", "Profile")} value={videoStream.profile} />
          )}
          {videoStream.pixelFormat && (
            <InfoRow label={t("player.streamInfo.pixelFormat", "Pixel Format")} value={videoStream.pixelFormat} />
          )}
          {streamInfo.videoDecision && (
            <InfoRow label={t("player.streamInfo.decision", "Decision")} value={streamInfo.videoDecision} />
          )}
        </>
      )}

      {audioStream && (
        <>
          <SectionHeader title={t("player.streamInfo.audio", "Audio")} />
          <InfoRow label={t("player.streamInfo.codec", "Codec")} value={(audioStream.codec ?? "-").toUpperCase()} />
          {audioStream.channels && (
            <InfoRow label={t("player.streamInfo.channels", "Channels")} value={formatChannels(audioStream.channels)} />
          )}
          {audioStream.language && (
            <InfoRow label={t("player.streamInfo.language", "Language")} value={audioStream.language} />
          )}
          {audioStream.bitrate && (
            <InfoRow label={t("player.streamInfo.bitrate", "Bitrate")} value={formatBitrate(audioStream.bitrate)} />
          )}
          {audioStream.sampleRate && (
            <InfoRow label={t("player.streamInfo.sampleRate", "Sample Rate")} value={`${audioStream.sampleRate} Hz`} />
          )}
          {streamInfo.audioDecision && (
            <InfoRow label={t("player.streamInfo.decision", "Decision")} value={streamInfo.audioDecision} />
          )}
        </>
      )}

      {streamInfo.transcodeReasons && streamInfo.transcodeReasons.length > 0 && (
        <>
          <SectionHeader title={t("player.streamInfo.transcodeReasons", "Transcode Reasons")} />
          <Text style={[styles.reasons, { color: theme.colors.textMuted }]}>{streamInfo.transcodeReasons.join(", ")}</Text>
        </>
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  section: {
    fontSize: 13,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginTop: 16,
    marginBottom: 6,
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: {
    fontSize: 14,
  },
  value: {
    fontSize: 14,
    fontWeight: "500",
    textAlign: "right",
    flexShrink: 1,
    marginLeft: 12,
  },
  reasons: {
    fontSize: 13,
    paddingVertical: 6,
  },
});
