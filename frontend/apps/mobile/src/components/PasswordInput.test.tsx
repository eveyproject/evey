import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  TextInput: ({ secureTextEntry, style, ...rest }: Record<string, unknown>) =>
    React.createElement("input", {
      ...rest,
      type: secureTextEntry ? "password" : "text",
      "data-style": JSON.stringify(style),
    }),
  Pressable: ({ children, onPress, accessibilityLabel }: Record<string, unknown>) =>
    React.createElement(
      "button",
      { onClick: onPress as () => void, "aria-label": accessibilityLabel as string },
      children as React.ReactNode,
    ),
  StyleSheet: { flatten: (s: unknown) => (Array.isArray(s) ? Object.assign({}, ...s) : (s ?? {})) },
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: { name: string }) => React.createElement("span", { "data-icon": name }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { textMuted: "#888" } }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_key: string, fallback: string) => fallback }),
}));

import { PasswordInput } from "./PasswordInput";

describe("PasswordInput", () => {
  it("masks the value by default and shows the open eye", () => {
    const { container } = render(<PasswordInput placeholder="Password" />);
    expect(container.querySelector("input")!.getAttribute("type")).toBe("password");
    expect(container.querySelector("[data-icon]")!.getAttribute("data-icon")).toBe("eye-outline");
  });

  it("reveals the value when the eye is pressed, masks it again on second press", () => {
    const { container, getByRole } = render(<PasswordInput placeholder="Password" />);
    fireEvent.click(getByRole("button"));
    expect(container.querySelector("input")!.getAttribute("type")).toBe("text");
    expect(container.querySelector("[data-icon]")!.getAttribute("data-icon")).toBe("eye-off-outline");

    fireEvent.click(getByRole("button"));
    expect(container.querySelector("input")!.getAttribute("type")).toBe("password");
  });

  // The login screen's shared input style carries a marginBottom. If it stayed
  // on the inner TextInput, the absolutely-positioned eye would center on
  // input+margin and sit visibly too high - the wrapper must absorb margins.
  it("moves outer margins from the input style to the wrapper", () => {
    const { container } = render(
      <PasswordInput style={{ marginBottom: 12, padding: 8 }} placeholder="Password" />,
    );
    const wrapperStyle = JSON.parse(container.querySelector("div")!.getAttribute("data-style")!);
    expect(wrapperStyle.marginBottom).toBe(12);
    const inputStyleParts = JSON.parse(container.querySelector("input")!.getAttribute("data-style")!) as object[];
    const inputStyle = Object.assign({}, ...inputStyleParts) as Record<string, unknown>;
    expect(inputStyle.marginBottom).toBeUndefined();
    expect(inputStyle.padding).toBe(8);
  });

  it("forwards text props to the underlying input", () => {
    const onChangeText = vi.fn();
    const { container } = render(
      <PasswordInput placeholder="Password" value="secret" onChangeText={onChangeText} />,
    );
    expect(container.querySelector("input")!.getAttribute("placeholder")).toBe("Password");
  });
});
