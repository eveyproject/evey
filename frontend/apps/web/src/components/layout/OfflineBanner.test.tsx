import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { OfflineBanner } from "./OfflineBanner";

let mockOnline = true;
vi.mock("@/lib/online-manager", () => ({
  useOnlineStatus: () => mockOnline,
}));

describe("OfflineBanner", () => {
  it("renders nothing while online", () => {
    mockOnline = true;
    render(<OfflineBanner />);
    expect(screen.queryByTestId("offline-banner")).not.toBeInTheDocument();
  });

  it("announces offline state to screen readers when offline", () => {
    mockOnline = false;
    render(<OfflineBanner />);
    const banner = screen.getByTestId("offline-banner");
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveAttribute("aria-live", "polite");
    expect(banner).toHaveTextContent(/offline/i);
  });
});
