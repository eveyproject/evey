import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  TextInput: ({ value, onChangeText, onSubmitEditing, placeholder }: any) =>
    React.createElement("input", {
      placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
      onKeyDown: (e: React.KeyboardEvent) => { if (e.key === "Enter") onSubmitEditing?.(); },
    }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }) }));
vi.mock("../TrickplayPreview", () => ({ TrickplayPreview: () => null }));
const pushMock = vi.fn();
vi.mock("expo-router", () => ({ router: { push: (...a: unknown[]) => pushMock(...a) } }));

import { SessionCard, formatBandwidth, playMethodLabel } from "./SessionCard";

const theme = {
  colors: { surface: "#111", border: "#333", accent: "#f00", text: "#fff", textMuted: "#888", warning: "#fa0", success: "#0f0", error: "#f33", onAccent: "#000", bg: "#000" },
  radii: { md: 8 },
} as any;
const t = (k: string, d?: string | Record<string, unknown>) => (typeof d === "string" ? d : k);

const session = {
  sessionId: "s1", mediaItemId: "m1", mediaTitle: "Heat", profileName: "alice", deviceName: "Firefox", deviceType: "web",
  playMethod: "transcode", resolution: "1080p", videoCodec: "hevc", audioCodec: "aac", bandwidthBps: "8000000",
  durationMs: "6000000", positionMs: "1500000", progress: 25, isPaused: false,
  seriesTitle: "Show", seasonNumber: 1, episodeNumber: 7, castDeviceName: "Salon TV",
} as any;

describe("SessionCard helpers", () => {
  it("formats bandwidth by magnitude and labels the play method", () => {
    expect(formatBandwidth(500)).toBe("500 bps");
    expect(formatBandwidth(64_000)).toBe("64 Kbps");
    expect(formatBandwidth(8_250_000)).toBe("8.3 Mbps");
    expect(playMethodLabel("transcode", t)).toBe("admin.sessions.transcode");
    expect(playMethodLabel("direct_play", t)).toBe("admin.sessions.directPlay");
    expect(playMethodLabel("", t)).toBe("Direct");
    expect(playMethodLabel("wasm", t)).toBe("wasm");
  });
});

describe("SessionCard", () => {
  it("opens the media page from the title", () => {
    pushMock.mockClear();
    render(<SessionCard session={session} theme={theme} t={t} onKill={vi.fn()} onCommand={vi.fn()} />);
    fireEvent.click(screen.getByText("Heat"));
    expect(pushMock).toHaveBeenCalledWith("/media/m1");
  });

  it("shows what is playing, for whom, how, and the time left", () => {
    render(<SessionCard session={session} theme={theme} t={t} onKill={vi.fn()} onCommand={vi.fn()} />);
    expect(screen.getByText("Heat")).toBeTruthy();
    expect(screen.getByText("Show - S01E07")).toBeTruthy();
    expect(screen.getByText("alice")).toBeTruthy();
    expect(screen.getByText("Firefox (web)")).toBeTruthy();
    expect(screen.getByText("8.0 Mbps")).toBeTruthy();
    expect(screen.getByText("admin.sessions.transcode")).toBeTruthy();
    expect(screen.getByText("-1:15:00")).toBeTruthy();
    expect(screen.getByText("admin.sessions.castingTo")).toBeTruthy();
  });

  it("drives play/pause, stop and a trimmed message through the parent's callbacks", () => {
    const onKill = vi.fn();
    const onCommand = vi.fn();
    const { rerender } = render(<SessionCard session={session} theme={theme} t={t} onKill={onKill} onCommand={onCommand} />);
    fireEvent.click(screen.getByLabelText("common.pause"));
    expect(onCommand).toHaveBeenCalledWith("s1", "pause");
    rerender(<SessionCard session={{ ...session, isPaused: true }} theme={theme} t={t} onKill={onKill} onCommand={onCommand} />);
    fireEvent.click(screen.getByLabelText("common.play"));
    expect(onCommand).toHaveBeenCalledWith("s1", "play");

    fireEvent.click(screen.getByLabelText("admin.sessions.stopSession"));
    expect(onKill).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "s1" }));

    fireEvent.click(screen.getByLabelText("admin.sessions.sendMessage"));
    const input = screen.getByPlaceholderText("Send a message...") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommand).not.toHaveBeenCalledWith("s1", expect.stringMatching(/^message:/));
    fireEvent.change(input, { target: { value: "  dinner in 5  " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommand).toHaveBeenCalledWith("s1", "message:dinner in 5");
    expect(screen.queryByPlaceholderText("Send a message...")).toBeNull();
  });
});
