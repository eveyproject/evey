import { useState } from "react";
import { View, Text, Pressable } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { formatBytes, formatChannelLayout, formatDurationMs, type V1MediaItem, type V1MediaStream } from "@evey/api";
import { resolutionLabel } from "@evey/client";
import { useTheme } from "../../providers/ThemeProvider";

function hdrLabel(s?: V1MediaStream): string | null {
  if (!s) return null;
  switch (s.colorTransfer?.trim().toLowerCase()) {
    case "smpte2084": return "HDR10";
    case "arib-std-b67": return "HLG";
  }
  const cs = s.colorSpace?.trim().toLowerCase() ?? "";
  if (cs.startsWith("bt2020") && (s.bitDepth ?? 0) >= 10) return "HDR10";
  return null;
}

export function FileInfoSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [open, setOpen] = useState(false);

  if (!item.filePath && !item.fileSize && !item.containerFormat && !(item.streams && item.streams.length > 0)) {
    return null;
  }

  const videoStream = item.streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const audioStreams = item.streams?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO") ?? [];
  const primaryAudio = audioStreams.find((s) => s.isDefault) ?? audioStreams[0];
  const techBadges = [
    resolutionLabel(videoStream),
    hdrLabel(videoStream),
    formatChannelLayout(primaryAudio?.channels) || null,
    Number(item.fileSize ?? 0) > 0 ? formatBytes(item.fileSize) : null,
  ].filter((b): b is string => !!b);

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      {techBadges.length > 0 && (
        <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginBottom: theme.spacing.sm }}>
          {techBadges.map((b, i) => <InfoBadge key={i} label={b} />)}
        </View>
      )}
      <Pressable
        onPress={() => setOpen((v) => !v)}
        style={{ flexDirection: "row", alignItems: "center", gap: 6, marginBottom: theme.spacing.sm }}
        accessible={true}
        accessibilityLabel={t("media.detail.fileInfo")}
        accessibilityRole="button"
      >
        <Ionicons
          name={open ? "chevron-down" : "chevron-forward"}
          size={16}
          color={theme.colors.textMuted}
        />
        <Text style={{ color: theme.colors.textMuted, fontSize: 14, fontWeight: "600" }}>
          {t("media.detail.fileInfo")}
        </Text>
      </Pressable>
      {open && (
        <View style={{ gap: 8 }}>
          {item.filePath ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={2}>
              {item.filePath}
            </Text>
          ) : null}
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
            {item.fileSize ? <InfoBadge label={formatBytes(item.fileSize)} /> : null}
            {item.containerFormat ? <InfoBadge label={item.containerFormat.toUpperCase()} /> : null}
            {item.bitrate ? <InfoBadge label={`${Math.round(parseInt(item.bitrate) / 1000)} kbps`} /> : null}
            {(() => { const r = formatDurationMs(item.durationMs); return r ? <InfoBadge label={r} /> : null; })()}
          </View>
          {item.streams && item.streams.length > 0 ? (
            <View style={{ gap: 4, marginTop: 4 }}>
              {item.streams.map((s, i) => {
                const typeLabel = s.type?.replace("MEDIA_STREAM_TYPE_", "") ?? "";
                const res = s.width && s.height ? `${s.width}x${s.height}` : undefined;
                return (
                  <View key={i} style={{ flexDirection: "row", gap: 8 }}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", width: 60 }}>
                      {typeLabel}
                    </Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, flex: 1 }} numberOfLines={1}>
                      {[s.codec, s.language, s.title, res].filter(Boolean).join(" - ")}
                    </Text>
                  </View>
                );
              })}
            </View>
          ) : null}
        </View>
      )}
    </View>
  );
}

function InfoBadge({ label }: { label: string }) {
  const theme = useTheme();
  return (
    <View style={{
      paddingHorizontal: 8,
      paddingVertical: 3,
      borderRadius: theme.radii.sm,
      backgroundColor: theme.colors.surface,
    }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "500" }}>{label}</Text>
    </View>
  );
}
