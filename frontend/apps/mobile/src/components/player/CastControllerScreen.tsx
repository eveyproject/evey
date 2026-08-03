import { View, Text, Pressable, StatusBar, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { CastButtonNative } from "../../hooks/useChromecast";
import { formatTime } from "../../lib/formatTime";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface CastControllerScreenProps {
  title: string;
  currentTime: number;
  duration: number;
  isPlaying: boolean;
  skipFwd: number;
  skipBwd: number;
  onSeekToPosition: (sec: number) => void;
  onPlayPause: () => void;
  onBack: () => void;
}

export function CastControllerScreen({
  title,
  currentTime,
  duration,
  isPlaying,
  skipFwd,
  skipBwd,
  onSeekToPosition,
  onPlayPause,
  onBack,
}: CastControllerScreenProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const castProgress = duration > 0 ? currentTime / duration : 0;
  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: 40 }}>
        <Ionicons name="tv" size={64} color={theme.colors.accent} />
        <Text style={{ color: "#fff", fontSize: FONT_SIZE.xl, fontWeight: "700", marginTop: 20 }}>
          {t("player.controls.castingTo", "Casting to")}
        </Text>
        <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.base, marginTop: 4 }}>
          {title}
        </Text>

        {/* Timeline */}
        <View style={{ width: "100%", marginTop: 32, paddingHorizontal: 8 }}>
          <View style={{ height: 4, borderRadius: 2, backgroundColor: "rgba(255,255,255,0.2)", overflow: "hidden" }}>
            <View style={{ height: "100%", borderRadius: 2, backgroundColor: theme.colors.accent, width: `${Math.min(100, castProgress * 100)}%` }} />
          </View>
          <View style={{ flexDirection: "row", justifyContent: "space-between", marginTop: 6 }}>
            <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.xs }}>{formatTime(currentTime)}</Text>
            <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.xs }}>{formatTime(duration)}</Text>
          </View>
        </View>

        {/* Controls */}
        <View style={{ flexDirection: "row", alignItems: "center", gap: 32, marginTop: 24 }}>
          <Pressable onPress={() => onSeekToPosition(Math.max(0, currentTime - skipBwd))} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })}>
            <Ionicons name="play-back" size={ICON_SIZE["2xl"]} color="#fff" />
          </Pressable>
          <Pressable onPress={() => {
            onPlayPause();
          }} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")}>
            <Ionicons name={isPlaying ? "pause" : "play"} size={ICON_SIZE["4xl"]} color="#fff" />
          </Pressable>
          <Pressable onPress={() => onSeekToPosition(currentTime + skipFwd)} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })}>
            <Ionicons name="play-forward" size={ICON_SIZE["2xl"]} color="#fff" />
          </Pressable>
        </View>
        {CastButtonNative && (
          <View style={{ marginTop: 32 }}>
            <CastButtonNative style={{ width: 48, height: 48, tintColor: theme.colors.accent }} />
          </View>
        )}
        <Pressable onPress={onBack} style={{ marginTop: 24 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>{t("player.goBack")}</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
});
