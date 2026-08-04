import { Text, Modal, Pressable, View, ScrollView, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { formatChannelLayout, type V1StreamInfo, type V1MediaStream } from "@evey/api";
import { FONT_SIZE } from "@evey/theme";

interface StreamInfoPanelProps {
  visible: boolean;
  onClose: () => void;
  streamInfo: V1StreamInfo | null;
}

function formatBitrate(bitrate?: string): string {
  if (!bitrate) return "-";
  const bps = Number(bitrate);
  if (!Number.isFinite(bps) || bps <= 0) return "-";
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

function InfoRow({ label, value }: { label: string; value: string }) {
  const theme = useTheme();
  return (
    <View style={[styles.row, { borderBottomColor: theme.colors.border }]}>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.value, { color: theme.colors.text }]}>{value}</Text>
    </View>
  );
}

// The decision belongs to its stream: colour-coded so copied (untouched, good) is
// visually distinct from transcoded (re-encoded, costly) and wasm (decoded
// client-side, so not transcoded at all) - mirrors web/mobile's badge.
function DecisionBadge({ decision }: { decision?: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  if (!decision) return null;
  const isCopy = decision === "copy" || decision === "direct";
  const isWasm = decision === "wasm";
  // bg switches between accent/success/warning, so the label colour must
  // follow: onAccent painted on a warning fill (or vice versa) is the exact
  // WCAG AA regression this badge already caused once before.
  const bg = isWasm ? theme.colors.accent : isCopy ? theme.colors.success : theme.colors.warning;
  const onBg = isWasm ? theme.colors.onAccent : isCopy ? theme.colors.onSuccess : theme.colors.onWarning;
  const label = isWasm
    ? t("player.streamInfo.decisionWasm", "Décodé client")
    : isCopy
      ? t("player.streamInfo.decisionCopy", "Copié")
      : t("player.streamInfo.decisionTranscode", "Transcodé");
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <Text style={[styles.badgeText, { color: onBg }]}>{label}</Text>
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

  const videoStream = streamInfo?.mediaStreams?.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const audioStream = streamInfo?.mediaStreams?.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_AUDIO");

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.colors.surface }]} onPress={(e) => e.stopPropagation()}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.streamInfo.title", "Stream Info")}
          </Text>
          {streamInfo ? (
            <ScrollView style={styles.body}>
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
                  <SectionHeader title={t("player.streamInfo.video", "Video")} decision={streamInfo.videoDecision} />
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
                </>
              )}

              {streamInfo.transcodeReasons && streamInfo.transcodeReasons.length > 0 && (
                <>
                  <SectionHeader title={t("player.streamInfo.transcodeReasons", "Transcode Reasons")} />
                  {streamInfo.transcodeReasons.map((reason) => (
                    <View key={reason} style={styles.reasonRow}>
                      <Text style={[styles.reasonBullet, { color: theme.colors.warning }]}>{"•"}</Text>
                      <Text style={[styles.reasons, { color: theme.colors.textMuted }]}>
                        {t(`player.streamInfo.reason.${reason}`, reason.replace(/_/g, " "))}
                      </Text>
                    </View>
                  ))}
                </>
              )}
            </ScrollView>
          ) : null}

          <FocusableCard
            hasTVPreferredFocus={true}
            onPress={onClose}
            accessibilityLabel={t("common.close", "Close")}
            style={[styles.closeRow, { borderColor: theme.colors.border }]}
          >
            <Text style={[styles.closeLabel, { color: theme.colors.accent }]}>{t("common.close", "Close")}</Text>
          </FocusableCard>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.7)",
  },
  sheet: {
    borderRadius: 16,
    padding: 24,
    minWidth: 420,
    maxWidth: 480,
    maxHeight: 600,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
    textAlign: "center",
    marginBottom: 12,
  },
  body: {
    marginBottom: 12,
  },
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
  closeRow: {
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingVertical: 12,
    alignItems: "center",
    borderRadius: 8,
  },
  closeLabel: {
    fontSize: FONT_SIZE.base,
    fontWeight: "600",
  },
});
