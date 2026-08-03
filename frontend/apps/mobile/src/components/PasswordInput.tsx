import { useState } from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import type { TextInputProps, ViewStyle } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";

// Password field with a show/hide eye. On a phone keyboard most sign-in
// failures are invisible typos - the eye turns a blind retype into a glance.
// The wrapper absorbs the outer margins of the passed style: left on the
// inner TextInput they would offset the absolutely-centered eye by half the
// margin (the wrapper's height would include it).
export function PasswordInput({ style, ...props }: TextInputProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const flat = (StyleSheet.flatten(style) ?? {}) as ViewStyle;
  const { margin, marginTop, marginBottom, marginLeft, marginRight, ...inputStyle } = flat;
  return (
    <View style={{ position: "relative", margin, marginTop, marginBottom, marginLeft, marginRight }}>
      <TextInput
        {...props}
        secureTextEntry={!visible}
        style={[inputStyle, { paddingRight: 48 }]}
      />
      <Pressable
        onPress={() => setVisible((v) => !v)}
        hitSlop={8}
        accessible={true}
        accessibilityRole="button"
        accessibilityLabel={
          visible
            ? t("mobile.auth.hidePassword", "Hide password")
            : t("mobile.auth.showPassword", "Show password")
        }
        style={{ position: "absolute", right: 0, top: 0, bottom: 0, justifyContent: "center", paddingHorizontal: 12 }}
      >
        <Ionicons name={visible ? "eye-off-outline" : "eye-outline"} size={20} color={theme.colors.textMuted} />
      </Pressable>
    </View>
  );
}
