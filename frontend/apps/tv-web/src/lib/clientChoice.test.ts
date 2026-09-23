import { describe, expect, it } from "vitest";
import { choiceInAddress, shouldAskWhichClient, shouldGoToWebClient } from "./clientChoice";

const served = { protocol: "https:", hostname: "evey.example.com", pathname: "/tv/", search: "", stored: null } as const;

describe("which client the viewer meant", () => {
  it("asks a served page that has never been answered", () => {
    expect(shouldAskWhichClient({ ...served })).toBe(true);
    expect(shouldGoToWebClient({ ...served })).toBe(false);
  });

  // The packaged application IS the television client, and there is no web
  // application beside it to offer.
  it("never asks the packaged application", () => {
    expect(shouldAskWhichClient({ ...served, protocol: "file:" })).toBe(false);
    expect(shouldGoToWebClient({ ...served, protocol: "file:", stored: "web" })).toBe(false);
  });

  it("never asks on a development host", () => {
    for (const hostname of ["localhost", "127.0.0.1", "192.168.1.42", "10.0.0.7", "evey.local"]) {
      expect(shouldAskWhichClient({ ...served, hostname })).toBe(false);
      expect(shouldGoToWebClient({ ...served, hostname, stored: "web" })).toBe(false);
    }
  });

  // Nothing to hand over to, and a redirect to "/" would reload itself.
  it("never asks when the television client IS what the root serves", () => {
    for (const pathname of ["/", "/index.html"]) {
      expect(shouldAskWhichClient({ ...served, pathname })).toBe(false);
      expect(shouldGoToWebClient({ ...served, pathname, stored: "web" })).toBe(false);
    }
  });

  it("remembers the answer instead of asking again", () => {
    expect(shouldAskWhichClient({ ...served, stored: "tv" })).toBe(false);
    expect(shouldGoToWebClient({ ...served, stored: "tv" })).toBe(false);
    expect(shouldAskWhichClient({ ...served, stored: "web" })).toBe(false);
    expect(shouldGoToWebClient({ ...served, stored: "web" })).toBe(true);
  });

  it("lets the address say it, which wins over the memory", () => {
    expect(shouldGoToWebClient({ ...served, search: "?client=web", stored: "tv" })).toBe(true);
    expect(shouldGoToWebClient({ ...served, search: "?client=tv", stored: "web" })).toBe(false);
    expect(shouldAskWhichClient({ ...served, search: "?client=tv" })).toBe(false);
    expect(choiceInAddress("?a=b&client=web")).toBe("web");
    expect(choiceInAddress("?client=nonsense")).toBe(null);
    expect(choiceInAddress("")).toBe(null);
  });
});
