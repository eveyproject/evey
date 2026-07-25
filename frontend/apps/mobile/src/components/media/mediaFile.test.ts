import { describe, it, expect } from "vitest";
import { hasMediaFile } from "./mediaFile";

describe("hasMediaFile", () => {
  it("is true for a non-admin item: fileSize present, filePath redacted", () => {
    expect(hasMediaFile({ fileSize: "1073741824", filePath: undefined })).toBe(true);
  });

  it("is true for an admin item: filePath present", () => {
    expect(hasMediaFile({ filePath: "/movies/x.mkv" })).toBe(true);
  });

  it('is false for a container: proto zero size "0" and no filePath', () => {
    expect(hasMediaFile({ fileSize: "0", filePath: undefined })).toBe(false);
  });

  it("is false when nothing is present", () => {
    expect(hasMediaFile({})).toBe(false);
  });
});
