import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CreateInviteForm } from "./CreateInviteForm";

const mutateMock = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceCreateInviteLink: () => ({ mutate: mutateMock, isPending: false }),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] }, isLoading: false }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("CreateInviteForm", () => {
  beforeEach(() => {
    mutateMock.mockReset();
  });

  it("renders maxUses + expiresAt inputs and submit button", () => {
    render(<CreateInviteForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Max uses (empty = unlimited)")).toBeInTheDocument();
    expect(screen.getByLabelText("Expires at (optional)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Invite Link" })).toBeInTheDocument();
  });

  it("submits with undefined fields when empty", async () => {
    render(<CreateInviteForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.click(screen.getByRole("button", { name: "Create Invite Link" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: { maxUses: undefined, expiresAt: undefined, isAdmin: false, libraryIds: [] },
    });
  });

  it("submits with parsed maxUses", async () => {
    render(<CreateInviteForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    await userEvent.type(screen.getByLabelText("Max uses (empty = unlimited)"), "5");
    await userEvent.click(screen.getByRole("button", { name: "Create Invite Link" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: { maxUses: 5, expiresAt: undefined, isAdmin: false, libraryIds: [] },
    });
  });

  it("submits with expiresAt when provided", async () => {
    render(<CreateInviteForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    const expires = screen.getByLabelText("Expires at (optional)");
    fireChange(expires, "2026-12-31T10:00");
    await userEvent.click(screen.getByRole("button", { name: "Create Invite Link" }));
    expect(mutateMock).toHaveBeenCalledWith({
      data: { maxUses: undefined, expiresAt: "2026-12-31T10:00", isAdmin: false, libraryIds: [] },
    });
  });
});

function fireChange(el: HTMLElement, value: string) {
  const input = el as HTMLInputElement;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
}
