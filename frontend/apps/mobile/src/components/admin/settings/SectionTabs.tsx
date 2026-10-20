import { useEffect, useRef } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";

export interface SectionTab {
  id: string;
  label: string;
}

interface SectionTabsProps {
  /** Accessible name of the tab list. */
  label: string;
  tabs: SectionTab[];
  value: string;
  onChange: (id: string) => void;
  testID?: string;
}

// SectionTabs is the section menu of a long settings screen on a phone: one
// horizontally scrolling row of tabs. Eight section names do not fit 390 px,
// so the row scrolls, and it scrolls by itself to keep the open tab in view
// (with a peek of its neighbour, so the row reads as scrollable).
export function SectionTabs({ label, tabs, value, onChange, testID }: SectionTabsProps) {
  const theme = useTheme();
  const scroller = useRef<ScrollView>(null);
  const spans = useRef<Record<string, { x: number; width: number }>>({});

  useEffect(() => {
    const span = spans.current[value];
    if (span) scroller.current?.scrollTo?.({ x: Math.max(0, span.x - 48), animated: true });
  }, [value]);

  return (
    <View style={[styles.bar, { backgroundColor: theme.colors.bg, borderBottomColor: theme.colors.border }]}>
      <ScrollView
        ref={scroller}
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={styles.row}
        accessibilityRole="tablist"
        accessibilityLabel={label}
        testID={testID}
      >
        {tabs.map((tab) => {
          const active = tab.id === value;
          return (
            <Pressable
              key={tab.id}
              onPress={() => onChange(tab.id)}
              onLayout={(e) => {
                spans.current[tab.id] = { x: e.nativeEvent.layout.x, width: e.nativeEvent.layout.width };
              }}
              accessibilityRole="tab"
              accessibilityLabel={tab.label}
              accessibilityState={{ selected: active }}
              testID={`section-tab-${tab.id}`}
              style={[
                styles.tab,
                {
                  borderRadius: theme.radii.md,
                  backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                  borderColor: active ? theme.colors.accent : theme.colors.border,
                },
              ]}
            >
              <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
                {tab.label}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: { borderBottomWidth: StyleSheet.hairlineWidth },
  row: { paddingHorizontal: 16, paddingVertical: 10, gap: 8 },
  tab: { borderWidth: 1, paddingHorizontal: 14, minHeight: 40, justifyContent: "center" },
});
