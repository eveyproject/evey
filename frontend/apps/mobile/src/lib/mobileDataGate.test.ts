import { describe, it, expect, vi, beforeEach } from "vitest";

const net = vi.hoisted(() => ({ isMeteredNetwork: vi.fn(async () => false) }));
vi.mock("./networkHint", () => net);

const rn = vi.hoisted(() => ({
  Alert: { alert: vi.fn() },
  Platform: { OS: "android" },
}));
vi.mock("react-native", () => rn);

const intent = vi.hoisted(() => ({
  startActivityAsync: vi.fn(async () => undefined),
  ActivityAction: { WIFI_SETTINGS: "android.settings.WIFI_SETTINGS" },
}));
vi.mock("expo-intent-launcher", () => intent);

import { confirmDownloadOnMobileData } from "./mobileDataGate";

const t = (_key: string, fallback: string) => fallback;

/** Fires the Alert button whose label matches, the way a tap would. */
function tap(label: string) {
  const buttons = rn.Alert.alert.mock.calls[0][2] as { text: string; onPress?: () => void }[];
  const btn = buttons.find((b) => b.text === label);
  if (!btn) throw new Error(`no button labelled ${label}, got: ${buttons.map((b) => b.text).join(", ")}`);
  btn.onPress?.();
}

describe("confirmDownloadOnMobileData", () => {
  beforeEach(() => {
    rn.Alert.alert.mockReset();
    intent.startActivityAsync.mockReset();
    intent.startActivityAsync.mockResolvedValue(undefined);
  });

  it("does not ask at all on Wi-Fi", async () => {
    net.isMeteredNetwork.mockResolvedValue(false);
    await expect(confirmDownloadOnMobileData(t)).resolves.toBe(true);
    expect(rn.Alert.alert).not.toHaveBeenCalled();
  });

  it("lets the download proceed when the user insists", async () => {
    // The whole point: this is a question, not a gate. Cellular downloading is
    // legitimate - a train, a hotel, a home line that is down.
    net.isMeteredNetwork.mockResolvedValue(true);
    const decision = confirmDownloadOnMobileData(t);
    await vi.waitFor(() => expect(rn.Alert.alert).toHaveBeenCalled());
    tap("Download anyway");
    await expect(decision).resolves.toBe(true);
  });

  it("opens the Wi-Fi panel and holds the download back", async () => {
    net.isMeteredNetwork.mockResolvedValue(true);
    const decision = confirmDownloadOnMobileData(t);
    await vi.waitFor(() => expect(rn.Alert.alert).toHaveBeenCalled());
    tap("Turn on Wi-Fi");
    // Not started: the user still has to pick a network, and we cannot know
    // when they are done. They come back and tap download again.
    await expect(decision).resolves.toBe(false);
    // waitFor, not a bare assertion: the panel is opened WITHOUT being awaited
    // (the decision must not hang on a settings screen), and the module itself
    // is loaded lazily - so the call lands a tick after the answer does.
    await vi.waitFor(() =>
      expect(intent.startActivityAsync).toHaveBeenCalledWith(
        "android.settings.panel.action.INTERNET_CONNECTIVITY",
      ),
    );
  });

  it("falls back to the full Wi-Fi settings when the panel is refused", async () => {
    net.isMeteredNetwork.mockResolvedValue(true);
    intent.startActivityAsync.mockRejectedValueOnce(new Error("no such activity"));
    const decision = confirmDownloadOnMobileData(t);
    await vi.waitFor(() => expect(rn.Alert.alert).toHaveBeenCalled());
    tap("Turn on Wi-Fi");
    await decision;
    await vi.waitFor(() =>
      expect(intent.startActivityAsync).toHaveBeenLastCalledWith("android.settings.WIFI_SETTINGS"),
    );
  });

  it("cancels without touching settings", async () => {
    net.isMeteredNetwork.mockResolvedValue(true);
    const decision = confirmDownloadOnMobileData(t);
    await vi.waitFor(() => expect(rn.Alert.alert).toHaveBeenCalled());
    tap("Cancel");
    await expect(decision).resolves.toBe(false);
    expect(intent.startActivityAsync).not.toHaveBeenCalled();
  });
});
