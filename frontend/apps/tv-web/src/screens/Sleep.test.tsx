import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

const goBack = vi.fn();
vi.mock("../app/router", () => ({ goBack: () => goBack() }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f: string) => f }) }));

import { SleepScreen } from "./Sleep";

describe("SleepScreen", () => {
  beforeEach(() => goBack.mockReset());

  it("is black and leaves on the first key, once", () => {
    const appBack = vi.fn();
    window.addEventListener("keydown", appBack);
    render(<SleepScreen />);
    expect(screen.getByTestId("sleep-screen")).toBeTruthy();
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "ArrowDown" });
    expect(goBack).toHaveBeenCalledTimes(1);
    // The app's global back never sees the key: no second step back.
    expect(appBack).not.toHaveBeenCalled();
    window.removeEventListener("keydown", appBack);
  });

  it("leaves on a click too", () => {
    render(<SleepScreen />);
    fireEvent.pointerDown(window);
    expect(goBack).toHaveBeenCalledTimes(1);
  });
});
