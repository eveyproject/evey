import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Modal } from "./Modal";

function renderModal(props: Partial<Parameters<typeof Modal>[0]> = {}) {
  const defaultProps = {
    open: true,
    onClose: vi.fn(),
    title: "Test Modal",
    children: <p>Body content</p>,
    ...props,
  };
  return { ...render(<Modal {...defaultProps} />), onClose: defaultProps.onClose };
}

describe("Modal", () => {
  it("renders when open=true", () => {
    renderModal();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Test Modal")).toBeInTheDocument();
    expect(screen.getByText("Body content")).toBeInTheDocument();
  });

  it("does not render when open=false", () => {
    renderModal({ open: false });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("calls onClose on Escape", async () => {
    const user = userEvent.setup();
    const { onClose } = renderModal();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("Escape closes only the topmost of two nested modals", async () => {
    const user = userEvent.setup();
    const onCloseParent = vi.fn();
    const onCloseChild = vi.fn();
    render(
      <Modal open onClose={onCloseParent} title="Parent">
        <p>Edit form</p>
        <Modal open onClose={onCloseChild} title="Child">
          <p>Identify</p>
        </Modal>
      </Modal>,
    );
    await user.keyboard("{Escape}");
    expect(onCloseChild).toHaveBeenCalledOnce();
    expect(onCloseParent).not.toHaveBeenCalled();
  });

  it("Escape reaches the parent again once the child is gone", async () => {
    const user = userEvent.setup();
    const onCloseParent = vi.fn();
    const { rerender } = render(
      <Modal open onClose={onCloseParent} title="Parent">
        <Modal open onClose={vi.fn()} title="Child">
          <p>Identify</p>
        </Modal>
      </Modal>,
    );
    rerender(
      <Modal open onClose={onCloseParent} title="Parent">
        <Modal open={false} onClose={vi.fn()} title="Child">
          <p>Identify</p>
        </Modal>
      </Modal>,
    );
    await user.keyboard("{Escape}");
    expect(onCloseParent).toHaveBeenCalledOnce();
  });

  it("calls onClose on backdrop click", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="X">
        <button>Inside</button>
      </Modal>,
    );
    // Modal renders via portal into document.body; backdrop is the fixed overlay div
    const backdrop = document.body.querySelector(".fixed.inset-0") as HTMLElement;
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("does not call onClose when clicking dialog content", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="X">
        <p>Inner</p>
      </Modal>,
    );
    await user.click(screen.getByText("Inner"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("traps focus with Tab cycling", async () => {
    const user = userEvent.setup();
    render(
      <Modal open onClose={vi.fn()} title="Focus trap">
        <button>First</button>
        <button>Second</button>
      </Modal>,
    );

    const dialog = screen.getByRole("dialog");
    const buttons = within(dialog).getAllByRole("button");
    const first = buttons[0];
    const last = buttons[buttons.length - 1];

    last.focus();
    expect(document.activeElement).toBe(last);

    await user.tab();
    expect(document.activeElement).toBe(first);
  });

  it("traps focus with Shift+Tab cycling", async () => {
    const user = userEvent.setup();
    render(
      <Modal open onClose={vi.fn()} title="Focus trap">
        <button>First</button>
        <button>Second</button>
      </Modal>,
    );

    const dialog = screen.getByRole("dialog");
    const buttons = within(dialog).getAllByRole("button");
    const first = buttons[0];
    const last = buttons[buttons.length - 1];

    first.focus();
    expect(document.activeElement).toBe(first);

    await user.tab({ shift: true });
    expect(document.activeElement).toBe(last);
  });

  it("renders without title", () => {
    render(
      <Modal open onClose={vi.fn()}>
        <p>No title</p>
      </Modal>,
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.queryByText("modal-title")).not.toBeInTheDocument();
  });
});
