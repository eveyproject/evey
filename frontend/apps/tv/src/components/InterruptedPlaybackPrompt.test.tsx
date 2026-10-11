import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, arg?: unknown) =>
      typeof arg === "string" ? arg : arg && typeof arg === "object" && "time" in arg ? `${key}:${(arg as { time: string }).time}` : key,
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", overlay: "rgba(0,0,0,.5)", border: "#333" } }),
}));

vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": hasTVPreferredFocus ? "1" : "0" }, children),
}));

import { InterruptedPlaybackPrompt } from "./InterruptedPlaybackPrompt";

const mark = { mediaId: "m1", profileId: "p1", title: "Le Film", positionMs: 2_530_000, pick: {}, at: 0 };

describe("InterruptedPlaybackPrompt", () => {
  it("renders nothing without an interrupted playback", () => {
    const { container } = render(<InterruptedPlaybackPrompt mark={null} onResume={vi.fn()} onDismiss={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("names the title and the position, resume holding the focus", () => {
    const onResume = vi.fn();
    const { getByText, getAllByRole } = render(<InterruptedPlaybackPrompt mark={mark} onResume={onResume} onDismiss={vi.fn()} />);
    expect(getByText("Le Film")).toBeTruthy();
    const [resume, dismiss] = getAllByRole("button");
    expect(resume.getAttribute("aria-label")).toMatch(/^player\.interruptedResume\.resumeAt:42:10$/);
    expect(resume.getAttribute("data-preferred")).toBe("1");
    expect(dismiss.getAttribute("data-preferred")).toBe("0");
    fireEvent.click(resume);
    expect(onResume).toHaveBeenCalledTimes(1);
  });

  it("dismisses", () => {
    const onDismiss = vi.fn();
    const { getAllByRole } = render(<InterruptedPlaybackPrompt mark={{ ...mark, positionMs: 0 }} onResume={vi.fn()} onDismiss={onDismiss} />);
    const [resume, dismiss] = getAllByRole("button");
    expect(resume.getAttribute("aria-label")).toBe("Resume");
    fireEvent.click(dismiss);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
