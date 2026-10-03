import Svg, { Defs, LinearGradient, Rect, Stop } from "react-native-svg";
import { View, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { heroGradientBands } from "./heroGradientBands";

interface MediaHeroBackdropProps {
  uri?: string;
  headers?: { Authorization: string };
  height: number;
  onBack: () => void;
  topInset: number;
  leftInset: number;
}

// One real vertical gradient over the full hero height: transparent at the top,
// the page background at the bottom, on the same eased (quadratic) curve as
// heroGradientBands, so there are no flat steps to read as bands.
const GRADIENT_STOPS = heroGradientBands(24);

export function MediaHeroBackdrop({ uri, headers, height, onBack, topInset, leftInset }: MediaHeroBackdropProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  return (
    <View style={{ width: "100%", height }}>
      {uri ? (
        <Image
          source={{ uri, headers }}
          style={StyleSheet.absoluteFillObject}
          contentFit="cover"
          transition={300}
        />
      ) : (
        <View style={[StyleSheet.absoluteFillObject, { backgroundColor: theme.colors.surface }]} />
      )}
      <Svg style={StyleSheet.absoluteFillObject} width="100%" height="100%" pointerEvents="none">
        <Defs>
          <LinearGradient id="heroFade" x1="0" y1="0" x2="0" y2="1">
            {GRADIENT_STOPS.map((opacity, i) => (
              <Stop key={i} offset={(i + 1) / GRADIENT_STOPS.length} stopColor={theme.colors.bg} stopOpacity={opacity} />
            ))}
          </LinearGradient>
        </Defs>
        <Rect x="0" y="0" width="100%" height="100%" fill="url(#heroFade)" />
      </Svg>
      <Pressable
        onPress={onBack}
        hitSlop={12}
        accessible={true}
        accessibilityLabel={t("common.back")}
        accessibilityRole="button"
        style={{
          position: "absolute",
          top: topInset + 8,
          left: leftInset + 8,
          width: 36,
          height: 36,
          borderRadius: 18,
          backgroundColor: "rgba(0,0,0,0.5)",
          justifyContent: "center",
          alignItems: "center",
        }}
      >
        <Ionicons name="chevron-back" size={22} color="#fff" />
      </Pressable>
    </View>
  );
}
