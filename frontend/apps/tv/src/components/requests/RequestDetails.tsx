import { Text, View } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { formatRating, formatReleaseDate } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

// Web/mobile parity (RequestDetails there): the release date and rating line
// and the synopsis, at TV reading sizes. No provider link: a TV has no
// browser to open it in.

/** Release date and rating on one muted line; nothing when both are unknown. */
export function RequestFacts({ releaseDate, rating }: { releaseDate?: string; rating?: number }) {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const date = formatReleaseDate(releaseDate, i18n?.language);
  const score = formatRating(rating);
  if (!date && !score) return null;
  return (
    <View style={{ flexDirection: "row", alignItems: "center", gap: 16, marginTop: 4 }} testID="request-facts">
      {date ? (
        <View style={{ flexDirection: "row", alignItems: "center", gap: 5 }} accessibilityLabel={t("requests.releaseDateLabel", { date })}>
          <Ionicons name="calendar-outline" size={15} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{date}</Text>
        </View>
      ) : null}
      {score ? (
        <View style={{ flexDirection: "row", alignItems: "center", gap: 5 }} accessibilityLabel={t("requests.ratingLabel", { rating: score })}>
          <Ionicons name="star" size={15} color={theme.colors.warning} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{score}</Text>
        </View>
      ) : null}
    </View>
  );
}

/** The synopsis, clamped; nothing when the provider had none. */
export function RequestOverview({ overview, lines = 2 }: { overview?: string; lines?: number }) {
  const theme = useTheme();
  const text = overview?.trim();
  if (!text) return null;
  return (
    <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 6, lineHeight: 20 }} numberOfLines={lines} testID="request-overview">
      {text}
    </Text>
  );
}
