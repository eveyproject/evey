import { describe, it, expect, vi, afterEach } from "vitest";
import { notificationUrl, translateNotificationTitle } from "./notificationUtils";

afterEach(() => {
  vi.useRealTimers();
});

describe("notificationUrl", () => {
  it("sends a media request page to the moderation queue, a title to its page, else nowhere", () => {
    expect(notificationUrl({ type: "media_request" })).toBe("/admin/media-requests");
    expect(notificationUrl({ type: "media_request", mediaItemId: "m1" })).toBe("/admin/media-requests");
    expect(notificationUrl({ type: "new_movie", mediaItemId: "m1" })).toBe("/media/m1");
    expect(notificationUrl({ type: "backup_failed" })).toBeUndefined();
    expect(notificationUrl({})).toBeUndefined();
  });
});

describe("translateNotificationTitle", () => {
  it("accepts a bare title string as well as a notification", () => {
    const t = (k: string, opts?: Record<string, unknown>) => (opts?.defaultValue as string) ?? k;
    expect(translateNotificationTitle(t, "Plain title")).toBe("Plain title");
    expect(translateNotificationTitle(t, { title: "Plain title" })).toBe("Plain title");
    expect(typeof translateNotificationTitle(t, undefined)).toBe("string");
  });
});
