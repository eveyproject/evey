import { describe, it, expect } from "vitest";
import {
  normaliseOptions,
  validateConfigField,
  parseEscalationEntries,
  type ConfigFieldSchema,
} from "./pluginConfigUtils";

describe("normaliseOptions", () => {
  it("returns an empty array when options is undefined", () => {
    expect(normaliseOptions(undefined)).toEqual([]);
  });

  it("maps a flat string list to value/label pairs", () => {
    expect(normaliseOptions(["a", "b"])).toEqual([
      { value: "a", label: "a" },
      { value: "b", label: "b" },
    ]);
  });

  it("preserves an explicit label and stringifies a non-string value", () => {
    expect(normaliseOptions([{ value: "1", label: "One" }, { value: "2" }])).toEqual([
      { value: "1", label: "One" },
      { value: "2", label: "2" },
    ]);
  });

  it("supports mixed string and object entries", () => {
    expect(normaliseOptions(["a", { value: "b", label: "B" }])).toEqual([
      { value: "a", label: "a" },
      { value: "b", label: "B" },
    ]);
  });
});

describe("validateConfigField", () => {
  it("flags a required field left empty", () => {
    const schema: ConfigFieldSchema = { required: true };
    expect(validateConfigField("apiKey", "", schema)).toBe("apiKey est requis");
    expect(validateConfigField("apiKey", "   ", schema)).toBe("apiKey est requis");
  });

  it("accepts a non-empty required field", () => {
    expect(validateConfigField("apiKey", "secret", { required: true })).toBeUndefined();
  });

  it("rejects a non-numeric value for number/integer types", () => {
    expect(validateConfigField("port", "abc", { type: "integer" })).toBe("port doit être un nombre");
    expect(validateConfigField("ratio", "abc", { type: "number" })).toBe("ratio doit être un nombre");
  });

  it("rejects a fractional value for an integer type", () => {
    expect(validateConfigField("port", "8080.5", { type: "integer" })).toBe("port doit être un entier");
  });

  it("accepts a fractional value for a number type", () => {
    expect(validateConfigField("ratio", "0.5", { type: "number" })).toBeUndefined();
  });

  it("enforces min/max bounds", () => {
    const schema: ConfigFieldSchema = { type: "integer", min: 1, max: 10 };
    expect(validateConfigField("count", "0", schema)).toBe("count doit être >= 1");
    expect(validateConfigField("count", "11", schema)).toBe("count doit être <= 10");
    expect(validateConfigField("count", "5", schema)).toBeUndefined();
  });

  it("does not validate an empty optional numeric field as NaN", () => {
    expect(validateConfigField("port", "", { type: "integer" })).toBeUndefined();
  });

  it("ignores type checks for plain string/select/boolean/password/text fields", () => {
    expect(validateConfigField("name", "anything", { type: "string" })).toBeUndefined();
    expect(validateConfigField("flag", "not-a-bool", { type: "boolean" })).toBeUndefined();
  });
});

describe("parseEscalationEntries", () => {
  it("extracts the comma-separated entries after the marker", () => {
    const msg = "rpc error: code = FailedPrecondition desc = new entries requested: permission:network, capability:foo";
    expect(parseEscalationEntries(msg)).toEqual(["permission:network", "capability:foo"]);
  });

  it("returns an empty array when the marker is absent", () => {
    expect(parseEscalationEntries("some unrelated error message")).toEqual([]);
  });

  it("trims whitespace and drops empty segments", () => {
    const msg = "new entries requested:  permission:network ,  , capability:foo ";
    expect(parseEscalationEntries(msg)).toEqual(["permission:network", "capability:foo"]);
  });

  it("handles a single entry", () => {
    expect(parseEscalationEntries("new entries requested: permission:network")).toEqual(["permission:network"]);
  });
});
