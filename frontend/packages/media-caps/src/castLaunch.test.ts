import { describe, it, expect } from "vitest";
import {
  buildCastCustomData,
  buildLaunchCredentials,
  CAST_CUSTOM_DATA_KEY,
  normalizeOrigin,
  parseCastCustomData,
  parseLaunchCredentials,
  shouldAcceptLaunch,
} from "./castLaunch";

describe("normalizeOrigin", () => {
  it("makes two spellings of one instance compare equal", () => {
    expect(normalizeOrigin("https://Evey.test/")).toBe(normalizeOrigin("https://evey.test"));
    expect(normalizeOrigin("http://192.168.1.10:9091/")).toBe("http://192.168.1.10:9091");
  });

  it("keeps the path off, and survives garbage without throwing", () => {
    expect(normalizeOrigin("https://evey.test/library/movies")).toBe("https://evey.test");
    expect(normalizeOrigin("not a url/")).toBe("not a url");
    expect(normalizeOrigin(undefined)).toBe("");
  });
});

describe("launch credentials", () => {
  it("round-trips what the checker needs", () => {
    const raw = buildLaunchCredentials({ serverOrigin: "https://evey.test/", userId: "u1", profileId: "p1" });
    expect(parseLaunchCredentials(raw)).toEqual({ v: 1, serverOrigin: "https://evey.test", userId: "u1", profileId: "p1" });
  });

  it("parses the JSON string the Cast SDK hands over, and refuses nonsense", () => {
    expect(parseLaunchCredentials('{"v":1,"serverOrigin":"https://a.test"}')?.serverOrigin).toBe("https://a.test");
    expect(parseLaunchCredentials('{"v":2,"serverOrigin":"https://a.test"}')).toBeNull();
    expect(parseLaunchCredentials('{"v":1}')).toBeNull();
    expect(parseLaunchCredentials("garbage")).toBeNull();
    expect(parseLaunchCredentials(null)).toBeNull();
  });
});

describe("media payload", () => {
  it("names the media by id and carries the resume point and picks", () => {
    const data = buildCastCustomData({
      mediaId: "m1",
      positionSec: 90.7,
      audioTrack: 2,
      subtitleTrack: "off",
      serverOrigin: "https://evey.test",
    });
    expect(parseCastCustomData(data)).toEqual({
      v: 1,
      mediaId: "m1",
      positionSec: 90,
      audioTrack: 2,
      subtitleTrack: "off",
      serverOrigin: "https://evey.test",
    });
  });

  it("drops a nonsensical resume instead of seeking somewhere absurd", () => {
    const p = parseCastCustomData(buildCastCustomData({ mediaId: "m1", positionSec: -5, serverOrigin: "x" }));
    expect(p?.positionSec).toBe(0);
    const q = parseCastCustomData({ [CAST_CUSTOM_DATA_KEY]: { v: 1, mediaId: "m1", positionSec: "12", serverOrigin: "x" } });
    expect(q?.positionSec).toBe(0);
  });

  it("refuses a payload with no media id, and one from a future version", () => {
    expect(parseCastCustomData({ [CAST_CUSTOM_DATA_KEY]: { v: 1, serverOrigin: "x" } })).toBeNull();
    expect(parseCastCustomData({ [CAST_CUSTOM_DATA_KEY]: { v: 2, mediaId: "m1" } })).toBeNull();
    expect(parseCastCustomData(undefined)).toBeNull();
  });

  it("accepts the bare payload too - some senders unwrap customData", () => {
    expect(parseCastCustomData({ v: 1, mediaId: "m1", serverOrigin: "https://a.test" })?.mediaId).toBe("m1");
  });
});

// The whole point of the checker: a "no" is not an error, it is the handover
// to the Web Receiver. Every case below used to end with the TV app opening
// the wrong thing, or nothing.
describe("shouldAcceptLaunch", () => {
  const tv = { signedIn: true, serverOrigin: "https://evey.test", userId: "u1" };
  const creds = parseLaunchCredentials(buildLaunchCredentials({ serverOrigin: "https://evey.test", userId: "u1" }));

  it("takes the cast when the TV is signed into the same instance and account", () => {
    expect(shouldAcceptLaunch({ credentials: creds, tv })).toBe(true);
  });

  it("hands back a TV that is not signed in", () => {
    expect(shouldAcceptLaunch({ credentials: creds, tv: { ...tv, signedIn: false } })).toBe(false);
  });

  it("hands back a TV on another server - the 19-09 case, dev TV / prod phone", () => {
    expect(shouldAcceptLaunch({ credentials: creds, tv: { ...tv, serverOrigin: "http://192.168.1.50:9091" } })).toBe(false);
  });

  it("hands back a TV signed in as someone else", () => {
    expect(shouldAcceptLaunch({ credentials: creds, tv: { ...tv, userId: "u2" } })).toBe(false);
  });

  it("hands back a sender that says nothing about itself (old Evey, third-party client)", () => {
    expect(shouldAcceptLaunch({ credentials: null, tv })).toBe(false);
  });

  it("accepts when only one side names an account - nothing contradicts", () => {
    const anon = parseLaunchCredentials(buildLaunchCredentials({ serverOrigin: "https://evey.test" }));
    expect(shouldAcceptLaunch({ credentials: anon, tv })).toBe(true);
    expect(shouldAcceptLaunch({ credentials: creds, tv: { ...tv, userId: null } })).toBe(true);
  });
});
