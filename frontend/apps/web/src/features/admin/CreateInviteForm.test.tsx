import { act, render, screen } from "@testing-library/react";
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

  it("converts the datetime-local expiresAt to RFC3339 before submit", async () => {
    render(<CreateInviteForm onSuccess={vi.fn()} />, { wrapper: createWrapper() });
    const expires = screen.getByLabelText("Expires at (optional)");
    fireChange(expires, "2026-12-31T10:00");
    await userEvent.click(screen.getByRole("button", { name: "Create Invite Link" }));
    // The proto expiresAt is a Timestamp (strict RFC3339); the raw "2026-12-31T10:00"
    // (no seconds/zone) would be rejected. Expected value computed the same way so
    // the assertion holds regardless of the runner's timezone.
    const expectedISO = new Date("2026-12-31T10:00").toISOString();
    expect(mutateMock).toHaveBeenCalledWith({
      data: { maxUses: undefined, expiresAt: expectedISO, isAdmin: false, libraryIds: [] },
    });
  });
});

// Raw DOM events bypass testing-library's act wrapper: dispatched here inside
// act so React sees the resulting state update as part of the test.
function fireChange(el: HTMLElement, value: string) {
  const input = el as HTMLInputElement;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}
