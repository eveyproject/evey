import { describe, it, expect } from "vitest";
import { downloadNotificationTexts, audioPassNotificationTitle } from "./downloadNotificationTexts";

describe("downloadNotificationTexts", () => {
  it("never hands the engine an empty text, even before the catalogue lands", () => {
    const texts = downloadNotificationTexts();
    for (const v of Object.values(texts)) expect(v).not.toBe("");
    expect(audioPassNotificationTitle()).not.toBe("");
  });

  it("keeps the engine's own {progress} placeholder in the progress line", () => {
    const { downloadProgress } = downloadNotificationTexts();
    expect(downloadProgress).toContain("{progress}");
    expect(downloadProgress).not.toContain("{{");
  });
});
