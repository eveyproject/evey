import { describe, expect, it } from "vitest";
import { SHED_CODES, isShedCode, shedI18nKey } from "./shed";

describe("shed contract", () => {
  it("recognises exactly the server's codes", () => {
    for (const c of SHED_CODES) expect(isShedCode(c)).toBe(true);
    expect(isShedCode("remote_access_disabled")).toBe(false);
    expect(isShedCode(null)).toBe(false);
    expect(isShedCode(undefined)).toBe(false);
  });

  it("gives every code its own copy", () => {
    const keys = SHED_CODES.map((c) => shedI18nKey(c, "title"));
    expect(new Set(keys).size).toBe(SHED_CODES.length);
    expect(shedI18nKey("file_needs_remux", "body")).toBe("player.shed.fileNeedsRemux.body");
  });
});
