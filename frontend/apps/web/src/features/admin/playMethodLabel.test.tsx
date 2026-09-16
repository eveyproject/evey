import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useSessionPlayMethodLabel } from "./playMethodLabel";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// One label per effective method (the real, per-track verdict, not the
// declared one), and an empty string when there is no method to name so the
// caller drops the badge - the Now Playing card and the sessions list had
// drifted on the wasm case before this was shared.
describe("useSessionPlayMethodLabel", () => {
  it("labels the effective method and stays empty when undeclared", () => {
    const { result } = renderHook(() => useSessionPlayMethodLabel());
    const label = result.current;
    expect(label({ playMethod: "direct_play" })).toBe("admin.sessions.directPlay");
    expect(label({ playMethod: "transcode", videoDecision: "copy", audioDecision: "copy" })).toBe("admin.sessions.directStream");
    expect(label({ playMethod: "transcode", videoDecision: "transcode", audioDecision: "copy" })).toBe("admin.sessions.transcode");
    // Audio re-encoded on the client (wasm) is not a server transcode.
    expect(label({ playMethod: "transcode", videoDecision: "copy", audioDecision: "transcode", wasmAudio: true })).toBe("admin.sessions.directStream");
    expect(label({ playMethod: "transcode", videoDecision: "copy", audioDecision: "transcode" })).toBe("admin.sessions.transcode");
    expect(label({ playMethod: "" })).toBe("");
    expect(label({})).toBe("");
  });
});
