import { describe, it, expect, vi, beforeEach } from "vitest";

// Regression harness for the 2026-07-26 cast cut: without POST_NOTIFICATIONS
// the Cast SDK's media notification (a foreground service) never shows on
// Android 13+, Android freezes the sender app ~60s after the screen turns
// off, and the receiver loses its sender - operator boxes then
// stop playback instead of continuing.
const check = vi.fn();
const request = vi.fn();
let platformOS = "android";
let platformVersion: number | string = 34;

vi.mock("react-native", () => ({
  get Platform() {
    return { OS: platformOS, Version: platformVersion };
  },
  PermissionsAndroid: {
    PERMISSIONS: { POST_NOTIFICATIONS: "android.permission.POST_NOTIFICATIONS" },
    check: (...args: unknown[]) => check(...args),
    request: (...args: unknown[]) => request(...args),
  },
}));

import { ensureNotificationPermission } from "./notificationPermission";

beforeEach(() => {
  platformOS = "android";
  platformVersion = 34;
  check.mockReset();
  request.mockReset();
});

describe("ensureNotificationPermission", () => {
  it("requests POST_NOTIFICATIONS on Android 13+ when not yet granted", async () => {
    check.mockResolvedValue(false);
    request.mockResolvedValue("granted");

    await ensureNotificationPermission();

    expect(check).toHaveBeenCalledWith("android.permission.POST_NOTIFICATIONS");
    expect(request).toHaveBeenCalledWith("android.permission.POST_NOTIFICATIONS");
  });

  it("does not re-request when the permission is already granted", async () => {
    check.mockResolvedValue(true);

    await ensureNotificationPermission();

    expect(request).not.toHaveBeenCalled();
  });

  it("is a no-op below Android 13 (permission does not exist there)", async () => {
    platformVersion = 32;

    await ensureNotificationPermission();

    expect(check).not.toHaveBeenCalled();
    expect(request).not.toHaveBeenCalled();
  });

  it("is a no-op on iOS", async () => {
    platformOS = "ios";

    await ensureNotificationPermission();

    expect(check).not.toHaveBeenCalled();
    expect(request).not.toHaveBeenCalled();
  });

  it("never throws when the request itself fails - a denied dialog must not break the cast", async () => {
    check.mockResolvedValue(false);
    request.mockRejectedValue(new Error("dialog dismissed"));

    await expect(ensureNotificationPermission()).resolves.toBeUndefined();
  });
});
