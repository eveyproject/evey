import { describe, it, expect, vi } from "vitest";
import { EARLY_FAILURE, subscribePlayerStatus, type StatusEvent } from "./playerStatus";

function fakePlayer(status: string) {
  const listeners: ((ev: StatusEvent) => void)[] = [];
  return {
    status,
    addListener: vi.fn((_e: "statusChange", l: (ev: StatusEvent) => void) => {
      listeners.push(l);
      return { remove: vi.fn() };
    }),
    emit: (ev: StatusEvent) => listeners.forEach((l) => l(ev)),
  };
}

describe("subscribePlayerStatus", () => {
  it("hands over a failure that happened before the subscription", () => {
    const onStatus = vi.fn();
    subscribePlayerStatus(fakePlayer("error"), onStatus);
    expect(onStatus).toHaveBeenCalledWith({ status: "error", error: { message: EARLY_FAILURE } });
  });

  it("stays quiet for a player that is still loading, and forwards later events", () => {
    const onStatus = vi.fn();
    const p = fakePlayer("loading");
    subscribePlayerStatus(p, onStatus);
    expect(onStatus).not.toHaveBeenCalled();
    p.emit({ status: "error", error: { message: "boom" } });
    expect(onStatus).toHaveBeenCalledWith({ status: "error", error: { message: "boom" } });
  });

  it("survives a player released before its status could be read", () => {
    const p = { addListener: () => ({ remove() {} }), get status(): string { throw new Error("released"); } };
    expect(() => subscribePlayerStatus(p, vi.fn())).not.toThrow();
  });
});
