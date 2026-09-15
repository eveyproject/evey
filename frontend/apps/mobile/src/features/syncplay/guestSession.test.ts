import { describe, it, expect, beforeEach } from "vitest";
import { writeGuestSession, readGuestSession, clearGuestSession } from "./guestSession";

// The guest session gates the public watch screen: readGuestSession must only
// return a session for the media it was minted for (a stale code from a
// previous room must not play a different title) and must go empty once the
// guest leaves. Mirror of the web guestSession contract.
beforeEach(() => {
  clearGuestSession();
});

describe("guest session store", () => {
  it("returns the session only for the matching media id", () => {
    writeGuestSession({ token: "tok", roomId: "r1", mediaId: "m1" });
    expect(readGuestSession("m1")).toEqual({ token: "tok", roomId: "r1", mediaId: "m1" });
    expect(readGuestSession("m2")).toBeNull();
  });

  it("returns null once cleared", () => {
    writeGuestSession({ token: "tok", roomId: "r1", mediaId: "m1" });
    clearGuestSession();
    expect(readGuestSession("m1")).toBeNull();
  });

  it("returns null when the session is missing a token or room", () => {
    writeGuestSession({ token: "", roomId: "r1", mediaId: "m1" });
    expect(readGuestSession("m1")).toBeNull();
    writeGuestSession({ token: "tok", roomId: "", mediaId: "m1" });
    expect(readGuestSession("m1")).toBeNull();
  });

  it("keeps only the latest session written", () => {
    writeGuestSession({ token: "a", roomId: "r1", mediaId: "m1" });
    writeGuestSession({ token: "b", roomId: "r2", mediaId: "m2" });
    expect(readGuestSession("m1")).toBeNull();
    expect(readGuestSession("m2")).toEqual({ token: "b", roomId: "r2", mediaId: "m2" });
  });
});
