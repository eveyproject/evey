import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { BottomSheet } from "./BottomSheet";
import {
  formatChannelLayout,
  effectivePlayMethod,
  playMethodI18nKey,
  reasonsHeadingI18nKey,
  visibleTranscodeReasons,
  type V1StreamInfo,
  type V1MediaStream,
} from "@evey/api";
import { FONT_SIZE } from "@evey/theme";

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

// 23.976023976023978 is a true value and a useless one to read. Show what a
// human says: 23.976, 24, 25, 30. Mirrors web's StreamInfoPanel.
function formatFrameRate(fps?: number): string | null {
  if (!fps || !Number.isFinite(fps) || fps <= 0) return null;
  const rounded = Math.round(fps * 1000) / 1000;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(3)} fps`;
}

// Per-stream bitrate is often absent from the probe (the container only
// carries a total). Deriving video ≈ total - audio beats showing a bare "-":
// it is the number the viewer actually wants, and it is honest about being
// an estimate. Mirrors web's StreamInfoPanel.
function deriveVideoBitrate(total?: string, audio?: string): string | null {
  const t = Number(total);
  if (!Number.isFinite(t) || t <= 0) return null;
  const a = Number(audio);
  if (Number.isFinite(a) && a > 0 && a < t) {
    return formatBitrate(String(t - a));
  }
  return formatBitrate(String(t));
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

// The decision belongs to its stream, not to a lonely row at the bottom of
// the section: colour-coded so copied (untouched, good) is visually distinct
// from transcoded (re-encoded, costly) and wasm (decoded client-side, not
// transcoded at all).
function DecisionBadge({ decision }: { decision?: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  if (!decision) return null;
  const isCopy = decision === "copy" || decision === "direct";
  const isWasm = decision === "wasm";
  const bg = isWasm ? theme.colors.accent : isCopy ? theme.colors.success : theme.colors.warning;
  const label = isWasm
    ? t("player.streamInfo.decisionWasm", "Décodé client")
    : isCopy
      ? t("player.streamInfo.decisionCopy", "Copié")
      : t("player.streamInfo.decisionTranscode", "Transcodé");
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <Text style={[styles.badgeText, { color: theme.colors.bg }]}>{label}</Text>
    </View>
  );
}

function SectionHeader({ title, decision }: { title: string; decision?: string }) {
  const theme = useTheme();
  return (
    <View style={styles.sectionHeader}>
      <Text style={[styles.section, { color: theme.colors.accent }]}>{title}</Text>
      <DecisionBadge decision={decision} />
    </View>
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

  const decisions = {
    playMethod: streamInfo.playMethod,
    videoDecision: streamInfo.videoDecision,
    audioDecision: streamInfo.audioDecision,
  };
  const methodKey = playMethodI18nKey(effectivePlayMethod(decisions));
  const reasons = visibleTranscodeReasons(streamInfo.transcodeReasons);

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.streamInfo.title", "Stream Info")}>
      <SectionHeader title={t("player.streamInfo.playback", "Playback")} />
      {methodKey ? <InfoRow label={t("player.streamInfo.method", "Method")} value={t(methodKey)} /> : null}
      {/* Source, not served: anything but direct play repackages to fMP4. */}
      {streamInfo.container && (
        <InfoRow label={t("player.streamInfo.containerSource", "Source container")} value={streamInfo.container.toUpperCase()} />
      )}
      {streamInfo.bitrate && (
        <InfoRow label={t("player.streamInfo.totalBitrate", "Total Bitrate")} value={formatBitrate(streamInfo.bitrate)} />
      )}

      {videoStream && (
        <>
          <SectionHeader title={t("player.streamInfo.video", "Video")} decision={streamInfo.videoDecision} />
          <InfoRow label={t("player.streamInfo.codec", "Codec")} value={(videoStream.codec ?? "-").toUpperCase()} />
          {videoStream.width && videoStream.height && (
            <InfoRow label={t("player.streamInfo.resolution", "Resolution")} value={`${videoStream.width}×${videoStream.height}`} />
          )}
          {(() => {
            if (videoStream.bitrate) {
              return <InfoRow label={t("player.streamInfo.bitrate", "Bitrate")} value={formatBitrate(videoStream.bitrate)} />;
            }
            const derived = deriveVideoBitrate(streamInfo.bitrate, audioStream?.bitrate);
            return derived ? (
              <InfoRow label={t("player.streamInfo.bitrate", "Bitrate")} value={`≈ ${derived}`} />
            ) : null;
          })()}
          {formatFrameRate(videoStream.frameRate) && (
            <InfoRow label={t("player.streamInfo.framerate", "Frame Rate")} value={formatFrameRate(videoStream.frameRate)!} />
          )}
          {videoStream.profile && (
            <InfoRow label={t("player.streamInfo.profile", "Profile")} value={videoStream.profile} />
          )}
          {videoStream.pixelFormat && (
            <InfoRow label={t("player.streamInfo.pixelFormat", "Pixel Format")} value={videoStream.pixelFormat} />
          )}
        </>
      )}

      {audioStream && (
        <>
          <SectionHeader title={t("player.streamInfo.audio", "Audio")} decision={streamInfo.audioDecision} />
          <InfoRow label={t("player.streamInfo.codec", "Codec")} value={(audioStream.codec ?? "-").toUpperCase()} />
          {audioStream.channels && (
            <InfoRow label={t("player.streamInfo.channels", "Channels")} value={formatChannelLayout(audioStream.channels)} />
          )}
          {audioStream.language && (
            <InfoRow label={t("player.streamInfo.language", "Language")} value={audioStream.language} />
          )}
          {/* The SERVED rate, not the source's: a Matroska track rarely declares
              one, so this row used to sit at "-" on every re-encoded track even
              though we picked the figure ourselves. Falls back to the source when
              the track is copied. Mirrors web's StreamInfoPanel. */}
          {(streamInfo.audioBitrate ?? audioStream.bitrate) && (
            <InfoRow
              label={t("player.streamInfo.bitrate", "Bitrate")}
              value={formatBitrate(streamInfo.audioBitrate ?? audioStream.bitrate)}
            />
          )}
          {audioStream.sampleRate && (
            <InfoRow label={t("player.streamInfo.sampleRate", "Sample Rate")} value={`${audioStream.sampleRate} Hz`} />
          )}
        </>
      )}

      {reasons.length > 0 && (
        <>
          <SectionHeader title={t(reasonsHeadingI18nKey(decisions))} />
          {reasons.map((reason) => (
            <View key={reason} style={styles.reasonRow}>
              <Text style={[styles.reasonBullet, { color: theme.colors.warning }]}>{"•"}</Text>
              <Text style={[styles.reasons, { color: theme.colors.textMuted }]}>
                {t(`player.streamInfo.reason.${reason}`, reason.replace(/_/g, " "))}
              </Text>
            </View>
          ))}
        </>
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  section: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  sectionHeader: {
    flexDirection: "row",
    alignItems: "center",
    marginTop: 16,
    marginBottom: 6,
  },
  badge: {
    marginLeft: 8,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeText: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "700",
    textTransform: "uppercase",
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: {
    fontSize: FONT_SIZE.sm,
  },
  value: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "500",
    textAlign: "right",
    flexShrink: 1,
    marginLeft: 12,
  },
  reasonRow: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 6,
    paddingVertical: 4,
  },
  reasonBullet: {
    fontSize: FONT_SIZE.xs,
  },
  reasons: {
    fontSize: FONT_SIZE.xs,
    flexShrink: 1,
  },
});
