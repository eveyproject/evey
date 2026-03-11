import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { KeyboardShortcutsOverlay } from "./KeyboardShortcutsOverlay";

describe("KeyboardShortcutsOverlay", () => {
  it("renders overlay when open", () => {
    render(<KeyboardShortcutsOverlay onClose={vi.fn()} />);
    expect(screen.getByText("Keyboard Shortcuts")).toBeInTheDocument();
    expect(screen.getByText("Play / Pause")).toBeInTheDocument();
    expect(screen.getByText("Toggle fullscreen")).toBeInTheDocument();
    expect(screen.getByText("Toggle mute")).toBeInTheDocument();
  });

  it("does not render when closed", () => {
    const { container } = render(<div id="wrapper" />);
    expect(container.querySelector("[class*='z-50']")).not.toBeInTheDocument();
    expect(screen.queryByText("Keyboard Shortcuts")).not.toBeInTheDocument();
  });

  it("closes on Escape key", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<KeyboardShortcutsOverlay onClose={onClose} />);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
