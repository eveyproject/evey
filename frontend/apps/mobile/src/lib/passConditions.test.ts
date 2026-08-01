import { describe, it, expect, vi, beforeEach } from "vitest";

// expo-battery/expo-network are native bridges; jsdom has no globalThis.expo
// so the real modules can't run. These mocks keep a live reference to the
// last-registered listener per module so tests can fire events by hand, and
// a `.remove()` spy so leak-on-resolve is checkable.
const battery = vi.hoisted(() => {
  const listeners = new Set<(e: { batteryState: number }) => void>();
  return {
    BatteryState: { UNKNOWN: 0, UNPLUGGED: 1, CHARGING: 2, FULL: 3 },
    getBatteryStateAsync: vi.fn(async () => 1 /* UNPLUGGED */),
    addBatteryStateListener: vi.fn((cb: (e: { batteryState: number }) => void) => {
      listeners.add(cb);
      return { remove: vi.fn(() => listeners.delete(cb)) };
    }),
    __listeners: listeners,
  };
});
vi.mock("expo-battery", () => battery);

const network = vi.hoisted(() => {
  const listeners = new Set<(e: { type: string }) => void>();
  return {
    NetworkStateType: { NONE: "NONE", UNKNOWN: "UNKNOWN", CELLULAR: "CELLULAR", WIFI: "WIFI" },
    getNetworkStateAsync: vi.fn(async () => ({ type: "CELLULAR", isConnected: true, isInternetReachable: true })),
    addNetworkStateListener: vi.fn((cb: (e: { type: string }) => void) => {
      listeners.add(cb);
      return { remove: vi.fn(() => listeners.delete(cb)) };
    }),
    __listeners: listeners,
  };
});
vi.mock("expo-network", () => network);

import { conditionsMet, waitingReasonFor, getPassConditions, waitForPassConditions } from "./passConditions";

beforeEach(() => {
  vi.clearAllMocks();
  battery.__listeners.clear();
  network.__listeners.clear();
  battery.getBatteryStateAsync.mockResolvedValue(1 /* UNPLUGGED */);
  network.getNetworkStateAsync.mockResolvedValue({ type: "CELLULAR", isConnected: true, isInternetReachable: true });
});

function fireBattery(state: number) {
  for (const cb of battery.__listeners) cb({ batteryState: state });
}
function fireNetwork(type: string) {
  for (const cb of network.__listeners) cb({ type });
}

describe("conditionsMet / waitingReasonFor", () => {
  it("is met only when both charging and unmeteredWifi are true", () => {
    expect(conditionsMet({ charging: true, unmeteredWifi: true })).toBe(true);
    expect(conditionsMet({ charging: true, unmeteredWifi: false })).toBe(false);
    expect(conditionsMet({ charging: false, unmeteredWifi: true })).toBe(false);
    expect(conditionsMet({ charging: false, unmeteredWifi: false })).toBe(false);
  });

  it("names the specific missing condition(s)", () => {
    expect(waitingReasonFor({ charging: true, unmeteredWifi: true })).toBeNull();
    expect(waitingReasonFor({ charging: true, unmeteredWifi: false })).toBe("wifi");
    expect(waitingReasonFor({ charging: false, unmeteredWifi: true })).toBe("charge");
    expect(waitingReasonFor({ charging: false, unmeteredWifi: false })).toBe("charge-and-wifi");
  });
});

describe("getPassConditions", () => {
  it("reads charging=true from CHARGING and FULL battery states", async () => {
    battery.getBatteryStateAsync.mockResolvedValue(2 /* CHARGING */);
    network.getNetworkStateAsync.mockResolvedValue({ type: "WIFI" });
    expect(await getPassConditions()).toEqual({ charging: true, unmeteredWifi: true });

    battery.getBatteryStateAsync.mockResolvedValue(3 /* FULL */);
    expect(await getPassConditions()).toEqual({ charging: true, unmeteredWifi: true });
  });

  it("reads unmeteredWifi=false for cellular and any non-Wi-Fi type", async () => {
    battery.getBatteryStateAsync.mockResolvedValue(2 /* CHARGING */);
    network.getNetworkStateAsync.mockResolvedValue({ type: "CELLULAR" });
    expect(await getPassConditions()).toEqual({ charging: true, unmeteredWifi: false });
  });

  it("fails closed to not-met when a probe throws", async () => {
    battery.getBatteryStateAsync.mockRejectedValue(new Error("no battery API"));
    network.getNetworkStateAsync.mockRejectedValue(new Error("no network API"));
    expect(await getPassConditions()).toEqual({ charging: false, unmeteredWifi: false });
  });
});

describe("waitForPassConditions", () => {
  it("resolves immediately when already met, without registering listeners", async () => {
    battery.getBatteryStateAsync.mockResolvedValue(2 /* CHARGING */);
    network.getNetworkStateAsync.mockResolvedValue({ type: "WIFI" });
    await waitForPassConditions();
    // Listeners ARE registered synchronously before the probe settles (to
    // avoid missing an event in that window), but must be removed again once
    // the already-met initial snapshot resolves the promise.
    expect(battery.__listeners.size).toBe(0);
    expect(network.__listeners.size).toBe(0);
  });

  it("waits for both conditions and reports every intermediate snapshot", async () => {
    const updates: unknown[] = [];
    const p = waitForPassConditions((c) => updates.push(c));
    await vi.waitFor(() => expect(battery.__listeners.size).toBe(1));

    fireNetwork("WIFI");
    await vi.waitFor(() => expect(updates.at(-1)).toEqual({ charging: false, unmeteredWifi: true }));

    let settled = false;
    p.then(() => { settled = true; });
    await new Promise((r) => setTimeout(r, 0));
    expect(settled).toBe(false);

    fireBattery(2 /* CHARGING */);
    await p;
    expect(settled).toBe(true);
    // Both native listeners are torn down once resolved - no leak across
    // repeated waits (the app may defer many passes over its lifetime).
    expect(battery.__listeners.size).toBe(0);
    expect(network.__listeners.size).toBe(0);
  });

  it("ignores a same-field initial probe that resolves after its own listener already reported", async () => {
    // Regression pin: the initial battery probe is a separate in-flight
    // promise from the battery listener. If a real charging event lands
    // first and the slow initial probe THEN resolves with a stale value, the
    // stale value must not revert what the listener already established -
    // an earlier combined-snapshot design let exactly this happen.
    let resolveBatteryProbe!: (v: number) => void;
    battery.getBatteryStateAsync.mockImplementation(() => new Promise((r) => { resolveBatteryProbe = r; }));
    network.getNetworkStateAsync.mockResolvedValue({ type: "WIFI" });

    const p = waitForPassConditions();
    await vi.waitFor(() => expect(battery.__listeners.size).toBe(1));
    fireBattery(2 /* CHARGING, via listener first */);
    resolveBatteryProbe(1 /* UNPLUGGED, stale initial probe answers late */);
    await p;
  });

  it("does not call onUpdate once conditions become fully met (resolves instead)", async () => {
    const updates: unknown[] = [];
    const p = waitForPassConditions((c) => updates.push(c));
    await vi.waitFor(() => expect(battery.__listeners.size).toBe(1));
    fireNetwork("WIFI");
    fireBattery(2 /* CHARGING */);
    await p;
    expect(updates.every((u) => !conditionsMet(u as { charging: boolean; unmeteredWifi: boolean }))).toBe(true);
  });
});
