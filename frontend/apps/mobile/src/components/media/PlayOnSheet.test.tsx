import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: any) => {
      if (key === "player.playOn.sent") return `Sent to ${opts?.name}`;
      return key;
    },
  }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" },
  }),
}));

vi.mock("../BottomSheet", () => ({
  BottomSheet: ({ visible, title, children }: any) =>
    visible ? React.createElement("div", { "data-testid": "sheet" }, React.createElement("span", null, title), children) : null,
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { success: (...a: unknown[]) => toastSuccess(...a), error: (...a: unknown[]) => toastError(...a) },
}));

const usePlayOnDevicesMock = vi.fn();
const mutateMock = vi.fn();
const refetchMock = vi.fn();
vi.mock("../../hooks/usePlayOnDevices", () => ({
  usePlayOnDevices: (...a: unknown[]) => usePlayOnDevicesMock(...a),
  usePlayOnCommand: () => ({ mutate: mutateMock, isPending: false }),
}));

import { PlayOnSheet } from "./PlayOnSheet";

describe("PlayOnSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when not visible", () => {
    usePlayOnDevicesMock.mockReturnValue({ data: [], isLoading: false, refetch: refetchMock });
    const { container } = render(<PlayOnSheet visible={false} mediaId="m1" onClose={vi.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("shows a loading indicator while devices are fetching", () => {
    usePlayOnDevicesMock.mockReturnValue({ data: undefined, isLoading: true, refetch: refetchMock });
    render(<PlayOnSheet visible={true} mediaId="m1" onClose={vi.fn()} />);
    expect(screen.getByTestId("loading")).toBeTruthy();
  });

  it("shows the honest empty state when no device is connected", () => {
    usePlayOnDevicesMock.mockReturnValue({ data: [], isLoading: false, refetch: refetchMock });
    render(<PlayOnSheet visible={true} mediaId="m1" onClose={vi.fn()} />);
    expect(screen.getByText("player.playOn.empty")).toBeTruthy();
  });

  it("lists connected devices by name", () => {
    usePlayOnDevicesMock.mockReturnValue({
      data: [
        { id: "dev-1", name: "Living Room TV", type: "tv", connectedAt: "" },
        { id: "dev-2", name: "Kitchen Tablet", type: "mobile", connectedAt: "" },
      ],
      isLoading: false,
      refetch: refetchMock,
    });
    render(<PlayOnSheet visible={true} mediaId="m1" onClose={vi.fn()} />);
    expect(screen.getByText("Living Room TV")).toBeTruthy();
    expect(screen.getByText("Kitchen Tablet")).toBeTruthy();
  });

  it("sends loadAndPlay for the tapped device and toasts success", () => {
    usePlayOnDevicesMock.mockReturnValue({
      data: [{ id: "dev-1", name: "Living Room TV", type: "tv", connectedAt: "" }],
      isLoading: false,
      refetch: refetchMock,
    });
    const onClose = vi.fn();
    render(<PlayOnSheet visible={true} mediaId="media-42" onClose={onClose} />);

    fireEvent.click(screen.getByLabelText("Living Room TV"));

    expect(mutateMock).toHaveBeenCalledWith(
      { deviceId: "dev-1", command: { type: "loadAndPlay", mediaId: "media-42" } },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );

    const opts = mutateMock.mock.calls[0][1];
    opts.onSuccess();
    expect(toastSuccess).toHaveBeenCalledWith("Sent to Living Room TV");
    expect(onClose).toHaveBeenCalled();
  });

  it("on a failed send (e.g. the device disconnected), toasts failure and refetches", async () => {
    usePlayOnDevicesMock.mockReturnValue({
      data: [{ id: "dev-1", name: "Living Room TV", type: "tv", connectedAt: "" }],
      isLoading: false,
      refetch: refetchMock,
    });
    render(<PlayOnSheet visible={true} mediaId="media-42" onClose={vi.fn()} />);

    fireEvent.click(screen.getByLabelText("Living Room TV"));
    const opts = mutateMock.mock.calls[0][1];
    opts.onError(Object.assign(new Error("not found"), { httpStatus: 404 }));

    expect(toastError).toHaveBeenCalledWith("player.playOn.failed");
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });
});
