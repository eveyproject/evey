import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { DEFAULT_SUBTITLE_STYLE, type SubtitleStyle } from "@evey/client";

let style: SubtitleStyle = { ...DEFAULT_SUBTITLE_STYLE };
let ready = true;
const update = vi.fn();
vi.mock("@evey/client", async (orig) => ({
  ...(await orig<typeof import("@evey/client")>()),
  useSubtitleStyleSetting: () => ({ style, update, ready }),
}));

import { SubtitleStyleSettings } from "./SubtitleStyleSettings";

describe("SubtitleStyleSettings", () => {
  beforeEach(() => {
    update.mockClear();
    ready = true;
    style = { ...DEFAULT_SUBTITLE_STYLE };
  });

  // The preview is the point of the section: it must show the profile's
  // style, not a fixed sample.
  it("draws the preview in the profile's style", () => {
    style = { ...DEFAULT_SUBTITLE_STYLE, fontSize: 150, color: "#fde047", bgColor: "#1e3a5f", bgOpacity: 50 };
    render(<SubtitleStyleSettings />);
    const preview = screen.getByTestId("subtitle-style-preview");
    expect(preview.style.color).toBe("#fde047");
    expect(preview.style.fontSize).toBe("24px");
    expect(preview.style.backgroundColor.replace(/\s/g, "")).toBe("rgba(30,58,95,0.5)");
  });

  it("sends each control's change as a patch", () => {
    render(<SubtitleStyleSettings />);
    fireEvent.change(screen.getByLabelText("Size"), { target: { value: "140" } });
    expect(update).toHaveBeenLastCalledWith({ fontSize: 140 });
    fireEvent.click(screen.getByLabelText("Yellow"));
    expect(update).toHaveBeenLastCalledWith({ color: "#fde047" });
    fireEvent.click(screen.getByRole("button", { name: "Top" }));
    expect(update).toHaveBeenLastCalledWith({ position: "top" });
    fireEvent.click(screen.getByLabelText("Text shadow"));
    expect(update).toHaveBeenLastCalledWith({ shadow: true });
  });

  // "None" is the same black as "Black" with no opacity: picking a colour
  // again after it must bring a visible background back.
  it("turns the background off with None and back on with a colour", () => {
    render(<SubtitleStyleSettings />);
    fireEvent.click(screen.getByLabelText("None"));
    expect(update).toHaveBeenLastCalledWith({ bgColor: "#000000", bgOpacity: 0 });

    style = { ...DEFAULT_SUBTITLE_STYLE, bgOpacity: 0 };
    update.mockClear();
    render(<SubtitleStyleSettings />);
    fireEvent.click(screen.getAllByLabelText("Navy")[1]);
    expect(update).toHaveBeenLastCalledWith({ bgColor: "#1e3a5f", bgOpacity: 75 });
  });

  it("locks the controls until the profile is known", () => {
    ready = false;
    render(<SubtitleStyleSettings />);
    expect(screen.getByLabelText("Size")).toBeDisabled();
  });
});
