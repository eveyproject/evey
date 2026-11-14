import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AddTrustedPublisherButton, TrustedPublishers, publisherInitials } from "./TrustedPublishers";

vi.mock("react-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-dom")>();
  return { ...actual, createPortal: (node: React.ReactNode) => node };
});

const mockAdd = vi.fn();
const mockRemove = vi.fn();
let publishers: object[] = [];

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminPluginServiceListTrustedPublishers: () => ({ data: { publishers }, isLoading: false }),
  useAdminPluginServiceAddTrustedPublisher: () => ({ mutate: mockAdd, isPending: false }),
  useAdminPluginServiceRemoveTrustedPublisher: () => ({ mutate: mockRemove, isPending: false }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoLocked: string[] }) => unknown) => sel({ demoLocked: [] }),
}));

function Wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  mockAdd.mockReset();
  mockRemove.mockReset();
  publishers = [
    { fingerprint: "sha256:aaaa1111bbbb2222", name: "Server operator key", seeded: true },
    { fingerprint: "sha256:cccc3333dddd4444", name: "Acme", seeded: false },
  ];
});

describe("TrustedPublishers", () => {
  it("lists publishers as tiles, badges the seeded one and hides its Remove button", async () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    expect(screen.getAllByTestId("publisher-row")).toHaveLength(2);
    expect(screen.getAllByTestId("publisher-seeded")).toHaveLength(1);
    // Details stay folded until a tile is opened.
    expect(screen.queryByTestId("publisher-details")).not.toBeInTheDocument();
    const [first, second] = screen.getAllByTestId("publisher-toggle");
    await userEvent.click(first!);
    await userEvent.click(second!);
    expect(screen.getAllByTestId("publisher-details")).toHaveLength(1);
    await userEvent.click(first!);
    expect(screen.getAllByTestId("publisher-details")).toHaveLength(1);
  });

  it("shows a tile's fingerprint and Remove once opened, never Remove on the seeded key", async () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    await userEvent.click(screen.getByRole("button", { name: /Acme/ }));
    expect(screen.getByText("sha256:cccc3333...")).toBeInTheDocument();
    expect(screen.getByTestId("publisher-remove")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Server operator key/ }));
    expect(screen.queryByTestId("publisher-remove")).not.toBeInTheDocument();
  });

  it("monograms a publisher from its first two words", () => {
    expect(publisherInitials("Evey project")).toBe("EP");
    expect(publisherInitials("acme")).toBe("A");
    expect(publisherInitials("  ")).toBe("?");
  });

  it("adds a publisher by name and PEM", async () => {
    render(<AddTrustedPublisherButton />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("publisher-add-open"));
    expect(screen.getByTestId("publisher-add-confirm")).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Name"), "New Pub");
    await userEvent.type(screen.getByLabelText("Public key (PEM)"), "KEY");
    await userEvent.click(screen.getByTestId("publisher-add-confirm"));
    expect(mockAdd).toHaveBeenCalledWith({ data: { name: "New Pub", publicKeyPem: "KEY" } });
  });

  it("removes a publisher after confirmation", async () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    await userEvent.click(screen.getByRole("button", { name: /Acme/ }));
    await userEvent.click(screen.getByTestId("publisher-remove"));
    await userEvent.click(screen.getByTestId("publisher-remove-confirm"));
    expect(mockRemove).toHaveBeenCalledWith({ fingerprint: "sha256:cccc3333dddd4444" });
  });
});
