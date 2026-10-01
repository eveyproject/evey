import { View, Text, TouchableOpacity } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { formatDurationSec } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

export interface AlbumHeaderAlbum {
  title: string;
  artist: string;
  year: number;
  trackCount: number;
  duration?: number;
}

// The head of an open album, same content as web's AlbumHeader: cover, name,
// who and when, how long, and the two ways to start it. Opening an album used
// to land on the bare track list under a filter chip.
export function AlbumHeader({
  album,
  coverUri,
  authHeaders,
  onPlay,
  onShuffle,
}: {
  album: AlbumHeaderAlbum;
  coverUri?: string;
  authHeaders?: Record<string, string>;
  onPlay: () => void;
  onShuffle: () => void;
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const facts = [
    album.artist,
    album.year > 0 ? String(album.year) : "",
    t("media.music.trackCount", { count: album.trackCount }),
    formatDurationSec(album.duration),
  ].filter(Boolean);

  return (
    <View testID="album-header" style={{ flexDirection: "row", gap: 14, alignItems: "flex-end", marginTop: 12 }}>
      {coverUri ? (
        <Image
          source={{ uri: coverUri, headers: authHeaders }}
          style={{ width: 112, height: 112, borderRadius: theme.radii.md }}
          contentFit="cover"
          transition={150}
        />
      ) : (
        <View style={{ width: 112, height: 112, borderRadius: theme.radii.md, backgroundColor: theme.colors.surfaceRaised, alignItems: "center", justifyContent: "center" }}>
          <Ionicons name="musical-notes" size={36} color={theme.colors.textMuted} />
        </View>
      )}
      <View style={{ flex: 1, gap: 6 }}>
        <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "700" }} numberOfLines={2}>{album.title || "-"}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={2}>{facts.join(" · ")}</Text>
        <View style={{ flexDirection: "row", gap: 8, marginTop: 4 }}>
          <TouchableOpacity
            testID="album-header-play"
            activeOpacity={0.7}
            onPress={onPlay}
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.play")}
            style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 14, paddingVertical: 8, borderRadius: theme.radii.sm, backgroundColor: theme.colors.accent }}
          >
            <Ionicons name="play" size={16} color={theme.colors.onAccent} />
            <Text style={{ color: theme.colors.onAccent, fontSize: 14, fontWeight: "600" }}>{t("media.detail.play")}</Text>
          </TouchableOpacity>
          <TouchableOpacity
            testID="album-header-shuffle"
            activeOpacity={0.7}
            onPress={onShuffle}
            accessibilityRole="button"
            accessibilityLabel={t("media.music.shuffle")}
            style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 14, paddingVertical: 8, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }}
          >
            <Ionicons name="shuffle" size={16} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("media.music.shuffle")}</Text>
          </TouchableOpacity>
        </View>
      </View>
    </View>
  );
}
