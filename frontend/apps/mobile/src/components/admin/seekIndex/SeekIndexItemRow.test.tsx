import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  ActivityIndicator: () => null,
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    // Interpolation matters here: the badge is only useful if it names the
    // number of seconds, so the fake renders it the way i18next would.
    t: (key: string, opts?: Record<string, unknown>) =>
      opts && "sec" in opts ? `${key}:${opts.sec}` : key,
  }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({ useTheme: () => THEME }));

import { SeekIndexItemRow } from "./SeekIndexItemRow";

const THEME = {
  colors: {
    bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#aaa",
    border: "#333", accent: "#88f", success: "#0a0", warning: "#fa0", info: "#08f", error: "#f00",
    divider: "#333",
  },
  radii: { sm: 4, md: 8, lg: 12 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
} as any;

const noop = () => {};
const row = (item: Record<string, unknown>) =>
  render(
    <SeekIndexItemRow
      item={{ mediaId: "m1", title: "Film", path: "/m/film.mkv", hasIndex: true, ...item } as any}
      theme={THEME}
      busy={false}
      containerFixAvailable={false}
      reinterleaveAvailable={true}
      onRepair={noop}
      onRepairResync={noop}
      onRollback={noop}
      onFixContainer={noop}
      onReinterleave={noop}
      onAck={noop}
      onDeleteBackup={noop}
      repairPending={false}
      repairResyncPending={false}
      rollbackPending={false}
      fixContainerPending={false}
      reinterleavePending={false}
      ackPending={false}
      deleteBackupPending={false}
    />,
  );

describe("SeekIndexItemRow, the audio offset badges", () => {
  // The framing the 2026-07-13 batch got wrong: a declared offset is how a dub
  // is lined up with a cut. Every player follows it, and rewriting it is what
  // desynchronised the films. Mobile painted it as a WARNING, which invites
  // exactly the repair that must not happen.
  it("shows a plain declared offset as a fact, not an alarm", () => {
    row({ audioDelaySec: 12 });
    const badge = screen.getByText("admin.seekIndex.audioOffsetBadge:12.0");
    expect(badge).toBeTruthy();
    // audioDelayBadge was the old warning-coloured label; the key is gone
    // from the catalogues now, and nothing may bring it back.
    expect(screen.queryByText(/audioDelayBadge/)).toBeNull();
  });

  // The measurement is the whole point: once the file's own tracks have been
  // compared and agree, the hint can say so instead of hedging.
  it("says the offset is proven deliberate once the tracks were compared", () => {
    row({ audioDelaySec: 12, audioTracksChecked: true });
    expect(screen.getByLabelText("admin.seekIndex.audioOffsetProvenHint")).toBeTruthy();
  });

  // The real defect, and the one mobile did not show at all: the file's own
  // tracks disagree, so one of them is in the wrong place.
  it("warns when the file's own tracks were measured to disagree", () => {
    row({ audioDelaySec: 0.02, audioTracksChecked: true, audioTracksLagSec: 11.94 });
    expect(screen.getByText("admin.seekIndex.audioTracksDisagreeBadge:11.9")).toBeTruthy();
    expect(screen.queryByText(/audioOffsetBadge/)).toBeNull();
  });

  it("shows nothing about audio on a file with neither", () => {
    row({});
    expect(screen.queryByText(/audioOffset|audioTracksDisagree/)).toBeNull();
  });
});

describe("SeekIndexItemRow, tracks stored in blocks", () => {
  it("offers the re-mux and no index rebuild for a chunked file", () => {
    row({ interleaveBackstepSec: 366, verdict: "repairable" });
    expect(screen.getByText("admin.seekIndex.brokenInterleaveBadge")).toBeTruthy();
    expect(screen.getByText("admin.seekIndex.reinterleaveBtn")).toBeTruthy();
    expect(screen.queryByText("admin.seekIndex.repair")).toBeNull();
  });
});
