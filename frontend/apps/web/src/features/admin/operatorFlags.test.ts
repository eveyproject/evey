import { describe, it, expect } from "vitest";
import { OPERATOR_FLAGS, findOperatorFlag } from "./operatorFlags";

describe("operator flag catalogue", () => {
  it("has no duplicate keys", () => {
    const keys = OPERATOR_FLAGS.map((f) => f.key);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("carries a key, an env var and a value hint for every entry", () => {
    // The panel prints all three next to the input; a blank one would render an
    // empty hint that says nothing.
    for (const f of OPERATOR_FLAGS) {
      expect(f.key).not.toBe("");
      expect(f.envVar).not.toBe("");
      expect(f.valueHint).not.toBe("");
    }
  });

  it("matches a known key, trimming what the operator typed", () => {
    expect(findOperatorFlag("transcoding.snapshot_fastpath")?.envVar).toBe("EVEY_SNAPSHOT_FASTPATH");
    expect(findOperatorFlag("  transcoding.snapshot_fastpath  ")?.envVar).toBe("EVEY_SNAPSHOT_FASTPATH");
  });

  it("returns undefined for a key outside the catalogue", () => {
    // Free-text rows stay allowed - the catalogue only helps, it never gates.
    expect(findOperatorFlag("some.custom.key")).toBeUndefined();
    expect(findOperatorFlag("")).toBeUndefined();
  });
});
