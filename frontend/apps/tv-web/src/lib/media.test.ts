import { describe, it, expect, vi } from "vitest";

vi.mock("@evey/api", () => ({ getBaseURL: () => "http://evey:9091" }));
vi.mock("@evey/store", () => ({ useAuthStore: { getState: () => ({ accessToken: "jwt" }) } }));

import { avatarSrc, posterSrc, serverImageSrc } from "./media";

// A collection poster is a TMDB url: the viewer's token must never ride to it.
describe("serverImageSrc", () => {
  it("leaves another host's url untouched", () => {
    expect(serverImageSrc("https://image.tmdb.org/t/p/w500/x.jpg")).toBe("https://image.tmdb.org/t/p/w500/x.jpg");
  });

  it("anchors the server's own path and adds the token", () => {
    expect(serverImageSrc("/api/v1/music/albums/a1/art")).toBe("http://evey:9091/api/v1/music/albums/a1/art?token=jwt");
  });
});

describe("posterSrc", () => {
  it("names the public artwork route without the token", () => {
    expect(posterSrc("m1")).toBe("http://evey:9091/api/v1/media/m1/artwork/poster?w=400");
  });
});

describe("avatarSrc", () => {
  it("names the public avatar route without the token", () => {
    expect(avatarSrc("p1")).toBe("http://evey:9091/api/v1/profiles/p1/avatar");
  });
});
