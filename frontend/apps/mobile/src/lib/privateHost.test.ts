import { describe, it, expect, vi } from "vitest";

vi.mock("react-native", () => ({ Alert: { alert: vi.fn() } }));

import { isPrivateHost, isCleartextToPublicHost } from "./privateHost";

describe("isPrivateHost", () => {
  it("accepts every address that cannot leave the home network", () => {
    for (const h of [
      "localhost",
      "127.0.0.1",
      "10.0.0.5",
      "192.168.1.42",
      "172.16.0.1",
      "172.31.255.254",
      "169.254.1.1", // link-local
      "100.64.0.1", // CGNAT, common behind a French box
      "nas", // bare hostname, how a LAN machine is addressed
      "evey.local",
      "server.lan",
      "::1",
      "[fe80::1]",
      "fd12:3456::1", // unique local
    ]) {
      expect(isPrivateHost(h), h).toBe(true);
    }
  });

  it("rejects public addresses", () => {
    for (const h of [
      "eveyproject.org",
      "8.8.8.8",
      "172.32.0.1", // just OUTSIDE 172.16/12 - the boundary that a naive
      // "starts with 172" check gets wrong
      "172.15.255.255", // the other side of the same boundary
      "11.0.0.1", // adjacent to 10/8
      "193.168.1.1", // one digit away from 192.168
      "2001:4860::1", // public IPv6
    ]) {
      expect(isPrivateHost(h), h).toBe(false);
    }
  });

  it("rejects malformed octets instead of reading them as private", () => {
    // "192.168.1.999" is not an address; treating it as private would let a
    // bogus host through the warning silently.
    expect(isPrivateHost("192.168.1.999")).toBe(false);
    expect(isPrivateHost("999.999.999.999")).toBe(false);
  });
});

describe("isCleartextToPublicHost", () => {
  it("flags plain HTTP out to the internet", () => {
    expect(isCleartextToPublicHost("http://evey.example.com:9091")).toBe(true);
    expect(isCleartextToPublicHost("http://8.8.8.8")).toBe(true);
  });

  it("says nothing about HTTP on the LAN - that is the normal setup", () => {
    expect(isCleartextToPublicHost("http://192.168.1.42:9091")).toBe(false);
    expect(isCleartextToPublicHost("http://nas:9091")).toBe(false);
    expect(isCleartextToPublicHost("http://evey.local")).toBe(false);
  });

  it("says nothing about HTTPS anywhere", () => {
    expect(isCleartextToPublicHost("https://evey.example.com")).toBe(false);
  });

  it("stays quiet on an unparseable address", () => {
    // The caller already reports a malformed URL; a second, scarier dialog on
    // top of it would just confuse.
    expect(isCleartextToPublicHost("http//broken")).toBe(false);
  });
});
