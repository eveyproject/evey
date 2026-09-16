import { describe, it, expect, vi, beforeEach } from "vitest";

const getVapidMock = vi.fn();
const registerMock = vi.fn();
const deleteMock = vi.fn();
vi.mock("@evey/api", () => ({
  pushServiceGetVAPIDKey: (...args: unknown[]) => getVapidMock(...args),
  pushServiceRegisterWebPush: (...args: unknown[]) => registerMock(...args),
  pushServiceDeleteWebPushSubscription: (...args: unknown[]) => deleteMock(...args),
}));

import { isPushSupported, subscribeToPush, unsubscribeFromPush, isPushSubscribed } from "./webpush";

interface FakeSubscription {
  endpoint: string;
  unsubscribe: ReturnType<typeof vi.fn>;
  toJSON: () => { endpoint: string; keys?: { p256dh?: string; auth?: string } };
}

function installBrowser(opts: {
  permission?: NotificationPermission;
  registration?: { pushManager: { subscribe?: ReturnType<typeof vi.fn>; getSubscription?: ReturnType<typeof vi.fn> } } | null;
}) {
  const reg = opts.registration;
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value: {
      serviceWorker: {
        ready: Promise.resolve(reg),
        getRegistration: vi.fn().mockResolvedValue(reg ?? undefined),
        register: vi.fn().mockResolvedValue(undefined),
      },
    },
  });
  (globalThis as unknown as { window: unknown }).window = { PushManager: class {}, Notification: class {} };
  (globalThis as unknown as { Notification: unknown }).Notification = {
    requestPermission: vi.fn().mockResolvedValue(opts.permission ?? "granted"),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("webpush", () => {
  it("is unsupported without a service worker, push manager or notifications", () => {
    Object.defineProperty(globalThis, "navigator", { configurable: true, value: {} });
    (globalThis as unknown as { window: unknown }).window = {};
    expect(isPushSupported()).toBe(false);
    installBrowser({ registration: null });
    expect(isPushSupported()).toBe(true);
  });

  it("does not subscribe nor register when the permission is refused", async () => {
    const subscribe = vi.fn();
    installBrowser({ permission: "denied", registration: { pushManager: { subscribe } } });
    await expect(subscribeToPush()).resolves.toBe(false);
    expect(subscribe).not.toHaveBeenCalled();
    expect(registerMock).not.toHaveBeenCalled();
  });

  it("subscribes with the decoded VAPID key and registers the endpoint and keys", async () => {
    // "AQID" is base64url for the bytes 1,2,3 (unpadded, as VAPID keys are).
    getVapidMock.mockResolvedValue({ publicKey: "AQID" });
    const sub: FakeSubscription = {
      endpoint: "https://push.example/ep",
      unsubscribe: vi.fn(),
      toJSON: () => ({ endpoint: "https://push.example/ep", keys: { p256dh: "P", auth: "A" } }),
    };
    const subscribe = vi.fn().mockResolvedValue(sub);
    installBrowser({ registration: { pushManager: { subscribe } } });

    await expect(subscribeToPush()).resolves.toBe(true);
    const opts = subscribe.mock.calls[0][0] as { userVisibleOnly: boolean; applicationServerKey: Uint8Array };
    expect(opts.userVisibleOnly).toBe(true);
    expect(Array.from(opts.applicationServerKey)).toEqual([1, 2, 3]);
    expect(opts.applicationServerKey.buffer).toBeInstanceOf(ArrayBuffer);
    expect(registerMock).toHaveBeenCalledWith({
      endpoint: "https://push.example/ep",
      keys: { p256dh: "P", auth: "A" },
    });
  });

  it("decodes a url-safe key with padding restored", async () => {
    // "-_8" is base64url for 0xFB 0xFF (needs one '=' of padding).
    getVapidMock.mockResolvedValue({ publicKey: "-_8" });
    const subscribe = vi.fn().mockResolvedValue({
      endpoint: "e", unsubscribe: vi.fn(), toJSON: () => ({ endpoint: "e" }),
    });
    installBrowser({ registration: { pushManager: { subscribe } } });
    await subscribeToPush();
    const key = (subscribe.mock.calls[0][0] as { applicationServerKey: Uint8Array }).applicationServerKey;
    expect(Array.from(key)).toEqual([0xfb, 0xff]);
  });

  it("unsubscribe is a no-op without a registration or a subscription", async () => {
    installBrowser({ registration: null });
    await unsubscribeFromPush();
    expect(deleteMock).not.toHaveBeenCalled();

    installBrowser({ registration: { pushManager: { getSubscription: vi.fn().mockResolvedValue(null) } } });
    await unsubscribeFromPush();
    expect(deleteMock).not.toHaveBeenCalled();
    await expect(isPushSubscribed()).resolves.toBe(false);
  });

  it("unsubscribes the browser first, then forgets the endpoint server-side", async () => {
    const order: string[] = [];
    const sub: FakeSubscription = {
      endpoint: "https://push.example/ep",
      unsubscribe: vi.fn().mockImplementation(async () => { order.push("browser"); return true; }),
      toJSON: () => ({ endpoint: "https://push.example/ep" }),
    };
    deleteMock.mockImplementation(async () => { order.push("server"); });
    installBrowser({ registration: { pushManager: { getSubscription: vi.fn().mockResolvedValue(sub) } } });

    await expect(isPushSubscribed()).resolves.toBe(true);
    await unsubscribeFromPush();
    expect(deleteMock).toHaveBeenCalledWith({ endpoint: "https://push.example/ep" });
    expect(order).toEqual(["browser", "server"]);
  });
});
