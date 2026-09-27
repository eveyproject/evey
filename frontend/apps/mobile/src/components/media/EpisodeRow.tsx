import { memo } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { formatDurationMs, type V1MediaItem } from "@evey/api";
import type { useTheme } from "../../providers/ThemeProvider";
import { ProgressBar } from "../ProgressBar";
import { artworkUrl } from "../../lib/artwork";
import { hasMediaFile } from "./mediaFile";

// Per-episode view of the shared watch-history fetch (percent is 0-100, the
// server's percent_complete scale - same as the row's progress bar).
export type EpisodeProgress = { percent: number; played: boolean };

export const EpisodeRow = memo(function EpisodeRow({
  item, serverUrl, theme, authHeaders, downloadProgress,
  progress, onDownload, onPlay,
}: {
  item: V1MediaItem;
  serverUrl: string;
  theme: ReturnType<typeof useTheme>;
  authHeaders?: { Authorization: string };
  downloadProgress?: number;
  progress?: EpisodeProgress;
  onDownload?: (item: V1MediaItem) => void;
  onPlay: (item: V1MediaItem) => void;
}) {
  const { t } = useTranslation();
  // "backdrop", not "poster": an episode's still lives under the backdrop type
  // (same as the web episode rows). Asking for a poster gave an empty 16:9 box
  // on most episodes, since episodes rarely carry one.
  const episodeUri = item.id ? artworkUrl(serverUrl, item.id, "backdrop", 240, item) : undefined;
  // No per-row fetch here: the parent derives every row's progress from ONE
  // GetWatchHistory call (audit 25-08 H3 - a season open used to fire one
  // GetWatchProgress per visible row, a 20-request burst on a weak NAS).
  const percent = progress?.percent ?? 0;
  // A fully-watched episode has its resume position reset, so `percent` is 0 -
  // it would look identical to a never-watched one. `played` is the truth for
  // "seen in full"; show the same check badge as the cards, and keep the
  // partial bar only for an episode left mid-way.
  const watched = progress?.played === true;
  const downloadable = !!onDownload && hasMediaFile(item);

  return (
    <Pressable
      onPress={() => item.id && onPlay(item)}
      style={{
        flexDirection: "row",
        padding: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={item.title ?? "Episode"}
      accessibilityRole="button"
    >
      <View style={{ width: 120, height: 68, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }}>
        <Image
          source={episodeUri ? { uri: episodeUri, headers: authHeaders } : undefined}
          style={{ width: 120, height: 68 }}
          contentFit="cover"
          transition={200}
        />
        {!watched && percent > 0 && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: 2 }}>
            <ProgressBar percent={percent} />
          </View>
        )}
        {watched && (
          <View style={{
            position: "absolute", top: 4, right: 4,
            width: 20, height: 20, borderRadius: 10,
            backgroundColor: theme.colors.success,
            alignItems: "center", justifyContent: "center",
          }}>
            <Ionicons name="checkmark" size={13} color={theme.colors.onSuccess} />
          </View>
        )}
      </View>
      <View style={{ flex: 1, marginLeft: theme.spacing.sm, justifyContent: "center" }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
          {item.indexNumber != null ? `${item.indexNumber}. ` : ""}{item.title}
        </Text>
        {(() => {
          const runtime = formatDurationMs(item.durationMs);
          return runtime ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
              {runtime}
            </Text>
          ) : null;
        })()}
        {item.overview ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }} numberOfLines={2}>
            {item.overview}
          </Text>
        ) : null}
      </View>
      {downloadable ? (
        <Pressable
          onPress={(e) => { e.stopPropagation(); onDownload!(item); }}
          disabled={downloadProgress !== undefined}
          hitSlop={8}
          style={{ justifyContent: "center", paddingHorizontal: theme.spacing.sm }}
          accessible={true}
          accessibilityLabel={t("common.download") || "Download"}
          accessibilityRole="button"
        >
          {downloadProgress !== undefined ? (
            Number.isFinite(downloadProgress) ? (
              <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "700" }}>
                {Math.round(downloadProgress * 100)}%
              </Text>
            ) : (
              <Ionicons name="cloud-download" size={18} color={theme.colors.accent} />
            )
          ) : (
            <Ionicons name="download-outline" size={18} color={theme.colors.textMuted} />
          )}
        </Pressable>
      ) : null}
    </Pressable>
  );
});
