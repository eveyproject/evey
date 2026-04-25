import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetCurrentUser = vi.fn();
const mockListProfiles = vi.fn();
const createMutate = vi.fn();
const updateMutate = vi.fn();
const deleteMutate = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => mockGetCurrentUser(...args),
  useUserServiceListProfiles: (...args: unknown[]) => mockListProfiles(...args),
  useUserServiceCreateProfile: (_opts: unknown) => ({ mutate: createMutate, isPending: false }),
  useUserServiceUpdateProfile: (_opts: unknown) => ({ mutate: updateMutate, isPending: false }),
  useUserServiceDeleteProfile: (_opts: unknown) => ({ mutate: deleteMutate, isPending: false }),
  useUserServiceSetDefaultProfile: (_opts: unknown) => ({ mutate: vi.fn(), isPending: false }),
  getUserServiceListProfilesQueryKey: (id: string) => ["/api/v1/users", id, "profiles"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getPage() {
  const mod = await import("./manage.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const defaultProfile = {
  id: "p-default",
  userId: "u-1",
  name: "Main",
  isDefault: true,
  isKids: false,
};
const kidsProfile = {
  id: "p-kid",
  userId: "u-1",
  name: "Junior",
  isDefault: false,
  isKids: true,
};
const secondaryProfile = {
  id: "p-secondary",
  userId: "u-1",
  name: "Guest",
  isDefault: false,
  isKids: false,
  locale: "fr",
  region: "FR",
};

describe("ProfileManagePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetCurrentUser.mockReturnValue({ data: { id: "u-1", username: "alice" } });
    mockListProfiles.mockReturnValue({
      data: { profiles: [defaultProfile, secondaryProfile] },
      isLoading: false,
    });
  });

  it("renders list of profiles with name and default badge", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Main")).toBeInTheDocument();
    expect(screen.getByText("Guest")).toBeInTheDocument();
    // Default badge is rendered exactly as "Default"
    expect(screen.getByText(/^default$/i)).toBeInTheDocument();
  });

  it("delete button disabled for default profile", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const deleteBtns = screen.getAllByRole("button", { name: /delete/i });
    // First profile (Main) is default, expect its delete disabled
    expect(deleteBtns[0]).toBeDisabled();
    // Second profile (Guest) delete should be enabled
    expect(deleteBtns[1]).not.toBeDisabled();
  });

  it("opens create dialog and submits createProfile with correct payload", async () => {
    const u = userEvent.setup();
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    await u.click(screen.getByRole("button", { name: /create profile/i }));
    const dialog = await screen.findByRole("dialog");
    const nameInput = screen.getByLabelText(/name/i);
    await u.type(nameInput, "Bob");
    // Submit via form submit button inside the dialog
    const saveBtn = Array.from(dialog.querySelectorAll("button")).find(
      (b) => b.getAttribute("type") === "submit",
    )!;
    fireEvent.click(saveBtn);
    await waitFor(() => expect(createMutate).toHaveBeenCalled());
    expect(createMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ name: "Bob" }) }),
    );
  });

  it("create dialog requires PIN when another profile is kids", async () => {
    mockListProfiles.mockReturnValue({
      data: { profiles: [defaultProfile, kidsProfile] },
      isLoading: false,
    });
    const u = userEvent.setup();
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    await u.click(screen.getByRole("button", { name: /create profile/i }));
    await screen.findByRole("dialog");
    await u.type(screen.getByLabelText(/name/i), "Adult");
    // PIN input should be visible
    const pinInput = screen.getByLabelText(/pin/i);
    expect(pinInput).toBeInTheDocument();
  });

  it("opens edit dialog prefilled and submits updateProfile", async () => {
    const u = userEvent.setup();
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const editBtns = screen.getAllByRole("button", { name: /edit/i });
    // Second profile (Guest)
    await u.click(editBtns[1]);
    const dialog = await screen.findByRole("dialog");
    const nameInput = screen.getByLabelText(/name/i) as HTMLInputElement;
    expect(nameInput.value).toBe("Guest");
    await u.clear(nameInput);
    await u.type(nameInput, "Renamed");
    const saveBtn = Array.from(dialog.querySelectorAll("button")).find(
      (b) => b.getAttribute("type") === "submit",
    )!;
    fireEvent.click(saveBtn);
    await waitFor(() => expect(updateMutate).toHaveBeenCalled());
    expect(updateMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "p-secondary", data: expect.objectContaining({ name: "Renamed" }) }),
    );
  });

  it("delete confirm dialog calls deleteProfile", async () => {
    const u = userEvent.setup();
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const deleteBtns = screen.getAllByRole("button", { name: /delete/i });
    await u.click(deleteBtns[1]); // Guest
    const dialog = await screen.findByRole("dialog");
    // Confirm delete button inside dialog
    const confirmBtns = Array.from(dialog.querySelectorAll("button")).filter((b) =>
      /delete/i.test(b.textContent ?? ""),
    );
    // Last matching button inside the dialog is the confirm
    fireEvent.click(confirmBtns[confirmBtns.length - 1]);
    await waitFor(() => expect(deleteMutate).toHaveBeenCalled());
    expect(deleteMutate).toHaveBeenCalledWith({ id: "p-secondary" });
  });
});
