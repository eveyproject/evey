import { memo } from "react";
import { Text, Pressable, StyleSheet, Linking } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import type { V1Trailer } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

export const TrailerButton = memo(function TrailerButton({ trailer }: { trailer: V1Trailer }) {
  const { t } = useTranslation();
  const theme = useTheme();

  const url = (() => {
    if (!trailer.key || !trailer.site) return null;
    switch (trailer.site.toLowerCase()) {
      case "youtube":
        return `https://www.youtube.com/watch?v=${trailer.key}`;
      case "vimeo":
        return `https://vimeo.com/${trailer.key}`;
      default:
        return null;
    }
  })();

  if (!url) return null;

  return (
    <Pressable
      onPress={() => Linking.openURL(url)}
      style={{
        flexDirection: "row",
        alignItems: "center",
        gap: 6,
        paddingHorizontal: 12,
        paddingVertical: 8,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surface,
        borderWidth: StyleSheet.hairlineWidth,
        borderColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={t("media.detail.trailer")}
      accessibilityRole="link"
    >
      <Ionicons name="play" size={14} color={theme.colors.text} />
      <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "500" }}>
        {t("media.detail.trailer")}
      </Text>
    </Pressable>
  );
});
