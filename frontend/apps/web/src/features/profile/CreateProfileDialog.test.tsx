import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CreateProfileDialog } from "./CreateProfileDialog";

const mockMutate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceCreateProfile: (_opts: unknown) => ({ mutate: mockMutate, isPending: false }),
  getUserServiceListProfilesQueryKey: (id: string) => ["/api/v1/users", id, "profiles"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const onClose = vi.fn();

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = mockInvalidateQueries as unknown as typeof qc.invalidateQueries;
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function renderDialog(props: { hasKidsProfile?: boolean } = {}) {
  return render(
    React.createElement(CreateProfileDialog, {
      open: true,
      onClose,
      userId: "u-1",
      hasKidsProfile: props.hasKidsProfile ?? false,
    }),
    { wrapper: createWrapper() },
  );
}

describe("CreateProfileDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Rendering ──────────────────────────────────────────────────────────────

  it("renders nothing when closed", () => {
    render(
      React.createElement(CreateProfileDialog, {
        open: false,
        onClose,
        userId: "u-1",
        hasKidsProfile: false,
      }),
      { wrapper: createWrapper() },
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders name input empty on fresh open", () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i) as HTMLInputElement;
    expect(nameInput.value).toBe("");
  });

  // ── Name validation ────────────────────────────────────────────────────────

  it("shows error and blocks mutate when name is empty on submit", async () => {
    renderDialog();
    const form = screen.getByLabelText(/name/i).closest("form")!;
    fireEvent.submit(form);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/name is required/i);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("shows error and blocks mutate when name is whitespace-only", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "   " } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/name is required/i);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  // ── PIN: pinRequired path (hasKidsProfile=true, isKids=false) ─────────────

  it("shows PIN field when hasKidsProfile=true and isKids is unchecked", () => {
    renderDialog({ hasKidsProfile: true });
    expect(screen.getByLabelText(/pin/i)).toBeInTheDocument();
  });

  it("does not show PIN field when hasKidsProfile=false", () => {
    renderDialog({ hasKidsProfile: false });
    expect(screen.queryByLabelText(/pin/i)).toBeNull();
  });

  it("blocks create with error when pinRequired and PIN is fewer than 4 digits", async () => {
    renderDialog({ hasKidsProfile: true });
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Bob" } });
    const pinInput = screen.getByLabelText(/pin/i);
    fireEvent.change(pinInput, { target: { value: "12" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/pin is required/i);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("strips non-numeric characters from PIN input", () => {
    renderDialog({ hasKidsProfile: true });
    const pinInput = screen.getByLabelText(/pin/i) as HTMLInputElement;
    fireEvent.change(pinInput, { target: { value: "ab12cd34" } });
    expect(pinInput.value).toBe("1234");
  });

  it("includes PIN in payload when hasKidsProfile=true and exactly 4 digits entered", async () => {
    renderDialog({ hasKidsProfile: true });
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Bob" } });
    const pinInput = screen.getByLabelText(/pin/i);
    fireEvent.change(pinInput, { target: { value: "5678" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ pin: "5678" }) }),
    );
  });

  // ── PIN: hides when isKids toggled on (pinRequired = hasKidsProfile && !isKids) ──

  it("hides the PIN field when isKids is toggled on (pinRequired becomes false)", () => {
    renderDialog({ hasKidsProfile: true });
    expect(screen.getByLabelText(/pin/i)).toBeInTheDocument();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    expect(screen.queryByLabelText(/pin/i)).toBeNull();
  });

  it("does NOT leak a typed PIN onto a kids profile after toggling isKids on", async () => {
    renderDialog({ hasKidsProfile: true });
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Junior" } });
    // Type a PIN while the field is shown (adult profile), then switch to kids:
    // the field unmounts but the pin state persists.
    fireEvent.change(screen.getByLabelText(/pin/i), { target: { value: "1234" } });
    fireEvent.click(screen.getByLabelText(/kids profile/i));
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    const arg = mockMutate.mock.calls[0][0];
    expect(arg.data.isKids).toBe(true);
    expect(arg.data.pin).toBeUndefined();
  });

  // ── Kids toggle + maxAge ───────────────────────────────────────────────────

  it("does not show max age rating select when isKids is false", () => {
    renderDialog();
    expect(screen.queryByLabelText(/max age rating/i)).toBeNull();
  });

  it("shows max age rating select when isKids is toggled on", () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    expect(screen.getByLabelText(/max age rating/i)).toBeInTheDocument();
  });

  it("hides max age rating select when isKids is toggled off again", () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    expect(screen.getByLabelText(/max age rating/i)).toBeInTheDocument();
    fireEvent.click(kidsCheckbox);
    expect(screen.queryByLabelText(/max age rating/i)).toBeNull();
  });

  it("includes isKids=true in payload when toggled on", async () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Kids" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ isKids: true }) }),
    );
  });

  it("does not include isKids in payload when it stays false", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Adult" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    const call = mockMutate.mock.calls[0][0] as { userId: string; data: Record<string, unknown> };
    expect(call.data).not.toHaveProperty("isKids");
  });

  it("includes maxAge in payload when isKids=true and a max age is selected", async () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Kids" } });
    const maxAgeSelect = screen.getByLabelText(/max age rating/i);
    fireEvent.change(maxAgeSelect, { target: { value: "12" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ isKids: true, maxAge: 12 }) }),
    );
  });

  it("does not include maxAge in payload when isKids=true but no max age selected", async () => {
    renderDialog();
    const kidsCheckbox = screen.getByLabelText(/kids profile/i);
    fireEvent.click(kidsCheckbox);
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Kids" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    const call = mockMutate.mock.calls[0][0] as { userId: string; data: Record<string, unknown> };
    expect(call.data).not.toHaveProperty("maxAge");
  });

  // ── Payload shape: userId + trimmed name ──────────────────────────────────

  it("sends trimmed name in create payload", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "  Alice  " } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ name: "Alice" }) }),
    );
  });

  it("sends locale in payload when language is changed", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Alice" } });
    const localeSelect = screen.getByLabelText(/language/i);
    fireEvent.change(localeSelect, { target: { value: "fr" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ userId: "u-1", data: expect.objectContaining({ locale: "fr" }) }),
    );
  });

  it("does not include locale in payload when no language is selected", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Alice" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalled());
    const call = mockMutate.mock.calls[0][0] as { userId: string; data: Record<string, unknown> };
    expect(call.data).not.toHaveProperty("locale");
  });

  // ── Create always calls mutate (no no-op skip) ────────────────────────────

  it("always calls mutate on valid submit, never skips as no-op", async () => {
    renderDialog();
    const nameInput = screen.getByLabelText(/name/i);
    fireEvent.change(nameInput, { target: { value: "Alice" } });
    fireEvent.submit(nameInput.closest("form")!);
    await waitFor(() => expect(mockMutate).toHaveBeenCalledOnce());
  });

  // ── Cancel ─────────────────────────────────────────────────────────────────

  it("calls onClose when Cancel button is clicked without creating", () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(mockMutate).not.toHaveBeenCalled();
  });
});
