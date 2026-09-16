import { describe, it, expect, vi } from "vitest";

// The modal pulls in expo-video and the auth store at module level; the helper
// under test is pure, so those imports are stubbed rather than exercised.
vi.mock("expo-video", () => ({ useVideoPlayer: () => ({}), VideoView: () => null }));
vi.mock("@evey/store", () => ({ useAuthStore: () => null, toast: {} }));
vi.mock("../../../providers/ThemeProvider", () => ({ useTheme: () => ({}) }));

import { suggestedShifts } from "./SyncPreviewModal";

// Mirror of the web SyncPreviewPanel helper, same contract: the offsets the
// scan measured on THIS file, both signs, de-duplicated, sorted, and neither
// past what the server accepts (60 s) nor below what an ear resolves.
describe("suggestedShifts (mobile)", () => {
  it("offers the measured offsets on both signs", () => {
    expect(suggestedShifts({ audioShortfallSec: 19, audioTracksLagSec: 0, audioDelaySec: 0 })).toEqual([-19, 19]);
    expect(suggestedShifts({ audioShortfallSec: 0, audioTracksLagSec: -2.5, audioDelaySec: 0.08 })).toEqual([-2.5, -0.08, 0.08, 2.5]);
  });

  it("drops what the server refuses or an ear cannot hear, and collapses duplicates", () => {
    expect(suggestedShifts({ audioShortfallSec: 900, audioTracksLagSec: 0.01, audioDelaySec: 0 })).toEqual([]);
    expect(suggestedShifts({ audioShortfallSec: 12, audioTracksLagSec: -12, audioDelaySec: 12.004 })).toEqual([-12, 12]);
    expect(suggestedShifts({})).toEqual([]);
  });
});
