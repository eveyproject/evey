import React from "react";
import { View, Text, Pressable, Dimensions } from "react-native";
import { Image } from "expo-image";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthenticatedImage } from "../hooks/useAuthenticatedImage";
import { ProgressBar } from "./ProgressBar";

interface MediaCardProps {
  item: V1MediaItem;
  onPress?: () => void;
  width?: number;
  watchPercent?: number;
}

const POSTER_RATIO = 1.5;
const DEFAULT_WIDTH = Dimensions.get("window").width * 0.35;

export function MediaCard({ item, onPress, width, watchPercent }: MediaCardProps) {
  const theme = useTheme();
  const cardWidth = width ?? DEFAULT_WIDTH;
  const cardHeight = cardWidth * POSTER_RATIO;
  const serverUrl = useServerUrl();

  const imageUri = serverUrl ? `${serverUrl}/api/v1/media/${item.id}/artwork/poster` : undefined;
  const imageSource = useAuthenticatedImage(imageUri);

  return (
    <Pressable onPress={onPress} style={{ width: cardWidth, marginRight: theme.spacing.sm }} accessible={true} accessibilityLabel={item.title} accessibilityRole="button">
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
          />
        )}
        {watchPercent != null && watchPercent > 0 && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: 4 }}>
            <ProgressBar percent={watchPercent} />
          </View>
        )}
      </View>
      <Text
        numberOfLines={1}
        style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}
      >
        {item.title}
      </Text>
      {item.year != null && (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.year}</Text>
      )}
    </Pressable>
  );
}
