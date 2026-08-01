// Gate for the audio compat pass: it is minutes of full-speed CPU rewriting
// a multi-gigabyte file, so it only runs while the device is plugged in AND
// on an unmetered network (Wi-Fi) - the same heuristic background sync tools
// use, even though the pass itself never touches the network. Deferred, not
// cancelled: waitForPassConditions resolves the moment both flip true.

import * as Battery from "expo-battery";
import * as Network from "expo-network";

export type WaitingReason = "charge" | "wifi" | "charge-and-wifi";

export interface PassConditions {
  charging: boolean;
  unmeteredWifi: boolean;
}

function isCharging(state: Battery.BatteryState): boolean {
  return state === Battery.BatteryState.CHARGING || state === Battery.BatteryState.FULL;
}

function isUnmeteredWifi(type: Network.NetworkStateType | undefined): boolean {
  return type === Network.NetworkStateType.WIFI;
}

export function conditionsMet(c: PassConditions): boolean {
  return c.charging && c.unmeteredWifi;
}

export function waitingReasonFor(c: PassConditions): WaitingReason | null {
  if (conditionsMet(c)) return null;
  if (!c.charging && !c.unmeteredWifi) return "charge-and-wifi";
  return c.charging ? "wifi" : "charge";
}

/**
 * One-shot snapshot of the two gating conditions. Battery and network are
 * independent native calls; either can throw (module absent, permission
 * denied) without taking the other down - a failed probe reads as "not met"
 * so the pass defers instead of running on an unverifiable state.
 */
export async function getPassConditions(): Promise<PassConditions> {
  const [batteryState, networkState] = await Promise.all([
    Battery.getBatteryStateAsync().catch(() => Battery.BatteryState.UNKNOWN),
    Network.getNetworkStateAsync().catch(() => ({ type: undefined }) as Network.NetworkState),
  ]);
  return {
    charging: isCharging(batteryState),
    unmeteredWifi: isUnmeteredWifi(networkState.type),
  };
}

/**
 * Resolves once charging AND unmetered-wifi are both true - immediately if
 * they already are. onUpdate is called with every intermediate snapshot so a
 * caller can keep a visible "waiting for X" state fresh (e.g. Wi-Fi comes
 * back before the charger does). Listeners are registered BEFORE the initial
 * probe resolves so a condition flipping true in that window is not missed.
 *
 * The two initial probes are seeded INDEPENDENTLY per field, each ignored
 * once its own listener has already reported - two in-flight async calls
 * racing a real device event otherwise let a stale initial snapshot stomp a
 * newer value for the OTHER field too (a single combined getPassConditions()
 * merge was tried first and does exactly that under this race).
 */
export function waitForPassConditions(onUpdate?: (c: PassConditions) => void): Promise<void> {
  return new Promise((resolve) => {
    let settled = false;
    let batterySeeded = false;
    let networkSeeded = false;
    let state: PassConditions = { charging: false, unmeteredWifi: false };

    function finish() {
      if (settled) return;
      settled = true;
      batterySub.remove();
      networkSub.remove();
      resolve();
    }

    function apply(next: Partial<PassConditions>) {
      state = { ...state, ...next };
      if (conditionsMet(state)) finish();
      else onUpdate?.(state);
    }

    const batterySub = Battery.addBatteryStateListener((e) => {
      batterySeeded = true;
      apply({ charging: isCharging(e.batteryState) });
    });
    const networkSub = Network.addNetworkStateListener((e) => {
      networkSeeded = true;
      apply({ unmeteredWifi: isUnmeteredWifi(e.type) });
    });

    Battery.getBatteryStateAsync()
      .then((s) => isCharging(s))
      .catch(() => false)
      .then((charging) => {
        if (batterySeeded) return;
        batterySeeded = true;
        apply({ charging });
      });
    Network.getNetworkStateAsync()
      .then((s) => isUnmeteredWifi(s.type))
      .catch(() => false)
      .then((unmeteredWifi) => {
        if (networkSeeded) return;
        networkSeeded = true;
        apply({ unmeteredWifi });
      });
  });
}
