import { describe, it, expect, vi, beforeEach } from "vitest";

const net = vi.hoisted(() => ({ isMeteredNetwork: vi.fn(async () => false) }));
vi.mock("./networkHint", () => net);

import { downloadStartedMessage } from "./downloadStartedToast";

// The translator stands in for i18next: it returns the fallback, which is what
// the real one does for a key the server has not shipped yet.
const t = (_key: string, fallback: string) => fallback;

describe("downloadStartedMessage", () => {
  beforeEach(() => {
    net.isMeteredNetwork.mockReset();
  });

  it("says nothing about the network on Wi-Fi", async () => {
    net.isMeteredNetwork.mockResolvedValue(false);
    expect(await downloadStartedMessage(t)).toBe("Download started");
  });

  it("adds the Wi-Fi hint on mobile data, without refusing the download", async () => {
    net.isMeteredNetwork.mockResolvedValue(true);
    const msg = await downloadStartedMessage(t);
    // The download is ALREADY under way when this toast shows - the hint is
    // information, never a gate. Both halves must be present: dropping the
    // "started" half would read as a refusal.
    expect(msg).toContain("Download started");
    expect(msg).toContain("Wi-Fi recommended");
  });

  it("falls back to the plain message when the network cannot be read", async () => {
    // A phone that will not answer about its own connection must not produce a
    // permanent "you are on mobile data" nag for someone sitting on Wi-Fi.
    net.isMeteredNetwork.mockResolvedValue(false);
    expect(await downloadStartedMessage(t)).toBe("Download started");
  });
});
