import React from "react";

// rnDom is a DOM double of the react-native primitives the admin settings
// screens use, for `vi.mock("react-native", async () => (await import("<path>/test/rnDom")).rnDom)`.
// It keeps what a test asserts on: testID (data-testid), the accessibility
// role, name and state (role, aria-label, aria-checked / aria-selected /
// aria-disabled), the press and change handlers, a Modal that renders its
// children only while visible, and the flattened style (data-style) so a
// layout rule such as "equal widths" can be read.

type AnyProps = Record<string, any>;

function flatStyle(style: unknown): Record<string, unknown> {
  if (!style) return {};
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flatStyle));
  if (typeof style === "object") return style as Record<string, unknown>;
  return {};
}

function a11y(p: AnyProps): AnyProps {
  const out: AnyProps = {};
  if (p.testID) out["data-testid"] = p.testID;
  if (p.accessibilityLabel !== undefined) out["aria-label"] = p.accessibilityLabel;
  const role = p.accessibilityRole;
  if (role && role !== "button" && role !== "none") out.role = role === "header" ? "heading" : role;
  const st = p.accessibilityState ?? {};
  if (role === "radio" || role === "checkbox") out["aria-checked"] = !!st.checked;
  if (role === "tab") out["aria-selected"] = !!st.selected;
  if (st.disabled) out["aria-disabled"] = true;
  if (p.style) out["data-style"] = JSON.stringify(flatStyle(p.style));
  return out;
}

const View = React.forwardRef<HTMLDivElement, AnyProps>(({ children, ...p }, ref) =>
  React.createElement("div", { ref, ...a11y(p) }, children),
);
const Text = ({ children, ...p }: AnyProps) => React.createElement("span", a11y(p), children);
const Pressable = React.forwardRef<HTMLButtonElement, AnyProps>(({ children, onPress, disabled, ...p }, ref) =>
  React.createElement(
    "button",
    { ref, type: "button", onClick: disabled ? undefined : onPress, disabled: !!disabled, ...a11y(p) },
    typeof children === "function" ? children({ pressed: false }) : children,
  ),
);
const TextInput = ({ value, onChangeText, placeholder, editable, secureTextEntry, ...p }: AnyProps) =>
  React.createElement("input", {
    value: value ?? "",
    placeholder,
    readOnly: editable === false,
    disabled: editable === false,
    type: secureTextEntry ? "password" : "text",
    onChange: (e: any) => onChangeText?.(e.target.value),
    ...a11y(p),
    "aria-label": p.accessibilityLabel ?? placeholder,
  });
const Switch = ({ value, onValueChange, disabled, ...p }: AnyProps) =>
  React.createElement("input", {
    type: "checkbox",
    role: "switch",
    checked: !!value,
    disabled: !!disabled,
    onChange: () => onValueChange?.(!value),
    ...a11y(p),
  });
const Modal = ({ visible, children }: AnyProps) => (visible ? React.createElement("div", { role: "dialog" }, children) : null);
const ScrollView = React.forwardRef<HTMLDivElement, AnyProps>(({ children, ...p }, ref) => {
  React.useImperativeHandle(ref, () => ({ scrollTo: () => {} }) as any);
  return React.createElement("div", a11y(p), children);
});
const FlatList = ({ data, renderItem, keyExtractor, ListEmptyComponent, ...p }: AnyProps) =>
  React.createElement(
    "div",
    a11y(p),
    (data ?? []).length === 0 && ListEmptyComponent
      ? typeof ListEmptyComponent === "function" ? React.createElement(ListEmptyComponent) : ListEmptyComponent
      : (data ?? []).map((item: unknown, index: number) =>
          React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, index) : index }, renderItem({ item, index })),
        ),
  );
const ActivityIndicator = () => React.createElement("div", { "data-testid": "spinner" });
const RefreshControl = () => null;

export const rnDom = {
  View,
  Text,
  Pressable,
  TouchableOpacity: Pressable,
  TextInput,
  Switch,
  Modal,
  ScrollView,
  FlatList,
  ActivityIndicator,
  RefreshControl,
  StyleSheet: { create: <T,>(s: T) => s, hairlineWidth: 1, absoluteFillObject: {}, flatten: flatStyle },
  AccessibilityInfo: { setAccessibilityFocus: () => {} },
  findNodeHandle: () => null,
  Platform: { OS: "android", select: (o: Record<string, unknown>) => o.android },
  Alert: { alert: () => {} },
  Linking: { openURL: () => Promise.resolve() },
};
