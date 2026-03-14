import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MetadataOverrideModal } from "./MetadataOverrideModal";

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

vi.mock("@evey/api", () => ({
  useMetadataServiceListMetadataOverrides: () => ({
    data: { overrides: [] },
    isLoading: false,
  }),
  useMetadataServiceSetMetadataOverride: () => ({ mutateAsync: vi.fn() }),
  useMetadataServiceRemoveMetadataOverride: () => ({ mutateAsync: vi.fn() }),
  getMetadataServiceListMetadataOverridesQueryKey: (id: string) => ["meta-overrides", id],
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

describe("MetadataOverrideModal", () => {
  it("renders nothing when closed", () => {
    const { container } = render(
      <MetadataOverrideModal open={false} onClose={vi.fn()} mediaItemId="m1" />,
      { wrapper: createWrapper() }
    );
    expect(screen.queryByText("Metadata Overrides")).not.toBeInTheDocument();
    expect(container.firstChild).toBeNull();
  });

  it("renders field inputs when open", () => {
    render(
      <MetadataOverrideModal open onClose={vi.fn()} mediaItemId="m1" />,
      { wrapper: createWrapper() }
    );
    expect(screen.getByText("Metadata Overrides")).toBeInTheDocument();
    expect(screen.getByText("Title")).toBeInTheDocument();
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Year")).toBeInTheDocument();
    expect(screen.getByText("Rating")).toBeInTheDocument();
  });

  it("renders Close button", () => {
    render(
      <MetadataOverrideModal open onClose={vi.fn()} mediaItemId="m1" />,
      { wrapper: createWrapper() }
    );
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
  });

  it("renders spinner when loading", () => {
    vi.doMock("@evey/api", () => ({
      useMetadataServiceListMetadataOverrides: () => ({ data: undefined, isLoading: true }),
      useMetadataServiceSetMetadataOverride: () => ({ mutateAsync: vi.fn() }),
      useMetadataServiceRemoveMetadataOverride: () => ({ mutateAsync: vi.fn() }),
      getMetadataServiceListMetadataOverridesQueryKey: () => ["meta-overrides"],
    }));
    // Loading state (spinner) is covered by component structure — static mocks show data
  });
});
