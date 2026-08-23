import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LibraryForm } from "./LibraryForm";

const mutateMock = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceCreateLibrary: () => ({ mutate: mutateMock, isPending: false }),
  useLibraryServiceUpdateLibrary: () => ({ mutate: vi.fn(), isPending: false }),
  eveyFetch: vi.fn().mockRejectedValue(new Error("not found")),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("LibraryForm", () => {
  it("renders Name input and Type select", () => {
    render(<LibraryForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Library" })).toBeInTheDocument();
  });

  it("Create Library button disabled when name empty", () => {
    render(<LibraryForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "Create Library" })).toBeDisabled();
  });

  it("submits library data on valid form", async () => {
    mutateMock.mockClear();
    render(<LibraryForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("Name"), "My Movies");
    const pathInput = screen.getByPlaceholderText("/media/movies");
    await userEvent.type(pathInput, "/media/movies");
    await userEvent.click(screen.getByRole("button", { name: "Create Library" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: {
        name: "My Movies",
        type: "LIBRARY_TYPE_MOVIES",
        paths: ["/media/movies"],
      },
    });
  });

  it("can add and remove paths", async () => {
    render(<LibraryForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.click(screen.getByRole("button", { name: "+ Add Path" }));
    const pathInputs = screen.getAllByPlaceholderText("/media/movies");
    expect(pathInputs).toHaveLength(2);
  });
});
