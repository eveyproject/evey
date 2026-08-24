import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { RouteErrorComponent } from "./RouteErrorComponent";
import { resetPreloadErrorForTests } from "@/lib/chunkError";

const mockNavigate = vi.fn();
const mockInvalidate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useRouter: () => ({ invalidate: mockInvalidate }),
  useNavigate: () => mockNavigate,
}));

describe("RouteErrorComponent", () => {
  const defaultError = new Error("Test error message");
  const defaultReset = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders title and error message", () => {
    render(<RouteErrorComponent error={defaultError} reset={defaultReset} />);
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    expect(screen.getByText("Test error message")).toBeInTheDocument();
  });

  it("renders fallback description when error has no message", () => {
    const emptyError = new Error("");
    render(<RouteErrorComponent error={emptyError} reset={defaultReset} />);
    expect(screen.getByText(/error occurred while loading/i)).toBeInTheDocument();
  });

  it("renders the fallback description when the thrown value carries no message", () => {
    // Regression: a route whose lazy chunk failed to load arrives here with
    // `undefined`, and `error.message` crashed this screen in turn.
    resetPreloadErrorForTests();
    sessionStorage.clear();
    render(<RouteErrorComponent error={undefined as unknown as Error} reset={defaultReset} />);
    expect(screen.getByText(/error occurred while loading/i)).toBeInTheDocument();
  });

  it("renders Retry and Go Home buttons", () => {
    render(<RouteErrorComponent error={defaultError} reset={defaultReset} />);
    expect(screen.getByRole("button", { name: "Retry loading page" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Go to home page" })).toBeInTheDocument();
  });

  it("calls reset and router.invalidate on Retry click", async () => {
    const user = userEvent.setup();
    render(<RouteErrorComponent error={defaultError} reset={defaultReset} />);
    await user.click(screen.getByRole("button", { name: "Retry loading page" }));
    expect(defaultReset).toHaveBeenCalledOnce();
    expect(mockInvalidate).toHaveBeenCalledOnce();
  });

  it("navigates to /home on Go Home click", async () => {
    const user = userEvent.setup();
    render(<RouteErrorComponent error={defaultError} reset={defaultReset} />);
    await user.click(screen.getByRole("button", { name: "Go to home page" }));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/home" });
  });
});
