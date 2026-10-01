import { View, Text } from "react-native";
import { useTranslation } from "react-i18next";
import { useDemoStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";

// Over every admin screen on the demo: the settings are shown, not editable -
// the web says the same above its read-only admin pages. The phone's admin
// screens have no shared control to grey, and the server refuses every write
// there anyway; this line is what tells a refused save apart from a bug.
export function DemoReadOnlyNotice() {
  const { t } = useTranslation();
  const theme = useTheme();
  const demo = useDemoStore((s) => s.demoMode);
  if (!demo) return null;
  return (
    <View
      testID="demo-readonly"
      style={{ paddingHorizontal: theme.spacing.md, paddingVertical: 6, backgroundColor: theme.colors.warning + "22" }}
    >
      <Text style={{ color: theme.colors.text, fontSize: 12, textAlign: "center" }}>{t("common.demo.readOnly")}</Text>
    </View>
  );
}
