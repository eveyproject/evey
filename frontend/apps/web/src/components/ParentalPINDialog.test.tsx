import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ParentalPINDialog } from "./ParentalPINDialog";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
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

const onClose = vi.fn();
const onVerified = vi.fn();

function renderDialog(open = true) {
  return render(
    <ParentalPINDialog
      open={open}
      onClose={onClose}
      onVerified={onVerified}
      profileId="profile-1"
    />,
  );
}

describe("ParentalPINDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when closed", () => {
    renderDialog(false);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders dialog when open", () => {
    renderDialog();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Restricted Content")).toBeTruthy();
    expect(screen.getByTestId("pin-input")).toBeTruthy();
  });

  it("submit button disabled when pin shorter than 4 chars", () => {
    renderDialog();
    const input = screen.getByTestId("pin-input");
    fireEvent.change(input, { target: { value: "12" } });
    expect(screen.getByTestId("pin-submit")).toBeDisabled();
  });

  it("submit button enabled when pin is 4 digits", () => {
    renderDialog();
    const input = screen.getByTestId("pin-input");
    fireEvent.change(input, { target: { value: "1234" } });
    expect(screen.getByTestId("pin-submit")).not.toBeDisabled();
  });

  it("calls onVerified when PIN is correct", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValueOnce({ valid: true });

    renderDialog();
    const input = screen.getByTestId("pin-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.click(screen.getByTestId("pin-submit"));

    await waitFor(() => expect(onVerified).toHaveBeenCalledOnce());
    expect(eveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/profiles/profile-1/parental/verify-pin",
      method: "POST",
      data: { pin: "1234" },
    });
  });

  it("shows error when PIN is incorrect", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValueOnce({ valid: false });

    renderDialog();
    const input = screen.getByTestId("pin-input");
    fireEvent.change(input, { target: { value: "9999" } });
    fireEvent.click(screen.getByTestId("pin-submit"));

    await waitFor(() => expect(screen.getByTestId("pin-error")).toBeTruthy());
    expect(screen.getByTestId("pin-error").textContent).toBe("Incorrect PIN");
    expect(onVerified).not.toHaveBeenCalled();
  });

  it("shows error when API throws", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockRejectedValueOnce(new Error("network error"));

    renderDialog();
    const input = screen.getByTestId("pin-input");
    fireEvent.change(input, { target: { value: "1234" } });
    fireEvent.click(screen.getByTestId("pin-submit"));

    await waitFor(() => expect(screen.getByTestId("pin-error")).toBeTruthy());
    expect(onVerified).not.toHaveBeenCalled();
  });

  it("calls onClose when Annuler is clicked", () => {
    renderDialog();
    fireEvent.click(screen.getByTestId("pin-cancel"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("strips non-numeric characters from PIN input", () => {
    renderDialog();
    const input = screen.getByTestId("pin-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "ab12cd34" } });
    expect(input.value).toBe("1234");
  });
});
