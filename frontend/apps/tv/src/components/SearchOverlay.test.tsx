import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Modal: ({ children, visible }: any) => (visible ? children : null),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
  useWindowDimensions: () => ({ width: 1920, height: 1080 }),
}));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#aaa", accent: "#88f", border: "#333" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// The panel is covered by its own suite (the /search screen test): here it is
// a stand-in that reports the props the overlay owes it.
vi.mock("./SearchPanel", () => ({
  SearchPanel: ({ hasTVPreferredFocus, onNavigate }: any) =>
    React.createElement(
      "button",
      { "aria-label": `panel focus=${String(hasTVPreferredFocus)}`, onClick: onNavigate },
      "panel",
    ),
}));

import { SearchOverlay } from "./SearchOverlay";

describe("SearchOverlay", () => {
  it("ne rend rien tant qu'elle n'est pas ouverte", () => {
    render(<SearchOverlay visible={false} onClose={vi.fn()} />);
    expect(screen.queryByLabelText(/^panel/)).toBeNull();
  });

  // The field must hold the remote from the first frame: an open dropdown
  // whose input is not focused is a dropdown you cannot type into.
  it("donne le focus initial au champ du panneau", () => {
    render(<SearchOverlay visible onClose={vi.fn()} />);
    expect(screen.getByLabelText("panel focus=true")).toBeTruthy();
  });

  it("se ferme quand le panneau emmène le spectateur ailleurs", () => {
    const onClose = vi.fn();
    render(<SearchOverlay visible onClose={onClose} />);
    fireEvent.click(screen.getByLabelText("panel focus=true"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("se ferme au clic sur le fond", () => {
    const onClose = vi.fn();
    render(<SearchOverlay visible onClose={onClose} />);
    fireEvent.click(screen.getByLabelText("common.close"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
