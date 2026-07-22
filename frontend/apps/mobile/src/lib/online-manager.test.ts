import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { onlineManager } from "@tanstack/react-query";
import * as Network from "expo-network";
import { setupOnlineManager } from "./online-manager";

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

const mockNetwork = vi.mocked(Network.getNetworkStateAsync);
let unsubscribe: (() => void) | undefined;

/**
 * Installs the bridge and waits for its first probe to land in onlineManager.
 * A subscriber has to exist first: onlineManager only runs its event listener
 * while something is watching.
 */
async function startBridge(): Promise<boolean> {
  unsubscribe = onlineManager.subscribe(() => {});
  setupOnlineManager();
  await vi.waitFor(() => expect(mockNetwork).toHaveBeenCalled());
  return onlineManager.isOnline();
}

beforeEach(() => {
  vi.clearAllMocks();
  onlineManager.setOnline(true);
});

afterEach(() => {
  unsubscribe?.();
  unsubscribe = undefined;
});

// These cases used to live on useOnlineStatus, which polled the OS itself. The
// polling moved here so React Query learns about connectivity too; the
// interpretation rules moved with it and are unchanged.
describe("setupOnlineManager", () => {
  it("reports online when connected with reachable internet", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: true,
      isInternetReachable: true,
    });
    await startBridge();
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(true));
  });

  it("reports offline when not connected", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: false,
      isInternetReachable: false,
    });
    await startBridge();
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(false));
  });

  // A captive portal answers DHCP but not the server. Treating it as online is
  // how an app insists it can reach something it cannot.
  it("reports offline on a captive portal", async () => {
    mockNetwork.mockResolvedValue({
      type: Network.NetworkStateType.WIFI,
      isConnected: true,
      isInternetReachable: false,
    });
    await startBridge();
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(false));
  });

  // expo-network can throw on the very first call after a cold start. Failing
  // closed there would pause every query on launch.
  it("assumes online when expo-network throws", async () => {
    mockNetwork.mockRejectedValue(new Error("native module not ready"));
    await startBridge();
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(true));
  });
});
