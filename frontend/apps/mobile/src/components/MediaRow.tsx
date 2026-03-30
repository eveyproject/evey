import { View, Text, FlatList } from "react-native";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { MediaCard } from "./MediaCard";

interface MediaRowProps {
  title: string;
  items: V1MediaItem[];
  onItemPress?: (item: V1MediaItem) => void;
  watchPercents?: Record<string, number>;
}

export function MediaRow({ title, items, onItemPress, watchPercents }: MediaRowProps) {
  const theme = useTheme();

  if (!items.length) return null;

  return (
    <View style={{ marginBottom: theme.spacing.lg }}>
      <Text style={{
        color: theme.colors.text,
        fontSize: 18,
        fontWeight: "600",
        marginBottom: theme.spacing.sm,
        paddingHorizontal: theme.spacing.md,
      }}>
        {title}
      </Text>
      <FlatList
        horizontal
        data={items}
        keyExtractor={(item) => item.id ?? ""}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: theme.spacing.md }}
        renderItem={({ item }) => (
          <MediaCard
            item={item}
            onPress={() => onItemPress?.(item)}
            watchPercent={watchPercents?.[item.id ?? ""]}
          />
        )}
      />
    </View>
  );
}
