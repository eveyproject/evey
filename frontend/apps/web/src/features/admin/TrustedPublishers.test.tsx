import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TrustedPublishers } from "./TrustedPublishers";

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
  it("lists publishers, badges the seeded one and hides its Remove button", () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    expect(screen.getAllByTestId("publisher-row")).toHaveLength(2);
    expect(screen.getAllByTestId("publisher-seeded")).toHaveLength(1);
    expect(screen.getAllByTestId("publisher-remove")).toHaveLength(1);
    expect(screen.getByText("sha256:cccc3333...")).toBeInTheDocument();
  });

  it("adds a publisher by name and PEM", async () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("publisher-add-open"));
    expect(screen.getByTestId("publisher-add-confirm")).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Name"), "New Pub");
    await userEvent.type(screen.getByLabelText("Public key (PEM)"), "KEY");
    await userEvent.click(screen.getByTestId("publisher-add-confirm"));
    expect(mockAdd).toHaveBeenCalledWith({ data: { name: "New Pub", publicKeyPem: "KEY" } });
  });

  it("removes a publisher after confirmation", async () => {
    render(<TrustedPublishers />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("publisher-remove"));
    await userEvent.click(screen.getByTestId("publisher-remove-confirm"));
    expect(mockRemove).toHaveBeenCalledWith({ fingerprint: "sha256:cccc3333dddd4444" });
  });
});
