import { describe, it, expect, vi } from "vitest";
import * as Network from "expo-network";
import { isMeteredNetwork } from "./networkHint";

// Only cellular is "paid by the megabyte"; an unreadable state must NOT nag
// people on their home Wi-Fi, so a failed probe reads as not metered.
describe("isMeteredNetwork", () => {
  it("is true on cellular only", async () => {
    const spy = vi.spyOn(Network, "getNetworkStateAsync");
    spy.mockResolvedValueOnce({ type: Network.NetworkStateType.CELLULAR, isConnected: true, isInternetReachable: true });
    expect(await isMeteredNetwork()).toBe(true);
    spy.mockResolvedValueOnce({ type: Network.NetworkStateType.WIFI, isConnected: true, isInternetReachable: true });
    expect(await isMeteredNetwork()).toBe(false);
    spy.mockResolvedValueOnce({ type: Network.NetworkStateType.UNKNOWN, isConnected: false, isInternetReachable: false });
    expect(await isMeteredNetwork()).toBe(false);
    spy.mockRejectedValueOnce(new Error("no bridge"));
    expect(await isMeteredNetwork()).toBe(false);
    spy.mockRestore();
  });
});
