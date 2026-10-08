import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EditProfileDialog } from "./EditProfileDialog";
import type { V1Profile } from "@evey/api";

const mockMutate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceUpdateProfile: (_opts: unknown) => ({ mutate: mockMutate, isPending: false }),
  getUserServiceListProfilesQueryKey: (id: string) => ["/api/v1/users", id, "profiles"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// Modal uses createPortal - jsdom document.body handles it correctly; no mock needed.

const BASE_PROFILE: V1Profile = {
  id: "p-1",
  userId: "u-1",
  name: "Alice",
  isDefault: true,
  isKids: false,
  locale: "",
  region: "",
};

const onClose = vi.fn();

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = mockInvalidateQueries as unknown as typeof qc.invalidateQueries;
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function renderDialog(profile: V1Profile = BASE_PROFILE, hasKidsProfile = false) {
  return render(
    React.createElement(EditProfileDialog, {
      open: true,
      onClose,
      profile,
      userId: "u-1",
      hasKidsProfile,
    }),
    { wrapper: createWrapper() },
  );
}

describe("EditProfileDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Rendering ──────────────────────────────────────────────────────────────

  it("renders nothing when closed", () => {
    render(
      React.createElement(EditProfileDialog, {
        open: false,
        onClose,
        profile: BASE_PROFILE,
        userId: "u-1",
        hasKidsProfile: false,
      }),
      { wrapper: createWrapper() },
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("pre-fills name input with profile name", () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i) as HTMLInputElement;
    expect(nameInput.value).toBe("Alice");
  });

  // ── Name validation ────────────────────────────────────────────────────────

  it("shows error and blocks mutate when name is cleared and form submitted", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/name is required/i);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("shows error when name is whitespace-only", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "   " } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(mockMutate).not.toHaveBeenCalled();
  });

  // ── PIN validation ─────────────────────────────────────────────────────────

  it("shows error when PIN is entered with fewer than 4 digits", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    // Change name so the form isn't a no-op
    fireEvent.change(nameInput, { target: { value: "Alice2" } });
    const pinInput = screen.getByLabelText(/pin/i);
    fireEvent.change(pinInput, { target: { value: "12" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/pin must be 4 digits/i);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("strips non-numeric characters from PIN input", () => {
    renderDialog();
    const pinInput = screen.getByLabelText(/pin/i) as HTMLInputElement;
    fireEvent.change(pinInput, { target: { value: "ab12cd34" } });
    // Component filters with replace(/\D/g, "").slice(0, 4)
    expect(pinInput.value).toBe("1234");
  });

  it("includes PIN in payload when exactly 4 digits, proven by the account password for a first PIN", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Alice" } });
    const pinInput = screen.getByLabelText(/pin \(4 digits\)/i);
    fireEvent.change(pinInput, { target: { value: "5678" } });
    // No PIN yet: the server wants the account password, not a current PIN.
    expect(screen.queryByLabelText(/current pin/i)).toBeNull();
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByRole("alert").textContent).toMatch(/password required/i);
    expect(mockMutate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/account password/i), { target: { value: "hunter2" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "p-1", data: expect.objectContaining({ pin: "5678", currentPassword: "hunter2" }) }),
    );
  });

  // The gRPC route now demands the same proof as the parental page: a profile
  // that already has a PIN changes it with the current one, never on the
  // session alone.
  it("asks the current PIN when the profile already has one and sends it", async () => {
    renderDialog({ ...BASE_PROFILE, hasPin: true });
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(screen.getByLabelText(/pin \(4 digits\)/i), { target: { value: "5678" } });
    expect(screen.queryByLabelText(/account password/i)).toBeNull();
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByRole("alert").textContent).toMatch(/enter your current pin/i);
    expect(mockMutate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/current pin/i), { target: { value: "1111" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "p-1", data: { pin: "5678", currentPin: "1111" } }),
    );
  });

  // ── Kids toggle + maxAge ───────────────────────────────────────────────────

  it("does not show max age rating select when isKids is false", () => {
    renderDialog();
    expect(screen.queryByLabelText(/max age rating/i)).toBeNull();
  });

  it("shows max age rating select when isKids is toggled on", () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i) as HTMLInputElement;
    fireEvent.click(kidsCheckbox);
    expect(screen.getByLabelText(/max age rating/i)).toBeInTheDocument();
  });

  it("hides max age rating select when isKids is toggled off again", () => {
    renderDialog({ ...BASE_PROFILE, isKids: true }, false);
    // Starts with isKids=true, select visible
    expect(screen.getByLabelText(/max age rating/i)).toBeInTheDocument();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i) as HTMLInputElement;
    fireEvent.click(kidsCheckbox);
    expect(screen.queryByLabelText(/max age rating/i)).toBeNull();
  });

  it("includes isKids=true in payload when toggled on", async () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    fireEvent.submit(kidsCheckbox.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "p-1", data: expect.objectContaining({ isKids: true }) }),
    );
  });

  // ── pinRequired hint text ──────────────────────────────────────────────────

  it("shows PIN-required hint when hasKidsProfile=true and profile is not kids", () => {
    // hasKidsProfile=true, profile.isKids=false, isKids=false → pinRequiredBecauseOfKids
    renderDialog(BASE_PROFILE, true);
    expect(screen.getByText(/a pin is required because a kids profile exists/i)).toBeInTheDocument();
  });

  it("shows keep-current-PIN hint when no kids profile exists", () => {
    renderDialog(BASE_PROFILE, false);
    expect(screen.getByText(/leave blank to keep current pin/i)).toBeInTheDocument();
  });

  // ── Payload / no-op ────────────────────────────────────────────────────────

  it("calls onClose without mutating when nothing changed", async () => {
    renderDialog();
    const form = screen.getByLabelText(/name/i).closest("form")!;
    fireEvent.submit(form);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("sends only changed fields in payload", async () => {
    renderDialog({ ...BASE_PROFILE, name: "Alice", locale: "en" });
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Bob" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    const call = mockMutate.mock.calls[0][0] as { id: string; data: Record<string, unknown> };
    // Only name changed; locale not changed so it must not appear
    expect(call.data).toHaveProperty("name", "Bob");
    expect(call.data).not.toHaveProperty("locale");
  });

  it("sends locale in payload when it changes", async () => {
    renderDialog();
    const localeSelect = screen.getByLabelText(/language/i);
    fireEvent.change(localeSelect, { target: { value: "fr" } });
    fireEvent.submit(localeSelect.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "p-1", data: expect.objectContaining({ locale: "fr" }) }),
    );
  });

  // ── Cancel ─────────────────────────────────────────────────────────────────

  it("calls onClose when Cancel button is clicked", () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(mockMutate).not.toHaveBeenCalled();
  });
});
