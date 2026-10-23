import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { PluginUpdateDialog } from "./PluginUpdateDialog";

// Mock createPortal so Modal renders inline in jsdom
vi.mock("react-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-dom")>();
  return {
    ...actual,
    createPortal: (node: React.ReactNode) => node,
  };
});

const mockEveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

const defaultPreview = {
  currentVersion: "1.0.0",
  candidateVersion: "2.0.0",
  permissionsAdded: ["network"],
  permissionsRemoved: [],
  capabilitiesAdded: [],
  capabilitiesRemoved: [],
  slotsAdded: [],
  slotsRemoved: [],
  externalFetchAdded: [],
  externalFetchRemoved: [],
  newFindings: [],
  signatureStatus: "unsigned",
  requiresAck: true,
  sourceHash: "abc123def456",
};

const safePreview = {
  ...defaultPreview,
  permissionsAdded: [],
  requiresAck: false,
};

beforeEach(() => {
  mockEveyFetch.mockReset();
});

describe("PluginUpdateDialog", () => {
  it("shows spinner while previewing", async () => {
    // Preview never resolves during this test
    mockEveyFetch.mockReturnValue(new Promise(() => {}));
    render(
      <PluginUpdateDialog
        pluginId="p1"
        pluginName="My Plugin"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    expect(screen.getByText("Analyzing candidate...")).toBeInTheDocument();
  });

  it("displays version header after preview", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        pluginName="My Plugin"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByText(/1\.0\.0.*2\.0\.0/)).toBeInTheDocument(),
    );
  });

  it("shows red banner when requiresAck is true", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("banner-requires-ack")).toBeInTheDocument(),
    );
  });

  it("shows green banner when requiresAck is false", async () => {
    mockEveyFetch.mockResolvedValueOnce(safePreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("banner-safe")).toBeInTheDocument(),
    );
  });

  it("displays added permission 'network' in red", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByText("network")).toBeInTheDocument(),
    );
    const networkEl = screen.getByText("network");
    // Parent li should contain a red "+" marker
    expect(screen.getByTestId("permission-added")).toBeInTheDocument();
    expect(networkEl).toBeInTheDocument();
  });

  it("Apply button is disabled when requiresAck=true and checkbox unchecked", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("apply-button")).toBeInTheDocument());
    expect(screen.getByTestId("apply-button")).toBeDisabled();
  });

  it("Apply button becomes enabled after checking ack checkbox", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("ack-checkbox")).toBeInTheDocument());
    await userEvent.click(screen.getByTestId("ack-checkbox"));
    expect(screen.getByTestId("apply-button")).not.toBeDisabled();
  });

  it("calls onApplied and onClose after successful apply", async () => {
    const onApplied = vi.fn();
    const onClose = vi.fn();
    // First call: preview, second call: apply
    mockEveyFetch
      .mockResolvedValueOnce(defaultPreview)
      .mockResolvedValueOnce({});

    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={onClose}
        onApplied={onApplied}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("ack-checkbox")).toBeInTheDocument());
    await userEvent.click(screen.getByTestId("ack-checkbox"));
    await userEvent.click(screen.getByTestId("apply-button"));

    await waitFor(() => {
      expect(onApplied).toHaveBeenCalledOnce();
      expect(onClose).toHaveBeenCalledOnce();
    });

    // Verify acknowledged_hash was sent
    expect(mockEveyFetch).toHaveBeenNthCalledWith(2, expect.objectContaining({
      data: expect.objectContaining({ acknowledged_hash: "abc123def456" }),
    }));
  });

  it("shows hash mismatch error on 409 Aborted response", async () => {
    const abortedError = new Error("409 Aborted");
    mockEveyFetch
      .mockResolvedValueOnce(defaultPreview)
      .mockRejectedValueOnce(abortedError);

    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("ack-checkbox")).toBeInTheDocument());
    await userEvent.click(screen.getByTestId("ack-checkbox"));
    await userEvent.click(screen.getByTestId("apply-button"));

    await waitFor(() =>
      expect(screen.getByTestId("error-message")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("error-message").textContent).toContain(
      "candidate hash no longer matches",
    );
  });

  it("shows generic error message on non-409 apply failure", async () => {
    mockEveyFetch
      .mockResolvedValueOnce(safePreview)
      .mockRejectedValueOnce(new Error("Internal server error"));

    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("apply-button")).not.toBeDisabled());
    await userEvent.click(screen.getByTestId("apply-button"));

    await waitFor(() =>
      expect(screen.getByTestId("error-message")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("error-message").textContent).toContain("Internal server error");
  });

  it("shows error message when preview itself fails", async () => {
    mockEveyFetch.mockRejectedValueOnce(new Error("Plugin not found"));
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/bad/path.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("error-message")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("error-message").textContent).toContain("Plugin not found");
  });

  it("calls onClose when Cancel is clicked", async () => {
    mockEveyFetch.mockResolvedValueOnce(defaultPreview);
    const onClose = vi.fn();
    render(
      <PluginUpdateDialog
        pluginId="p1"
        source="/path/to/plugin.zip"
        onClose={onClose}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByText("Cancel")).toBeInTheDocument());
    await userEvent.click(screen.getByText("Cancel"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("requires the publisher checkbox when the key changed and sends accept_new_publisher", async () => {
    mockEveyFetch.mockResolvedValueOnce({
      ...safePreview,
      signatureStatus: "publisher-changed",
      requiresAck: true,
      signature: { verified: true, trusted: false, keyFingerprint: "sha256:aaaa", continuity: "changed" },
    });
    mockEveyFetch.mockResolvedValueOnce({});
    render(
      <PluginUpdateDialog
        pluginId="p1"
        pluginName="My Plugin"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("apply-button")).toBeInTheDocument());
    expect(screen.getByTestId("banner-publisher")).toBeInTheDocument();
    expect(screen.queryByTestId("ack-checkbox")).not.toBeInTheDocument();
    expect(screen.getByTestId("apply-button")).toBeDisabled();
    await userEvent.click(screen.getByTestId("publisher-ack-checkbox"));
    expect(screen.getByTestId("apply-button")).not.toBeDisabled();
    await userEvent.click(screen.getByTestId("apply-button"));
    await waitFor(() => expect(mockEveyFetch).toHaveBeenCalledTimes(2));
    expect(mockEveyFetch).toHaveBeenLastCalledWith(
      expect.objectContaining({ data: expect.objectContaining({ accept_new_publisher: true }) }),
    );
  });

  it("does not show the publisher checkbox on a plain update", async () => {
    mockEveyFetch.mockResolvedValueOnce(safePreview);
    render(
      <PluginUpdateDialog
        pluginId="p1"
        pluginName="My Plugin"
        source="/path/to/plugin.zip"
        onClose={vi.fn()}
        onApplied={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId("apply-button")).toBeInTheDocument());
    expect(screen.queryByTestId("publisher-ack-checkbox")).not.toBeInTheDocument();
  });
});
