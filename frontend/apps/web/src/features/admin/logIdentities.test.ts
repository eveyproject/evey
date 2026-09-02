import { describe, it, expect } from "vitest";
import { buildIdentityIndex, labelFor, resolveUserFilter } from "./logIdentities";

const users = [
  { id: "usr-1", username: "alice" },
  { id: "usr-2", username: "bob" },
  { id: "usr-3" }, // no username: unresolvable, must not poison the index
];
const profiles = [
  { id: "prf-1", userId: "usr-1", name: "Kids" },
  { id: "prf-2", userId: "usr-9", name: "Orphan" }, // owner unknown
];

describe("logIdentities", () => {
  const idx = buildIdentityIndex(users, profiles);

  it("labels users by name and profiles by name + owner", () => {
    expect(labelFor(idx, "usr-1")).toBe("alice (usr-1)");
    expect(labelFor(idx, "prf-1")).toBe("Kids (alice) (prf-1)");
    expect(labelFor(idx, "prf-2")).toBe("Orphan (prf-2)");
  });

  it("leaves anything it does not know untouched", () => {
    expect(labelFor(idx, "usr-3")).toBe("usr-3");
    expect(labelFor(idx, "/media/x.mkv")).toBe("/media/x.mkv");
    expect(labelFor(idx, "")).toBe("");
  });

  it("turns a username into its id for the filter, case-insensitively, else passes through", () => {
    expect(resolveUserFilter(users, " Bob ")).toBe("usr-2");
    expect(resolveUserFilter(users, "usr-77")).toBe("usr-77");
    expect(resolveUserFilter(users, "   ")).toBe("");
  });
});
