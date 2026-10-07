import { Linking, Pressable, Text, View } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { externalTitleSite, externalTitleUrl, formatRating, formatReleaseDate } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

// Web parity (features/requests/RequestDetails.tsx): the link to the
// provider's page in the viewer's language, the release date and rating line,
// and the synopsis. Shared by the requests screen and the admin queue.

type TitleRef = { provider?: string; providerId?: string; mediaType?: string };

/** Opens the title's TMDB / IMDb / MusicBrainz page in the system browser. */
export function useExternalTitleLink(title: TitleRef): { url?: string; label?: string; open: () => void } {
  const { t, i18n } = useTranslation();
  const url = externalTitleUrl(title, i18n?.language);
  const site = externalTitleSite(title.provider);
  const label = url && site ? t("requests.openOn", { site }) : undefined;
  return {
    url,
    label,
    open: () => {
      if (url) void Linking.openURL(url);
    },
  };
}

/**
 * The title, pressable when it has a page to open. A free-text request (no
 * provider id) renders as plain text.
 */
export function RequestTitleLink({ title, text, year, size = 14, lines = 2 }: { title: TitleRef; text?: string; year?: number; size?: number; lines?: number }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const link = useExternalTitleLink(title);
  const label = `${text ?? ""}${year != null && year > 0 ? ` ${t("requests.year", { year })}` : ""}`;
  if (!link.url) {
    return (
      <Text style={{ color: theme.colors.text, fontSize: size, fontWeight: "600" }} numberOfLines={lines}>
        {label}
      </Text>
    );
  }
  return (
    <Pressable
      onPress={link.open}
      hitSlop={6}
      accessibilityRole="link"
      accessibilityLabel={link.label}
      accessibilityHint={label}
      testID="request-external-link"
      style={{ flexDirection: "row", alignItems: "center", gap: 4, alignSelf: "flex-start", maxWidth: "100%" }}
    >
      <Text style={{ color: theme.colors.text, fontSize: size, fontWeight: "600", flexShrink: 1 }} numberOfLines={lines}>
        {label}
      </Text>
      <Ionicons name="open-outline" size={size} color={theme.colors.textMuted} />
    </Pressable>
  );
}

/** Release date and rating on one muted line; nothing when both are unknown. */
export function RequestFacts({ releaseDate, rating }: { releaseDate?: string; rating?: number }) {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const date = formatReleaseDate(releaseDate, i18n?.language);
  const score = formatRating(rating);
  if (!date && !score) return null;
  return (
    <View style={{ flexDirection: "row", alignItems: "center", gap: 10, marginTop: 2 }} testID="request-facts">
      {date ? (
        <View style={{ flexDirection: "row", alignItems: "center", gap: 3 }} accessibilityLabel={t("requests.releaseDateLabel", { date })}>
          <Ionicons name="calendar-outline" size={11} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{date}</Text>
        </View>
      ) : null}
      {score ? (
        <View style={{ flexDirection: "row", alignItems: "center", gap: 3 }} accessibilityLabel={t("requests.ratingLabel", { rating: score })}>
          <Ionicons name="star" size={11} color={theme.colors.warning} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{score}</Text>
        </View>
      ) : null}
    </View>
  );
}

/** The synopsis, clamped; nothing when the provider had none. */
export function RequestOverview({ overview, lines = 3 }: { overview?: string; lines?: number }) {
  const theme = useTheme();
  const text = overview?.trim();
  if (!text) return null;
  return (
    <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 4, lineHeight: 16 }} numberOfLines={lines} testID="request-overview">
      {text}
    </Text>
  );
}
