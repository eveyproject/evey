import { describe, it, expect, vi, beforeEach } from "vitest";

// Free disk space behind a getter on a stable Paths object: the module imports
// the binding by name, so reassigning the export from a test would not reach
// it - mutating this holder does. Same shape as downloads.test.ts.
const { disk } = vi.hoisted(() => ({ disk: { available: 500 * 1024 ** 3 } }));
vi.mock("expo-file-system", () => ({
  Paths: { get availableDiskSpace() { return disk.available; } },
}));

interface AlertButton { text: string; style?: string; onPress?: () => void }
const alerts: { title: string; body: string; buttons: AlertButton[] }[] = [];
vi.mock("react-native", () => ({
  Alert: {
    alert: (title: string, body: string, buttons: AlertButton[]) => {
      alerts.push({ title, body, buttons });
    },
  },
}));

const planMock = vi.fn();
vi.mock("./reducedDownload", () => ({ fetchReducedPlan: (...a: unknown[]) => planMock(...a) }));

import { chooseDownloadQuality } from "./downloadQualityGate";

// Passes the default through, and interpolates, so assertions read as the
// English a user would see.
const t = (_k: string, d: string, o?: Record<string, unknown>) =>
  d.replace(/\{\{(\w+)\}\}/g, (_, k) => String(o?.[k] ?? ""));

const PLAN = {
  available: true, offer: false, height: 720, width: 1280,
  estimatedBytes: 2 * 1024 ** 3, sourceBytes: 40 * 1024 ** 3,
  slices: 12, sliceSec: 600, durationMs: 7_200_000,
};

function press(label: RegExp) {
  const button = alerts.at(-1)!.buttons.find((b) => label.test(b.text));
  expect(button, `no button matching ${label} in ${alerts.at(-1)!.buttons.map((b) => b.text).join(" | ")}`).toBeTruthy();
  button!.onPress?.();
}

beforeEach(() => {
  alerts.length = 0;
  planMock.mockReset();
  disk.available = 500 * 1024 ** 3;
});

describe("chooseDownloadQuality", () => {
  // No dialog is the common case, and it has to STAY the common case: asking
  // about every download would be worse than never offering the choice.
  it("says nothing when the server has no lighter copy to offer", async () => {
    planMock.mockResolvedValue(null);
    expect(await chooseDownloadQuality(t, "m1")).toEqual({ quality: "original" });
    expect(alerts).toHaveLength(0);
  });

  it("says nothing when the file is under the threshold and fits", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: false });
    expect(await chooseDownloadQuality(t, "m1")).toEqual({ quality: "original" });
    expect(alerts).toHaveLength(0);
  });

  it("asks when the operator's threshold is crossed", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: true });
    const choice = chooseDownloadQuality(t, "m1");
    // The Alert goes up only after the plan lookup resolves, never on the
    // same tick as the call.
    await vi.waitFor(() => expect(alerts).toHaveLength(1));
    expect(alerts[0].body).toContain("40.0 GB");
    expect(alerts[0].body).toContain("2.0 GB");
    press(/720p/);
    expect(await choice).toEqual({ quality: "reduced", plan: { ...PLAN, offer: true } });
  });

  // The device's own limit is invisible to the server, so it is a second,
  // independent trigger - a phone with 3 GB free must be asked about a 40 GB
  // film even on an instance that never set a threshold.
  it("asks when the original will not fit on this device", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: false });
    disk.available = 3 * 1024 ** 3;
    const choice = chooseDownloadQuality(t, "m1");
    await vi.waitFor(() => expect(alerts).toHaveLength(1));
    expect(alerts[0].body).toContain("will not fit");
    // Android emphasises the LAST button: when the original cannot land at
    // all, the emphasised one must not be the original.
    expect(alerts[0].buttons.at(-1)!.text).toContain("720p");
    press(/720p/);
    expect((await choice).quality).toBe("reduced");
  });

  it("keeps the original as the emphasised choice when it does fit", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: true });
    const choice = chooseDownloadQuality(t, "m1");
    await vi.waitFor(() => expect(alerts).toHaveLength(1));
    expect(alerts[0].buttons.at(-1)!.text).toContain("Original");
    press(/Original/);
    expect(await choice).toEqual({ quality: "original" });
  });

  it("treats dismissal as a cancel, not as a silent original", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: true });
    const choice = chooseDownloadQuality(t, "m1");
    await vi.waitFor(() => expect(alerts).toHaveLength(1));
    press(/Cancel/);
    expect(await choice).toEqual({ quality: "cancel" });
  });

  // An unreadable free-space figure must not invent a problem: fall back to
  // the server's opinion rather than guessing the device is full.
  it("does not claim the file will not fit when free space is unknown", async () => {
    planMock.mockResolvedValue({ ...PLAN, offer: false });
    disk.available = 0;
    expect(await chooseDownloadQuality(t, "m1")).toEqual({ quality: "original" });
    expect(alerts).toHaveLength(0);
  });
});
