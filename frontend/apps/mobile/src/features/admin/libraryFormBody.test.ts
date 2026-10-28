import { describe, it, expect } from "vitest";
import { libraryCreateBody, libraryUpdateBody, emptyLibraryForm } from "./libraryFormBody";

describe("library form bodies", () => {
  // A new library is watched unless the operator turns it off.
  it("creates with real-time monitoring on by default", () => {
    const form = { ...emptyLibraryForm(), name: " Films ", paths: ["/media/films", " "] };
    expect(libraryCreateBody(form)).toEqual({
      name: "Films",
      type: "LIBRARY_TYPE_MOVIES",
      paths: ["/media/films"],
      realtimeMonitoring: true,
    });
  });

  // The edit form showed no switch and the update dropped it: a library
  // created without it could never be watched.
  it("sends the switch on update", () => {
    const form = { ...emptyLibraryForm(), name: "Series", paths: ["/media/series"], realtimeMonitoring: false };
    expect(libraryUpdateBody(form)).toEqual({ name: "Series", paths: ["/media/series"], realtimeMonitoring: false });
  });
});
