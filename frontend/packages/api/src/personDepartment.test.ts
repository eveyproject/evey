import { describe, it, expect } from "vitest";
import { personDepartmentKey } from "./personDepartment";

describe("personDepartmentKey", () => {
  it("maps known TMDB departments to i18n keys", () => {
    expect(personDepartmentKey("Acting")).toBe("media.person.department.acting");
    expect(personDepartmentKey("Costume & Make-Up")).toBe("media.person.department.costumeMakeup");
    expect(personDepartmentKey("Visual Effects")).toBe("media.person.department.visualEffects");
  });

  it("tolerates surrounding whitespace", () => {
    expect(personDepartmentKey(" Directing ")).toBe("media.person.department.directing");
  });

  it("returns null for unknown or missing departments so callers fall back to the raw value", () => {
    expect(personDepartmentKey("Underwater Basketry")).toBeNull();
    expect(personDepartmentKey("")).toBeNull();
    expect(personDepartmentKey(undefined)).toBeNull();
    expect(personDepartmentKey(null)).toBeNull();
  });
});
