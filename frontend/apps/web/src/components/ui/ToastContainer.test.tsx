import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { useToastStore } from "@evey/store";
import { ToastContainer } from "./ToastContainer";

describe("ToastContainer", () => {
  beforeEach(() => {
    useToastStore.setState({ toasts: [] });
  });

  it("renders nothing when no toasts", () => {
    const { container } = render(<ToastContainer />);
    expect(container.firstChild).toBeNull();
  });

  it("renders toast messages", () => {
    useToastStore.setState({
      toasts: [
        { id: "t1", type: "success", message: "Saved successfully" },
        { id: "t2", type: "error", message: "Something went wrong" },
      ],
    });
    render(<ToastContainer />);
    expect(screen.getByText("Saved successfully")).toBeInTheDocument();
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("renders correct icon per type", () => {
    useToastStore.setState({
      toasts: [{ id: "t1", type: "info", message: "Info msg" }],
    });
    render(<ToastContainer />);
    expect(screen.getByText("i")).toBeInTheDocument();
  });

  it("dismiss button removes toast", async () => {
    const user = userEvent.setup();
    vi.useFakeTimers({ shouldAdvanceTime: true });
    useToastStore.getState().addToast({ type: "warning", message: "Watch out" });

    render(<ToastContainer />);
    expect(screen.getByText("Watch out")).toBeInTheDocument();

    const dismissBtn = screen.getByRole("button", { name: "Dismiss" });
    await user.click(dismissBtn);
    expect(screen.queryByText("Watch out")).not.toBeInTheDocument();

    vi.useRealTimers();
  });

  it("multiple toasts each have dismiss buttons", () => {
    useToastStore.setState({
      toasts: [
        { id: "t1", type: "success", message: "A" },
        { id: "t2", type: "error", message: "B" },
      ],
    });
    render(<ToastContainer />);
    const dismissBtns = screen.getAllByRole("button", { name: "Dismiss" });
    expect(dismissBtns).toHaveLength(2);
  });

  it("toast has role=alert for accessibility", () => {
    useToastStore.setState({
      toasts: [{ id: "t1", type: "success", message: "OK" }],
    });
    render(<ToastContainer />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });
});
