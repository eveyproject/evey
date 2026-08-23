import { describe, it, expect } from "vitest";
import { toVoiceSearchResult } from "./voiceResult";

describe("toVoiceSearchResult", () => {
  it("reports unavailable when the recognizer cannot run, regardless of any text", () => {
    expect(toVoiceSearchResult(false, null)).toEqual({ status: "unavailable" });
    expect(toVoiceSearchResult(false, "ignored")).toEqual({ status: "unavailable" });
  });

  it("reports empty on a null result - user cancelled or nothing matched", () => {
    expect(toVoiceSearchResult(true, null)).toEqual({ status: "empty" });
    expect(toVoiceSearchResult(true, undefined)).toEqual({ status: "empty" });
  });

  it("reports empty rather than recognized for blank or whitespace-only text", () => {
    expect(toVoiceSearchResult(true, "")).toEqual({ status: "empty" });
    expect(toVoiceSearchResult(true, "   ")).toEqual({ status: "empty" });
  });

  it("reports the trimmed recognized text", () => {
    expect(toVoiceSearchResult(true, "  some movie  ")).toEqual({ status: "recognized", text: "some movie" });
  });
});
