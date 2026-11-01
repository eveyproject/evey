import { describe, expect, it } from "vitest";
import { SHED_CODES, canRetryShed, goneReasonFromError, isShedCode, isTitleGone, shedI18nKey } from "./shed";

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

  // Both mean the title cannot come back by retrying: the player offers the
  // next episode or the way back, and refreshes the lists.
  it("reads a deleted file and a title out of the catalogue as gone", () => {
    expect(isTitleGone("file_missing")).toBe(true);
    expect(isTitleGone("media_unavailable")).toBe(true);
    expect(isTitleGone("too_many_streams")).toBe(false);
    expect(isTitleGone(null)).toBe(false);
  });

  // A source too heavy for the server is final: retry would meet the same
  // refusal, while a busy server is worth another try.
  it("offers a retry only when one can succeed", () => {
    expect(canRetryShed("too_many_streams")).toBe(true);
    expect(canRetryShed("file_not_ready")).toBe(true);
    expect(canRetryShed("source_too_heavy")).toBe(false);
    expect(canRetryShed("file_missing")).toBe(false);
    expect(canRetryShed("media_unavailable")).toBe(false);
  });

  // One reading of a refused start for the four players.
  it("reads the gone reason from a fetch error", () => {
    expect(goneReasonFromError({ httpStatus: 404, errorCode: "media_file_missing" })).toBe("file_missing");
    expect(goneReasonFromError({ httpStatus: 404 })).toBe("media_unavailable");
    expect(goneReasonFromError({ httpStatus: 500 })).toBeNull();
    expect(goneReasonFromError(new Error("network"))).toBeNull();
    expect(goneReasonFromError(null)).toBeNull();
  });
});
