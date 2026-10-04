import { describe, it, expect } from "vitest";
import { observeCastStall, initialCastStallTracker, CAST_STALL_SEC, PLAYER_STATE_IDLE, PLAYER_STATE_PLAYING } from "./castStallBeacon";

const t0 = 1_000_000;
function playing(pos: number, nowMs: number) {
  return { connected: true, playerState: PLAYER_STATE_PLAYING, posSec: pos, nowMs };
}

describe("observeCastStall", () => {
  it("reports a PLAYING position frozen for the stall window, once", () => {
    let tr = initialCastStallTracker;
    ({ next: tr } = observeCastStall(tr, playing(100, t0)));
    ({ next: tr } = observeCastStall(tr, playing(101, t0 + 1000)));
    let r = observeCastStall(tr, playing(101, t0 + 1000 + (CAST_STALL_SEC - 1) * 1000));
    expect(r.report).toBeNull();
    r = observeCastStall(r.next, playing(101, t0 + 1000 + CAST_STALL_SEC * 1000));
    expect(r.report).toBe(`state=playing frozen=${CAST_STALL_SEC}s pos=101`);
    const again = observeCastStall(r.next, playing(101, t0 + 1000 + (CAST_STALL_SEC + 30) * 1000));
    expect(again.report).toBeNull();
  });

  it("a moving position clears the stall and re-arms a later one", () => {
    let tr = initialCastStallTracker;
    ({ next: tr } = observeCastStall(tr, playing(50, t0)));
    tr = observeCastStall(tr, playing(50, t0 + CAST_STALL_SEC * 1000)).next;
    expect(tr.reported).toBe("stall");
    tr = observeCastStall(tr, playing(51, t0 + CAST_STALL_SEC * 1000 + 1000)).next;
    expect(tr.reported).toBeNull();
    const r = observeCastStall(tr, playing(51, t0 + 2 * CAST_STALL_SEC * 1000 + 1000));
    expect(r.report).toMatch(/^state=playing frozen=/);
  });

  it("does not count a paused or buffering receiver as stalled", () => {
    let tr = initialCastStallTracker;
    ({ next: tr } = observeCastStall(tr, playing(10, t0)));
    const paused = observeCastStall(tr, { connected: true, playerState: "paused", posSec: 10, nowMs: t0 + 60_000 });
    expect(paused.report).toBeNull();
    // resuming from the pause starts a fresh window
    const r = observeCastStall(paused.next, playing(10, t0 + 61_000));
    expect(r.report).toBeNull();
  });

  it("reports an IDLE receiver with its reason once, and again for a different reason", () => {
    let r = observeCastStall(initialCastStallTracker, { connected: true, playerState: PLAYER_STATE_IDLE, idleReason: "ERROR", posSec: 3266, nowMs: t0 });
    expect(r.report).toBe("state=idle reason=ERROR pos=3266");
    r = observeCastStall(r.next, { connected: true, playerState: PLAYER_STATE_IDLE, idleReason: "ERROR", posSec: 3266, nowMs: t0 + 5000 });
    expect(r.report).toBeNull();
    r = observeCastStall(r.next, { connected: true, playerState: PLAYER_STATE_IDLE, idleReason: 4, posSec: 3266, nowMs: t0 + 9000 });
    expect(r.report).toBe("state=idle reason=4 pos=3266");
  });

  it("an IDLE state without a reason (a normal load) is not a report", () => {
    const r = observeCastStall(initialCastStallTracker, { connected: true, playerState: PLAYER_STATE_IDLE, idleReason: null, posSec: 0, nowMs: t0 });
    expect(r.report).toBeNull();
  });

  it("disconnecting resets everything", () => {
    let tr = observeCastStall(initialCastStallTracker, playing(10, t0)).next;
    tr = observeCastStall(tr, playing(10, t0 + CAST_STALL_SEC * 1000)).next;
    const r = observeCastStall(tr, { connected: false, posSec: 0, nowMs: t0 + 99_000 });
    expect(r.next).toEqual(initialCastStallTracker);
  });
});
