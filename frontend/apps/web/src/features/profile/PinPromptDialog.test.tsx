import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { PinPromptDialog } from "./PinPromptDialog";
import type { V1Profile } from "@evey/api";

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  parentalServiceVerifyPin: vi.fn(),
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: { open: boolean; children: React.ReactNode; title?: string }) =>
    open ? (
      <div role="dialog">
        {title && <h2>{title}</h2>}
        {children}
      </div>
    ) : null,
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({
    children,
    loading,
    disabled,
    onClick,
    type,
    ...rest
  }: {
    children: React.ReactNode;
    loading?: boolean;
    disabled?: boolean;
    onClick?: () => void;
    type?: string;
    [k: string]: unknown;
  }) => (
    <button type={(type as "button" | "submit" | "reset") ?? "button"} disabled={disabled || loading} onClick={onClick} {...rest}>
      {loading ? "loading" : children}
    </button>
  ),
}));

vi.mock("@/components/ui/Input", () => ({
  Input: React.forwardRef(function MockInput(
    {
      onChange,
      value,
      ...rest
    }: {
      onChange?: React.ChangeEventHandler<HTMLInputElement>;
      value?: string;
      [k: string]: unknown;
    },
    ref: React.Ref<HTMLInputElement>,
  ) {
    return <input ref={ref} onChange={onChange} value={value} {...rest} />;
  }),
}));

const BASE_PROFILE: V1Profile = {
  id: "p-1",
  userId: "u-1",
  name: "Kids",
  isDefault: false,
  isKids: true,
  locale: "",
  region: "",
};

const onCancel = vi.fn();
const onVerified = vi.fn();

function renderDialog(profile: V1Profile = BASE_PROFILE) {
  return render(
    React.createElement(PinPromptDialog, {
      target: profile,
      onCancel,
      onVerified,
    }),
  );
}

describe("PinPromptDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Rendering ──────────────────────────────────────────────────────────────

  it("renders the dialog with PIN input", () => {
    renderDialog();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByTestId("profile-pin-prompt-input")).toBeTruthy();
  });

  it("shows the profile name in the prompt body", () => {
    renderDialog();
    expect(screen.getByText(/kids/i)).toBeTruthy();
  });

  // ── Submit button gating: disabled until exactly 4 digits ─────────────────

  it("submit button is disabled initially (empty input)", () => {
    renderDialog();
    expect(screen.getByRole("button", { name: /confirm/i })).toBeDisabled();
  });

  it("submit button is disabled when fewer than 4 digits entered", () => {
    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "123" } });
    expect(screen.getByRole("button", { name: /confirm/i })).toBeDisabled();
  });

  it("submit button is enabled when exactly 4 digits entered", () => {
    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    expect(screen.getByRole("button", { name: /confirm/i })).not.toBeDisabled();
  });

  // ── Input: only digits, max 4 ──────────────────────────────────────────────

  it("strips non-numeric characters from PIN input", () => {
    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "ab12cd34" } });
    // Component applies: replace(/\D/g, "").slice(0, 4)
    expect(input.value).toBe("1234");
  });

  it("truncates input to 4 digits maximum", () => {
    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "123456" } });
    expect(input.value).toBe("1234");
  });

  // ── PIN format validation on submit ───────────────────────────────────────

  it("shows validation error and does NOT call verify when PIN is fewer than 4 digits on submit", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    // Manually set a short value bypassing the onChange filter (simulates edge case)
    fireEvent.change(input, { target: { value: "12" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => {
      expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy();
    });
    expect(screen.getByTestId("profile-pin-prompt-error").textContent).toMatch(/4 digit/i);
    expect(parentalServiceVerifyPin).not.toHaveBeenCalled();
    // SECURITY: gate must stay closed
    expect(onVerified).not.toHaveBeenCalled();
  });

  // ── Correct PIN: gate opens ────────────────────────────────────────────────

  it("calls parentalServiceVerifyPin with correct profile id and PIN", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: true, pinToken: "tok-abc" });

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(parentalServiceVerifyPin).toHaveBeenCalledOnce());
    expect(parentalServiceVerifyPin).toHaveBeenCalledWith("p-1", { pin: "1234" });
  });

  it("calls onVerified with target profile and pinToken when PIN is correct - the gate opens", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: true, pinToken: "tok-abc" });

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(onVerified).toHaveBeenCalledOnce());
    expect(onVerified).toHaveBeenCalledWith(BASE_PROFILE, "tok-abc");
  });

  // ── SECURITY: wrong PIN - gate stays closed ────────────────────────────────

  it("shows error and does NOT call onVerified when API returns valid=false - gate stays closed", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: false });

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "9999" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy());
    expect(screen.getByTestId("profile-pin-prompt-error").textContent).toMatch(/incorrect pin/i);
    // SECURITY: onVerified must NOT fire on wrong PIN
    expect(onVerified).not.toHaveBeenCalled();
  });

  it("does NOT call onVerified when API rejects (network/server error) - gate stays closed", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockRejectedValueOnce(new Error("network error"));

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy());
    expect(screen.getByTestId("profile-pin-prompt-error").textContent).toMatch(/network error/i);
    // SECURITY: gate stays closed on error
    expect(onVerified).not.toHaveBeenCalled();
  });

  it("shows generic error message when API rejects with a non-Error value", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockRejectedValueOnce("unknown");

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy());
    expect(onVerified).not.toHaveBeenCalled();
  });

  it("dialog stays open (error shown, no onVerified) after wrong PIN submission", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: false });

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "0000" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy());
    // Dialog is still rendered
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(onVerified).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  // ── Retry after wrong PIN ──────────────────────────────────────────────────

  it("clears previous error and succeeds on retry with correct PIN", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");
    // First call: wrong PIN
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: false });
    // Second call: correct PIN
    vi.mocked(parentalServiceVerifyPin).mockResolvedValueOnce({ valid: true, pinToken: "tok-retry" });

    renderDialog();
    const input = screen.getByTestId("profile-pin-prompt-input");

    // Wrong attempt
    fireEvent.change(input, { target: { value: "0000" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(screen.getByTestId("profile-pin-prompt-error")).toBeTruthy());
    expect(onVerified).not.toHaveBeenCalled();

    // Correct retry
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(onVerified).toHaveBeenCalledOnce());
    expect(onVerified).toHaveBeenCalledWith(BASE_PROFILE, "tok-retry");
  });

  // ── Cancel ─────────────────────────────────────────────────────────────────

  it("calls onCancel and does NOT call onVerified or verify API when Cancel is clicked", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");

    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onCancel).toHaveBeenCalledOnce();
    expect(onVerified).not.toHaveBeenCalled();
    expect(parentalServiceVerifyPin).not.toHaveBeenCalled();
  });

  // ── Profile with no id: handleSubmit returns early ────────────────────────

  it("does NOT call verify API when target profile has no id", async () => {
    const { parentalServiceVerifyPin } = await import("@evey/api");

    const noIdProfile: V1Profile = { ...BASE_PROFILE, id: undefined };
    renderDialog(noIdProfile);
    const input = screen.getByTestId("profile-pin-prompt-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.submit(input.closest("form")!);

    // Early return: no API call, no onVerified
    await new Promise((r) => setTimeout(r, 50));
    expect(parentalServiceVerifyPin).not.toHaveBeenCalled();
    expect(onVerified).not.toHaveBeenCalled();
  });
});
