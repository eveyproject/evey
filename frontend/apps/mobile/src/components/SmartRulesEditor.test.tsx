import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "aria-selected": accessibilityState?.selected ?? undefined },
      children,
    ),
  TextInput: ({ value, onChangeText, placeholder, accessibilityLabel }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel ?? placeholder,
      placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", bg: "#000", border: "#333", onAccent: "#fff", overlay: "rgba(0,0,0,.5)", error: "#f33" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));
vi.mock("../hooks/useAllGenres", () => ({ useAllGenres: () => ({ data: ["Aventure", "Drame"] }) }));

import { SmartRulesEditor, COLLECTION_FIELDS } from "./SmartRulesEditor";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SmartRulesEditor", () => {
  // The parent re-renders with a fresh rules array (a refetch, a parent state
  // change): what was typed must survive until the editor is closed.
  it("keeps the rules being edited when the parent re-renders while open", () => {
    const rules = () => [{ field: "year" as const, operator: "gte" as const, value: "2020" }];
    const { rerender } = render(<SmartRulesEditor visible onClose={vi.fn()} rules={rules()} onSave={vi.fn()} />);
    const input = () => screen.getByDisplayValue(/^(2020|1999)$/) as HTMLInputElement;
    fireEvent.change(input(), { target: { value: "1999" } });
    rerender(<SmartRulesEditor visible onClose={vi.fn()} rules={rules()} onSave={vi.fn()} />);
    expect(input().value).toBe("1999");

    // Closed then reopened: back to what the parent holds.
    rerender(<SmartRulesEditor visible={false} onClose={vi.fn()} rules={rules()} onSave={vi.fn()} />);
    rerender(<SmartRulesEditor visible onClose={vi.fn()} rules={rules()} onSave={vi.fn()} />);
    expect(input().value).toBe("2020");
  });

  it("offers the playlist fields by default and rating only when a collection opts in", () => {
    const { unmount } = render(<SmartRulesEditor visible onClose={vi.fn()} rules={[{ field: "genre", operator: "eq", value: "" }]} onSave={vi.fn()} />);
    expect(screen.getByText("Library")).toBeTruthy();
    expect(screen.queryByText("Rating ≥")).toBeNull();
    unmount();

    render(<SmartRulesEditor visible onClose={vi.fn()} rules={[{ field: "genre", operator: "eq", value: "" }]} onSave={vi.fn()} availableFields={COLLECTION_FIELDS} />);
    expect(screen.getByText("Rating ≥")).toBeTruthy();
  });

  it("keeps the rating input to digits and a single dot, and saves it as a gte rule", () => {
    const onSave = vi.fn();
    const onClose = vi.fn();
    render(<SmartRulesEditor visible onClose={onClose} rules={[{ field: "genre", operator: "eq", value: "" }]} onSave={onSave} availableFields={COLLECTION_FIELDS} />);

    fireEvent.click(screen.getByText("Rating ≥"));
    const input = screen.getByPlaceholderText("7.5") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "7a.5.2" } });
    expect(input.value).toBe("7.52");

    fireEvent.click(screen.getByLabelText("common.save"));
    expect(onSave).toHaveBeenCalledWith([{ field: "rating_gte", operator: "gte", value: "7.52" }]);
    expect(onClose).toHaveBeenCalled();
  });

  it("drops blank rules on save and picks real genres from the libraries", () => {
    const onSave = vi.fn();
    render(
      <SmartRulesEditor
        visible
        onClose={vi.fn()}
        rules={[
          { field: "genre", operator: "eq", value: "" },
          { field: "year", operator: "gte", value: "2020" },
        ]}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByLabelText("Drame"));
    fireEvent.click(screen.getByLabelText("Add Rule"));
    fireEvent.click(screen.getByLabelText("common.save"));
    expect(onSave).toHaveBeenCalledWith([
      { field: "genre", operator: "eq", value: "Drame" },
      { field: "year", operator: "gte", value: "2020" },
    ]);
  });

  it("re-syncs its draft from the parent's rules when re-opened", () => {
    const onSave = vi.fn();
    const { rerender } = render(<SmartRulesEditor visible onClose={vi.fn()} rules={[{ field: "year", operator: "gte", value: "1999" }]} onSave={onSave} />);
    expect((screen.getByPlaceholderText("2020") as HTMLInputElement).value).toBe("1999");
    rerender(<SmartRulesEditor visible={false} onClose={vi.fn()} rules={[{ field: "year", operator: "gte", value: "1999" }]} onSave={onSave} />);
    rerender(<SmartRulesEditor visible onClose={vi.fn()} rules={[{ field: "year", operator: "lte", value: "2010" }]} onSave={onSave} />);
    expect((screen.getByPlaceholderText("2020") as HTMLInputElement).value).toBe("2010");
  });
});
