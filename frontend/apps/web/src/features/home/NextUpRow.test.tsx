import { describe, it, expect, vi } from "vitest";
import { renderWithQueryClient } from "@/test/test-utils";
import { screen } from "@testing-library/react";

vi.mock("@evey/api", () => ({
  usePlaybackServiceGetNextUp: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: Record<string, unknown>) => (
    <a {...props}>{children as React.ReactNode}</a>
  ),
  useNavigate: () => vi.fn(),
}));

vi.mock("../library/MediaCardWithModals", () => ({
  MediaCardWithModals: ({ item }: { item: { title?: string } }) => (
    <div data-testid="media-card">{item.title}</div>
  ),
}));

import { usePlaybackServiceGetNextUp } from "@evey/api";
import { NextUpRow } from "./NextUpRow";
const mockHook = vi.mocked(usePlaybackServiceGetNextUp);

describe("NextUpRow", () => {
  it("renders nothing when empty and not loading", () => {
    mockHook.mockReturnValue({
      data: { items: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof usePlaybackServiceGetNextUp>);
    const { container } = renderWithQueryClient(<NextUpRow />);
    expect(container.innerHTML).toBe("");
  });

  it("renders slider when loading", () => {
    mockHook.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof usePlaybackServiceGetNextUp>);
    renderWithQueryClient(<NextUpRow />);
    expect(screen.getByTestId("media-slider")).toBeInTheDocument();
  });

  it("renders media cards for returned items", () => {
    mockHook.mockReturnValue({
      data: {
        items: [
          { id: "m1", title: "Movie One", artwork: [] },
          { id: "m2", title: "Movie Two", artwork: [] },
        ],
      },
      isLoading: false,
    } as unknown as ReturnType<typeof usePlaybackServiceGetNextUp>);
    renderWithQueryClient(<NextUpRow />);
    expect(screen.getByText("Movie One")).toBeInTheDocument();
    expect(screen.getByText("Movie Two")).toBeInTheDocument();
  });
});
