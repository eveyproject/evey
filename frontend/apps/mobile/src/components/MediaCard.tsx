import React, { useMemo } from "react";
import { View, Text, Pressable, Dimensions } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import type { V1MediaItem, V1Artwork } from "@evey/api";
import { displayTitle } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthenticatedImage } from "../hooks/useAuthenticatedImage";
import { ProgressBar } from "./ProgressBar";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface MediaCardProps {
  item: V1MediaItem;
  onPress?: () => void;
  width?: number;
  watchPercent?: number;
  /** When true, render the "watched" check badge in the bottom-right corner. */
  watched?: boolean;
  /**
   * Explicit blurhash placeholder used when the caller has a FeedItem (which
   * carries primaryBlurhash inline) rather than a full MediaItem with its
   * artwork array. Falls back to item.artwork lookup when omitted.
   */
  posterBlurhash?: string;
}

const POSTER_RATIO = 1.5;
const DEFAULT_WIDTH = Dimensions.get("window").width * 0.35;

export const MediaCard = React.memo(function MediaCard({ item, onPress, width, watchPercent, watched, posterBlurhash: posterBlurhashProp }: MediaCardProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const cardWidth = width ?? DEFAULT_WIDTH;
  const cardHeight = cardWidth * POSTER_RATIO;
  const serverUrl = useServerUrl();

  const imageUri = serverUrl ? `${serverUrl}/api/v1/media/${item.id}/artwork/poster?w=400` : undefined;
  const imageSource = useAuthenticatedImage(imageUri);

  // Primary blurhash drives the expo-image placeholder; the explicit prop
  // (passed by feed sliders carrying FeedItem.primaryBlurhash) wins. Falls
  // back to primary → backdrop on item.artwork for detail pages. Empty hash
  // = expo-image shows surfaceColor while loading (pre-feature baseline).
  const blurhash = useMemo(() => {
    if (posterBlurhashProp) return posterBlurhashProp;
    const arts = (item.artwork ?? []) as V1Artwork[];
    const primary = arts.find((a) => a.type === "ARTWORK_TYPE_PRIMARY");
    if (primary?.blurhash) return primary.blurhash;
    const backdrop = arts.find((a) => a.type === "ARTWORK_TYPE_BACKDROP");
    return backdrop?.blurhash ?? "";
  }, [item.artwork, posterBlurhashProp]);

  return (
    <Pressable onPress={onPress} style={{ width: cardWidth, marginRight: theme.spacing.sm }} accessible={true} accessibilityLabel={displayTitle(item, t)} accessibilityRole="button">
      <View style={{
        width: cardWidth,
        height: cardHeight,
        borderRadius: theme.radii.md,
        backgroundColor: theme.colors.surface,
        overflow: "hidden",
      }}>
        {imageSource && (
          <Image
            source={imageSource}
            style={{ width: cardWidth, height: cardHeight }}
            contentFit="cover"
            transition={200}
            placeholder={blurhash ? { blurhash } : undefined}
            placeholderContentFit="cover"
          />
        )}
        {watchPercent != null && watchPercent > 0 && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: 4 }}>
            <ProgressBar percent={watchPercent} />
          </View>
        )}
        {watched && (
          <View style={{
            position: "absolute",
            top: 6,
            right: 6,
            width: 22,
            height: 22,
            borderRadius: 11,
            backgroundColor: theme.colors.success,
            alignItems: "center",
            justifyContent: "center",
          }}>
            <Ionicons name="checkmark" size={ICON_SIZE.sm} color={theme.colors.onSuccess} />
          </View>
        )}
      </View>
      <Text
        numberOfLines={1}
        style={{ color: theme.colors.text, fontSize: FONT_SIZE.xs, marginTop: 4 }}
      >
        {displayTitle(item, t)}
      </Text>
      {item.year != null && (
        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{item.year}</Text>
      )}
    </Pressable>
  );
});
