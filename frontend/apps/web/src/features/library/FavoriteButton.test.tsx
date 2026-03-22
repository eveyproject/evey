import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { FavoriteButton } from "./FavoriteButton";

const mutateMock = vi.fn();

vi.mock("@/hooks/useFavorite", () => ({
  useIsFavorite: () => ({ data: false }),
  useToggleFavorite: () => ({ mutate: mutateMock }),
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("FavoriteButton", () => {
  it("renders heart button", () => {
    render(<FavoriteButton mediaId="1" />, { wrapper: Wrapper });
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("calls toggle on click", async () => {
    const user = userEvent.setup();
    mutateMock.mockClear();
    render(<FavoriteButton mediaId="1" />, { wrapper: Wrapper });
    await user.click(screen.getByRole("button"));
    expect(mutateMock).toHaveBeenCalledOnce();
  });

  it("shows 'Add to Favorites' title when not favorited", () => {
    render(<FavoriteButton mediaId="1" />, { wrapper: Wrapper });
    expect(screen.getByTitle("Add to Favorites")).toBeInTheDocument();
  });
});
