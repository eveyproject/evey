import { describe, it, expect } from "vitest";
import { personTypeLabelKey } from "./personRole";

describe("personTypeLabelKey", () => {
  it("maps every crew job the server sends to a translated key", () => {
    expect(personTypeLabelKey("PERSON_TYPE_DIRECTOR")).toBe("media.person.director");
    expect(personTypeLabelKey("PERSON_TYPE_WRITER")).toBe("media.person.writer");
    expect(personTypeLabelKey("PERSON_TYPE_PRODUCER")).toBe("media.person.producer");
    expect(personTypeLabelKey("PERSON_TYPE_COMPOSER")).toBe("media.person.composer");
  });

  // The fallback is what keeps a new enum value from printing a key on screen.
  it("returns null for an unknown or missing type", () => {
    expect(personTypeLabelKey("PERSON_TYPE_GAFFER")).toBeNull();
    expect(personTypeLabelKey(undefined)).toBeNull();
    expect(personTypeLabelKey("")).toBeNull();
  });
});
