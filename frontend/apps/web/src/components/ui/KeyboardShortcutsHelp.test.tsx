import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { KeyboardShortcutsHelp } from "./KeyboardShortcutsHelp";

vi.mock("./Modal", () => ({
  Modal: ({
    open,
    children,
    title,
    onClose,
  }: {
    open: boolean;
    children: React.ReactNode;
    title: string;
    onClose: () => void;
  }) =>
    open ? (
      <div role="dialog" aria-label={title}>
        <button onClick={onClose}>Close</button>
        {children}
      </div>
    ) : null,
}));

describe("KeyboardShortcutsHelp", () => {
  it("renders nothing when closed", () => {
    render(<KeyboardShortcutsHelp open={false} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("renders dialog when open", () => {
    render(<KeyboardShortcutsHelp open onClose={vi.fn()} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("shows Keyboard Shortcuts title", () => {
    render(<KeyboardShortcutsHelp open onClose={vi.fn()} />);
    expect(screen.getByRole("dialog", { name: "Keyboard Shortcuts" })).toBeInTheDocument();
  });

  it("lists known shortcut keys", () => {
    render(<KeyboardShortcutsHelp open onClose={vi.fn()} />);
    expect(screen.getByText("/")).toBeInTheDocument();
    expect(screen.getByText("Esc")).toBeInTheDocument();
    expect(screen.getByText("h")).toBeInTheDocument();
    expect(screen.getByText("?")).toBeInTheDocument();
  });

  it("shows shortcut descriptions", () => {
    render(<KeyboardShortcutsHelp open onClose={vi.fn()} />);
    expect(screen.getAllByText("Focus search")).toHaveLength(2);
    expect(screen.getByText("Go to Home")).toBeInTheDocument();
    expect(screen.getByText("Toggle this help")).toBeInTheDocument();
  });

  it("calls onClose when close button clicked", async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(<KeyboardShortcutsHelp open onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("shows note about shortcuts disabled while typing", () => {
    render(<KeyboardShortcutsHelp open onClose={vi.fn()} />);
    expect(screen.getByText(/Shortcuts are disabled while typing/)).toBeInTheDocument();
  });
});
