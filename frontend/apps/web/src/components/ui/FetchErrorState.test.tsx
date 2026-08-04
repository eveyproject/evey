import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { FetchErrorState } from "./FetchErrorState";

// Failing to load is not the same as having nothing, and the two must not read
// alike: a library that would not load said "this library is empty", and the
// requests page said the administrator had turned requests off.

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title, description, action }: { title: string; description?: string; action?: { label: string; onClick: () => void } }) =>
    React.createElement(
      "div",
      null,
      React.createElement("h2", null, title),
      React.createElement("p", null, description),
      action ? React.createElement("button", { onClick: action.onClick }, action.label) : null,
    ),
}));

describe("FetchErrorState", () => {
  it("tells a dropped connection apart from a server error", () => {
    const { rerender } = render(<FetchErrorState error={new Error("no network")} onRetry={vi.fn()} />);
    // No httpStatus on the error means the request never reached the server.
    expect(screen.getByText("common.offline.title")).toBeInTheDocument();

    rerender(<FetchErrorState error={Object.assign(new Error("boom"), { httpStatus: 500 })} onRetry={vi.fn()} />);
    expect(screen.getByText("common.errorBoundary.title")).toBeInTheDocument();
  });

  it("offers a retry that actually calls back", () => {
    const onRetry = vi.fn();
    render(<FetchErrorState error={new Error("x")} onRetry={onRetry} />);
    fireEvent.click(screen.getByText("common.retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
