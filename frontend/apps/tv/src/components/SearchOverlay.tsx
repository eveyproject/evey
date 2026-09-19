import { View, Modal, Pressable, StyleSheet, useWindowDimensions } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { SearchPanel } from "./SearchPanel";

// The dropdown the owner asked for (2026-09-19): the header's field drops a
// large panel instead of navigating away, so searching never costs the screen
// the viewer was on. It is the SAME search as the /search route - the panel is
// shared - only anchored under the field and dismissed with BACK.
//
// Mounted once by the tabs shell, above every tab screen, because the header
// is part of that shell: one overlay, one piece of state, no per-screen copy.
export interface SearchOverlayProps {
  visible: boolean;
  onClose: () => void;
}

// The panel hangs below the header rather than filling the screen: the field
// it belongs to stays visible above it, which is what makes it read as a
// dropdown and not as a page.
const HEADER_CLEARANCE = 104;
const BOTTOM_GAP = 48;
const SIDE_GAP = 64;
const MAX_WIDTH = 1400;

export function SearchOverlay({ visible, onClose }: SearchOverlayProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  // Measured, not flexed. Inside an Android Modal the window has no size when
  // React first lays its children out, and a `flex: 1` chain never recovers:
  // the results list came out 2 px tall on the box (2026-09-19) with its rows
  // correctly built behind it. Explicit width and height give every child
  // below a definite box from the first frame.
  const { width, height } = useWindowDimensions();
  if (!visible) return null;
  const panelHeight = Math.max(240, height - HEADER_CLEARANCE - BOTTOM_GAP);
  const panelWidth = Math.min(MAX_WIDTH, width - SIDE_GAP * 2);
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      {/* The scrim closes on press for a pointer (the emulator, a mouse on a
          box); the remote closes it with BACK, which onRequestClose carries.
          It is deliberately NOT focusable: a D-pad must never land on a
          backdrop. */}
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t("common.close")}
        focusable={false}
        onPress={onClose}
        style={[styles.scrim, { backgroundColor: theme.colors.bg + "E6" }]}
      />
      <View style={[styles.anchor, { width, height }]} pointerEvents="box-none">
        <View
          style={[
            styles.panel,
            {
              width: panelWidth,
              height: panelHeight,
              backgroundColor: theme.colors.bg,
              borderRadius: theme.radii.lg ?? theme.radii.md,
              borderColor: theme.colors.border,
              shadowColor: theme.colors.shadow ?? "#000",
            },
          ]}
        >
          <SearchPanel hasTVPreferredFocus onNavigate={onClose} />
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  scrim: { ...StyleSheet.absoluteFillObject },
  anchor: {
    position: "absolute",
    top: 0,
    left: 0,
    paddingTop: HEADER_CLEARANCE,
    alignItems: "center",
  },
  // Wide on purpose: the results are posters and titles read from the sofa,
  // and the owner asked for a big dropdown, not a suggestion strip.
  panel: {
    borderWidth: 2,
    overflow: "hidden",
    elevation: 12,
    shadowOffset: { width: 0, height: 6 },
    shadowOpacity: 0.4,
    shadowRadius: 16,
  },
});
