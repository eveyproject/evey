import { describe, it, expect } from "vitest";
import { streamSelectionParams } from "./streamSelectionParams";

describe("streamSelectionParams", () => {
  it("names stream 0 in the fields that carry its presence", () => {
    expect(streamSelectionParams(0, 0)).toEqual({
      audioStreamIndex: 0, selectedAudioStreamIndex: 0,
      subtitleStreamIndex: 0, selectedSubtitleStreamIndex: 0,
    });
  });

  it("sends both fields for any other track", () => {
    expect(streamSelectionParams(4, undefined)).toEqual({ audioStreamIndex: 4, selectedAudioStreamIndex: 4 });
  });

  it("sends nothing for no selection", () => {
    expect(streamSelectionParams(undefined, -1)).toEqual({});
  });
});
