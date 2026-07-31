import { describe, it, expect, vi, beforeEach } from "vitest";

const getDocumentAsyncMock = vi.fn();
vi.mock("expo-document-picker", () => ({
  getDocumentAsync: (...args: unknown[]) => getDocumentAsyncMock(...args),
}));

const readAsStringAsyncMock = vi.fn();
vi.mock("expo-file-system/legacy", () => ({
  readAsStringAsync: (...args: unknown[]) => readAsStringAsyncMock(...args),
}));

import { pickExternalSubtitle } from "./externalSubtitle";

beforeEach(() => {
  getDocumentAsyncMock.mockReset();
  readAsStringAsyncMock.mockReset();
});

describe("pickExternalSubtitle", () => {
  it("returns null when the user cancels the picker", async () => {
    getDocumentAsyncMock.mockResolvedValue({ canceled: true, assets: null });
    const result = await pickExternalSubtitle();
    expect(result).toBeNull();
    expect(readAsStringAsyncMock).not.toHaveBeenCalled();
  });

  it("converts a picked SRT file to WebVTT", async () => {
    getDocumentAsyncMock.mockResolvedValue({
      canceled: false,
      assets: [{ name: "movie.srt", uri: "file:///cache/movie.srt", lastModified: 0 }],
    });
    readAsStringAsyncMock.mockResolvedValue("1\n00:00:01,000 --> 00:00:03,500\nHello\n");
    const result = await pickExternalSubtitle();
    expect(result?.name).toBe("movie.srt");
    expect(result?.vtt).toMatch(/^WEBVTT/);
    expect(result?.vtt).toContain("00:00:01.000 --> 00:00:03.500");
    expect(readAsStringAsyncMock).toHaveBeenCalledWith("file:///cache/movie.srt");
  });

  it("converts a picked ASS file to WebVTT", async () => {
    getDocumentAsyncMock.mockResolvedValue({
      canceled: false,
      assets: [{ name: "movie.ass", uri: "file:///cache/movie.ass", lastModified: 0 }],
    });
    readAsStringAsyncMock.mockResolvedValue(
      [
        "[Events]",
        "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
        "Dialogue: 0,0:00:01.00,0:00:03.50,Default,,0,0,0,,Hello",
      ].join("\n"),
    );
    const result = await pickExternalSubtitle();
    expect(result?.name).toBe("movie.ass");
    expect(result?.vtt).toMatch(/^WEBVTT/);
    expect(result?.vtt).toContain("00:00:01.000 --> 00:00:03.500");
  });

  it("passes a picked VTT file through unchanged (aside from the read)", async () => {
    const vtt = "WEBVTT\n\n00:00:01.000 --> 00:00:03.000\nHi\n";
    getDocumentAsyncMock.mockResolvedValue({
      canceled: false,
      assets: [{ name: "movie.vtt", uri: "file:///cache/movie.vtt", lastModified: 0 }],
    });
    readAsStringAsyncMock.mockResolvedValue(vtt);
    const result = await pickExternalSubtitle();
    expect(result?.vtt).toBe(vtt);
  });

  it("returns null for an unsupported extension without reading the file", async () => {
    getDocumentAsyncMock.mockResolvedValue({
      canceled: false,
      assets: [{ name: "notes.txt", uri: "file:///cache/notes.txt", lastModified: 0 }],
    });
    const result = await pickExternalSubtitle();
    expect(result).toBeNull();
    expect(readAsStringAsyncMock).not.toHaveBeenCalled();
  });

  it("returns null instead of throwing when the file read fails", async () => {
    getDocumentAsyncMock.mockResolvedValue({
      canceled: false,
      assets: [{ name: "movie.srt", uri: "file:///cache/movie.srt", lastModified: 0 }],
    });
    readAsStringAsyncMock.mockRejectedValue(new Error("boom"));
    const result = await pickExternalSubtitle();
    expect(result).toBeNull();
  });

  it("returns null instead of throwing when the picker itself rejects", async () => {
    getDocumentAsyncMock.mockRejectedValue(new Error("boom"));
    const result = await pickExternalSubtitle();
    expect(result).toBeNull();
  });
});
