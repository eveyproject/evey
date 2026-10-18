import { describe, it, expect, vi } from "vitest";
import type { TFunction } from "i18next";
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

// The messages go through t(): they were French literals, shown as-is to an
// admin reading the app in any language.
const t = ((key: string) => key) as unknown as TFunction;

describe("validateConfigField", () => {
  it("flags a required field left empty", () => {
    const schema: ConfigFieldSchema = { required: true };
    expect(validateConfigField("apiKey", "", schema, t)).toBe("admin.plugins.validation.required");
    expect(validateConfigField("apiKey", "   ", schema, t)).toBe("admin.plugins.validation.required");
  });

  it("accepts a non-empty required field", () => {
    expect(validateConfigField("apiKey", "secret", { required: true }, t)).toBeUndefined();
  });

  it("rejects a non-numeric value for number/integer types", () => {
    expect(validateConfigField("port", "abc", { type: "integer" }, t)).toBe("admin.plugins.validation.notNumber");
    expect(validateConfigField("ratio", "abc", { type: "number" }, t)).toBe("admin.plugins.validation.notNumber");
  });

  it("rejects a fractional value for an integer type", () => {
    expect(validateConfigField("port", "8080.5", { type: "integer" }, t)).toBe("admin.plugins.validation.notInteger");
  });

  it("accepts a fractional value for a number type", () => {
    expect(validateConfigField("ratio", "0.5", { type: "number" }, t)).toBeUndefined();
  });

  it("enforces min/max bounds", () => {
    const schema: ConfigFieldSchema = { type: "integer", min: 1, max: 10 };
    expect(validateConfigField("count", "0", schema, t)).toBe("admin.plugins.validation.min");
    expect(validateConfigField("count", "11", schema, t)).toBe("admin.plugins.validation.max");
    expect(validateConfigField("count", "5", schema, t)).toBeUndefined();
  });

  it("does not validate an empty optional numeric field as NaN", () => {
    expect(validateConfigField("port", "", { type: "integer" }, t)).toBeUndefined();
  });

  it("hands the field name and the bound to the translation", () => {
    const spy = vi.fn((key: string) => key);
    validateConfigField("count", "0", { type: "integer", min: 1 }, spy as unknown as TFunction);
    expect(spy).toHaveBeenCalledWith("admin.plugins.validation.min", { field: "count", min: 1 });
  });

  it("ignores type checks for plain string/select/boolean/password/text fields", () => {
    expect(validateConfigField("name", "anything", { type: "string" }, t)).toBeUndefined();
    expect(validateConfigField("flag", "not-a-bool", { type: "boolean" }, t)).toBeUndefined();
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
