import { describe, it, expect } from "vitest";
import { tvKeyOf } from "./keys";

describe("tvKeyOf", () => {
  it("reads the five-way keys from key names and from the old engines' keyCodes", () => {
    expect(tvKeyOf({ key: "ArrowLeft", keyCode: 37 })).toBe("left");
    expect(tvKeyOf({ key: "Unidentified", keyCode: 39 })).toBe("right");
    expect(tvKeyOf({ key: "Enter", keyCode: 13 })).toBe("select");
    expect(tvKeyOf({ key: "Escape", keyCode: 27 })).toBe("back");
  });
  it("knows the LG and Samsung back and media buttons", () => {
    expect(tvKeyOf({ key: "Unidentified", keyCode: 461 })).toBe("back");
    expect(tvKeyOf({ key: "Unidentified", keyCode: 10009 })).toBe("back");
    expect(tvKeyOf({ key: "Unidentified", keyCode: 415 })).toBe("play");
    expect(tvKeyOf({ key: "Unidentified", keyCode: 10252 })).toBe("playPause");
  });
  it("answers null for a key it does not map", () => {
    expect(tvKeyOf({ key: "q", keyCode: 81 })).toBeNull();
  });
});
