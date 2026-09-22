import { describe, it, expect, vi } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import { SettingsPanel, type PanelSection } from "./SettingsPanel";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k }) }));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}` }));

const key = (k: string, keyCode: number) => fireEvent.keyDown(window, { key: k, keyCode });

function sections(onPick = vi.fn()): PanelSection[] {
  return [
    { key: "audio", label: "Audio", value: "ENG", choices: [{ key: "1", label: "ENG", selected: true }, { key: "2", label: "FRA" }], onPick },
    { key: "quality", label: "Quality", value: "Auto", choices: [{ key: "0", label: "Auto", selected: true }, { key: "720", label: "720p" }], onPick },
  ];
}

// The remote walks the menu with up/down, OK opens a list on the current
// choice, a down right after OK moves to the next entry (it used to be
// wiped by the effect that set the row after the open), OK picks and closes,
// back climbs one level, and a stray left/right never reaches the player.
describe("SettingsPanel", () => {
  it("opens a list, keeps a fast key after OK, picks and closes", () => {
    const onPick = vi.fn();
    const onClose = vi.fn();
    render(<SettingsPanel sections={sections(onPick)} onClose={onClose} />);
    key("Enter", 13);
    expect(screen.getByText("FRA")).toBeTruthy();
    key("ArrowDown", 40);
    key("Enter", 13);
    expect(onPick).toHaveBeenCalledWith("2");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("back closes the list first, then the menu, and left/right are swallowed", () => {
    const onClose = vi.fn();
    render(<SettingsPanel sections={sections()} onClose={onClose} />);
    const seen: string[] = [];
    const spy = (e: KeyboardEvent) => seen.push(e.key);
    window.addEventListener("keydown", spy);
    key("ArrowDown", 40);
    key("Enter", 13);
    expect(screen.getByText("720p")).toBeTruthy();
    key("ArrowRight", 39);
    key("Escape", 27);
    expect(screen.queryByText("720p")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    key("GoBack", 461);
    expect(onClose).toHaveBeenCalledTimes(1);
    // The panel listens at capture and stops propagation: nothing reached the bubbling listener.
    expect(seen).toEqual([]);
    window.removeEventListener("keydown", spy);
  });
});
