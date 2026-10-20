import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { InfoTooltip } from "./InfoTooltip";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// The "i" opens a dialog carrying the detail; the X, the backdrop and a
// second press close it; none of those clicks leak to a clickable parent
// (a collapsible header would otherwise toggle along).
describe("InfoTooltip", () => {
  it("opens on the trigger and closes on X, backdrop or a second press", () => {
    render(<InfoTooltip label="More info">Detail text</InfoTooltip>);
    const trigger = screen.getByRole("button", { name: "More info" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "More info" })).toHaveTextContent("Detail text");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "common.aria.close" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(trigger);
    fireEvent.click(document.querySelector(".fixed.inset-0") as HTMLElement);
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(trigger);
    fireEvent.click(trigger);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows an 'i' icon, not a question mark", () => {
    const { container } = render(<InfoTooltip label="More info">Detail</InfoTooltip>);
    expect(container.querySelector("svg.lucide-info")).not.toBeNull();
    expect(container.querySelector("svg.lucide-circle-help, svg.lucide-help-circle")).toBeNull();
  });

  it("opens from the keyboard and closes on Escape", () => {
    render(<InfoTooltip label="More info">Detail</InfoTooltip>);
    const trigger = screen.getByRole("button", { name: "More info" });
    expect(trigger).toHaveAttribute("tabindex", "0");
    fireEvent.keyDown(trigger, { key: "Enter" });
    expect(screen.getByRole("dialog", { name: "More info" })).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.keyDown(trigger, { key: " " });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("still opens inside a disabled fieldset (the demo's read-only pages)", () => {
    render(
      <fieldset disabled>
        <InfoTooltip label="More info">Detail</InfoTooltip>
      </fieldset>,
    );
    fireEvent.click(screen.getByRole("button", { name: "More info" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Detail");
  });

  it("never lets a click reach a clickable parent", () => {
    const parent = vi.fn();
    render(
      <div onClick={parent}>
        <InfoTooltip label="More info">Detail</InfoTooltip>
      </div>,
    );
    const trigger = screen.getByRole("button", { name: "More info" });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("dialog"));
    fireEvent.click(screen.getByRole("button", { name: "common.aria.close" }));
    fireEvent.click(trigger);
    fireEvent.click(document.querySelector(".fixed.inset-0") as HTMLElement);
    expect(parent).not.toHaveBeenCalled();
  });
});
