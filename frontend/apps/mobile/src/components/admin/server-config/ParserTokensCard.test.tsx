import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ onChangeText, value, testID, ...p }: any) =>
    React.createElement("input", {
      "data-testid": testID,
      value: value ?? "",
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  TouchableOpacity: ({ children, onPress, disabled, testID, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888" },
    radii: { sm: 4 },
  }),
}));

import { ParserTokensCard } from "./ParserTokensCard";

const saveEntry = vi.fn();
beforeEach(() => saveEntry.mockClear());

describe("ParserTokensCard", () => {
  it("seeds the fields from the persisted parser.config blob", () => {
    render(
      <ParserTokensCard
        entries={[{ key: "parser.config", value: '{"extra_qualities":["hdrip","hdr"],"extra_edition_suffixes":["imax"]}' }]}
        saveEntry={saveEntry}
        saving={false}
      />,
    );
    expect((screen.getByTestId("parser-tokens-qualities") as HTMLInputElement).value).toBe("hdrip, hdr");
    expect((screen.getByTestId("parser-tokens-editions") as HTMLInputElement).value).toBe("imax");
  });

  it("saves a normalized JSON blob (trimmed, empties dropped)", () => {
    render(<ParserTokensCard entries={[]} saveEntry={saveEntry} saving={false} />);
    fireEvent.change(screen.getByTestId("parser-tokens-qualities"), { target: { value: " hdrip , , web-dl " } });
    fireEvent.click(screen.getByTestId("parser-tokens-save-btn"));

    expect(saveEntry).toHaveBeenCalledWith({
      key: "parser.config",
      value: JSON.stringify({ extra_qualities: ["hdrip", "web-dl"], extra_edition_suffixes: [] }),
    });
  });

  it("does nothing until a field was edited", () => {
    render(<ParserTokensCard entries={[]} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("parser-tokens-save-btn"));
    expect(saveEntry).not.toHaveBeenCalled();
  });
});
