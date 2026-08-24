import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ErrorBoundary } from "./ErrorBoundary";
import { resetPreloadErrorForTests } from "@/lib/chunkError";

// Component that throws on demand
function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) throw new Error("test error");
  return <p>Normal content</p>;
}

// What a failed lazy chunk actually throws: React hands the boundary `undefined`,
// not an Error (measured in Firefox against the deployed bundle, 2026-08-24).
function ShapelessBomb(): never {
  throw undefined;
}

beforeEach(() => {
  resetPreloadErrorForTests();
  sessionStorage.clear();
});

describe("ErrorBoundary", () => {
  it("renders children normally", () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={false} />
      </ErrorBoundary>
    );
    expect(screen.getByText("Normal content")).toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  it("renders fallback on error", () => {
    // Suppress jsdom console.error for the caught render error
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>
    );
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload the page" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Go to home page" })).toBeInTheDocument();
    consoleSpy.mockRestore();
  });

  it("shows error message and go home button on error", () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>
    );
    expect(screen.getByText(/unexpected error/i)).toBeInTheDocument();
    consoleSpy.mockRestore();
  });

  it("renders its fallback when the thrown value is not an Error", () => {
    // Regression: componentDidCatch read `error.name` and threw on `undefined`,
    // so the boundary rendered NOTHING and its own TypeError escaped to the
    // route error screen ("can't access property \"name\", t is undefined").
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <ShapelessBomb />
      </ErrorBoundary>
    );
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    consoleSpy.mockRestore();
  });

  it("reloads once when a failed chunk is what broke the page", () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const reloadMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { reload: reloadMock, href: "/" },
      writable: true,
    });
    // Vite's own signal, the only evidence left once the rejection reaches
    // React as `undefined`.
    window.dispatchEvent(new Event("vite:preloadError"));
    render(
      <ErrorBoundary>
        <ShapelessBomb />
      </ErrorBoundary>
    );
    expect(reloadMock).toHaveBeenCalledOnce();
    consoleSpy.mockRestore();
  });

  it("Try Again reloads the page", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const reloadMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { reload: reloadMock, href: "/" },
      writable: true,
    });
    const user = userEvent.setup();
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>
    );
    await user.click(screen.getByRole("button", { name: "Reload the page" }));
    expect(reloadMock).toHaveBeenCalled();
    consoleSpy.mockRestore();
  });
});
