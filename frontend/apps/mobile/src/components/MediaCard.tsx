import React, { useEffect, useMemo, useState } from "react";
import { View, Text, Pressable } from "react-native";
import { ROW_CARD_WIDTH } from "../lib/rowCardWidth";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import type { V1MediaItem, V1Artwork } from "@evey/api";
import { displayTitle } from "@evey/api";
import { artworkUrl } from "@evey/client";
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

/**
 * Height of ONE line of the card's caption (title, and year under it).
 *
 * Set explicitly, and exported, because a grid that hands FlatList a row height
 * through getItemLayout needs that height to be a fact rather than a guess.
 * Left to the platform's default leading for a 13px font, the two lines came out
 * a couple of pixels taller than the cell reserved for them and the year was
 * sliced in half (emulator, 2026-09-05).
 */
export const CARD_LINE_HEIGHT = 17;
/** Gap between the poster and the first caption line. */
export const CARD_TEXT_GAP = 4;
/** Exact height of the caption block: title + year. */
export const CARD_CAPTION_HEIGHT = CARD_TEXT_GAP + CARD_LINE_HEIGHT * 2;
const DEFAULT_WIDTH = ROW_CARD_WIDTH;

export const MediaCard = React.memo(function MediaCard({ item, onPress, width, watchPercent, watched, posterBlurhash: posterBlurhashProp }: MediaCardProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const cardWidth = width ?? DEFAULT_WIDTH;
  const cardHeight = cardWidth * POSTER_RATIO;
  const serverUrl = useServerUrl();

  // Versioned by the item's updatedAt: artwork is served immutable for a year,
  // so a replaced poster must change its URL to reach the phone's image cache.
  const imageUri = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 400, item) : undefined;
  const imageSource = useAuthenticatedImage(imageUri);

  // A title without a poster answers 404: show a film glyph instead of an empty rectangle.
  const [posterFailed, setPosterFailed] = useState(false);
  useEffect(() => setPosterFailed(false), [imageUri]);

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
        {posterFailed && (
          <View style={{ position: "absolute", top: 0, left: 0, right: 0, bottom: 0, alignItems: "center", justifyContent: "center" }}>
            <Ionicons name="film-outline" size={ICON_SIZE["3xl"]} color={theme.colors.textMuted} />
          </View>
        )}
        {imageSource && !posterFailed && (
          <Image
            source={imageSource}
            style={{ width: cardWidth, height: cardHeight }}
            contentFit="cover"
            onError={() => setPosterFailed(true)}
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
        style={{ color: theme.colors.text, fontSize: FONT_SIZE.xs, lineHeight: CARD_LINE_HEIGHT, marginTop: CARD_TEXT_GAP }}
      >
        {displayTitle(item, t)}
      </Text>
      {!!item.year && (
        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs, lineHeight: CARD_LINE_HEIGHT }}>{item.year}</Text>
      )}
    </Pressable>
  );
});
