import { useEffect, useState } from "react";
import { View, type StyleProp, type ViewStyle } from "react-native";
import { Image, type ImageStyle } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTheme } from "../providers/ThemeProvider";
import { useLiteMode } from "../lib/liteMode";

// A poster that falls back to a film glyph on the card surface when the
// server has none (it answers 404) or the fetch fails, instead of leaving an
// empty dark rectangle in the rail.
//
// No fade-in: on a television every poster that scrolls in redrew its whole
// row for 200 ms, and the GPU, not the CPU, is what a modest set runs out of.
// The light mode also skips the blurhash, decoded per card before the poster.
export function PosterImage({
  uri,
  headers,
  style,
  blurhash,
  recyclingKey,
}: {
  uri: string;
  headers?: { Authorization: string };
  style: StyleProp<ImageStyle>;
  blurhash?: string;
  recyclingKey?: string;
}) {
  const theme = useTheme();
  const lite = useLiteMode();
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [uri]);

  if (failed) {
    return (
      <View
        style={[style as StyleProp<ViewStyle>, { backgroundColor: theme.colors.surface, alignItems: "center", justifyContent: "center" }]}
        testID="poster-fallback"
      >
        <Ionicons name="film-outline" size={40} color={theme.colors.textMuted} />
      </View>
    );
  }
  return (
    <Image
      cachePolicy="memory-disk"
      source={{ uri, headers }}
      style={style}
      contentFit="cover"
      recyclingKey={recyclingKey}
      placeholder={blurhash && !lite ? { blurhash } : undefined}
      placeholderContentFit="cover"
      onError={() => setFailed(true)}
    />
  );
}
