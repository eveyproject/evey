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

// See heroGradientBands.ts for why this is a stack of flat-opacity layers
// rather than a real gradient, and why it spans the FULL hero height (the
// previous version stopped at 60%, which is what produced the reported
// "visible band, brutal cut" - nothing darkened the top 40% at all, then a
// coarse 4-step jump below it).
const GRADIENT_STEPS = 24;
const GRADIENT_BANDS = heroGradientBands(GRADIENT_STEPS);

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
      <View style={[StyleSheet.absoluteFillObject, { flexDirection: "column" }]}>
        {GRADIENT_BANDS.map((opacity, i) => (
          <View key={i} style={{ flex: 1, backgroundColor: theme.colors.bg, opacity }} />
        ))}
      </View>
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
