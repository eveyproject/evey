import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import * as Network from "expo-network";
import { useOnlineStatus } from "./useOnlineStatus";

vi.mock("expo-network", () => ({
  NetworkStateType: { WIFI: "WIFI", CELLULAR: "CELLULAR" },
  getNetworkStateAsync: vi.fn(),
}));

vi.mock("react-native", async () => {
  const actual = await vi.importActual<typeof import("react-native")>("react-native");
  return {
    ...actual,
    AppState: {
      addEventListener: vi.fn(() => ({ remove: vi.fn() })),
      currentState: "active",
    },
  };
});

beforeEach(() => {
  vi.clearAllMocks();
});

const mockNetwork = vi.mocked(Network.getNetworkStateAsync);

describe("useOnlineStatus", () => {
  it("reports online when both isConnected and isInternetReachable are true", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: true,
      isInternetReachable: true,
    });

    const { result } = renderHook(() => useOnlineStatus());
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("reports offline when not connected", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: false,
      isInternetReachable: false,
    });

    const { result } = renderHook(() => useOnlineStatus());
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("reports offline on captive portal (connected but no internet)", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: true,
      isInternetReachable: false,
    });

    const { result } = renderHook(() => useOnlineStatus());
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("falls back to true when expo-network throws", async () => {
    mockNetwork.mockRejectedValue(new Error("native module not ready"));
    const { result } = renderHook(() => useOnlineStatus());
    // Initial state is true; throw shouldn't flip it.
    await waitFor(() => expect(result.current).toBe(true));
  });
});
