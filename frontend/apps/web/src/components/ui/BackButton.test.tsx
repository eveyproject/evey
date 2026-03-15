import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { BackButton } from "./BackButton";

const backMock = vi.fn();
const navigateMock = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useRouter: () => ({
    history: { back: backMock },
    navigate: navigateMock,
  }),
}));

describe("BackButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders a button with 'Back' text", () => {
    render(<BackButton />);
    expect(screen.getByRole("button", { name: /back/i })).toBeInTheDocument();
  });

  it("calls router.history.back() when history length > 1", async () => {
    const user = userEvent.setup();
    Object.defineProperty(window, "history", { value: { length: 2 }, configurable: true });
    render(<BackButton />);
    await user.click(screen.getByRole("button"));
    expect(backMock).toHaveBeenCalledOnce();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("navigates to fallback when history length is 1", async () => {
    const user = userEvent.setup();
    Object.defineProperty(window, "history", { value: { length: 1 }, configurable: true });
    render(<BackButton fallback="/library" />);
    await user.click(screen.getByRole("button"));
    expect(navigateMock).toHaveBeenCalledWith({ to: "/library" });
    expect(backMock).not.toHaveBeenCalled();
  });

  it("uses /home as default fallback", async () => {
    const user = userEvent.setup();
    Object.defineProperty(window, "history", { value: { length: 1 }, configurable: true });
    render(<BackButton />);
    await user.click(screen.getByRole("button"));
    expect(navigateMock).toHaveBeenCalledWith({ to: "/home" });
  });

  it("applies extra className", () => {
    render(<BackButton className="my-class" />);
    expect(screen.getByRole("button")).toHaveClass("my-class");
  });
});
