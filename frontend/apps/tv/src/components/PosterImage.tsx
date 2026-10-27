import { View, type StyleProp, type ViewStyle } from "react-native";
import { Image, type ImageStyle } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTheme } from "../providers/ThemeProvider";
import { useLiteMode } from "../lib/liteMode";
import { useImageRetry } from "@evey/client";

// A poster that falls back to a film glyph on the card surface when the
// server has none (it answers 404) or the fetch fails, instead of leaving an
// empty dark rectangle in the rail.
//
// No fade-in: on a television every poster that scrolls in redrew its whole
// row for 200 ms, and the GPU, not the CPU, is what a modest set runs out of.
// The light mode also skips the blurhash, decoded per card before the poster.
export type PosterPriority = "low" | "normal" | "high";

export function PosterImage({
  uri,
  headers,
  style,
  blurhash,
  recyclingKey,
  priority,
}: {
  uri: string;
  headers?: { Authorization: string };
  style: StyleProp<ImageStyle>;
  blurhash?: string;
  recyclingKey?: string;
  /**
   * Where this poster queues among the others: the image loader decodes cached
   * posters one at a time, slowly on a small television, so what is on screen
   * goes first and what the home renders below the fold goes last.
   */
  priority?: PosterPriority;
}) {
  const theme = useTheme();
  const lite = useLiteMode();
  // Retried on failure, revived on a new token (see useImageRetry). A path
  // without a host is the server not known yet (a screen's first render):
  // nothing to load, nothing to fail.
  const absolute = /^https?:/i.test(uri) ? uri : undefined;
  const { src, failed, onError, onLoad } = useImageRetry(absolute, headers?.Authorization);

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
  if (!src) return <View style={[style as StyleProp<ViewStyle>, { backgroundColor: theme.colors.surface }]} />;
  return (
    <Image
      cachePolicy="memory-disk"
      source={{ uri: src, headers }}
      style={style}
      contentFit="cover"
      recyclingKey={recyclingKey}
      priority={priority}
      placeholder={blurhash && !lite ? { blurhash } : undefined}
      placeholderContentFit="cover"
      onError={onError}
      onLoad={onLoad}
    />
  );
}
