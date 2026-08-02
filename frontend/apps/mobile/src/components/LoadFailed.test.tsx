import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", textMuted: "#888", accent: "#f00" } }),
}));

import { LoadFailed } from "./LoadFailed";

describe("LoadFailed", () => {
  it("dit que le chargement a echoue", () => {
    render(<LoadFailed />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
  });

  it("propose un reessai quand il y a quelque chose a relancer", () => {
    const onRetry = vi.fn();
    render(<LoadFailed onRetry={onRetry} />);
    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("n'affiche pas de bouton quand il n'y a rien a relancer", () => {
    render(<LoadFailed />);
    expect(screen.queryByLabelText("common.retry")).toBeNull();
  });

  it("prefere l'explication fournie par l'appelant", () => {
    render(<LoadFailed message="Le serveur redemarre" />);
    expect(screen.getByText("Le serveur redemarre")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });
});
