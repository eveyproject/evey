import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LibraryAccessModal } from "./LibraryAccessModal";

const getMock = vi.fn();
const setMutateAsync = vi.fn();
const revokeMutateAsync = vi.fn();

vi.mock("@evey/api", () => ({
  useLibraryAccessServiceGetUserLibraryAccess: (_id: string, _opts: unknown) => getMock(),
  useLibraryAccessServiceSetUserLibraryAccess: () => ({ mutateAsync: setMutateAsync, isPending: false }),
  useLibraryAccessServiceRevokeAllUserLibraryAccess: () => ({ mutateAsync: revokeMutateAsync, isPending: false }),
  getLibraryAccessServiceGetUserLibraryAccessQueryKey: (id?: string) => ["library-access", id],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const user = { id: "u1", username: "alice" };

describe("LibraryAccessModal", () => {
  beforeEach(() => {
    getMock.mockReset();
    setMutateAsync.mockReset().mockResolvedValue({});
    revokeMutateAsync.mockReset().mockResolvedValue({});
    getMock.mockReturnValue({
      data: {
        libraryIds: ["lib1"],
        libraries: [
          { id: "lib1", name: "Movies" },
          { id: "lib2", name: "Shows" },
        ],
      },
      isLoading: false,
    });
  });

  it("renders nothing when closed", () => {
    render(<LibraryAccessModal user={user} open={false} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("renders title and libraries when open", () => {
    render(<LibraryAccessModal user={user} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/Library Access/)).toBeInTheDocument();
    expect(screen.getByLabelText("Movies")).toBeInTheDocument();
    expect(screen.getByLabelText("Shows")).toBeInTheDocument();
  });

  it("pre-checks granted libraries", () => {
    render(<LibraryAccessModal user={user} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Movies")).toBeChecked();
    expect(screen.getByLabelText("Shows")).not.toBeChecked();
  });

  it("toggles a library checkbox on click", async () => {
    const u = userEvent.setup();
    render(<LibraryAccessModal user={user} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    const shows = screen.getByLabelText("Shows");
    expect(shows).not.toBeChecked();
    await u.click(shows);
    expect(shows).toBeChecked();
  });

  it("calls SetUserLibraryAccess on Save with selected ids", async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    render(<LibraryAccessModal user={user} open={true} onClose={onClose} />, { wrapper: createWrapper() });
    await u.click(screen.getByLabelText("Shows"));
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(setMutateAsync).toHaveBeenCalledWith({
      userId: "u1",
      data: { libraryIds: expect.arrayContaining(["lib1", "lib2"]) },
    });
    expect(onClose).toHaveBeenCalled();
  });

  it("Cancel calls onClose without mutation", async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    render(<LibraryAccessModal user={user} open={true} onClose={onClose} />, { wrapper: createWrapper() });
    await u.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
    expect(setMutateAsync).not.toHaveBeenCalled();
    expect(revokeMutateAsync).not.toHaveBeenCalled();
  });
});
