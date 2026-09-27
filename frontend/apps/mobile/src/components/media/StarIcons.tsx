import { View } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTheme } from "../../providers/ThemeProvider";

export function StarIcons({ rating }: { rating: number }) {
  const theme = useTheme();
  // rating out of 10, show 5 stars
  const filled = Math.round(rating / 2);
  return (
    <View style={{ flexDirection: "row", gap: 1 }}>
      {[1, 2, 3, 4, 5].map((i) => (
        <Ionicons
          key={i}
          name={i <= filled ? "star" : "star-outline"}
          size={12}
          color={i <= filled ? "#f5c518" : theme.colors.textMuted}
        />
      ))}
    </View>
  );
}
