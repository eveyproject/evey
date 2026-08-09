import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  View,
  Text,
  Pressable,
  Modal,
  Animated,
  ScrollView,
  StyleSheet,
  Dimensions,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

const SCREEN_HEIGHT = Dimensions.get("screen").height;

interface BottomSheetProps {
  visible: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}

export function BottomSheet({ visible, onClose, title, children }: BottomSheetProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const translateY = useRef(new Animated.Value(SCREEN_HEIGHT)).current;

  useEffect(() => {
    if (visible) {
      Animated.spring(translateY, {
        toValue: 0,
        useNativeDriver: true,
        damping: 20,
        stiffness: 200,
      }).start();
    } else {
      Animated.timing(translateY, {
        toValue: SCREEN_HEIGHT,
        duration: 200,
        useNativeDriver: true,
      }).start();
    }
  }, [visible, translateY]);

  // A hidden RN Modal keeps its children MOUNTED (it only hides the native
  // presentation), so every child of every sheet re-rendered on each of the
  // player's ~2/s currentTime ticks for the whole playback. Rendering nothing
  // while hidden costs nothing visible either: Modal visible={false} already
  // hides instantly, the 200ms slide-out above only reset translateY.
  if (!visible) return null;

  return (
    <Modal visible={visible} transparent animationType="none" onRequestClose={onClose}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose} accessible={true} accessibilityLabel={t("common.close", "Close")} accessibilityRole="button">
        <View />
      </Pressable>
      {/* Safe-area padding so the last options never land under the gesture
          nav bar / bottom cutout (the "exclusion zone"), and so the sheet
          clears the left/right cutouts in landscape - the player is often
          watched sideways. Without this the bottom rows were unreachable. */}
      <Animated.View
        style={[
          styles.sheet,
          {
            transform: [{ translateY }],
            backgroundColor: theme.colors.surface,
            paddingBottom: Math.max(20, insets.bottom + 12),
            paddingLeft: insets.left,
            paddingRight: insets.right,
          },
        ]}
      >
        <View style={[styles.handle, { backgroundColor: theme.colors.textMuted }]} />
        <Text style={[styles.title, { color: theme.colors.text, borderBottomColor: theme.colors.divider }]}>{title}</Text>
        <ScrollView style={styles.content} bounces={false}>
          {children}
        </ScrollView>
      </Animated.View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
  },
  sheet: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    borderTopLeftRadius: 16,
    borderTopRightRadius: 16,
    maxHeight: "60%",
    paddingBottom: 20,
  },
  handle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    alignSelf: "center",
    marginTop: 10,
    marginBottom: 8,
  },
  title: {
    fontSize: FONT_SIZE.base,
    fontWeight: "600",
    textAlign: "center",
    paddingBottom: 12,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  content: {
    paddingHorizontal: 16,
    paddingTop: 8,
  },
});
