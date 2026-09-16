import { describe, it, expect, beforeEach } from "vitest";
import { writeGuestSession, readGuestSession, clearGuestSession } from "./guestSession";

const session = { token: "guest-tok", roomId: "room-1", mediaId: "media-1" };

beforeEach(() => {
  sessionStorage.clear();
});

// The guest token is media-scoped: the public watch route must only find it
// for the media the code was entered for, and never after the tab is done.
describe("guestSession", () => {
  it("round-trips for the same media and refuses another", () => {
    writeGuestSession(session);
    expect(readGuestSession("media-1")).toEqual(session);
    expect(readGuestSession("media-2")).toBeNull();
  });

  it("ignores an incomplete or corrupt entry", () => {
    sessionStorage.setItem("evey-guest-session", JSON.stringify({ mediaId: "media-1", token: "" }));
    expect(readGuestSession("media-1")).toBeNull();
    sessionStorage.setItem("evey-guest-session", "{not json");
    expect(readGuestSession("media-1")).toBeNull();
    expect(readGuestSession("media-1")).toBeNull();
  });

  it("clears the entry, and tolerates a missing one", () => {
    writeGuestSession(session);
    clearGuestSession();
    expect(readGuestSession("media-1")).toBeNull();
    clearGuestSession();
  });

  it("lives in sessionStorage, never localStorage", () => {
    writeGuestSession(session);
    expect(localStorage.getItem("evey-guest-session")).toBeNull();
    expect(sessionStorage.getItem("evey-guest-session")).not.toBeNull();
  });
});
